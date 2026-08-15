// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
// Server-only auth helpers: cookie management + proxy functions that call the
// backend auth endpoints and set the jc_token cookie. Used by both the (auth)
// route form actions and the /api/auth/* proxy endpoints (for the modal).

import { API_V1, ApiError, apiContext, type ApiContext } from '$lib/server/api';
import type { AuthSession, AuthTotpChallenge, AuthUser, OnboardingStatus } from '$lib/types';
import type { RequestEvent } from '@sveltejs/kit';

/** Access token cookie name — read by hooks.server.ts to resolve the session. */
export const SESSION_COOKIE = 'jc_token';
/** Refresh token cookie name — set by the backend, forwarded by the SSR proxy. */
export const REFRESH_COOKIE = 'jc_refresh';
/** Access token TTL in seconds (15 minutes — matches the backend JWT expiry). */
const ACCESS_TOKEN_TTL = 15 * 60;
/** Refresh token TTL in seconds (30 days — matches the backend default). */
const REFRESH_TOKEN_TTL = 30 * 24 * 60 * 60;

/** Set the jc_token cookie on the response. */
export function setSessionCookie(event: RequestEvent, accessToken: string): void {
	event.cookies.set(SESSION_COOKIE, accessToken, {
		path: '/',
		httpOnly: true,
		secure: true,
		sameSite: 'lax',
		maxAge: ACCESS_TOKEN_TTL
	});
}

/** Set the jc_refresh cookie on the response. The value comes from the
 * backend's Set-Cookie header; we re-set it with path=/ so the SvelteKit
 * SSR load functions can forward it to /me/sessions for current-session
 * identification (JEOS §50). */
export function setRefreshCookie(event: RequestEvent, refreshToken: string): void {
	event.cookies.set(REFRESH_COOKIE, refreshToken, {
		path: '/',
		httpOnly: true,
		secure: true,
		sameSite: 'strict',
		maxAge: REFRESH_TOKEN_TTL
	});
}

/** Clear the jc_token cookie (logout). */
export function clearSessionCookie(event: RequestEvent): void {
	event.cookies.delete(SESSION_COOKIE, { path: '/' });
	event.cookies.delete(REFRESH_COOKIE, { path: '/' });
}

/** Extract the jc_refresh token value from a backend response's Set-Cookie
 * header. Returns null when the cookie is absent. */
function extractRefreshToken(res: Response): string | null {
	const cookies = res.headers.getSetCookie?.() ?? [];
	for (const c of cookies) {
		// Parse "jc_refresh=VALUE; Path=/api/v1; ..."
		if (c.startsWith('jc_refresh=')) {
			const value = c.split(';')[0].slice('jc_refresh='.length);
			return value || null;
		}
	}
	return null;
}

/** POST /auth/login — returns { access_token, user, refresh_token } or, when
 * the account has TOTP enabled, { totp_required, challenge_token, user }.
 * Throws ApiError. The refresh_token is extracted from the backend's
 * Set-Cookie header so the SSR proxy can re-set it as a browser cookie. */
export async function authenticateLogin(
	ctx: ApiContext,
	email: string,
	password: string,
	website?: string
): Promise<AuthSession | AuthTotpChallenge> {
	const res = await fetch(`${API_V1}/auth/login`, {
		method: 'POST',
		headers: { 'content-type': 'application/json', host: ctx.host, 'x-forwarded-host': ctx.host },
		body: JSON.stringify({ email: email.toLowerCase().trim(), password, website: website ?? '' })
	});
	const payload = await res.json();
	if (!res.ok) {
		const err = payload?.error;
		throw new ApiError(res.status, err?.code ?? 'api_error', err?.message ?? res.statusText);
	}
	const data = payload.data as AuthSession | AuthTotpChallenge;
	if (data.access_token) {
		const rt = extractRefreshToken(res);
		if (rt) (data as AuthSession).refresh_token = rt;
	}
	return data;
}

