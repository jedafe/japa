import { error, type Handle } from '@sveltejs/kit';
import { building } from '$app/environment';
import { env } from '$env/dynamic/private';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { SessionUser, ToolSurface } from '$lib/types';

/**
 * ADR-086 / §78: detect which tool surface the request host maps to.
 *
 * The canonical hub host is `tools.japaconnect.com`. Each tool can have its own
 * subdomain (e.g. `remittance.japaconnect.com`) or custom domain (e.g.
 * `japacalculator.com`), configured via `system_settings` keys:
 *   {tool}.subdomain  → {subdomain}.japaconnect.com
 *   {tool}.custom_domain → exact host match
 *
 * The host→surface mapping is built from system_settings on each request. If no
 * tool host matches, the surface defaults to 'hub'. Additional hub hosts can be
 * configured via the JC_TOOLS_HUB_HOSTS env var (comma-separated).
 *
 * Ref: ADR-086 (multi-surface architecture), §78 (Tools Web Surface),
 *      ADR-063 (Japa University host-detection precedent).
 */

/** system_settings keys that control per-tool domains (ADR-086). */
const TOOL_SETTINGS_KEYS: Array<{ key: string; surface: ToolSurface }> = [
	{ key: 'remittance', surface: 'remittance' },
	{ key: 'scholarship_finder', surface: 'scholarships' },
	{ key: 'col_calculator', surface: 'cost-of-living' },
	{ key: 'checklist', surface: 'visa-checklist' },
	{ key: 'assessment', surface: 'readiness' },
	{ key: 'japabot', surface: 'japabot' },
	// V2 functional tools (§§90–97, §§99–101) — geo-aware tool pattern.
	{ key: 'visa_routes', surface: 'visa-routes' },
	{ key: 'appointment_alert', surface: 'embassy-alerter' },
	{ key: 'arrival_checklist', surface: 'arrival-checklist' },
	{ key: 'visa_timeline', surface: 'processing-times' },
	{ key: 'credential_checker', surface: 'credential-checker' },
	{ key: 'cv_formatter', surface: 'cv-formatter' },
	{ key: 'housing_guide', surface: 'housing-guide' },
	{ key: 'attestation_guide', surface: 'attestation-guide' },
	{ key: 'language_test_hub', surface: 'language-test' },
	{ key: 'tax_calculator', surface: 'tax-calculator' },
	{ key: 'community_locator', surface: 'community-locator' },
	// SEO growth tools (§§107–110).
	{ key: 'crs_calculator', surface: 'crs-calculator' },
	{ key: 'pof_calculator', surface: 'pof-calculator' },
	{ key: 'eca_guide', surface: 'eca-guide' },
	{ key: 'sop_generator', surface: 'sop-generator' }
];

/** Default subdomains per tool (from system-settings-catalog.md). */
const DEFAULT_SUBDOMAINS: Record<string, string> = {
	remittance: 'remittance',
	scholarship_finder: 'scholarships',
	col_calculator: 'cost-of-living',
	checklist: 'visa-checklist',
	assessment: 'readiness',
	japabot: 'japabot',
	// V2 functional tools — default subdomains match the route slugs.
	visa_routes: 'visa-routes',
	appointment_alert: 'embassy-alerter',
	arrival_checklist: 'arrival-checklist',
	visa_timeline: 'processing-times',
	credential_checker: 'credential-checker',
	cv_formatter: 'cv-formatter',
	housing_guide: 'housing-guide',
	attestation_guide: 'attestation-guide',
	language_test_hub: 'language-test',
	tax_calculator: 'tax-calculator',
	community_locator: 'community-locator',
	// SEO growth tools — default subdomains match the route slugs.
	crs_calculator: 'crs-calculator',
	pof_calculator: 'pof-calculator',
	eca_guide: 'eca-guide',
	sop_generator: 'sop-generator'
};

/**
 * Build the host→surface map from system_settings. Custom domains take
 * precedence (exact match), then subdomains ({sub}.japaconnect.com).
 *
 * Accepts the already-fetched public settings (shared with theme/locale
 * resolution) to avoid a duplicate backend round-trip per request.
 */
