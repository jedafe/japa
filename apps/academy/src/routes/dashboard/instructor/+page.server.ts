import type { PageServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { loginUrl, MAIN_ORIGIN } from '#lib/appOrigins.js';

/**
 * Instructor dashboard — auth gate only. All instructor data (profile, tracks,
 * earnings, payouts) is CSR-only (ADR-063 SSR/CSR boundary, ADR-060).
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		// Login lives on the Main app (shared jc_token cookie). The redirect
		// must point back to THIS Academy app (event.url.origin), not the Main
		// app, since /dashboard/instructor only exists on Academy.
		throw redirect(302, loginUrl(`${event.url.origin}/dashboard/instructor`), { external: [MAIN_ORIGIN] });
	}
	return {};
};
