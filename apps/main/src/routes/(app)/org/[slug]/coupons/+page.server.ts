// Ref: japaconnect-engineering/sections/114-services-marketplace.md
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { OrgCoupon, MyOrganization, PublicOrganization } from '$lib/types';

const PAGE_SIZE = 20;

/**
 * Org coupon management at /org/[slug]/coupons (§114) — the selling org's
 * own service coupons: create/edit/suspend/delete plus redemption +
 * attachment rollups. Editor-gated to mirror the backend's
 * requireOrgEditor (owner|admin|editor — plain members get 403 from the
 * API too); the whole surface sits behind service_coupons.enabled.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['service_coupons.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Service coupons are not available' });
	}

	if (!event.locals.user) {
		throw redirect(
			307,
			`/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`
		);
	}

	const ctx = apiContext(event);
	const slug = event.params.slug;

	let org: PublicOrganization;
	try {
		org = await apiGet<PublicOrganization>(ctx, `/organizations/${slug}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Organization not found' });
		}
		throw error(500, { code: 'load_failed', message: 'Failed to load organization' });
	}

	// requireOrgEditor parity: owner|admin|editor only. Membership comes
	// from /me/organizations — the API re-checks on every call.
	let memberRole = '';
	try {
		const myOrgs = await apiList<MyOrganization>(ctx, '/me/organizations');
		memberRole = myOrgs.items.find((o) => o.id === org.id)?.member_role ?? '';
	} catch {
		memberRole = '';
	}
	if (!['owner', 'admin', 'editor'].includes(memberRole)) {
		throw error(403, {
			code: 'not_org_editor',
			message: 'Only organization owners, admins and editors can manage coupons'
		});
	}

	const page = Math.max(1, Number(event.url.searchParams.get('page') ?? '1') || 1);

	let coupons: OrgCoupon[] = [];
	try {
		const res = await apiList<OrgCoupon>(
			ctx,
			`/organizations/${org.id}/coupons?page=${page}&page_size=${PAGE_SIZE}`
		);
		coupons = res.items;
	} catch (err) {
		if (err instanceof ApiError && (err.status === 403 || err.status === 404)) {
			throw error(403, {
				code: 'not_org_editor',
				message: 'Only organization owners, admins and editors can manage coupons'
			});
		}
		coupons = [];
	}

	return { org, coupons, currentPage: page, pageSize: PAGE_SIZE, memberRole };
};
