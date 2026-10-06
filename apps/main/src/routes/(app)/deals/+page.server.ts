import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { hasPerm } from '$lib/server/auth';
import type { Coupon, DealPartnerRef, MyOrganization } from '$lib/types';

/**
 * Deals index at /deals — canonical public listing for live coupons/deals.
 * Rows link to /deal/{provider}/{slug}; the category pills are derived from
 * each coupon's provider_category. /coupons and /deal 301-redirect here.
 *
 * Search/filter/sort are server-side — the query params (?q=, ?provider_id=,
 * ?type=, ?sort=) map straight onto GET /coupons (§55). The provider dropdown
 * options come from GET /deals/partners (Rule 16 — API-driven option list).
 *
 * Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md (Deal Pages)
 */
const PAGE_SIZE = 48;
const VALID_SORTS = new Set(['newest', 'expiring', 'popular']);

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const params = event.url.searchParams;

	const q = params.get('q')?.trim() ?? '';
	const providerId = params.get('provider_id')?.trim() ?? '';
	const couponType = params.get('type')?.trim() ?? '';
	const rawSort = params.get('sort')?.trim() ?? '';
	const sort = VALID_SORTS.has(rawSort) ? rawSort : 'newest';

	let coupons: Coupon[] = [];
	let partners: DealPartnerRef[] = [];
	let loadError = false;

	const qs = new URLSearchParams();
	qs.set('page_size', String(PAGE_SIZE));
	if (q) qs.set('q', q);
	if (providerId) qs.set('provider_id', providerId);
	if (couponType) qs.set('type', couponType);
	if (sort !== 'newest') qs.set('sort', sort);

	try {
		coupons = (await apiList<Coupon>(ctx, `/coupons?${qs}`)).items;
	} catch {
		loadError = true;
	}

	// Provider options are best-effort — a partners outage must not take the
	// listing down; the dropdown just degrades to "All providers".
	try {
		partners = (await apiList<DealPartnerRef>(ctx, '/deals/partners')).items;
	} catch {
		partners = [];
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
	return {
		coupons,
		partners,
		pageSize: PAGE_SIZE,
		loadError,
		canSubmit,
		filters: { q, providerId, couponType, sort }
	};
};
