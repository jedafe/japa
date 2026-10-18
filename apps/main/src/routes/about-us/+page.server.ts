import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * Legacy /about-us URL — the About page lives at the canonical /about
 * (the Page Builder slug the main_footer menu links to). Permanently
 * redirect so any pre-existing external links keep working.
 */
export const load: PageServerLoad = () => {
	throw redirect(308, '/about');
};
