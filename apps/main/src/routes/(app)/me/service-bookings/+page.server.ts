// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { ServiceBooking } from '$lib/types';

/**
 * My service bookings at /me/service-bookings (§111) — the traveler's
 * bookings + inquiries inbox. Auth-gated; the whole surface is behind the
 * service_packages.enabled feature flag like the backend routes it calls.
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
	const status = event.url.searchParams.get('status') ?? '';
	const intent = event.url.searchParams.get('intent') ?? '';
	const page = Number(event.url.searchParams.get('page') ?? '1') || 1;

	const params = new URLSearchParams();
	if (status) params.set('status', status);
	if (intent) params.set('intent', intent);
	params.set('page', String(page));
	params.set('limit', '20');

	let bookings: ServiceBooking[] = [];
	let meta: { total?: number; page?: number; limit?: number } | undefined;
	try {
		const res = await apiList<ServiceBooking>(ctx, `/me/service-bookings?${params.toString()}`);
		bookings = res.items;
		meta = res.meta;
	} catch {
		bookings = [];
	}

	return { bookings, meta, status, intent, currentPage: page, features };
};
