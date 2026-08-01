import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/** Legacy redirect: /organizations/new → /org/new */
export const load: PageServerLoad = async () => {
	throw redirect(301, '/org/new');
};
