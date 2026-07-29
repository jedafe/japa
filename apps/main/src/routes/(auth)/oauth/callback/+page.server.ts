// Ref: japaconnect-engineering/sections/02-authentication-sessions.md (OAuth callback)
// The backend handles the OAuth callback entirely (GET /api/v1/auth/oauth/{provider}/callback)
// — it exchanges the code, creates/links the account, sets cookies, and redirects
// to /, /onboarding, or /login?oauth_error=... . This frontend route is a safety
// net: if a user lands here directly (e.g. bookmarked the wrong URL), redirect
// them home. The login page already surfaces ?oauth_error= from the backend redirect.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ url }) => {
	// If we arrive with an oauth_error param, forward it to /login so the error
	// banner shows. Otherwise just go home.
	const oauthError = url.searchParams.get('oauth_error');
	if (oauthError) {
		throw redirect(302, `/login?oauth_error=${encodeURIComponent(oauthError)}`);
	}
	throw redirect(302, '/');
};
