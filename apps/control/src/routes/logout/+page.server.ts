import { redirect } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';
import { API_BASE } from '$lib/server/api';

// A direct GET to /logout (e.g. a bookmarked link) just sends the visitor
// back to login rather than rendering anything.
export const load: PageServerLoad = async () => {
	throw redirect(303, '/login');
};

export const actions: Actions = {
	default: async ({ cookies, fetch }) => {
		const token = cookies.get('jc_token');

		if (token) {
			// Best-effort: revokes the refresh token + access-token JTI
			// server-side so a captured token can't be replayed. Never block
			// the redirect on this — clearing jc_token below is what actually
			// ends the session as far as this app is concerned.
			try {
				await fetch(`${API_BASE}/auth/logout`, {
					method: 'POST',
					headers: { Authorization: `Bearer ${token}` }
				});
			} catch {
				// Ignore — proceed to clear the cookie regardless.
			}
		}

		cookies.delete('jc_token', { path: '/', domain: '.japaconnect.com' });
		throw redirect(303, '/login');
	}
};
