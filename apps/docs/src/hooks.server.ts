import type { Handle } from '@sveltejs/kit';
import { building } from '$app/environment';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { SessionUser, Locale, DocAudience } from '$lib/types';
import { AUDIENCE_ORDER } from '$lib/types';

/**
 * Resolve which audiences the current session may read.
 *
 * 'api' is always public (ADR-060 — /docs/api is fully public, no session
 * needed). The remaining audiences are gated by the user's groups:
 *   - users:        everyone (authed or guest)
 *   - premium:      users with a 'Premium' or 'Pro' group
 *   - organizations: users with an 'Organization Owner' / 'Org Admin' group
 *   - moderators:   users with 'Moderator' / 'Moderators'
 *   - admin:        users with 'Admin'
 *   - superadmin:   users with 'Super Admin'
 *
 * The nav tree's audience switcher only offers these; gated pages 403 →
 * /docs/access-denied (server-side gating only — never client checks).
 */
function resolveAudiences(user: SessionUser | null): DocAudience[] {
	const groups = user?.groups ?? [];
	const audiences: DocAudience[] = ['users', 'api'];
	if (groups.some((g) => /^(Premium|Pro)$/i.test(g))) audiences.push('premium');
	if (groups.some((g) => /Organization/i.test(g))) audiences.push('organizations');
	if (groups.some((g) => /Moderator/i.test(g))) audiences.push('moderators');
	if (groups.some((g) => g === 'Admin')) audiences.push('admin');
	if (groups.some((g) => g === 'Super Admin')) audiences.push('superadmin');
	// Preserve canonical display order.
	return AUDIENCE_ORDER.filter((a) => audiences.includes(a));
}

export const handle: Handle = async ({ event, resolve }) => {
	// Hostname excludes the port; the backend resolves the tenant from it.
	event.locals.tenantHost = event.url.hostname;
	event.locals.token = event.cookies.get('jc_token') ?? null;

	// Theme + locale: prefer the user's cookie, then fall back to the control
	// panel appearance/i18n settings, then defaults. The settings endpoint is
	// fetched at most once per request (shared between theme + locale).
	const themeCookie = event.cookies.get('jc_theme');
	const localeCookie = event.cookies.get('jc_locale');
	const needSettings = themeCookie !== 'dark' && themeCookie !== 'light';
	const needLocaleSettings = localeCookie !== 'fr' && localeCookie !== 'en';

	let settings: Record<string, unknown> = {};
	if ((needSettings || needLocaleSettings) && !building) {
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

	const defaultLang = (settings['i18n.default_language'] as string | undefined) ?? 'en';
	event.locals.locale = (localeCookie === 'fr' || localeCookie === 'en'
		? localeCookie
		: defaultLang === 'fr'
			? 'fr'
			: 'en') as Locale;

	event.locals.user = null;

	// The docs app is public: an absent or invalid token simply means "guest".
	// Audience gating is resolved from the user's groups (below), not by
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
	}

	event.locals.accessibleAudiences = resolveAudiences(event.locals.user);

	return resolve(event, {
		transformPageChunk: ({ html }) =>
			html
				.replace('%jc.theme%', event.locals.theme)
				.replace('lang="en"', `lang="${event.locals.locale}"`)
	});
};
