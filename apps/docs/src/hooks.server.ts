import type { Handle } from '@sveltejs/kit';
import { building } from '$app/environment';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
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
 */
function resolveAudiences(user: SessionUser | null, audienceGates: Record<string, string>): DocAudience[] {
	const permissions = new Set(user?.permissions ?? []);
	const audiences: DocAudience[] = ['users', 'api'];
	for (const aud of AUDIENCE_ORDER) {
		const required = audienceGates[aud];
		if (required && permissions.has(required)) {
			audiences.push(aud as DocAudience);
		}
	}
	return AUDIENCE_ORDER.filter((a) => audiences.includes(a));
}

export const handle: Handle = async ({ event, resolve }) => {
	// Hostname excludes the port; the backend resolves the tenant from it.
	event.locals.tenantHost = event.url.hostname;
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
	const needSettings =
		(themeCookie !== 'dark' && themeCookie !== 'light') ||
		!localeCookie ||
		pathLocale !== null;

	let settings: Record<string, unknown> = {};
	if (needSettings && !building) {
		try {
			settings =
				(await apiGet<Record<string, unknown>>(apiContext(event), '/site/settings/public')) ?? {};
		} catch {
			settings = {};
		}
	}

	event.locals.theme =
		themeCookie === 'dark' || themeCookie === 'light'
			? themeCookie
			: settings['appearance.dark_mode_default'] === true
				? 'dark'
				: 'light';

	// Locale resolution: URL prefix > cookie > i18n.default_language > 'en'.
	// Valid prefix → set cookie + rewrite URL so SvelteKit routes to the
	// unprefixed page tree. Browser URL stays /fr/docs; page renders French.
	// Ref: §18, conventions/system-settings-catalog.md (i18n.enabled_languages)
	const enabledLanguages =
		((settings['i18n.enabled_languages'] as string | undefined) ?? 'en,fr')
			.split(',')
			.map((l) => l.trim())
			.filter(Boolean);

	if (pathLocale && enabledLanguages.includes(pathLocale)) {
		event.locals.locale = pathLocale;
		event.cookies.set('jc_locale', pathLocale, {
			path: '/',
			maxAge: 60 * 60 * 24 * 365,
			sameSite: 'lax'
		});
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
		if (event.locals.user) {
			try {
				const perms = await apiGet<string[]>(apiContext(event), '/me/permissions');
				event.locals.user.permissions = perms ?? [];
			} catch {
				event.locals.user.permissions = [];
			}
		}
	}

	// DOC-007: fetch the RBAC -> audience gate map from the backend so the
	// frontend never drifts from the source-of-truth in internal/docs.
	event.locals.audienceGates = {};
	if (!building) {
		try {
			const gates = await apiGet<Record<string, string>>(apiContext(event), '/docs/audience-gates');
			event.locals.audienceGates = gates ?? {};
		} catch {
			event.locals.audienceGates = {};
		}
	}

	event.locals.accessibleAudiences = resolveAudiences(event.locals.user, event.locals.audienceGates);

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

	return resolve(event, {
		transformPageChunk: ({ html }) =>
			html
				.replace('%jc.theme%', event.locals.theme)
				.replace('lang="en"', `lang="${event.locals.locale}"`)
	});
};
