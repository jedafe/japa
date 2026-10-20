// api/nextActions.ts — §122 Next-Action Suggestions (ADR-132).
// Wraps /api/v1/next-actions — OptionalAuth, serves guests + members.
// getNextActions is memoized ~60s so the bar/popup/sidebar/in-article
// renderers share one request per page view; mutations bust the memo.
import { api } from '$lib/api/client';
import type { NextAction } from '$lib/types';

const MEMO_MS = 60_000;

let memo: { surface: string; at: number; promise: Promise<NextAction[]> } | null = null;

export function getNextActions(surface: string): Promise<NextAction[]> {
	const now = Date.now();
	if (memo && memo.surface === surface && now - memo.at < MEMO_MS) {
		return memo.promise;
	}
	const promise = (async () => {
		try {
			const res = await api.get<NextAction[]>(`/next-actions?surface=${surface}`);
			return res.data ?? [];
		} catch {
			return [];
		}
	})();
	memo = { surface, at: now, promise };
	return promise;
}

export function invalidateNextActions() {
	memo = null;
}

export function dismissNextAction(id: string, snoozeHours?: number) {
	invalidateNextActions();
	return api.post(`/next-actions/${id}/dismiss`, snoozeHours ? { snooze_hours: snoozeHours } : {}).catch(() => {});
}

export function completeNextAction(id: string) {
	invalidateNextActions();
	return api.post(`/next-actions/${id}/complete`, {}).catch(() => {});
}

export function trackNextAction(id: string, event: 'shown' | 'clicked') {
	void api.post(`/next-actions/${id}/track`, { event }).catch(() => {});
}
