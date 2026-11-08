/**
 * Comment view tracker — batches per-comment view pings into a single
 * POST /comments/views flush (Guide §4.3: one round trip per thread, not
 * per comment). Deduped per session; silent failures are dropped.
 */
import { api } from '#lib/api/client.js';
import { browser } from '$app/env';

const recorded = new Set<string>();
let pending = new Set<string>();
let timer: ReturnType<typeof setTimeout> | null = null;

const FLUSH_MS = 400;
const BATCH_MAX = 200;

export function queueCommentView(id: string, authed: boolean): void {
	if (!browser || !authed || recorded.has(id)) return;
	recorded.add(id);
	pending.add(id);
	if (timer === null) {
		timer = setTimeout(flush, FLUSH_MS);
	}
}

function flush() {
	timer = null;
	const ids = Array.from(pending).slice(0, BATCH_MAX);
	pending = new Set();
	if (ids.length === 0) return;
	api.post('/comments/views', { ids }, { 'x-silent-error': '1' }).then((res) => {
		// Re-queue failures so a transient error doesn't permanently lose
		// the view — except client-side rejections (4xx), which will recur.
		if (res.error && (res.error.status ?? 0) >= 500) {
			for (const id of ids) {
				recorded.delete(id);
				pending.add(id);
			}
			if (timer === null) timer = setTimeout(flush, 2000);
		}
	});
}
