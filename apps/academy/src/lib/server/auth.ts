// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
// Server-only session helpers: silent access-token refresh via the jc_refresh
// cookie + shared-domain cookie management. Mirrors the main app's
// $lib/server/auth contract so an expired jc_token (15min TTL) re-mints
// invisibly instead of bouncing the user through the Main-app login.

import { PUBLIC_JC_API_URL } from '$app/env/public';
import { buildApiUrl } from '@japaconnect/api-url';
import { ApiError, API_BASE, apiContext, type ApiContext } from '#lib/server/api.js';
import type { RequestEvent } from '@sveltejs/kit';

const SESSION_COOKIE = 'jc_token';
const REFRESH_COOKIE = 'jc_refresh';
const ACCESS_TOKEN_TTL = 15 * 60;
const REFRESH_TOKEN_TTL = 30 * 24 * 60 * 60;

/**
 * Parent domain shared by sibling surfaces — session cookies are minted on
 * the main app with Domain=.{base}, so cookies set/cleared here must target
 * the same domain. Derived from the public API URL (api.japa.to → .japa.to);
 * empty in dev so cookies stay host-only.
 */
export function sessionCookieDomain(): string {
	try {
		const apiHost = new URL(PUBLIC_JC_API_URL ?? '').hostname;
		return apiHost.startsWith('api.') ? `.${apiHost.slice(4)}` : '';
	} catch {
		return '';
	}
}

export function setSessionCookie(event: RequestEvent, accessToken: string): void {
	event.cookies.set(SESSION_COOKIE, accessToken, {
		path: '/',
		...sessionCookieDomain() ? { domain: sessionCookieDomain() } : {},
		httpOnly: true,
		secure: true,
		sameSite: 'lax',
		maxAge: ACCESS_TOKEN_TTL
	});
}

export function setRefreshCookie(event: RequestEvent, refreshToken: string): void {
	event.cookies.set(REFRESH_COOKIE, refreshToken, {
		path: '/',
		...sessionCookieDomain() ? { domain: sessionCookieDomain() } : {},
		httpOnly: true,
		secure: true,
		sameSite: 'strict',
		maxAge: REFRESH_TOKEN_TTL
	});
}

/** Clear the session cookies — both host-only and shared-domain variants,
 * since cookies minted at different times may exist in either scope. */
export function clearSessionCookies(event: RequestEvent): void {
	event.cookies.delete(SESSION_COOKIE, { path: '/' });
	event.cookies.delete(REFRESH_COOKIE, { path: '/' });
	const domain = sessionCookieDomain();
	if (domain) {
		event.cookies.delete(SESSION_COOKIE, { path: '/', domain });
		event.cookies.delete(REFRESH_COOKIE, { path: '/', domain });
	}
}

/** Extract the jc_refresh token value from a backend response's Set-Cookie
 * headers. The backend rotates the refresh token on /auth/refresh. */
function extractRefreshToken(res: Response): string | null {
	for (const c of res.headers.getSetCookie?.() ?? []) {
		if (c.startsWith('jc_refresh=')) {
			const value = c.split(';')[0].slice(('jc_refresh=').length);
			return value || null;
		}
	}
	return null;
}

/** POST /auth/refresh — exchanges the jc_refresh cookie for a new access
 * token and rotated refresh token. The backend revokes the old refresh
 * token, so call this only when the access token is known to be expired.
 * Throws ApiError on failure. */
async function refreshSession(ctx: ApiContext, refreshCookie: string): Promise<{ access_token: string; refresh_token?: string }> {
	const headers: Record<string, string> = {
		'content-type': 'application/json',
		host: ctx.host,
		'x-forwarded-host': ctx.host,
		'x-jc-surface': 'academy',
		cookie: `jc_refresh=${refreshCookie}`
	};
	if (ctx.clientIP) headers['x-forwarded-for'] = ctx.clientIP;

	const res = await ctx.fetch(buildApiUrl('/auth/refresh', API_BASE), {
		method: 'POST',
		headers,
		body: '{}'
	});

	const payload = await res.json().catch(() => null);
	if (!res.ok) {
		const err = (payload as { error?: { code?: string; message?: string } } | null)?.error;
		throw new ApiError(res.status, err?.code ?? 'api_error', err?.message ?? res.statusText);
	}
	const accessToken = (payload as { data?: { access_token?: string } } | null)?.data?.access_token;
	if (!accessToken) {
		throw new ApiError(500, 'token_error', 'Refresh did not return an access token');
	}
	const refreshToken = extractRefreshToken(res);
	return { access_token: accessToken, refresh_token: refreshToken ?? undefined };
}

/**
 * Attempt a one-time silent refresh using the jc_refresh cookie. On success
 * both jc_token and jc_refresh are rotated (shared domain) and
 * event.locals.token is updated. Permanently-invalid refresh tokens clear
 * the session cookies so the request degrades to guest instead of retrying
 * a dead token on every hit. Returns true when a fresh token is in place.
 */
export async function tryRefreshSession(event: RequestEvent): Promise<boolean> {
	const refreshCookie = event.cookies.get(REFRESH_COOKIE);
	if (!refreshCookie) return false;
	try {
		const result = await refreshSession(apiContext(event), refreshCookie);
		setSessionCookie(event, result.access_token);
		if (result.refresh_token) setRefreshCookie(event, result.refresh_token);
		event.locals.token = result.access_token;
		return true;
	} catch (err) {
		if (err instanceof ApiError && (err.code === 'no_refresh_token' || err.code === 'invalid_refresh_token' || err.code === 'refresh_revoked')) {
			clearSessionCookies(event);
			event.locals.token = null;
		}
		return false;
	}
}
