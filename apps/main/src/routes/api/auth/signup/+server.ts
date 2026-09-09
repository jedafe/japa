// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
// Server-side proxy for the modal signup flow. New users always redirect to
// /onboarding after the session cookie is set.
import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { apiContext, authenticateSignup, getOnboardingStatus, resolvePostLoginRedirect, setRefreshCookie, setSessionCookie } from '$lib/server/auth';

export const POST: RequestHandler = async (event) => {
	const body = await event.request.json().catch(() => ({}));
	const email = body?.email;
	const username = body?.username;
	const password = body?.password;
	const redirect = body?.redirect ?? '/';
	const website = body?.website; // §41 Layer 2 honeypot

	if (!email || !username || !password) {
		return json({ error: { code: 'missing_fields', message: 'Email, username and password are required.', status: 400 } }, { status: 400 });
	}

	const ctx = apiContext(event);
	try {
		const session = await authenticateSignup(ctx, email, username, password, website);
		setSessionCookie(event, session.access_token);
		if (session.refresh_token) setRefreshCookie(event, session.refresh_token);

		const onboarding = await getOnboardingStatus({ ...ctx, token: session.access_token });
		const target = resolvePostLoginRedirect(onboarding, redirect);

		return json({ user: session.user, redirect: target });
	} catch (err) {
		const status = err instanceof Error && 'status' in err ? (err as { status: number }).status : 500;
		const code = err instanceof Error && 'code' in err ? (err as { code: string }).code : 'api_error';
		const message = err instanceof Error ? err.message : 'Signup failed';
		return json({ error: { code: code || 'api_error', message: message || 'Request failed', status } }, { status });
	}
};