function resolveToolSurfaceFromSettings(
	host: string,
	settings: Record<string, unknown>
): ToolSurface {
	const canonicalHub = 'tools.japaconnect.com';
	const extraHubHosts =
		env.JC_TOOLS_HUB_HOSTS?.split(',')
			.map((h) => h.trim().toLowerCase())
			.filter(Boolean) ?? [];

	if (host === canonicalHub || extraHubHosts.includes(host)) {
		return 'hub';
	}

	// ADR-086 / §78: the tools hub itself may have a custom_domain setting
	// (e.g. `japatools.com`). Match it before per-tool domains so a hub
	// custom domain is never misread as a single-tool host.
	// Ref: conventions/system-settings-catalog.md (tools.custom_domain).
	const hubCustomDomain = settings['tools.custom_domain'] as string | undefined;
	if (hubCustomDomain && hubCustomDomain.toLowerCase() === host) {
		return 'hub';
	}

	for (const { key, surface } of TOOL_SETTINGS_KEYS) {
		const customDomain = settings[`${key}.custom_domain`] as string | undefined;
		if (customDomain && customDomain.toLowerCase() === host) {
			return surface;
		}
	}

	for (const { key, surface } of TOOL_SETTINGS_KEYS) {
		const subdomain = (settings[`${key}.subdomain`] as string | undefined) ?? DEFAULT_SUBDOMAINS[key];
		if (`${subdomain}.japaconnect.com` === host) {
			return surface;
		}
	}

	return 'hub';
}

export const handle: Handle = async ({ event, resolve }) => {
	// Hostname excludes the port; forwarded to the backend for host-based routing.
	event.locals.requestHost = event.url.hostname;
	event.locals.token = event.cookies.get('jc_token') ?? null;

	// Detect locale prefix in the URL path (e.g. /fr/visa-checklist → 'fr').
	// Ref: §18 (i18n language packs), ADR-013.
	const localePrefixMatch = event.url.pathname.match(/^\/([a-z]{2})(?=\/|$)/);
	const pathLocale = localePrefixMatch?.[1] ?? null;

	// Fetch public site settings once per request and reuse for theme, locale,
	// locale-prefix validation, and tool-surface resolution. Previously these
	// were separate fetches to /site/settings/public on every request.
	// Ref: conventions/system-settings-catalog.md
	const ctx = apiContext(event);
	let siteSettings: Record<string, unknown> = {};
	if (!building) {
		try {
			siteSettings = (await apiGet<Record<string, unknown>>(ctx, '/site/settings/public')) ?? {};
		} catch {
			siteSettings = {};
		}
	}

	// Feature flags fetched once per request and reused by layout/page load
	// functions. Degrades gracefully to an empty object on error.
	let features: Record<string, boolean> = {};
	if (!building) {
		try {
			features = (await apiGet<Record<string, boolean>>(ctx, '/feature-flags')) ?? {};
		} catch {
			features = {};
		}
	}
	event.locals.features = features;

	// Theme: prefer cookie, then appearance.dark_mode_default, then 'light'.
	const themeCookie = event.cookies.get('jc_theme');
	event.locals.theme =
		themeCookie === 'dark' || themeCookie === 'light'
			? themeCookie
			: siteSettings['appearance.dark_mode_default'] === true
				? 'dark'
				: 'light';

	// Locale resolution: URL prefix > cookie > i18n.default_language > 'en'.
	// Valid prefix → set cookie + rewrite URL so SvelteKit routes to the
	// unprefixed page tree. Browser URL stays /fr/visa-checklist; page renders
	// in French. Ref: §18, ADR-013.
	const localeCookie = event.cookies.get('jc_locale');
	const enabledLanguages =
		((siteSettings['i18n.enabled_languages'] as string | undefined) ?? 'en,fr')
			.split(',')
			.map((l) => l.trim())
			.filter(Boolean);

	// The reroute hook (src/hooks.ts) strips any 2-letter path segment before
	// route matching, so an unsupported prefix like /zz/visa-checklist would
	// otherwise render the unprefixed page. 404 it to preserve the previous
	// behavior for non-enabled locales.
	if (pathLocale && !enabledLanguages.includes(pathLocale)) {
		throw error(404, { code: 'not_found', message: 'Not found' });
	}

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
		const defaultLang = (siteSettings['i18n.default_language'] as string | undefined) ?? 'en';
		event.locals.locale = enabledLanguages.includes(defaultLang) ? defaultLang : 'en';
	}

	event.locals.user = null;

	// ADR-086 / §78: resolve the tool surface from the request host.
	// Uses the already-fetched siteSettings instead of a second fetch.
	event.locals.toolSurface = resolveToolSurfaceFromSettings(
		event.url.hostname,
		siteSettings
	);

	// Store the settings for reuse by layout/page load functions (avoids
	// redundant /site/settings/public fetches in +layout.server.ts etc.).
	event.locals.siteSettings = siteSettings;

	// Resolve the session. The Tools app is public: an absent or invalid token
	// simply means "guest" — never a redirect to login. Progress-saving and
	// alert-creation gate behind auth (with registration prompts).
	if (event.locals.token && !building) {
		try {
			event.locals.user = await apiGet<SessionUser>(ctx, '/me');
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
	// response so client-side CSR mutations can echo it back.
	// Ref: japa-bugs-hunters.md §5.2 (CSRF) — audit finding C1.
	event.locals.csrfToken = null;
	if (event.locals.token && !building) {
		try {
			const csrf = await apiGet<{ csrfToken: string }>(ctx, '/csrf');
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
