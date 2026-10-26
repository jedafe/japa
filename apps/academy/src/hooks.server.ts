import type { Handle } from '@sveltejs/kit';
import { building } from '$app/environment';
import { env } from '$env/dynamic/private';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import { buildThemeOverrides } from '@japaconnect/ui/theme';
import { isRtl } from '$lib/i18n/rtl';

/**
 * In-process cache for GET /site/settings/public (same pattern as the main
 * app's hooks). Appearance/theme settings are global and change rarely, so
 * a 30s TTL avoids a serial backend round-trip on every request while still
 * applying admin changes quickly. Keyed by request host; stale entries are
 * served on fetch failure; misses are not cached.
 * Ref: conventions/system-settings-catalog.md
 */
const SETTINGS_TTL_MS = 30_000;
const settingsCache = new Map<string, { value: Record<string, unknown>; expiresAt: number }>();
import type { SessionUser } from '$lib/types';

/**
 * Japa Academy — the single learning surface (ADR-063 consolidation).
 * The former dual-shell (Learn vs University) has been unified: the app
 * always renders the Japa Academy shell with the creator-economy
 * marketplace. The canonical host is configured via the
 * `academy.subdomain` and `academy.custom_domain` system_settings
 * (ADR-063 §2); the edge/proxy routes the configured host to this app
 * and the backend uses the hostname for host-based routing. The
 * academy.japa.to host redirects to the configured academy host
 * (handled by the edge/proxy).
 * Ref: japaconnect-engineering/conventions/system-settings-catalog.md
 */
// Environments that must never be indexed (fail-open: unset/unrecognized
// env is treated as indexable so misconfigured prod can't deindex itself).
const BLOCKS_INDEXING = new Set(['development', 'dev', 'staging', 'stage', 'test', 'preview', 'local']);

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
	// Always fetch: admin appearance overrides (appearance.*) are compiled into
	// %jc.themevars% on every response, so settings are needed on all requests.
	const needSettings = true;

	let siteSettings: Record<string, unknown> = {};
	if (needSettings && !building) {
		const host = event.url.host;
		const cached = settingsCache.get(host);
		const now = Date.now();
		if (cached && cached.expiresAt > now) {
			siteSettings = cached.value;
		} else {
			try {
				siteSettings =
					(await apiGet<Record<string, unknown>>(apiContext(event), '/site/settings/public')) ?? {};
				settingsCache.set(host, { value: siteSettings, expiresAt: now + SETTINGS_TTL_MS });
			} catch {
				siteSettings = cached?.value ?? {};
			}
		}
	}

	// Theme: prefer cookie, then appearance.dark_mode_default, then 'light'.
	// 'system' is a valid selection — the SSR markup can't know the OS
	// preference, so resolvedTheme falls back to dark_mode_default and the
	// client theme store corrects data-theme after hydration.
	event.locals.theme =
		themeCookie === 'dark' || themeCookie === 'light' || themeCookie === 'system'
			? themeCookie
			: siteSettings['appearance.dark_mode_default'] === true
				? 'dark'
				: 'light';
	event.locals.resolvedTheme =
		event.locals.theme === 'system'
			? siteSettings['appearance.dark_mode_default'] === true
				? 'dark'
				: 'light'
			: event.locals.theme;

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
	event.locals.siteSettings = siteSettings;

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

	// §124 — PWA head tags: theme-color (pwa.theme_color → appearance
	// .primary_color) + iOS install meta. Admin-controlled name is
	// attribute-escaped; colors are hex-whitelist-validated.
	// Ref: .jeos/sections/124-progressive-web-app.md
	const pwaThemeColor = (() => {
		for (const v of [siteSettings['pwa.theme_color'], siteSettings['appearance.primary_color']]) {
			if (typeof v === 'string' && /^#[0-9a-fA-F]{3,8}$/.test(v.trim())) return v.trim();
		}
		return '#3b49df';
	})();
	const pwaAppName = `${String(siteSettings['pwa.short_name'] || siteSettings['pwa.name'] || siteSettings['general.site_name'] || 'JapaConnect').replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;')} Academy`;
	// The admin-uploaded manifest icon doubles as the iOS home-screen icon —
	// bundled 180px default when unset.
	const pwaAppleIcon = String(siteSettings['pwa.icon_192'] || '/apple-touch-icon.png')
		.replace(/&/g, '&amp;')
		.replace(/"/g, '&quot;')
		.replace(/</g, '&lt;');
	const pwaMeta = [
		`<meta name="theme-color" content="${pwaThemeColor}" />`,
		`<meta name="mobile-web-app-capable" content="yes" />`,
		`<meta name="apple-mobile-web-app-capable" content="yes" />`,
		`<meta name="apple-mobile-web-app-status-bar-style" content="default" />`,
		`<meta name="apple-mobile-web-app-title" content="${pwaAppName}" />`,
		`<meta name="application-name" content="${pwaAppName}" />`,
		`<link rel="apple-touch-icon" href="${pwaAppleIcon}" />`
	].join('');

	const response = await resolve(event, {
		transformPageChunk: ({ html }) =>
			html
				.replace('%jc.pwa-meta%', () => pwaMeta)
				.replace('%jc.themevars%', () => buildThemeOverrides(siteSettings))
				.replace('%jc.theme%', () => event.locals.resolvedTheme)
				.replace(
					'lang="en"',
					() => `lang="${event.locals.locale}" dir="${isRtl(event.locals.locale ?? '') ? 'rtl' : 'ltr'}"`
				)
	});
	response.headers.set('X-Content-Type-Options', 'nosniff');
	response.headers.set('Referrer-Policy', 'strict-origin-when-cross-origin');
	response.headers.set('Permissions-Policy', 'camera=(), microphone=(), geolocation=(), payment=()');
	if (BLOCKS_INDEXING.has((env.PUBLIC_JC_ENV ?? env.JC_ENV ?? '').toLowerCase())) {
		response.headers.set('X-Robots-Tag', 'noindex');
	}
	if (response.headers.get('content-type')?.startsWith('text/html')) {
		response.headers.set('X-Accel-Buffering', 'no');
	}
	// §124 — personalized responses are never cacheable (service worker,
	// browser HTTP cache, or shared proxy). Endpoints that set their own
	// Cache-Control keep it.
	if (event.locals.user && !response.headers.has('cache-control')) {
		const ct = response.headers.get('content-type') ?? '';
		if (ct.startsWith('text/html') || ct.startsWith('application/json')) {
			response.headers.set('cache-control', 'private, no-store');
		}
	}
	return response;
};
