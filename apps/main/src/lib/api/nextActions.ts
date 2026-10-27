// api/nextActions.ts — §122 Next-Action Suggestions (ADR-132).
// Wraps /api/v1/next-actions — OptionalAuth, serves guests + members.
// getNextActions is memoized ~60s so the bar/popup/sidebar/in-article
// renderers share one request per page view; mutations bust the memo.
import { api } from '$lib/api/client';
import { page } from '$app/state';
import { goto } from '$app/navigation';
import type { NextAction } from '$lib/types';

const MEMO_MS = 60_000;

interface NextActionsEnvelope {
	actions: NextAction[];
	popupMax: number;
}

// nextActionsViewer returns the identity the suggestion list is evaluated
// for. data.viewerId (root layout) is preferred over data.user?.id — pages
// that render another user's profile override page.data.user with the
// VIEWED user, which would key suggestions to the wrong identity.
//
// The memo and claim set are keyed on this value: without it a guest's
// cached list (e.g. "Create account" nudges) kept rendering for up to 60s
// after an in-app login/signup, and member-targeted suggestions leaked to
// a guest after logout.
export function nextActionsViewer(): string {
	const d = page.data as { viewerId?: string | null; user?: { id?: string | null } | null };
	return d?.viewerId ?? d?.user?.id ?? 'guest';
}

// nextActionsSurface maps the current route to the §122 ?surface= param.
// The /tools/* pages live inside the main app shell but are their own
// audience surface — rules scoped to "tools" (or excluded from "main")
// must resolve correctly here.
export function nextActionsSurface(): string {
	return page.route.id?.startsWith('/(app)/tools') ? 'tools' : 'main';
}

let memo: { key: string; viewer: string; at: number; promise: Promise<NextActionsEnvelope> } | null = null;

function fetchNextActions(surface: string): Promise<NextActionsEnvelope> {
	const viewer = nextActionsViewer();
	const key = `${viewer}|${surface}`;
	const now = Date.now();
	if (memo && memo.viewer !== viewer) {
		// Identity change — the cached list AND the cross-channel claims
		// were recorded for a different viewer; drop both.
		claimed.clear();
		memo = null;
	} else if (memo && memo.key !== key) {
		// Same viewer, different surface — refetch, but keep claims: the
		// persistent shell renderers still display their suggestions, so
		// releasing them would let a second channel re-render the same one.
		memo = null;
	}
	if (memo && now - memo.at < MEMO_MS) {
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
	memo = { key, viewer, at: now, promise };
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
// isNextActionClaimed before picking theirs. Claims are released when the
// claiming renderer unmounts so a suggestion visible nowhere can surface on
// another channel later; the set is also dropped on identity/surface change.
const claimed = new Set<string>();

export function claimNextAction(id: string) {
	claimed.add(id);
}

export function releaseNextAction(id: string) {
	claimed.delete(id);
}

export function isNextActionClaimed(id: string): boolean {
	return claimed.has(id);
}

export function invalidateNextActions() {
	memo = null;
}

// goToNextAction navigates a CTA. Relative paths route through the SvelteKit
// router; absolute URLs (allowed via nextactions.allowed_cta_origins) get a
// full navigation — goto() is internal-only.
export function goToNextAction(url: string) {
	if (url.startsWith('/')) {
		void goto(url);
	} else {
		window.location.href = url;
	}
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
