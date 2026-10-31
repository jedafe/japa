import { error, type Handle } from '@sveltejs/kit';
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

/**
 * The audience gate map and canonical audience order change only when the
 * backend ships new audiences/permissions — cache them briefly (60s) like
 * the public settings instead of paying two backend round-trips per page.
 */
const AUDIENCE_META_TTL_MS = 60_000;
let audienceMetaCache: {
	gates: Record<string, string>;
	order: DocAudience[];
	expiresAt: number;
} | null = null;
import type { SessionUser, DocAudience } from '$lib/types';
import { AUDIENCE_ORDER } from '$lib/types';

/**
 * Resolve which audiences the current session may read.
 *
 * 'api' is always public (ADR-060 — /docs/api is fully public, no session
 * needed). The remaining audiences are gated by RBAC permission nodes
 * fetched from GET /docs/audience-gates (DOC-007).
 *
 * The nav tree's audience switcher only offers these; gated pages 403 →
 * /docs/access-denied (server-side gating only — never client checks).
 *
 * The canonical order comes from the backend (GET /docs/audiences) so the
 * UI never drifts from the source of truth (Rule 16).
 */
function resolveAudiences(
	user: SessionUser | null,
	audienceGates: Record<string, string>,
	audienceOrder: DocAudience[]
): DocAudience[] {
	const permissions = new Set(user?.permissions ?? []);
	// The "*" system wildcard (super_admin) implicitly holds every node —
	// mirror the backend's HasPermission semantics.
	const wildcard = permissions.has('*');
	// Mirror the backend's accessibleAudiences: an audience absent from the
	// gates map is public; a gated audience requires its permission node.
	return audienceOrder.filter((aud) => {
		const required = audienceGates[aud];
		return !required || wildcard || permissions.has(required);
	});
}

// Environments that must never be indexed (fail-open: unset/unrecognized
// env is treated as indexable so misconfigured prod can't deindex itself).
const BLOCKS_INDEXING = new Set(['development', 'dev', 'staging', 'stage', 'test', 'preview', 'local']);

