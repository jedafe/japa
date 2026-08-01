import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * Legacy redirect: /organizations → /org. The public org list was renamed
 * from /organizations to /org to avoid British/American English ambiguity.
 * Old bookmarks and links are forwarded permanently.
 */
export const load: PageServerLoad = async (event) => {
	const q = event.url.searchParams.get('q');
	const target = q ? `/org?q=${encodeURIComponent(q)}` : '/org';
	throw redirect(301, target);
};
