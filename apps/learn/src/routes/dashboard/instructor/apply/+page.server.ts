import type { PageServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { loginUrl } from '$lib/appOrigins';

/**
 * Creator application — auth gate only. The application form is CSR
 * (POST /learn/university/instructor/apply).
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		// Login lives on the Main app (shared jc_token cookie).
		throw redirect(302, loginUrl('/dashboard/instructor/apply'));
	}
	return {};
};
