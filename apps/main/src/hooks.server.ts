import type { Handle } from '@sveltejs/kit';
import { building } from '$app/environment';
import { env } from '$env/dynamic/private';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import {
	refreshSession,
	setSessionCookie,
	setRefreshCookie
} from '$lib/server/auth';
import { buildThemeOverrides } from '@japaconnect/ui/theme';
import { sanitizeCustomCss } from '$lib/utils/safeHtml';
import type { SessionUser } from '$lib/types';

/**
 * In-process cache for GET /site/settings/public. Public site settings
 * (theme default, enabled languages, default language) change rarely — at
 * most a few times per week when an admin toggles something — yet the
 * previous hooks.server.ts fetched them on every request that lacked a
 * theme/locale cookie (i.e. essentially every first visit, and every
 * locale-prefixed URL). That added a serial backend round-trip to the SSR
 * critical path for data that is effectively global.
 *
 * The cache is keyed by request host.
 * It lives for SETTINGS_TTL_MS. A stale entry is served on fetch failure so
 * a transient backend blip doesn't degrade the page. The cache is positive-
 * only: misses are not cached, so the next request retries immediately.
 *
 * Ref: conventions/system-settings-catalog.md
 */
const SETTINGS_TTL_MS = 30_000;
interface CachedSettings {
	value: Record<string, unknown>;
	expiresAt: number;
}
const settingsCache = new Map<string, CachedSettings>();

/**
 * In-process cache for the per-session CSRF token. The token is tied to the
 * session (not the request) and does not change between navigations within
 * the same session, so fetching GET /csrf on every authenticated request —
 * as the previous hooks.server.ts did — was pure overhead on the SSR
 * critical path. Authenticated navigations paid an extra serial round-trip
 * for data that was almost always identical to the previous request's.
 *
 * Keyed by access token, TTL CSRF_TTL_MS. Positive-only: a fetch failure is
 * not cached, so the next request retries. If the backend rotates the token
 * (e.g. on refresh) the cached value goes stale, but the existing catch in
 * the /csrf path already degrades gracefully and the next request re-fetches.
 *
 * Ref: §5.2 (CSRF double-submit), audit finding C1.
 */
const CSRF_TTL_MS = 5 * 60_000;
interface CachedCsrf {
	token: string;
	expiresAt: number;
}
const csrfCache = new Map<string, CachedCsrf>();

/**
 * Base domain for canonical surface hosts. Defaults to the production
 * domain; staging deployments set JC_BASE_DOMAIN so canonical hosts resolve
 * under e.g. staging.japa.to.
 */
const BASE_DOMAIN = (env.JC_BASE_DOMAIN ?? 'japa.to').trim().toLowerCase() || 'japa.to';

/**
 * ADR-063: detect whether the request host is the Japa Academy branded
 * surface. The canonical host is academy.{BASE_DOMAIN}; additional custom
 * domains (e.g. japauni.com) can be configured via the JC_ACADEMY_HOSTS env
 * var as a comma-separated list.
 */
function isAcademySurface(host: string): boolean {
	const canonical = `academy.${BASE_DOMAIN}`;
	const configured = env.JC_ACADEMY_HOSTS?.split(',').map((h) => h.trim().toLowerCase()).filter(Boolean) ?? [];
	const hosts = [canonical, ...configured];
	return hosts.includes(host.toLowerCase());
}

/**
 * ADR-127: detect whether the request host is the public roadmap surface.
 * The canonical host is roadmap.{BASE_DOMAIN}; additional custom domains
 * come from the roadmap.custom_domain system setting (comma-separated, §113)
 * and the JC_ROADMAP_HOSTS env var (comma-separated).
 */
function isRoadmapSurface(host: string, customDomainSetting?: unknown): boolean {
	const canonical = `roadmap.${BASE_DOMAIN}`;
	const configured = env.JC_ROADMAP_HOSTS?.split(',').map((h) => h.trim().toLowerCase()).filter(Boolean) ?? [];
	const custom = typeof customDomainSetting === 'string'
		? customDomainSetting.split(',').map((h) => h.trim().toLowerCase()).filter(Boolean)
		: [];
	return [canonical, ...configured, ...custom].includes(host.toLowerCase());
}

