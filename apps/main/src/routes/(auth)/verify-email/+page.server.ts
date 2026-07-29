// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
// Verifies the email token server-side and returns the result. Authed users
// can access this route (the layout gate exempts verify-email).
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, verifyEmail } from '$lib/server/auth';

export const load: PageServerLoad = async (event) => {
	const token = event.url.searchParams.get('token');
	if (!token) {
		return { status: 'invalid' as const, message: 'auth.verify_email.invalid_token' };
	}

	const ctx = apiContext(event);
	try {
		await verifyEmail(ctx, token);
		return { status: 'success' as const };
	} catch (err) {
		const code = err instanceof Error && 'code' in err ? (err as { code: string }).code : '';
		if (code === 'already_verified') {
			return { status: 'already_verified' as const };
		}
		return { status: 'invalid' as const, message: 'auth.verify_email.invalid_token' };
	}
};
