import { redirect } from '@sveltejs/kit';
import { loginUrl, MAIN_ORIGIN } from '#lib/appOrigins.js';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		// Login lives on the Main app (shared jc_token cookie). The redirect
		// must point back to THIS Academy app (event.url.origin), not the Main
		// app, since this route only exists on Academy.
		throw redirect(302, loginUrl(`${event.url.origin}${event.url.pathname}${event.url.search}`), { external: [MAIN_ORIGIN] });
	}
	return {};
};
