// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';

export const load: PageServerLoad = async (event) => {
	const token = event.url.searchParams.get('token');
	if (!token) {
		throw error(400, { code: 'missing_fields', message: 'Missing reset token' });
	}

	// Validate the reset token against the backend before showing the form.
	// The token is consumed on POST, not here.
	const ctx = apiContext(event);
	let valid = false;
	try {
		const res = await apiGet<{ valid: boolean }>(ctx, `/auth/reset-password?token=${encodeURIComponent(token)}`);
		valid = res?.valid ?? false;
	} catch {
		valid = false;
	}

	if (!valid) {
		throw error(400, { code: 'invalid_token', message: 'Reset token is invalid or expired' });
	}

	return { token };
};
