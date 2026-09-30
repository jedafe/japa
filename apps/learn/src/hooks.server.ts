import type { Handle } from '@sveltejs/kit';
import { building } from '$app/environment';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { SessionUser } from '$lib/types';

/**
 * Japa University — the single learning surface (ADR-063 consolidation).
 * The former dual-shell (Learn vs University) has been unified: the app
 * always renders the Japa University shell with the creator-economy
 * marketplace. The canonical host is configured via the
 * `university.subdomain` and `university.custom_domain` system_settings
 * (ADR-063 §2); the edge/proxy routes the configured host to this app
 * and the backend uses the hostname for host-based routing. The
 * learn.japaconnect.com host redirects to the configured university host
 * (handled by the edge/proxy).
 * Ref: japaconnect-engineering/conventions/system-settings-catalog.md
 */
export const handle: Handle = async ({ event, resolve }) => {
	// Hostname excludes the port; forwarded to the backend for host-based routing.
	event.locals.requestHost = event.url.hostname;
	event.locals.token = event.cookies.get('jc_token') ?? null;

	// Detect locale prefix in the URL path (e.g. /fr/tracks → 'fr').
	// Ref: §18 (i18n language packs), ADR-013.
	const localePrefixMatch = event.url.pathname.match(/^\/([a-z]{2})(?=\/|$)/);
	const pathLocale = localePrefixMatch?.[1] ?? null;

	// Fetch public site settings once per request, shared between theme
	// and locale resolution. Ref: conventions/system-settings-catalog.md
	const themeCookie = event.cookies.get('jc_theme');
	const localeCookie = event.cookies.get('jc_locale');
	const needSettings =
		(themeCookie !== 'dark' && themeCookie !== 'light') ||
		!localeCookie ||
		pathLocale !== null;

	let siteSettings: Record<string, unknown> = {};
	if (needSettings && !building) {
		try {
			siteSettings =
				(await apiGet<Record<string, unknown>>(apiContext(event), '/site/settings/public')) ?? {};
		} catch {
			siteSettings = {};
		}
	}

	// Theme: prefer cookie, then appearance.dark_mode_default, then 'light'.
	event.locals.theme =
		themeCookie === 'dark' || themeCookie === 'light'
			? themeCookie
			: siteSettings['appearance.dark_mode_default'] === true
				? 'dark'
				: 'light';

	// Locale resolution: URL prefix > cookie > i18n.default_language > 'en'.
	// Valid prefix → set cookie + rewrite URL so SvelteKit routes to the
	// unprefixed page tree. Browser URL stays /fr/tracks; page renders French.
	// Ref: §18, conventions/system-settings-catalog.md (i18n.enabled_languages)
	const enabledLanguages =
		((siteSettings['i18n.enabled_languages'] as string | undefined) ?? 'en,fr')
			.split(',')
			.map((l) => l.trim())
			.filter(Boolean);

	if (pathLocale && enabledLanguages.includes(pathLocale)) {
		event.locals.locale = pathLocale;
		event.cookies.set('jc_locale', pathLocale, {
			path: '/',
			maxAge: 60 * 60 * 24 * 365,
			httpOnly: false,
			sameSite: 'lax'
		});
		event.url.pathname = event.url.pathname.slice(3) || '/';
	} else if (localeCookie && enabledLanguages.includes(localeCookie)) {
		event.locals.locale = localeCookie;
	} else {
		const defaultLang = (siteSettings['i18n.default_language'] as string | undefined) ?? 'en';
		event.locals.locale = enabledLanguages.includes(defaultLang) ? defaultLang : 'en';
	}
	event.locals.user = null;

	// Resolve the session. The app is public: an absent or invalid token
	// simply means "guest" — never a redirect to login. Enrollment and progress
	// are CSR-only (ADR-060 SSR/CSR boundary).
	if (event.locals.token && !building) {
		try {
			event.locals.user = await apiGet<SessionUser>(apiContext(event), '/me');
		} catch (err) {
			if (err instanceof ApiError && (err.status === 401 || err.status === 403)) {
				// Stale credential — drop it and continue as a guest.
				event.cookies.delete('jc_token', { path: '/' });
				event.locals.token = null;
			}
			// Any other failure (backend unreachable) also degrades to guest.
		}
	}

	// Fetch the double-submit CSRF token for authenticated users so server-side
	// mutations (form actions, apiPost/apiPatch/apiDelete) can attach the
	// X-CSRF-Token header. Also mirror the csrf_token cookie onto the browser
	// response so client-side CSR mutations (enrollment/progress — ADR-060) can
	// echo it back. The backend's Set-Cookie from the server-side /csrf fetch is
	// consumed by SvelteKit's server fetch and never reaches the browser, so we
	// set it explicitly here.
	// Ref: japa-bugs-hunters.md §5.2 (CSRF) — audit finding C1.
	event.locals.csrfToken = null;
	if (event.locals.token && !building) {
		try {
			const csrf = await apiGet<{ csrfToken: string }>(apiContext(event), '/csrf');
			event.locals.csrfToken = csrf?.csrfToken ?? null;
			if (event.locals.csrfToken) {
				event.cookies.set('csrf_token', event.locals.csrfToken, {
					path: '/',
					httpOnly: false,
					secure: true,
					sameSite: 'lax'
				});
			}
		} catch {
			event.locals.csrfToken = null;
		}
	}

	return resolve(event, {
		transformPageChunk: ({ html }) =>
			html
				.replace('%jc.theme%', event.locals.theme)
				.replace('%jc.theme-color%', event.locals.theme === 'dark' ? '#0a0a0a' : '#3b49df')
				.replace('lang="en"', `lang="${event.locals.locale}"`)
	});
};
