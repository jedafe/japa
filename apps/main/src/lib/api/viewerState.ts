import { api } from '$lib/api/client';

export interface ViewerContentState {
	reactionKeys: string[];
	bookmarked: boolean;
}

type RawStates = Record<string, { reaction_keys?: string[]; bookmarked?: boolean }>;

const EMPTY: ViewerContentState = { reactionKeys: [], bookmarked: false };

/** Backend caps ids per request (api-routes-catalog: /me/content-states). */
const MAX_IDS = 100;

const promises = new Map<string, Promise<ViewerContentState>>();
const resolvers = new Map<string, (s: ViewerContentState) => void>();
const lastStates = new Map<string, ViewerContentState>();
let flushQueued = false;

/**
 * Batched viewer-interaction state for content cards (reactions + bookmark).
 * Every card asks for its own id; pending asks are collected for one
 * microtask and resolved by a single GET /me/content-states round-trip
 * instead of two requests per card (/reactions/me + /bookmark).
 *
 * Ref: api-routes-catalog — GET /me/content-states?ids={csv}
 */
export function getViewerState(contentId: string): Promise<ViewerContentState> {
	let p = promises.get(contentId);
	if (!p) {
		p = new Promise<ViewerContentState>((resolve) => {
			resolvers.set(contentId, resolve);
		});
		promises.set(contentId, p);
		scheduleFlush();
	}
	return p;
}

/**
 * Merge locally-known state after a successful toggle so components
 * remounting within the session see fresh values without refetching.
 */
export function updateViewerState(contentId: string, patch: Partial<ViewerContentState>): void {
	const next = { ...(lastStates.get(contentId) ?? EMPTY), ...patch };
	lastStates.set(contentId, next);
	resolvers.delete(contentId);
	promises.set(contentId, Promise.resolve(next));
}

function scheduleFlush(): void {
	if (flushQueued) return;
	flushQueued = true;
	queueMicrotask(() => {
		flushQueued = false;
		void flush();
	});
}

async function flush(): Promise<void> {
	const ids = [...resolvers.keys()];
	if (ids.length === 0) return;

	const batch = new Map(resolvers);
	for (const id of ids) resolvers.delete(id);

	for (let i = 0; i < ids.length; i += MAX_IDS) {
		const chunk = ids.slice(i, i + MAX_IDS);
		const res = await api.get<RawStates>(
			`/me/content-states?ids=${encodeURIComponent(chunk.join(','))}`
		);
		const data = res.data ?? {};
		for (const id of chunk) {
			const raw = data[id];
			const state: ViewerContentState = raw
				? { reactionKeys: raw.reaction_keys ?? [], bookmarked: raw.bookmarked ?? false }
				: EMPTY;
			lastStates.set(id, state);
			batch.get(id)?.(state);
		}
	}
}
