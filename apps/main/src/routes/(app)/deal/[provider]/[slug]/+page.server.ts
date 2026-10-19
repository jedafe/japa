import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import { hasPerm } from '$lib/server/auth';
import { dealPath, type Coupon, type DealPartnerRef } from '$lib/types';

/**
 * Deal detail at /deal/{provider}/{slug} — public, SSR, canonical slug URL
 * for a coupon. The backend resolves renamed slugs via slug_history; this
 * load 301s to the canonical path when the provider segment or slug in the
 * URL is stale.
 *
 * Also loads sibling deals from the same provider ("Other ways to save")
 * and — only for staff holding coupons.manage — the provider list that the
 * inline DealForm edit needs for its provider select.
 *
 * Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md (Deal Pages)
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	try {
		const canManage = hasPerm(event.locals.user?.permissions, 'coupons.manage');
		// The provider list (staff edit form) doesn't depend on the coupon —
		// fire it alongside the detail fetch.
		const providersPromise = canManage
			? apiList<DealPartnerRef>(ctx, '/deals/partners?page_size=100')
					.then((r) => r.items)
					.catch(() => [] as DealPartnerRef[])
			: Promise.resolve([] as DealPartnerRef[]);

		const coupon = await apiGet<Coupon>(ctx, `/deals/${event.params.slug}`);
		const canonical = dealPath(coupon);
		if (event.url.pathname !== canonical) {
			throw redirect(301, canonical);
		}

		// Sibling deals — GET /coupons accepts a provider_id filter
		// (ListPublicCoupons → ListActiveCoupons). Public endpoint, so this
		// runs for every visitor; failures degrade to an empty list.
		let related: Coupon[] = [];
		if (coupon.provider_id) {
			try {
				related = (
					await apiList<Coupon>(ctx, `/coupons?provider_id=${coupon.provider_id}&page_size=12`)
				).items.filter((c) => c.id !== coupon.id);
			} catch {
				related = [];
			}
		}

		const providers = await providersPromise;

		return {
			coupon,
			related,
			canManage,
			providers,
			canManageProviders: hasPerm(
				event.locals.user?.permissions,
				'affiliate.providers.manage'
			)
		};
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Deal not found' });
		}
		throw err;
	}
};
