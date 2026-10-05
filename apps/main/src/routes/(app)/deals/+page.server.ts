import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { Coupon } from '$lib/types';

/**
 * Deals index at /deals — canonical public listing for live coupons/deals.
 * Cards link to /deal/{provider}/{slug}; the partner strip is derived from
 * the embedded provider_* fields on each coupon. /coupons and /deal
 * 301-redirect here.
 *
 * Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md (Deal Pages)
 */
const PAGE_SIZE = 48;

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
