// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ url }) => {
	// Surface OAuth error from the callback redirect (?oauth_error=code).
	const oauthError = url.searchParams.get('oauth_error');
	return { oauthError };
};
