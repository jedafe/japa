// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
// Browser-facing auth proxy at /api/auth/*. The presentational auth forms
// (LoginForm, SignupForm, etc.) and OAuth buttons call these local endpoints;
// this file forwards them to the Go backend using the server-only helpers in
// $lib/server/auth.ts and sets the session cookies on success.
//
// NOTE: /api/v1/* is proxied by Vite in dev, but /api/auth/* are intentionally
// SvelteKit server routes so cookie setting and response shaping stay under
// SvelteKit control.

import { json, type RequestEvent } from '@sveltejs/kit';
import { apiContext, ApiError } from '$lib/server/api';
import {
	authenticateLogin,
	authenticateSignup,
	requestPasswordReset,
	resetPassword,
	verifyTotpChallenge,
	consumeTotpBackup,
	initiateOAuth,
	forwardSetCookies,
	refreshSession,
	setSessionCookie,
	setRefreshCookie,
	clearSessionCookie,
	sanitizePostLoginTarget
} from '$lib/server/auth';

const ALLOWED_PATHS = new Set([
	'login',
	'signup',
	'oauth',
	'refresh',
	'forgot-password',
	'reset-password',
	'totp/verify',
	'totp/backup'
]);

function badRequest(code: string, message: string) {
	return json({ error: { code, message, status: 400 } }, { status: 400 });
}

function serverError(message = 'Internal server error') {
	return json({ error: { code: 'server_error', message, status: 500 } }, { status: 500 });
}

function apiErrorResponse(err: ApiError | unknown) {
	if (err instanceof ApiError) {
		return json({ error: { code: err.code, message: err.message, status: err.status } }, { status: err.status });
	}
	if (err instanceof Error) return serverError(err.message);
	return serverError();
}

