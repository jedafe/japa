// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { loadBookingsSurface } from '$lib/server/serviceBookings';
import { bookingsInboxEnabled } from '$lib/features';

/**
 * Service bookings inbox at /messages/bookings — auth-gated. NOT gated on
 * feature.dm: bookings are usable without Direct Messages. The surface is
 * gated on service_directory.enabled when that flag is present, else
 * service_packages.enabled — with mentorship.enabled as an escape hatch so
 * mentorship-only deployments still reach their bookings.
 *
 * URL params: role=sent|received (default sent), status, intent, kind,
 * page, b=<booking id> (deep-link — opens the detail pane).
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	// Shared with the MessagesTabs visibility check (lib/features.ts) —
	// a visible tab never lands on a 404.
	if (!bookingsInboxEnabled(features)) {
		throw error(404, {
			code: 'feature_disabled',
			message: 'Service bookings are not available'
		});
	}

	if (!event.locals.user) {
		throw redirect(
			307,
			`/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`
		);
	}

	return loadBookingsSurface(event, {
		allowKindFilter: true,
		mentorshipEnabled: !!features['mentorship.enabled']
	});
};
