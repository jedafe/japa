import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import { dealPath, type Coupon } from '$lib/types';

/**
 * Legacy coupon detail at /coupons/{id} — permanently redirects to the
 * canonical slug URL /deal/{provider}/{slug} (Section 55 Deal Pages).
 *
 * Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const id = event.params.id;
	try {
		const coupon = await apiGet<Coupon>(ctx, `/coupons/${id}`);
		throw redirect(301, dealPath(coupon));
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Coupon not found' });
		}
		throw err;
	}
};