/** POST /auth/register — returns { access_token, user, refresh_token } or
 * throws ApiError. */
export async function authenticateSignup(
	ctx: ApiContext,
	email: string,
	username: string,
	password: string,
	website?: string
): Promise<AuthSession> {
	const res = await fetch(`${API_V1}/auth/register`, {
		method: 'POST',
		headers: { 'content-type': 'application/json', host: ctx.host, 'x-forwarded-host': ctx.host },
		body: JSON.stringify({ email: email.toLowerCase().trim(), username: username.trim(), password, website: website ?? '' })
	});
	const payload = await res.json();
	if (!res.ok) {
		const err = payload?.error;
		throw new ApiError(res.status, err?.code ?? 'api_error', err?.message ?? res.statusText);
	}
	const data = payload.data as AuthSession;
	const rt = extractRefreshToken(res);
	if (rt) data.refresh_token = rt;
	return data;
}

/** POST /auth/logout — best-effort; always clears the cookie locally. */
export async function backendLogout(ctx: ApiContext): Promise<void> {
	try {
		await fetch(`${API_V1}/auth/logout`, {
			method: 'POST',
			headers: { host: ctx.host, 'x-forwarded-host': ctx.host }
		});
	} catch {
		// Best-effort — the cookie is cleared locally regardless.
	}
}

/** GET /onboarding/status — returns the onboarding status for the current user. */
export async function getOnboardingStatus(ctx: ApiContext): Promise<OnboardingStatus | null> {
	try {
		const res = await fetch(`${API_V1}/onboarding/status`, {
			headers: {
				host: ctx.host,
				'x-forwarded-host': ctx.host,
				...(ctx.token ? { authorization: `Bearer ${ctx.token}` } : {})
			}
		});
		if (!res.ok) return null;
		const payload = await res.json();
		return payload.data as OnboardingStatus;
	} catch {
		return null;
	}
}

/** POST /auth/forgot-password — sends a reset email. Throws ApiError on failure. */
export async function requestPasswordReset(ctx: ApiContext, email: string, website?: string): Promise<void> {
	const res = await fetch(`${API_V1}/auth/forgot-password`, {
		method: 'POST',
		headers: { 'content-type': 'application/json', host: ctx.host, 'x-forwarded-host': ctx.host },
		body: JSON.stringify({ email: email.toLowerCase().trim(), website: website ?? '' })
	});
	if (!res.ok) {
		const payload = await res.json().catch(() => null);
		const err = payload?.error;
		throw new ApiError(res.status, err?.code ?? 'api_error', err?.message ?? res.statusText);
	}
}

/** POST /auth/reset-password — resets password with a token. Throws ApiError. */
export async function resetPassword(
	ctx: ApiContext,
	token: string,
	newPassword: string,
	website?: string
): Promise<void> {
	const res = await fetch(`${API_V1}/auth/reset-password`, {
		method: 'POST',
		headers: { 'content-type': 'application/json', host: ctx.host, 'x-forwarded-host': ctx.host },
		body: JSON.stringify({ token, new_password: newPassword, website: website ?? '' })
	});
	if (!res.ok) {
		const payload = await res.json().catch(() => null);
		const err = payload?.error;
		throw new ApiError(res.status, err?.code ?? 'api_error', err?.message ?? res.statusText);
	}
}

/** GET /auth/verify-email?token= — verifies an email token. Throws ApiError. */
export async function verifyEmail(ctx: ApiContext, token: string): Promise<void> {
	const res = await fetch(`${API_V1}/auth/verify-email?token=${encodeURIComponent(token)}`, {
		headers: { host: ctx.host, 'x-forwarded-host': ctx.host }
	});
	if (!res.ok) {
		const payload = await res.json().catch(() => null);
		const err = payload?.error;
		throw new ApiError(res.status, err?.code ?? 'api_error', err?.message ?? res.statusText);
	}
}