export async function POST(event: RequestEvent) {
	const path = event.params.path ?? '';
	if (!ALLOWED_PATHS.has(path)) {
		return json({ error: { code: 'route_not_found', message: 'Not found', status: 404 } }, { status: 404 });
	}

	// Read the raw body so empty/missing bodies (e.g. /api/auth/refresh) default
	// to {} instead of failing the JSON parser.
	let body: Record<string, unknown> = {};
	try {
		const text = await event.request.text();
		if (text.trim() !== '') {
			body = JSON.parse(text) as Record<string, unknown>;
		}
	} catch {
		return badRequest('invalid_body', 'Request body must be valid JSON.');
	}

	const ctx = apiContext(event);
	// Sanitize the post-auth target — an unsanitized echo here is an open
	// redirect (navigatePostLogin location.assign's absolute URLs). The
	// dedicated /api/auth/{login,signup} routes resolve this through
	// resolvePostLoginRedirect; the catch-all must not be weaker.
	const redirect = sanitizePostLoginTarget(typeof body.redirect === 'string' ? body.redirect : null);

	try {
		switch (path) {
			case 'login': {
				const email = String(body.email ?? '');
				const password = String(body.password ?? '');
				const website = typeof body.website === 'string' ? body.website : '';
				if (!email || !password) return badRequest('missing_fields', 'Email and password are required.');

				const result = await authenticateLogin(ctx, email, password, website);

				if ('totp_required' in result && result.totp_required) {
					const params = new URLSearchParams({
						challenge_token: result.challenge_token,
						redirect
					});
					if (result.user?.email) params.set('email', result.user.email);
					return json({
						totp_required: true,
						redirect: `/verify-totp?${params.toString()}`
					});
				}

				const session = result as { access_token: string; refresh_token?: string; user: { id: string; email: string; username: string } };
				setSessionCookie(event, session.access_token);
				if (session.refresh_token) setRefreshCookie(event, session.refresh_token);
				return json({ user: session.user, redirect });
			}

			case 'signup': {
				const email = String(body.email ?? '');
				const username = String(body.username ?? '');
				const password = String(body.password ?? '');
				const website = typeof body.website === 'string' ? body.website : '';
				if (!email || !username || !password) {
					return badRequest('missing_fields', 'Email, username and password are required.');
				}

				const session = await authenticateSignup(ctx, email, username, password, website);
				setSessionCookie(event, session.access_token);
				if (session.refresh_token) setRefreshCookie(event, session.refresh_token);
				return json({ user: session.user, redirect });
			}

			case 'forgot-password': {
				const email = String(body.email ?? '');
				const website = typeof body.website === 'string' ? body.website : '';
				if (!email) return badRequest('missing_fields', 'Email is required.');
				await requestPasswordReset(ctx, email, website);
				return json({ ok: true });
			}

			case 'reset-password': {
				const token = String(body.token ?? '');
				const newPassword = String(body.new_password ?? '');
				const website = typeof body.website === 'string' ? body.website : '';
				if (!token || !newPassword) return badRequest('missing_fields', 'Token and new password are required.');
				await resetPassword(ctx, token, newPassword, website);
				return json({ ok: true });
			}

			case 'totp/verify': {
				const challengeToken = String(body.challenge_token ?? '');
				const code = String(body.code ?? '');
				if (!challengeToken || !code) return badRequest('missing_fields', 'Challenge token and code are required.');

				const result = await verifyTotpChallenge(ctx, challengeToken, code);
				setSessionCookie(event, result.access_token);
				if (result.refresh_token) setRefreshCookie(event, result.refresh_token);
				return json({ redirect });
			}

			case 'totp/backup': {
				const challengeToken = String(body.challenge_token ?? '');
				const backupCode = String(body.backup_code ?? '');
				if (!challengeToken || !backupCode) {
					return badRequest('missing_fields', 'Challenge token and backup code are required.');
				}

				const result = await consumeTotpBackup(ctx, challengeToken, backupCode);
				setSessionCookie(event, result.access_token);
				if (result.refresh_token) setRefreshCookie(event, result.refresh_token);
				return json({ redirect });
			}

			case 'refresh': {
				let result;
				try {
					result = await refreshSession(ctx);
				} catch (err) {
					// A terminally-invalid refresh token (revoked/expired/absent)
					// leaves dead jc_token + jc_refresh cookies that make every
					// later request retry a refresh that can never succeed.
					// Clear them — same degradation as tryRefreshSession in
					// hooks.server.ts — so the client treats the user as a guest.
					if (
						err instanceof ApiError &&
						(err.code === 'no_refresh_token' || err.code === 'invalid_refresh_token')
					) {
						clearSessionCookie(event);
					}
					throw err;
				}
				setSessionCookie(event, result.access_token);
				if (result.refresh_token) setRefreshCookie(event, result.refresh_token);
				// The browser-side token store ($lib/api/token.svelte) needs the
				// fresh access token — the HttpOnly cookie update alone can't
				// reach client JS. The api authenticates Bearer only.
				return json({ ok: true, access_token: result.access_token });
			}

			default:
				return json({ error: { code: 'route_not_found', message: 'Not found', status: 404 } }, { status: 404 });
		}
	} catch (err) {
		return apiErrorResponse(err);
	}
}

export async function GET(event: RequestEvent) {
	const path = event.params.path ?? '';
	if (path !== 'oauth') {
		return json({ error: { code: 'route_not_found', message: 'Not found', status: 404 } }, { status: 404 });
	}

	const provider = event.url.searchParams.get('provider') ?? '';
	const redirect = sanitizePostLoginTarget(event.url.searchParams.get('redirect'));
	if (!provider) return badRequest('missing_fields', 'Provider is required.');

	try {
		const ctx = apiContext(event);
		const { authUrl, setCookies } = await initiateOAuth(ctx, provider, redirect);
		// Forward the backend's OAuth flow cookies to the browser (see
		// /api/auth/oauth — without them the callback fails missing_state).
		forwardSetCookies(event, setCookies);
		return json({ redirect_url: authUrl });
	} catch (err) {
		return apiErrorResponse(err);
	}
}
