import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { hasPerm } from '$lib/server/auth';
import type { Coupon, MyOrganization } from '$lib/types';

/**
 * Deals index at /deals — canonical public listing for live coupons/deals.
 * Rows link to /deal/{provider}/{slug}; the category pills are derived from
 * each coupon's provider_category. /coupons and /deal 301-redirect here.
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

	// "New deal" CTA (§55 org submissions): staff holding the coupons.manage
	// RBAC node, or any member with an owner/admin/editor role in at least
	// one organization (resolved via GET /me/organizations).
	let canSubmit = hasPerm(event.locals.user?.permissions, 'coupons.manage');
	if (!canSubmit && event.locals.user) {
		try {
			const orgs = await apiList<MyOrganization>(ctx, '/me/organizations?page_size=50');
			canSubmit = orgs.items.some((o) => ['owner', 'admin', 'editor'].includes(o.member_role));
		} catch {
			canSubmit = false;
		}
	}
	return { coupons, pageSize: PAGE_SIZE, loadError, canSubmit };
};
