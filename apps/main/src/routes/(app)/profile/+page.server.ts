import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

// Canonical "my profile" URL for static nav links (e.g. the admin-managed
// mobile_bottom_bar menu item) that cannot interpolate the viewer's username.
// Signed-in users land on their real profile; guests go to /login.
export const load: PageServerLoad = async (event) => {
	const user = event.locals.user;
	if (!user?.username) {
		throw redirect(302, '/login?redirect=/profile');
	}
	throw redirect(302, `/u/${user.username}`);
};
