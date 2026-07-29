import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { Coupon } from '$lib/types';

/**
 * Coupon detail at /coupons/{id} — public, SSR. Shows the full coupon with
 * code, discount, provider, terms and a CTA to the referral URL.
 *
 * Ref: japaconnect-engineering/sections/21-coupons.md
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const id = event.params.id;
	try {
		const coupon = await apiGet<Coupon>(ctx, `/coupons/${id}`);
		return { coupon };
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Coupon not found' });
		}
		throw err;
	}
};
