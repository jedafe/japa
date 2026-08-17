import { redirect } from '@sveltejs/kit';
import { loginUrl } from '$lib/appOrigins';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		// Login lives on the Main app (shared jc_token cookie). The redirect
		// must point back to THIS Learn app (event.url.origin), not the Main
		// app, since this route only exists on Learn.
		throw redirect(302, loginUrl(`${event.url.origin}${event.url.pathname}${event.url.search}`));
	}
	return {};
};
