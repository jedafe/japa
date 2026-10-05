import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import { hasPerm } from '$lib/server/auth';
import type { DealPartner } from '$lib/types';

/**
 * Partner landing page at /deal/{provider} — public, SSR, indexable. Shows
 * the affiliate provider profile and its active deals.
 *
 * Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md (Deal Pages)
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	try {
		const partner = await apiGet<DealPartner>(ctx, `/deals/partners/${event.params.provider}`);
		return {
			partner,
			// Staff gate for the inline provider edit form (§55).
			canManageProviders: hasPerm(
				event.locals.user?.permissions,
				'affiliate.providers.manage'
			)
		};
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Partner not found' });
		}
		throw err;
	}
};
