import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/** Legacy slug redirect — canonical route is /processing-times/my-applications (§93). */
export const load: PageServerLoad = async (event) => {
	throw redirect(301, `/processing-times/my-applications${event.url.search}`);
};
