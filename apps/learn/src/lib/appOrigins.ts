import { PUBLIC_MAIN_ORIGIN } from '$env/static/public';
import type { Locale } from '$lib/i18n';

/**
 * Cross-app origin helpers.
 *
 * The Learn app (learn.japaconnect.com / :3003) shares the `jc_token` cookie
 * with the Main app (japaconnect.com / :3000), which owns the login, signup,
 * and user-profile surfaces. Learn has no `/login` or `/u/{username}` route of
 * its own, so auth/profile links must point at the Main app origin.
 *
 * `PUBLIC_MAIN_ORIGIN` is the canonical Main app origin (no trailing slash).
 * In dev it defaults to http://localhost:3000; in prod it is
 * https://japaconnect.com (set via .env).
 */
export const MAIN_ORIGIN = (PUBLIC_MAIN_ORIGIN || 'http://localhost:3000').replace(/\/$/, '');

/** Build a Main app login URL with an optional redirect back to Learn. */
export function loginUrl(redirectPath?: string): string {
	const base = `${MAIN_ORIGIN}/login`;
	if (!redirectPath) return base;
	const abs = redirectPath.startsWith('http')
		? redirectPath
		: `${MAIN_ORIGIN}${redirectPath}`;
	return `${base}?redirect=${encodeURIComponent(abs)}`;
}

/** Build a Main app user-profile URL. */
export function profileUrl(username: string): string {
	return `${MAIN_ORIGIN}/u/${username}`;
}

/**
 * Switch the UI locale by setting the `jc_locale` cookie and reloading.
 *
 * The Learn app resolves the locale from the `jc_locale` cookie (see
 * hooks.server.ts) — it has NO `/en/*` or `/fr/*` URL-prefixed routes (unlike
 * the Main app). Navigating to `/en/<path>` therefore 404s. The correct way to
 * switch locale here is to set the cookie and reload the current page so SSR
 * re-resolves with the new locale. No-op during SSR.
 */
export function switchLocale(locale: Locale): void {
	if (typeof document === 'undefined') return;
	document.cookie = `jc_locale=${locale}; path=/; max-age=31536000; samesite=lax`;
	// Reload the current page so the server re-resolves the locale.
	window.location.reload();
}

