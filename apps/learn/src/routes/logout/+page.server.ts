import { redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { Actions } from './$types';

export const actions: Actions = {
	default: async ({ cookies }) => {
		const cookieDomain = env.JC_COOKIE_DOMAIN || undefined;
		cookies.delete('jc_token', { path: '/', ...(cookieDomain ? { domain: cookieDomain } : {}) });
		throw redirect(303, '/login');
	}
};
