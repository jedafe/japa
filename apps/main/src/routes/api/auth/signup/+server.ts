// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
// Server-side proxy for the modal signup flow. New users always redirect to
// /onboarding after the session cookie is set.
import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { apiContext, authenticateSignup, setRefreshCookie, setSessionCookie } from '$lib/server/auth';

export const POST: RequestHandler = async (event) => {
	const body = await event.request.json().catch(() => ({}));
	const email = body?.email;
	const username = body?.username;
	const password = body?.password;
	const website = body?.website; // §41 Layer 2 honeypot

	if (!email || !username || !password) {
		throw error(400, { code: 'missing_fields', message: 'missing_fields' });
	}

	const ctx = apiContext(event);
	try {
		const session = await authenticateSignup(ctx, email, username, password, website);
		setSessionCookie(event, session.access_token);
		if (session.refresh_token) setRefreshCookie(event, session.refresh_token);
		return json({ user: session.user, redirect: '/onboarding' });
	} catch (err) {
		const status = err instanceof Error && 'status' in err ? (err as { status: number }).status : 500;
		const code = err instanceof Error && 'code' in err ? (err as { code: string }).code : 'api_error';
		const message = err instanceof Error ? err.message : 'Signup failed';
		throw error(status, { code: code || 'api_error', message: message || 'Request failed' });
	}
};
