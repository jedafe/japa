// Ref: japaconnect-engineering/sections/102-mentorship-matching.md (000824 —
// mentorship requests are user-provider service_bookings rows)
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { loadBookingsSurface } from '$lib/server/serviceBookings';

/**
 * Mentorship inbox at /messages/mentorship — auth-gated on
 * mentorship.enabled only (NOT feature.dm: mentorship requests are usable
 * without Direct Messages). Lists kind=mentorship bookings for both roles —
 * sent = the caller's mentee requests, received = requests sent to the
 * caller as mentor (role=provider, accept/decline/complete via
 * POST /me/service-bookings/:id/respond).
 *
 * URL params: role=sent|received (default sent), status, intent, page,
 * b=<booking id> (deep-link — opens the detail pane; notification target).
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['mentorship.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Mentorship is not available' });
	}

	if (!event.locals.user) {
		throw redirect(
			307,
			`/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`
		);
	}

	return loadBookingsSurface(event, {
		fixedKind: 'mentorship',
		mentorshipEnabled: true
	});
};
