import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * /coupons → 301 /deals — deals replaced the legacy coupons admin page (§55).
 */
export const load: PageServerLoad = () => {
	redirect(301, '/deals');
};
