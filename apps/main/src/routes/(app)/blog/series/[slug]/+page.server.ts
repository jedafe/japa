import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * Legacy route — series detail moved to /series/[slug] (§14: series are
 * cross-type). 301 preserves the slug + any query string.
 */
export const load: PageServerLoad = async (event) => {
	throw redirect(301, `/series/${event.params.slug}${event.url.search}`);
};
