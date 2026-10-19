// Ref: japaconnect-engineering — Japa Intelligence System spec §7
// Single radar item detail. SSR-fetches /intel/radar/feed/{id} so the page
// is indexable and renders without a client round-trip. Public: the backend
// route is OptionalAuth and the /radar geo-filtered feed is public — guests
// must be able to read an opportunity after browsing to it. Only the Save
// action is auth-gated (client-side requireAuthOrRedirect).
// Renders inside the unified app shell (ADR-058) via the (app) layout group.
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { RadarItem } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	// Explicit invalidation dependency so the Save toggle's
	// invalidate('/api/v1/intel/radar/saved') re-runs this load (SSR fetches
	// hit the backend origin, which relative-path invalidate never matches).
	event.depends('/api/v1/intel/radar/saved');

	// Forward the item type hint so the backend short-circuits the
	// cross-domain probe to a single table lookup.
	const itemType = event.url.searchParams.get('item_type') ?? event.url.searchParams.get('type') ?? '';
	const path = itemType
		? `/intel/radar/feed/${event.params.id}?item_type=${encodeURIComponent(itemType)}`
		: `/intel/radar/feed/${event.params.id}`;

	try {
		const item = await apiGet<RadarItem>(ctx, path);
		return { item, authed: !!event.locals.user };
	} catch (err) {
		if (err instanceof ApiError) {
			throw error(err.status, { code: err.code, message: err.message });
		}
		throw error(500, { code: 'api_error', message: 'Radar item could not be loaded' });
	}
};
