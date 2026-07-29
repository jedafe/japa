import type { PageServerLoad } from './$types';
import { apiContext, apiList, ApiError } from '$lib/server/api';
import type { Coupon } from '$lib/types';

/**
 * Coupons index at /coupons — public, SSR, indexable. Lists active tenant
 * coupons with provider info; each card links to /coupons/{id}.
 *
 * Ref: japaconnect-engineering/sections/21-coupons.md
 */
const PAGE_SIZE = 24;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	let coupons: Coupon[] = [];
	let loadError = false;
	try {
		coupons = (await apiList<Coupon>(ctx, `/coupons?page_size=${PAGE_SIZE}`)).items;
	} catch {
		loadError = true;
	}
	return { coupons, pageSize: PAGE_SIZE, loadError };
};
