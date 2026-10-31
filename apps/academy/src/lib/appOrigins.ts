import { PUBLIC_MAIN_ORIGIN } from '$env/static/public';
import { env } from '$env/dynamic/public';

/**
 * Cross-app origin helpers.
 *
 * The Academy app (academy.japa.to / :3003) shares the `jc_token` cookie
 * with the Main app (japa.to / :3000), which owns the login, signup,
 * and user-profile surfaces. Academy has no `/login` or `/u/{username}` route of
 * its own, so auth/profile links must point at the Main app origin.
 *
 * `PUBLIC_MAIN_ORIGIN` is the canonical Main app origin (no trailing slash).
 * In dev it defaults to http://localhost:3000; in prod it is
 * https://japa.to (set via .env).
 */
export const MAIN_ORIGIN = (PUBLIC_MAIN_ORIGIN || 'http://localhost:3000').replace(/\/$/, '');

/**
 * Build a Main app login URL with an optional redirect back to the Academy app.
 *
 * `redirectPath` is a path on the Academy app (e.g. `/dashboard/instructor`).
 * The redirect target must be an absolute URL pointing back to the Academy app
 * (NOT the Main app, which doesn't host Academy routes). On the client side the
 * Academy origin is `window.location.origin`; on the server side the caller must
 * pass an already-absolute URL (e.g. `${event.url.origin}/dashboard/instructor`).
 */
export function loginUrl(redirectPath?: string): string {
	const base = `${MAIN_ORIGIN}/login`;
	if (!redirectPath) return base;
	// If the caller already passed an absolute URL (server-side with
	// event.url.origin), use it as-is. Otherwise (client-side relative path)
	// prefix with the current Academy app origin from the browser.
	let abs: string;
	if (redirectPath.startsWith('http')) {
		abs = redirectPath;
	} else if (typeof window !== 'undefined') {
		abs = `${window.location.origin}${redirectPath}`;
	} else {
		// SSR without an absolute URL: the target is an Academy route the
		// Main app doesn't host, so a Main-app-relative redirect would 404
		// after login. Return bare login rather than a broken deep link (F9).
		return base;
	}
	return `${base}?redirect=${encodeURIComponent(abs)}`;
}

/**
 * Canonical Academy origin for SEO/OG URLs (S6). Reads the configured
 * academy host (custom-domain aware via PUBLIC_JC_ACADEMY_ORIGIN); falls
 * back to the default academy subdomain.
 */
export const ACADEMY_ORIGIN = (env.PUBLIC_JC_ACADEMY_ORIGIN || 'https://academy.japa.to').replace(
	/\/$/,
	''
);

/** Build a Main app user-profile URL. */
export function profileUrl(username: string): string {
	return `${MAIN_ORIGIN}/u/${username}`;
}

// Path prefixes this app actually routes — menu items pointing anywhere else
// (login, signup, docs, privacy, settings, …) are platform pages hosted on the
// Main app and must be rewritten to absolute URLs or they 404 on Academy.
const ACADEMY_PATH_PREFIXES = [
	'/tracks',
	'/search',
	'/my-learning',
	'/certificates',
	'/bookmarks',
	'/instructors',
	'/dashboard'
];

/**
 * Resolve an admin-managed menu item URL for the Academy app.
 *
 * Menu items are seeded platform-wide (e.g. academy_footer links to /docs,
 * /privacy, /login) — relative URLs that have no route on Academy would 404.
 * Anything outside the Academy route set is rewritten to an absolute Main-app
 * URL (which also makes the browser treat it as cross-origin navigation —
 * correct, since the Main app owns those pages). Absolute URLs, anchors and
 * non-http schemes pass through unchanged.
 */
export function resolveMenuUrl(url: string): string {
	if (!url) return '/';
	// `main:` scheme — an explicit link to the Main app. Needed because `/` is
	// a real Academy route (the catalog), so a menu item that means "Main app
	// home" cannot use `/` without self-linking (e.g. academy_footer's
	// "Main App" item, which shares `/` with "Explore"). Must be checked before
	// the generic scheme pass-through below.
	if (url.startsWith('main:')) return `${MAIN_ORIGIN}${url.slice(5) || '/'}`;
	// External URL, anchor, or non-http scheme (mailto:, tel:) — leave as-is.
	if (/^([a-z][a-z0-9+.-]*:|#)/i.test(url)) return url;
	if (!url.startsWith('/')) return url;

	const path = url.split(/[?#]/)[0];
	if (path === '/login') return loginUrl();
	if (path === '/' || ACADEMY_PATH_PREFIXES.some((p) => path === p || path.startsWith(`${p}/`))) {
		return url;
	}
	return `${MAIN_ORIGIN}${url}`;
}

/**
 * Switch the UI locale by navigating to the locale-prefixed URL.
 *
 * The hooks.server.ts handle hook detects the locale prefix in the URL
 * (e.g. /fr/tracks), sets the jc_locale cookie, and rewrites the URL to
 * the unprefixed path for routing. The browser URL stays /fr/tracks and
 * the page renders in the chosen locale. No-op during SSR.
 * Ref: §18 (i18n language packs), ADR-013.
 */
export function switchLocale(locale: string, defaultLang = 'en'): void {
	if (typeof document === 'undefined') return;
	const path = window.location.pathname.replace(/^\//, '');
	const href = locale === defaultLang ? `/${path}` : `/${locale}/${path}`;
	window.location.href = href;
}

