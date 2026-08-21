// Ref: japaconnect-engineering/sections/02-authentication-sessions.md (TOTP backup codes)
// Server-side proxy for the 2FA backup-code login path. Accepts a challenge
// token and a one-time backup code, calls POST /auth/totp/backup, sets the
// jc_token cookie, and returns { redirect }.
import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import {
	apiContext,
	consumeTotpBackup,
	getOnboardingStatus,
	resolvePostLoginRedirect,
	setRefreshCookie,
	setSessionCookie
} from '$lib/server/auth';

export const POST: RequestHandler = async (event) => {
	const body = await event.request.json().catch(() => ({}));
	const challengeToken = body?.challenge_token;
	const backupCode = body?.backup_code;
	const redirect = body?.redirect ?? '/';

	if (!challengeToken || !backupCode) {
		throw error(400, { code: 'missing_fields', message: 'missing_fields' });
	}

	const ctx = apiContext(event);
	try {
		const result = await consumeTotpBackup(ctx, challengeToken, backupCode);
		setSessionCookie(event, result.access_token);
		if (result.refresh_token) setRefreshCookie(event, result.refresh_token);

		const onboarding = await getOnboardingStatus({ ...ctx, token: result.access_token });
		const target = resolvePostLoginRedirect(onboarding, redirect);
		return json({ redirect: target });
	} catch (err) {
		const status = err instanceof Error && 'status' in err ? (err as { status: number }).status : 500;
		const code = err instanceof Error && 'code' in err ? (err as { code: string }).code : 'api_error';
		const message = err instanceof Error ? err.message : 'Backup code verification failed';
		throw error(status, { code: code || 'api_error', message: message || 'Request failed' });
	}
};