/**
 * Resolve the current user and CSRF token for an already-present access token.
 * Returns { ok: true } when the token is valid and the user is attached. On
 * 401/403 it clears the token and any cached CSRF entry and reports
 * shouldRefresh so the caller can attempt a silent token refresh. The function
 * is self-contained so it can be called a second time after a successful refresh.
 */
async function resolveSession(
	event: Parameters<Handle>[0]['event']
): Promise<{ ok: boolean; shouldRefresh: boolean }> {
	const token = event.locals.token;
	if (!token) return { ok: false, shouldRefresh: false };
	const now = Date.now();

	// Check the CSRF cache before issuing any fetches.
	const cachedCsrf = csrfCache.get(token);
	const csrfFromCache = cachedCsrf && cachedCsrf.expiresAt > now ? cachedCsrf.token : null;

	// /me is always fetched (validates the session). /csrf is only fetched
	// on a cache miss. Both run in parallel when a /csrf fetch is needed.
	const mePromise = apiGet<SessionUser>(apiContext(event), '/me').then(
		(u) => ({ ok: true as const, user: u }),
		(err) => ({ ok: false as const, err })
	);
	const csrfPromise =
		csrfFromCache !== null
			? Promise.resolve({ ok: true as const, token: csrfFromCache, cached: true })
			: apiGet<{ csrfToken: string }>(apiContext(event), '/csrf')
					.then((c) => ({ ok: true as const, token: c?.csrfToken ?? null, cached: false }))
					.catch(() => ({ ok: false as const, token: null, cached: false }));

	const [meResult, csrfResult] = await Promise.all([mePromise, csrfPromise]);

	if (!meResult.ok) {
		const err = meResult.err;
		const isAuthFailure = err instanceof ApiError && (err.status === 401 || err.status === 403);
		if (isAuthFailure) {
			// Stale credential — drop it and continue as a guest. Also
			// evict any CSRF entry for the now-invalid token.
			event.cookies.delete('jc_token', { path: '/' });
			event.locals.token = null;
			csrfCache.delete(token);
		}
		return { ok: false, shouldRefresh: isAuthFailure };
	}

	event.locals.user = meResult.user;

	// Apply the CSRF token (from cache or fresh fetch). We also set the
	// csrf_token cookie on the browser response so client-side mutations
	// (apps/main/src/lib/api/client.ts) can read it and echo it back as
	// X-CSRF-Token. The backend's Set-Cookie from the server-side /csrf
	// fetch is consumed by SvelteKit's server fetch and never reaches the
	// browser, so without this step the browser would lack the cookie
	// after email/password login (the SvelteKit login route only forwards
	// jc_token, not csrf_token).
	if (event.locals.token && csrfResult.ok && csrfResult.token) {
		event.locals.csrfToken = csrfResult.token;
		// Only populate the cache on a fresh (non-cached) successful fetch.
		if (!csrfResult.cached) {
			csrfCache.set(token, { token: csrfResult.token, expiresAt: now + CSRF_TTL_MS });
		}
		event.cookies.set('csrf_token', csrfResult.token, {
			path: '/',
			httpOnly: false,
			secure: true,
			sameSite: 'lax'
		});
	} else if (event.locals.token) {
		event.locals.csrfToken = null;
	}

	return { ok: true, shouldRefresh: false };
}

/**
 * Attempt a one-time silent refresh using the jc_refresh cookie. On success
 * both jc_token and jc_refresh are rotated and event.locals.token is updated.
 */
async function tryRefreshSession(event: Parameters<Handle>[0]['event']): Promise<boolean> {
	if (!event.cookies.get('jc_refresh')) return false;
	try {
		const result = await refreshSession(apiContext(event));
		setSessionCookie(event, result.access_token);
		if (result.refresh_token) setRefreshCookie(event, result.refresh_token);
		event.locals.token = result.access_token;
		return true;
	} catch (err) {
		// If the refresh token is missing or permanently invalid, clear it so the
		// request degrades cleanly and the browser doesn't keep retrying with a
		// dead token. Network/backend errors leave the cookies in place.
		if (err instanceof ApiError) {
			const code = err.code;
			if (code === 'no_refresh_token' || code === 'invalid_refresh_token') {
				event.cookies.delete('jc_token', { path: '/' });
				event.cookies.delete('jc_refresh', { path: '/' });
				event.locals.token = null;
			}
		}
		return false;
	}
}

