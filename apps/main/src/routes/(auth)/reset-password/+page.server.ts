// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ url }) => {
	const token = url.searchParams.get('token');
	if (!token) {
		throw error(400, { code: 'missing_fields', message: 'Missing reset token' });
	}
	return { token };
};
