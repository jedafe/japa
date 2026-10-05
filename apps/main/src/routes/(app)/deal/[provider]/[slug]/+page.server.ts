import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import { dealPath, type Coupon } from '$lib/types';

/**
 * Deal detail at /deal/{provider}/{slug} — public, SSR, canonical slug URL
 * for a coupon. The backend resolves renamed slugs via slug_history; this
 * load 301s to the canonical path when the provider segment or slug in the
 * URL is stale.
 *
 * Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md (Deal Pages)
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	try {
		const coupon = await apiGet<Coupon>(ctx, `/deals/${event.params.slug}`);
		const canonical = dealPath(coupon);
		if (event.url.pathname !== canonical) {
			throw redirect(301, canonical);
		}
		return { coupon };
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Deal not found' });
		}
		throw err;
	}
};