export const handle: Handle = async ({ event, resolve }) => {
	// Hostname excludes the port; forwarded to the backend for host-based routing.
	event.locals.requestHost = event.url.hostname;
	event.locals.token = event.cookies.get('jc_token') ?? null;

	// Detect locale prefix in the URL path (e.g. /fr/blog → 'fr', /blog → null).
	// Only 2-letter lowercase prefixes are candidates; the enabled-languages
	// list from system_settings validates whether it's a real locale or part
	// of a non-locale route name. Ref: §18 (i18n language packs), ADR-013.
	const localePrefixMatch = event.url.pathname.match(/^\/([a-z]{2})(?=\/|$)/);
	const pathLocale = localePrefixMatch?.[1] ?? null;

	// Fetch public site settings, shared between theme, locale, and
	// locale-prefix validation. Previously these were fetched on every
	// request that lacked a theme/locale cookie (i.e. most first visits and
	// every locale-prefixed URL). Now served from an in-process cache
	// (settingsCache, TTL SETTINGS_TTL_MS) keyed by request host so the
	// backend is only hit once per TTL window.
	// Ref: conventions/system-settings-catalog.md
	const themeCookie = event.cookies.get('jc_theme');
	const localeCookie = event.cookies.get('jc_locale');
	// Always load public site settings so they are available in
	// event.locals.siteSettings for page load functions.
	const needSettings = true;

	let siteSettings: Record<string, unknown> = {};
	if (needSettings && !building) {
		const requestHost = event.locals.requestHost;
		const cached = settingsCache.get(requestHost);
		const now = Date.now();
		if (cached && cached.expiresAt > now) {
			siteSettings = cached.value;
		} else {
			try {
				const fresh =
					(await apiGet<Record<string, unknown>>(apiContext(event), '/site/settings/public')) ??
					{};
				siteSettings = fresh;
				settingsCache.set(requestHost, { value: fresh, expiresAt: now + SETTINGS_TTL_MS });
			} catch {
				// Serve stale entry if available (transient backend blip),
				// otherwise degrade to empty defaults.
				siteSettings = cached?.value ?? {};
			}
		}
	}

	// Theme: prefer cookie, then appearance.dark_mode_default, then 'light'.
	// 'system' is a valid selection — the SSR markup can't know the OS
	// preference, so data-theme falls back to dark_mode_default and the
	// client store resolves prefers-color-scheme after hydration.
	event.locals.theme =
		themeCookie === 'dark' || themeCookie === 'light' || themeCookie === 'system'
			? themeCookie
			: siteSettings['appearance.dark_mode_default'] === true
				? 'dark'
				: 'light';
	// The value actually written to <html data-theme>. 'system' has no CSS
	// block — render the admin default server-side; the theme store corrects
	// it on hydration.
	event.locals.resolvedTheme =
		event.locals.theme === 'system'
			? siteSettings['appearance.dark_mode_default'] === true
				? 'dark'
				: 'light'
			: event.locals.theme;

	// Locale resolution: URL prefix > cookie > i18n.default_language > 'en'.
	// The enabled-languages list from i18n.enabled_languages (system_settings)
	// validates the prefix/cookie — unknown codes fall back to the default.
	// When a valid prefix is present, the cookie is set and the URL is rewritten
	// to remove the prefix so SvelteKit routes to the existing (unprefixed) page
	// tree. The browser URL stays /fr/blog; the page renders in French.
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
		// Route matching for the prefix happens in the `reroute` hook
		// (src/hooks.ts). Do NOT mutate event.url.pathname here — SSR page
		// data and hydration must see the same URL as the browser.
	} else if (localeCookie && enabledLanguages.includes(localeCookie)) {
		event.locals.locale = localeCookie;
	} else {
		const defaultLang = (siteSettings['i18n.default_language'] as string | undefined) ?? 'en';
		event.locals.locale = enabledLanguages.includes(defaultLang) ? defaultLang : 'en';
	}
	event.locals.user = null;
	event.locals.csrfToken = null;
	event.locals.siteSettings = siteSettings;

	// ADR-063: detect the Japa Academy branded surface by host.
	// academy.japa.to (and configurable custom domains like japauni.com)
	// render the Japa Academy shell instead of the diaspora community shell.
	event.locals.isAcademyHost = isAcademySurface(event.url.hostname);

	// ADR-127: detect the public roadmap surface by host.
	// roadmap.japa.to renders a minimal roadmap-branded shell inside
	// the same app — unlike the university surface it does not redirect.
	// Extra hosts come from the roadmap.custom_domain setting (§113).
	event.locals.isRoadmapHost = isRoadmapSurface(
		event.url.hostname,
		siteSettings['roadmap.custom_domain']
	);

	// Resolve the session and (for authenticated users) the CSRF token.
	// The main app is public: an absent or invalid token simply means "guest"
	// — never a redirect to login (that's the admin app's job).
	//
	// If the access token is expired, attempt a single silent refresh using the
	// HttpOnly jc_refresh cookie before degrading to guest. This keeps SSR pages
	// rendering as the authenticated user without forcing a client-side round-trip.
	//
	// Skip resolution for the client-facing /api/auth/refresh endpoint: the
	// endpoint itself performs the refresh, and running it here first would
	// rotate the refresh token an extra time and waste a backend round-trip.
	// Ref: japa-bugs-hunters.md §5.2 (CSRF) — audit finding C1.
	if (!building && event.url.pathname !== '/api/auth/refresh') {
		if (event.locals.token) {
			const { ok, shouldRefresh } = await resolveSession(event);
			if (!ok && shouldRefresh && event.cookies.get('jc_refresh')) {
				const refreshed = await tryRefreshSession(event);
				if (refreshed) {
					await resolveSession(event);
				}
			}
		} else if (event.cookies.get('jc_refresh')) {
			// jc_token's cookie maxAge equals the JWT TTL, so on natural expiry the
			// browser drops the cookie entirely and locals.token is null. Without
			// this branch the request renders as a guest even though jc_refresh is
			// still valid — silently disabling every authed SSR surface (e.g. the
			// org Manage controls) until client-side JS refreshes the session.
			const refreshed = await tryRefreshSession(event);
			if (refreshed) {
				await resolveSession(event);
			}
		}
	}

	// CSP nonce: generated per request by SvelteKit (kit.csp mode='nonce'
	// in svelte.config.js) and exposed as event.locals.cspNonce, which the
	// layout threads to the staff-gated custom-code blocks
	// (`site.custom_css` / `site.custom_js`). The CSP response header is
	// set by SvelteKit — do NOT set it manually here or the bootstrap
	// script nonce and the header will disagree.
	//
	// The nonce alone does NOT make the payload safe — custom code is a
	// staff-only setting with a server-side audit trail (admin settings
	// writes are audit-logged), custom CSS passes through the
	// sanitizeCustomCss blocklist before render, and custom JS renders only
	// when a super_admin-approved `site.custom_js_approved` flag is set.
	// Admin appearance overrides — compile appearance.* public settings into a
	// CSS block injected into the static <style id="jc-theme-overrides"> tag in
	// app.html. Server-side so there is no flash of default theme.
	// Ref: conventions/system-settings-catalog.md (Appearance / Branding)
	const themeVars = buildThemeOverrides(siteSettings);
	// §57 custom CSS — sanitized and injected into the static
	// <style id="jc-custom-css"> tag in app.html (nonce via %sveltekit.nonce%).
	const customCss = sanitizeCustomCss(
		(siteSettings['site.custom_css'] as string | undefined) ?? ''
	);

	const response = await resolve(event, {
		transformPageChunk: ({ html }) =>
			html
				.replace('%jc.themevars%', () => themeVars)
				.replace('%jc.customcss%', () => customCss)
				.replace('%jc.theme%', () => event.locals.resolvedTheme)
				.replace('lang="en"', `lang="${event.locals.locale}"`)
	});
	response.headers.set('X-Content-Type-Options', 'nosniff');
	response.headers.set('Referrer-Policy', 'strict-origin-when-cross-origin');
	response.headers.set('Permissions-Policy', 'camera=(), microphone=(), geolocation=(), payment=()');
	// Let nginx/CDN flush SSR chunks as they stream — without this, a proxy
	// buffers the full body and streaming SSR gains nothing. Scoped to HTML
	// page loads; API/asset responses keep default buffering.
	if (response.headers.get('content-type')?.startsWith('text/html')) {
		response.headers.set('X-Accel-Buffering', 'no');
	}
	return response;
};
