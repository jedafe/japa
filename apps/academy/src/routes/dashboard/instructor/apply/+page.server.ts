import type { PageServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { loginUrl } from '$lib/appOrigins';

/**
 * Creator application — auth gate only. The application form is CSR
 * (POST /academy/instructor/apply).
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		// Login lives on the Main app (shared jc_token cookie). The redirect
		// must point back to THIS Academy app (event.url.origin), not the Main
		// app, since /dashboard/instructor/apply only exists on Academy.
		throw redirect(302, loginUrl(`${event.url.origin}/dashboard/instructor/apply`));
	}
	return {};
};
