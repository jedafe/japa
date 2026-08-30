import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * /u index — route to the authenticated user's own profile when logged in,
 * or to the public user directory for guests.
 */
export const load: PageServerLoad = async (event) => {
	if (event.locals.user?.username) {
		throw redirect(302, `/u/${event.locals.user.username}`);
	}
	throw redirect(302, '/users');
};
