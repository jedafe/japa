import { goto } from '$app/navigation';
import { page } from '$app/state';

/**
 * Guest-access helpers for auth-gated client-side actions.
 *
 * The main app is public by default (hooks.server.ts: an absent/invalid token
 * means "guest", never an automatic redirect). Auth-gated *operations*
 * (follow, bookmark, like, vote, comment, join space) must therefore guard
 * themselves: when a guest clicks an action button, redirect them to /login
 * with a `redirect` return-to query param instead of calling the API and
 * surfacing a raw 401.
 *
 * Uses `$app/state` — `page` is a live state object, so reading it at call
 * time (inside an event handler, not a reactive context) always yields the
 * current value. `$app/stores` was removed in SvelteKit 3.
 *
 * Ref: japaconnect-engineering/Prompts/japa-bugs-hunters.md §3.1 (401 loop),
 *      §5.2 (auth/sessions). Uses `goto()` (not `window.location`) so the SPA
 *      session context is preserved (§1.2 — never window.location for nav).
 */

/**
 * Returns true when the current viewer is authenticated. Uses the
 * `isAuthed` boolean from the root layout data (not `page.data.user`,
 * which may be overridden by page-level load functions — e.g. the profile
 * page returns the viewed profile's user as `user`).
 */
export function isAuthed(): boolean {
	return page.data.isAuthed === true;
}

/**
 * Guards an auth-gated client-side action. If the viewer is a guest, redirects
 * to `/login?redirect=<current path>` and returns `false` so the caller aborts.
 * If authenticated, returns `true` so the caller may proceed with the API call.
 *
 * Usage:
 * ```ts
 * async function toggle() {
 *   if (!(await requireAuthOrRedirect())) return;
 *   // ...perform the mutation...
 * }
 * ```
 */
export async function requireAuthOrRedirect(): Promise<boolean> {
	if (isAuthed()) return true;
	const returnTo = page.url.pathname + page.url.search;
	await goto(`/login?redirect=${encodeURIComponent(returnTo)}`);
	return false;
}
