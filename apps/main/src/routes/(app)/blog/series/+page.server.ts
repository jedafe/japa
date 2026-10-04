import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * Legacy route — series moved to the top-level /series directory because
 * they are cross-type (§14). 301 so search engines + saved links converge
 * on the canonical URL.
 */
export const load: PageServerLoad = async (event) => {
	throw redirect(301, `/series${event.url.search}`);
};
