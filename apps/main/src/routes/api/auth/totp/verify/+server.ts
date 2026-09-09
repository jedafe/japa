// Ref: japaconnect-engineering/sections/02-authentication-sessions.md (TOTP login challenge)
// Server-side proxy for the 2FA verification step. Accepts a challenge token
// (issued by /api/auth/login when totp_required=true) and a 6-digit code,
// calls POST /auth/totp/verify-challenge, sets the jc_token cookie, and
// returns { redirect }. The client then invalidates + navigates.
import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import {
	apiContext,
	verifyTotpChallenge,
	getOnboardingStatus,
	resolvePostLoginRedirect,
	setRefreshCookie,
	setSessionCookie
} from '$lib/server/auth';

export const POST: RequestHandler = async (event) => {
	const body = await event.request.json().catch(() => ({}));
	const challengeToken = body?.challenge_token;
	const code = body?.code;
	const redirect = body?.redirect ?? '/';

	if (!challengeToken || !code) {
		return json({ error: { code: 'missing_fields', message: 'Challenge token and code are required.', status: 400 } }, { status: 400 });
	}

	const ctx = apiContext(event);
	try {
		const result = await verifyTotpChallenge(ctx, challengeToken, code);
		setSessionCookie(event, result.access_token);
		if (result.refresh_token) setRefreshCookie(event, result.refresh_token);

		const onboarding = await getOnboardingStatus({ ...ctx, token: result.access_token });
		const target = resolvePostLoginRedirect(onboarding, redirect);
		return json({ redirect: target });
	} catch (err) {
		const status = err instanceof Error && 'status' in err ? (err as { status: number }).status : 500;
		const code = err instanceof Error && 'code' in err ? (err as { code: string }).code : 'api_error';
		const message = err instanceof Error ? err.message : 'Verification failed';
		return json({ error: { code: code || 'api_error', message: message || 'Request failed', status } }, { status });
	}
};
