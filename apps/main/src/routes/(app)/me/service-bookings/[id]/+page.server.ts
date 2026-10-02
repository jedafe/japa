// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { ServiceBooking } from '$lib/types';

/**
 * Booking detail at /me/service-bookings/[id] (§111) — full booking view
 * for the booker (GET /me/service-bookings/:id is booker/org-member/admin
 * scoped server-side). Behind service_packages.enabled like the inbox.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['service_packages.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Service packages are not available' });
	}

	if (!event.locals.user) {
		throw redirect(
			307,
			`/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`
		);
	}

	const ctx = apiContext(event);
	try {
		const booking = await apiGet<ServiceBooking>(ctx, `/me/service-bookings/${event.params.id}`);
		return { booking, user: event.locals.user };
	} catch (err) {
		if (err instanceof ApiError && (err.status === 404 || err.status === 403)) {
			throw error(404, { code: 'not_found', message: 'Booking not found' });
		}
		throw error(500, { code: 'load_failed', message: 'Failed to load booking' });
	}
};
