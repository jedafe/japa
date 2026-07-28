import type { PageServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';

/**
 * Instructor dashboard — auth gate only. All instructor data (profile, tracks,
 * earnings, payouts) is CSR-only (ADR-063 SSR/CSR boundary, ADR-060).
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(302, `/login?redirect=/dashboard/instructor`);
	}
	return {};
};
