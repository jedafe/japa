import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * /coupons → 301 /deals (canonical deals index).
 *
 * Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md (Deal Pages)
 */
export const load: PageServerLoad = () => {
	redirect(301, '/deals');
};
