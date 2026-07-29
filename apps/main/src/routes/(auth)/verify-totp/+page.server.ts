// Ref: japaconnect-engineering/sections/02-authentication-sessions.md (TOTP login challenge)
// Surfaces the challenge token, email hint, and post-login redirect target
// from the query string. The challenge token is issued by /api/auth/login
// when the account has 2FA enabled; without it the user is sent back to login.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ url }) => {
	const challengeToken = url.searchParams.get('challenge_token');
	const email = url.searchParams.get('email');
	const redirectTarget = url.searchParams.get('redirect') ?? '/';

	if (!challengeToken) {
		throw redirect(303, '/login');
	}

	return { challengeToken, email, redirectTarget };
};