/** POST /auth/resend-verification — resends the verification email. */
export async function resendVerification(ctx: ApiContext, email: string): Promise<void> {
	const res = await fetch(`${API_V1}/auth/resend-verification`, {
		method: 'POST',
		headers: { 'content-type': 'application/json', host: ctx.host, 'x-forwarded-host': ctx.host },
		body: JSON.stringify({ email: email.toLowerCase().trim() })
	});
	if (!res.ok) {
		const payload = await res.json().catch(() => null);
		const err = payload?.error;
		throw new ApiError(res.status, err?.code ?? 'api_error', err?.message ?? res.statusText);
	}
}

/** POST /auth/totp/verify-challenge — verifies a TOTP code with a challenge
 * token (issued by /auth/login when the user has 2FA enabled). Returns
 * { access_token } on success. Throws ApiError on failure. */
/** Result of a TOTP verification or backup code consumption — the access
 * token plus the optional refresh token extracted from Set-Cookie. */
export interface TotpAuthResult {
	access_token: string;
	refresh_token?: string;
}

export async function verifyTotpChallenge(
	ctx: ApiContext,
	challengeToken: string,
	code: string
): Promise<TotpAuthResult> {
	const res = await fetch(`${API_V1}/auth/totp/verify-challenge`, {
		method: 'POST',
		headers: { 'content-type': 'application/json', host: ctx.host, 'x-forwarded-host': ctx.host },
		body: JSON.stringify({ challenge_token: challengeToken, code })
	});
	const payload = await res.json();
	if (!res.ok) {
		const err = payload?.error;
		throw new ApiError(res.status, err?.code ?? 'api_error', err?.message ?? res.statusText);
	}
	const data = payload.data;
	const result: TotpAuthResult = { access_token: data.access_token as string };
	const rt = extractRefreshToken(res);
	if (rt) result.refresh_token = rt;
	return result;
}

/** POST /auth/totp/backup — consumes a one-time backup code with a challenge
 * token. Returns { access_token, refresh_token } on success. Throws ApiError. */
export async function consumeTotpBackup(
	ctx: ApiContext,
	challengeToken: string,
	backupCode: string
): Promise<TotpAuthResult> {
	const res = await fetch(`${API_V1}/auth/totp/backup`, {
		method: 'POST',
		headers: { 'content-type': 'application/json', host: ctx.host, 'x-forwarded-host': ctx.host },
		body: JSON.stringify({ challenge_token: challengeToken, backup_code: backupCode })
	});
	const payload = await res.json();
	if (!res.ok) {
		const err = payload?.error;
		throw new ApiError(res.status, err?.code ?? 'api_error', err?.message ?? res.statusText);
	}
	const data = payload.data;
	const result: TotpAuthResult = { access_token: data.access_token as string };
	const rt = extractRefreshToken(res);
	if (rt) result.refresh_token = rt;
	return result;
}

/** POST /auth/oauth/{provider}/initiate — returns { auth_url }. */
export async function initiateOAuth(
	ctx: ApiContext,
	provider: string,
	redirect: string
): Promise<string> {
	const res = await fetch(`${API_V1}/auth/oauth/${provider}/initiate`, {
		method: 'POST',
		headers: { 'content-type': 'application/json', host: ctx.host, 'x-forwarded-host': ctx.host },
		body: JSON.stringify({ redirect })
	});
	const payload = await res.json();
	if (!res.ok) {
		const err = payload?.error;
		throw new ApiError(res.status, err?.code ?? 'api_error', err?.message ?? res.statusText);
	}
	return payload.data.auth_url as string;
}

/** Resolve the post-login redirect: /onboarding if incomplete, else the target or /. */
export function resolvePostLoginRedirect(
	onboarding: OnboardingStatus | null,
	redirectTarget: string | null
): string {
	if (onboarding && !onboarding.completed) return '/onboarding';
	return redirectTarget || '/';
}

/** Re-export apiContext for convenience in form actions. */
export { apiContext };

/** Re-export AuthUser for type usage in form action returns. */
export type { AuthUser };
