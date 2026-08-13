// Ref: japaconnect-engineering — Japa Intelligence System spec §7
// Single radar item detail. SSR-fetches /intel/radar/feed/{id} so the page
// is indexable and renders without a client round-trip. Auth-gated: guests
// are redirected to login (the radar is a personalised surface). Renders
// inside the unified app shell (ADR-058) via the (app) layout group.
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { RadarItem } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(
			307,
			`/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`
		);
	}

	const ctx = apiContext(event);
	try {
		const item = await apiGet<RadarItem>(ctx, `/intel/radar/feed/${event.params.id}`);
		return { item };
	} catch (err) {
		if (err instanceof ApiError) {
			throw error(err.status, { code: err.code, message: err.message });
		}
		throw error(500, { code: 'api_error', message: 'Radar item could not be loaded' });
	}
};
