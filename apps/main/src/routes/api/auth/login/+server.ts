// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
// Server-side proxy for the modal login flow. Calls the backend, sets the
// jc_token cookie, and returns { user, redirect }. Routes use form actions
// that call the same authenticateLogin helper.
import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { apiContext, authenticateLogin, getOnboardingStatus, resolvePostLoginRedirect, sanitizePostLoginTarget, setRefreshCookie, setSessionCookie } from '$lib/server/auth';

export const POST: RequestHandler = async (event) => {
	const body = await event.request.json().catch(() => ({}));
	const email = body?.email;
	const password = body?.password;
	// Sanitize before it is echoed into the /verify-totp URL — the raw value
	// would resurface as the post-2FA navigation target (open redirect).
	const redirect = sanitizePostLoginTarget(typeof body?.redirect === 'string' ? body.redirect : null);
	const website = body?.website; // §41 Layer 2 honeypot

	if (!email || !password) {
		return json({ error: { code: 'missing_fields', message: 'Email and password are required.', status: 400 } }, { status: 400 });
	}

	const ctx = apiContext(event);
	try {
		const result = await authenticateLogin(ctx, email, password, website);
		// 2FA-enabled accounts: backend returns { totp_required, challenge_token, user }
		// instead of an access_token. The client redirects to /verify-totp.
		if (result.totp_required) {
			return json({
				totp_required: true,
				challenge_token: result.challenge_token,
				user: result.user,
				redirect: `/verify-totp?challenge_token=${encodeURIComponent(result.challenge_token)}&email=${encodeURIComponent(email)}&redirect=${encodeURIComponent(redirect)}`
			});
		}
		setSessionCookie(event, result.access_token);
		if (result.refresh_token) setRefreshCookie(event, result.refresh_token);

		// Check onboarding status to decide the redirect.
		const onboarding = await getOnboardingStatus({ ...ctx, token: result.access_token });
		const target = resolvePostLoginRedirect(onboarding, redirect);

		return json({ user: result.user, redirect: target });
	} catch (err) {
		const status = err instanceof Error && 'status' in err ? (err as { status: number }).status : 500;
		const code = err instanceof Error && 'code' in err ? (err as { code: string }).code : 'api_error';
		const message = err instanceof Error ? err.message : 'Login failed';
		return json({ error: { code: code || 'api_error', message: message || 'Request failed', status } }, { status });
	}
};
