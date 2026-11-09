import { redirect } from '@sveltejs/kit';
import type { LayoutServerLoad } from './$types';

/**
 * Auth gate for the whole /org tree — the directory, org profiles, coupons,
 * jobs, service bookings, and invite acceptance all require a signed-in
 * viewer. Guests are sent to /login and return to the requested URL after
 * signing in (same convention as the per-page gates on /org/new and
 * /org/invite/[token]).
 *
 * Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
 */
export const load: LayoutServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(
			307,
			`/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`
		);
	}
	return {};
};
