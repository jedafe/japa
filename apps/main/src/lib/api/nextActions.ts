// api/nextActions.ts — §122 Next-Action Suggestions (ADR-132).
// Wraps /api/v1/next-actions — OptionalAuth, serves guests + members.
// getNextActions is memoized ~60s so the bar/popup/sidebar/in-article
// renderers share one request per page view; mutations bust the memo.
import { api } from '$lib/api/client';
import type { NextAction } from '$lib/types';

const MEMO_MS = 60_000;

interface NextActionsEnvelope {
	actions: NextAction[];
	popupMax: number;
}

let memo: { surface: string; at: number; promise: Promise<NextActionsEnvelope> } | null = null;

function fetchNextActions(surface: string): Promise<NextActionsEnvelope> {
	const now = Date.now();
	if (memo && memo.surface === surface && now - memo.at < MEMO_MS) {
		return memo.promise;
	}
	const promise = (async () => {
		try {
			const res = await api.getEnvelope<NextAction[]>(`/next-actions?surface=${surface}`);
			const meta = res.envelope?.['meta'] as { popup_max_per_session?: number } | undefined;
			return {
				actions: res.data ?? [],
				popupMax: Math.max(0, meta?.popup_max_per_session ?? 1)
			};
		} catch {
			return { actions: [], popupMax: 1 };
		}
	})();
	memo = { surface, at: now, promise };
	return promise;
}

export function getNextActions(surface: string): Promise<NextAction[]> {
	return fetchNextActions(surface).then((r) => r.actions);
}

// getPopupMaxPerSession returns the server-configured popup cap for the
// session (nextactions.popup_max_per_session) — resolves after the shared
// memoized fetch, so renderers must call it inside their onMount flow.
export function getPopupMaxPerSession(surface: string): Promise<number> {
	return fetchNextActions(surface).then((r) => r.popupMax);
}

// --- Cross-channel claim set ------------------------------------------------
// A suggestion that has already been rendered on one channel must not render
// on another in the same page view (popup + bar + sidebar + in-article would
// otherwise show identical copy simultaneously). Renderers call
// claimNextAction when they actually display a suggestion and filter with
// isNextActionClaimed before picking theirs.
const claimed = new Set<string>();

export function claimNextAction(id: string) {
	claimed.add(id);
}

export function isNextActionClaimed(id: string): boolean {
	return claimed.has(id);
}

export function invalidateNextActions() {
	memo = null;
}

// --- Pending completion -----------------------------------------------------
// CTA clicks record which suggestion sent the user onward; destination flows
// (register success, assessment submit, tool run) call
// completePendingNextAction() once the REAL action succeeds — rules without
// a completion_signal otherwise have no honest completion path.
const PENDING_KEY = 'jc_na_pending';

export function markNextActionPending(id: string) {
	try { sessionStorage.setItem(PENDING_KEY, id); } catch { /* private mode */ }
}

export function completePendingNextAction() {
	try {
		const id = sessionStorage.getItem(PENDING_KEY);
		if (id) {
			sessionStorage.removeItem(PENDING_KEY);
			void completeNextAction(id);
		}
	} catch { /* private mode */ }
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
