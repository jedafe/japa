// Ref: japaconnect-engineering/sections/02-authentication-sessions.md (TOTP login challenge)
// Surfaces the challenge token, email hint, and post-login redirect target
// from the query string. The challenge token is issued by /api/auth/login
// when the account has 2FA enabled; without it the user is sent back to login.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { sanitizePostLoginTarget } from '$lib/server/auth';

export const load: PageServerLoad = async ({ url }) => {
	const challengeToken = url.searchParams.get('challenge_token');
	const email = url.searchParams.get('email');
	// Sanitize up front — the value is echoed into the verify POST body and
	// becomes the post-2FA navigation target.
	const redirectTarget = sanitizePostLoginTarget(url.searchParams.get('redirect'));

	if (!challengeToken) {
		throw redirect(303, '/login');
	}

	return { challengeToken, email, redirectTarget };
};
