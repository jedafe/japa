import type { Handle } from '@sveltejs/kit';
import { building } from '$app/environment';
import { env } from '$env/dynamic/private';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { SessionUser, ThemeName, Locale } from '$lib/types';

/**
 * In-process cache for GET /site/settings/public.
 * Ref: conventions/system-settings-catalog.md
 */
const SETTINGS_TTL_MS = 30_000;
interface CachedSettings {
	value: Record<string, unknown>;
	expiresAt: number;
}
const settingsCache = new Map<string, CachedSettings>();

/**
 * In-process cache for the per-session CSRF token.
 * Ref: §5.2 (CSRF double-submit)
 */
const CSRF_TTL_MS = 5 * 60_000;
interface CachedCsrf {
	token: string;
	expiresAt: number;
}
const csrfCache = new Map<string, CachedCsrf>();

/**
 * Resolve theme from cookie or site settings default.
 */
function resolveTheme(cookies: { get(name: string): string | undefined }, settings: Record<string, unknown>): ThemeName {
	const cookie = cookies.get('jc_theme');
	if (cookie === 'dark' || cookie === 'light') return cookie;
	const defaultTheme = settings?.['theme.default'];
	if (defaultTheme === 'dark' || defaultTheme === 'light') return defaultTheme;
	return 'dark';
}

/**
 * Resolve locale from URL prefix, cookie, or site settings default.
 */
function resolveLocale(cookies: { get(name: string): string | undefined }, settings: Record<string, unknown>): Locale {
	const cookie = cookies.get('jc_locale');
	if (cookie) return cookie as Locale;
	const defaultLocale = (settings?.['i18n.default_language'] as string) ?? 'en';
	return defaultLocale as Locale;
}

async function fetchPublicSettings(ctx: Awaited<ReturnType<typeof apiContext>>): Promise<Record<string, unknown>> {
	try {
		return await apiGet<Record<string, unknown>>(ctx, '/site/settings/public');
	} catch {
		return {};
	}
}

async function fetchCsrf(ctx: Awaited<ReturnType<typeof apiContext>>): Promise<string | null> {
	try {
		const res = await apiGet<{ token: string }>(ctx, '/csrf');
		return res.token ?? null;
	} catch {
		return null;
	}
}

export const handle: Handle = async ({ event, resolve }) => {
	if (building) {
		event.locals.token = null;
		event.locals.user = null;
		event.locals.theme = 'dark';
		event.locals.locale = 'en';
		event.locals.tenantHost = 'localhost';
		event.locals.csrfToken = null;
		event.locals.siteSettings = {};
		event.locals.features = {};
		return resolve(event);
	}

	const tenantHost = (event.request.headers.get('host') ?? env.PUBLIC_DEFAULT_HOST ?? 'localhost').split(':')[0];
	event.locals.tenantHost = tenantHost;

	const tokenCookie = event.cookies.get('jc_token');
	event.locals.token = tokenCookie ?? null;

	const ctx = apiContext(event);

	// Public settings cache.
	let siteSettings: Record<string, unknown> = {};
	const now = Date.now();
	const cachedSettings = settingsCache.get(tenantHost);
	if (cachedSettings && cachedSettings.expiresAt > now) {
		siteSettings = cachedSettings.value;
	} else {
		siteSettings = await fetchPublicSettings(ctx);
		settingsCache.set(tenantHost, { value: siteSettings, expiresAt: now + SETTINGS_TTL_MS });
	}

	// Resolve user.
	let user: SessionUser | null = null;
	if (tokenCookie) {
		try {
			user = await apiGet<SessionUser>(ctx, '/me');
		} catch {
			user = null;
		}
	}

	// CSRF cache.
	let csrfToken: string | null = null;
	if (tokenCookie) {
		const cachedCsrf = csrfCache.get(tokenCookie);
		if (cachedCsrf && cachedCsrf.expiresAt > now) {
			csrfToken = cachedCsrf.token;
		} else {
			csrfToken = await fetchCsrf(ctx);
			if (csrfToken) csrfCache.set(tokenCookie, { token: csrfToken, expiresAt: now + CSRF_TTL_MS });
		}
	}

	// Feature flags.
	let features: Record<string, boolean> = {};
	try {
		features = await apiGet<Record<string, boolean>>(ctx, '/feature-flags');
	} catch {
		features = {};
	}

	event.locals.user = user;
	event.locals.theme = resolveTheme(event.cookies, siteSettings);
	event.locals.locale = resolveLocale(event.cookies, siteSettings);
	event.locals.csrfToken = csrfToken;
	event.locals.siteSettings = siteSettings;
	event.locals.features = features;

	const response = await resolve(event);
	return response;
};
