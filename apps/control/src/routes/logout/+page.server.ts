import { redirect } from '@sveltejs/kit';
import type { Actions } from './$types';

export const actions: Actions = {
	default: async ({ cookies }) => {
		cookies.delete('jc_token', { path: '/', domain: '.japaconnect.com' });
		throw redirect(303, '/login');
	}
};
