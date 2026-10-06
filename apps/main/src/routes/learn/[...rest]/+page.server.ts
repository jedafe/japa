import type { PageServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { ACADEMY_ORIGIN } from '$lib/appOrigins';

/**
 * Legacy deep links: the Learn surface was renamed to Academy. `/learn/*`
 * bookmarks on the main domain forward to the Academy app, preserving the
 * remainder of the path and the query string.
 */
export const load: PageServerLoad = async ({ url, params }) => {
	const rest = params.rest ?? '';
	const path = rest ? `/${rest}` : '';
	throw redirect(308, `${ACADEMY_ORIGIN}${path}${url.search}`);
};

export const prerender = false;