export const handle: Handle = async ({ event, resolve }) => {
	// Hostname excludes the port; forwarded to the backend for host-based routing.
	event.locals.requestHost = event.url.hostname;
	event.locals.token = event.cookies.get('jc_token') ?? null;

	// Detect locale prefix in the URL path (e.g. /fr/docs → 'fr').
	// Ref: §18 (i18n language packs), ADR-013.
	const localePrefixMatch = event.url.pathname.match(/^\/([a-z]{2})(?=\/|$)/);
	const pathLocale = localePrefixMatch?.[1] ?? null;

	// Theme + locale: prefer the user's cookie, then fall back to the control
	// panel appearance/i18n settings, then defaults. The settings endpoint is
	// fetched at most once per request (shared between theme + locale + prefix
	// validation).
	const themeCookie = event.cookies.get('jc_theme');
	const localeCookie = event.cookies.get('jc_locale');
	// Always fetch: admin appearance overrides (appearance.*) are compiled into
	// %jc.themevars% on every response, so settings are needed on all requests.
	const needSettings = true;

	let settings: Record<string, unknown> = {};
	if (needSettings && !building) {
		const host = event.url.host;
		const cached = settingsCache.get(host);
		const now = Date.now();
		if (cached && cached.expiresAt > now) {
			settings = cached.value;
		} else {
			try {
				settings =
					(await apiGet<Record<string, unknown>>(apiContext(event), '/site/settings/public')) ?? {};
				settingsCache.set(host, { value: settings, expiresAt: now + SETTINGS_TTL_MS });
			} catch {
				settings = cached?.value ?? {};
			}
		}
	}

	// 'system' is a valid selection — the SSR markup can't know the OS
	// preference, so resolvedTheme falls back to dark_mode_default and the
	// client theme store corrects data-theme after hydration.
	event.locals.theme =
		themeCookie === 'dark' || themeCookie === 'light' || themeCookie === 'system'
			? themeCookie
			: settings['appearance.dark_mode_default'] === true
				? 'dark'
				: 'light';
	event.locals.resolvedTheme =
		event.locals.theme === 'system'
			? settings['appearance.dark_mode_default'] === true
				? 'dark'
				: 'light'
			: event.locals.theme;

	// Locale resolution: URL prefix > cookie > i18n.default_language > 'en'.
	// Valid prefix → set cookie + rewrite URL so SvelteKit routes to the
	// unprefixed page tree. Browser URL stays /fr/docs; page renders French.
	// Ref: §18, conventions/system-settings-catalog.md (i18n.enabled_languages)
	const enabledLanguages =
		((settings['i18n.enabled_languages'] as string | undefined) ?? 'en,fr')
			.split(',')
			.map((l) => l.trim())
			.filter(Boolean);

	// A 2-letter prefix that is not an enabled language is not a docs route
	// (the reroute hook stripped it for route matching) — return a real 404
	// instead of silently serving default-locale content.
	if (pathLocale && !enabledLanguages.includes(pathLocale)) {
		throw error(404, { code: 'not_found', message: 'Not found' });
	}

	if (pathLocale && enabledLanguages.includes(pathLocale)) {
		event.locals.locale = pathLocale;
		// Not HttpOnly: the language_toggle widget writes this cookie from
		// client JS (switchLanguage), and JS cannot overwrite an HttpOnly
		// cookie — a HttpOnly set here would permanently break the toggle
		// for anyone who has ever visited a locale-prefixed URL. The value
		// is a UI preference (like jc_theme), not a credential.
		event.cookies.set('jc_locale', pathLocale, {
			path: '/',
			maxAge: 60 * 60 * 24 * 365,
			httpOnly: false,
			sameSite: 'lax'
		});
		// Normalize event.url so downstream loads (sidebar widget resolution,
		// login redirect targets) see the canonical unprefixed path. Routing
		// itself is handled by the reroute hook in src/hooks.ts.
		event.url.pathname = event.url.pathname.slice(3) || '/';
	} else if (localeCookie && enabledLanguages.includes(localeCookie)) {
		event.locals.locale = localeCookie;
	} else {
		const defaultLang = (settings['i18n.default_language'] as string | undefined) ?? 'en';
		event.locals.locale = enabledLanguages.includes(defaultLang) ? defaultLang : 'en';
	}

	event.locals.user = null;

	// The docs app is public: an absent or invalid token simply means "guest".
	// Audience gating is resolved from /me/permissions (DOC-007), not by
	// redirecting to login — guests see 'users' + 'api'.
	if (event.locals.token && !building) {
		try {
			event.locals.user = await apiGet<SessionUser>(apiContext(event), '/me');
		} catch (err) {
			if (err instanceof ApiError && (err.status === 401 || err.status === 403)) {
				event.cookies.delete('jc_token', { path: '/' });
				event.locals.token = null;
			}
			// Any other failure (backend unreachable) degrades to guest.
		}

		// DOC-007: resolve docs audiences from RBAC permission nodes, not groups.
		// GET /me/permissions returns { permissions: string[] }, not a bare array.
		if (event.locals.user) {
			try {
				const res = await apiGet<{ permissions?: string[] } | string[]>(
					apiContext(event),
					'/me/permissions'
				);
				event.locals.user.permissions = Array.isArray(res)
					? res
					: (res?.permissions ?? []);
			} catch {
				event.locals.user.permissions = [];
			}
		}
	}

	// DOC-007: fetch the RBAC -> audience gate map from the backend so the
	// frontend never drifts from the source-of-truth in internal/docs.
	// DOC-009: canonical audience order from GET /docs/audiences (Rule 16).
	// Both are near-static — fetched together, cached for 60s, and the
	// compile-time constant is the fallback when the backend is unreachable.
	event.locals.audienceGates = {};
	event.locals.audienceOrder = AUDIENCE_ORDER;
	if (!building) {
		const now = Date.now();
		if (audienceMetaCache && audienceMetaCache.expiresAt > now) {
			event.locals.audienceGates = audienceMetaCache.gates;
			event.locals.audienceOrder = audienceMetaCache.order;
		} else {
			const ctx = apiContext(event);
			const [gatesRes, orderRes] = await Promise.allSettled([
				apiGet<Record<string, string>>(ctx, '/docs/audience-gates'),
				apiGet<DocAudience[]>(ctx, '/docs/audiences')
			]);
			const gates =
				gatesRes.status === 'fulfilled' && gatesRes.value ? gatesRes.value : {};
			const order =
				orderRes.status === 'fulfilled' && orderRes.value?.length
					? orderRes.value
					: AUDIENCE_ORDER;
			event.locals.audienceGates = gates;
			event.locals.audienceOrder = order;
			// Only cache a successful fetch — caching an empty/failed gates
			// map would briefly treat every audience as ungated.
			if (gatesRes.status === 'fulfilled' && Object.keys(gates).length > 0) {
				audienceMetaCache = { gates, order, expiresAt: now + AUDIENCE_META_TTL_MS };
			}
		}
	}

	// ADR-058: nav collapsed state is persisted via a durable cookie. The
	// backend /me/account endpoint does not currently store UI preferences,
	// so a cookie is the source of truth for the docs shell.
	event.locals.navCollapsed = event.cookies.get('jc_nav_collapsed') === 'true';

	event.locals.accessibleAudiences = resolveAudiences(
		event.locals.user,
		event.locals.audienceGates,
		event.locals.audienceOrder
	);

	// Fetch the double-submit CSRF token for authenticated users so server-side
	// mutations (form actions, apiPost/apiPatch/apiDelete) can attach the
	// X-CSRF-Token header. Also mirror the csrf_token cookie onto the browser
	// response so any client-side fetches can echo it back. The backend's
	// Set-Cookie from the server-side /csrf fetch is consumed by SvelteKit's
	// server fetch and never reaches the browser, so we set it explicitly here.
	// Ref: japa-bugs-hunters.md §5.2 (CSRF) — audit finding C1.
	event.locals.csrfToken = null;
	if (event.locals.token && !building) {
		try {
			const csrf = await apiGet<{ csrfToken: string }>(apiContext(event), '/csrf');
			event.locals.csrfToken = csrf?.csrfToken ?? null;
			if (event.locals.csrfToken) {
				// `secure` must be false in local HTTP dev so the browser accepts
				// the csrf_token cookie; production HTTPS still sets it securely.
				const isHttps = event.url.protocol === 'https:';
				event.cookies.set('csrf_token', event.locals.csrfToken, {
					path: '/',
					httpOnly: false,
					secure: isHttps,
					sameSite: 'lax'
				});
			}
		} catch {
			event.locals.csrfToken = null;
		}
	}

	event.locals.siteSettings = settings;

	// §124 — PWA head tags: theme-color (pwa.theme_color → appearance
	// .primary_color) + iOS install meta. Admin-controlled name is
	// attribute-escaped; colors are hex-whitelist-validated.
	// Ref: .jeos/sections/124-progressive-web-app.md
	const pwaThemeColor = (() => {
		for (const v of [settings['pwa.theme_color'], settings['appearance.primary_color']]) {
			if (typeof v === 'string' && /^#[0-9a-fA-F]{3,8}$/.test(v.trim())) return v.trim();
		}
		return '#3b49df';
	})();
	const pwaAppName = `${String(settings['pwa.short_name'] || settings['pwa.name'] || settings['general.site_name'] || 'JapaConnect').replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;')} Docs`;
	// The admin-uploaded manifest icon doubles as the iOS home-screen icon —
	// bundled 180px default when unset.
	const pwaAppleIcon = String(settings['pwa.icon_192'] || '/apple-touch-icon.png')
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
				.replace('%jc.themevars%', () => buildThemeOverrides(settings))
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
