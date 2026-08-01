import type { Handle } from '@sveltejs/kit';
import { building } from '$app/environment';
import { env } from '$env/dynamic/private';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { SessionUser, Locale } from '$lib/types';

/**
 * ADR-063: detect whether the request host is the Japa University branded
 * surface. The canonical host is university.japaconnect.com; additional custom
 * domains (e.g. japauni.com) can be configured via the JC_UNIVERSITY_HOSTS env
 * var as a comma-separated list.
 *
 * On the Learn app, the default shell is the Learn shell (ADR-060); the
 * university host swaps to the Japa University shell variant.
 */
function isUniversitySurface(host: string): boolean {
	const canonical = 'university.japaconnect.com';
	const configured =
		env.JC_UNIVERSITY_HOSTS?.split(',')
			.map((h) => h.trim().toLowerCase())
			.filter(Boolean) ?? [];
	const hosts = [canonical, ...configured];
	return hosts.includes(host.toLowerCase());
}

export const handle: Handle = async ({ event, resolve }) => {
	// Hostname excludes the port; the backend resolves the tenant from it.
	event.locals.tenantHost = event.url.hostname;
	event.locals.token = event.cookies.get('jc_token') ?? null;

	// Theme: prefer the user's cookie, then fall back to the control panel
	// appearance.dark_mode_default setting, then 'light'.
	// Ref: conventions/system-settings-catalog.md (appearance.dark_mode_default)
	const themeCookie = event.cookies.get('jc_theme');
	if (themeCookie === 'dark' || themeCookie === 'light') {
		event.locals.theme = themeCookie;
	} else {
		try {
			const settings =
				(await apiGet<Record<string, unknown>>(apiContext(event), '/site/settings/public')) ?? {};
			event.locals.theme = settings['appearance.dark_mode_default'] === true ? 'dark' : 'light';
		} catch {
			event.locals.theme = 'light';
		}
	}

	// Locale: prefer the user's cookie, then fall back to the control panel
	// i18n.default_language setting, then 'en'.
	const localeCookie = event.cookies.get('jc_locale');
	if (localeCookie === 'fr' || localeCookie === 'en') {
		event.locals.locale = localeCookie as Locale;
	} else {
		try {
			const settings =
				(await apiGet<Record<string, unknown>>(apiContext(event), '/site/settings/public')) ?? {};
			const defaultLang = (settings['i18n.default_language'] as string | undefined) ?? 'en';
			event.locals.locale = (defaultLang === 'fr' ? 'fr' : 'en') as Locale;
		} catch {
			event.locals.locale = 'en' as Locale;
		}
	}
	event.locals.user = null;

	// ADR-063: detect the Japa University branded surface by host.
	event.locals.isUniversityHost = isUniversitySurface(event.url.hostname);

	// Resolve the session. The Learn app is public: an absent or invalid token
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
