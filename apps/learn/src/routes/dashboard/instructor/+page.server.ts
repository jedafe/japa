import type { PageServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { loginUrl } from '$lib/appOrigins';

/**
 * Instructor dashboard — auth gate only. All instructor data (profile, tracks,
 * earnings, payouts) is CSR-only (ADR-063 SSR/CSR boundary, ADR-060).
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		// Login lives on the Main app (shared jc_token cookie).
		throw redirect(302, loginUrl('/dashboard/instructor'));
	}
	return {};
};
