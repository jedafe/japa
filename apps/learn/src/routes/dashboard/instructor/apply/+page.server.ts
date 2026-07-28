import type { PageServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';

/**
 * Creator application — auth gate only. The application form is CSR
 * (POST /learn/university/instructor/apply).
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(302, `/login?redirect=/dashboard/instructor/apply`);
	}
	return {};
};
