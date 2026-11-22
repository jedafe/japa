import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * Legacy /support URL — emails sent before the support link pointed at the
 * Page Builder /contact page still carry it. Permanently redirect so those
 * links keep working.
 */
export const load: PageServerLoad = () => {
	throw redirect(308, '/contact');
};
