import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * Legacy slug redirect — the spec-canonical route is /processing-times
 * (§93). Keep the old path as a permanent redirect so existing links and
 * bookmarks keep working.
 */
export const load: PageServerLoad = async (event) => {
	const search = event.url.search;
	throw redirect(301, `/processing-times${search}`);
};
