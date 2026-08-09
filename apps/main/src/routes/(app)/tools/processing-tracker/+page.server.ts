import type { PageServerLoad } from './$types';
import { apiContext, apiList, ApiError } from '$lib/server/api';
import type { GeoSpaceOption } from '$lib/types';

/**
 * Visa Processing Time Tracker at /tools/processing-tracker (§93, reuses §83 visa timeline backend).
 * SSR loads the destination country list only. Stats and trend data require
 * a destination + visa category selection, so they are fetched client-side.
 *
 * API: GET /spaces?level=country — destination selector options.
 *      GET /visa-timelines/stats?dest=&origin=&visa= — processing stats.
 *      GET /visa-timelines/stats/trend?dest=&visa=&months=12 — trend.
 *
 * Ref: §93 (Processing Time Tracker), ROADMAP-TOOLS 02.06, ADR-088.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	// Destination country list — the primary data axis for every V2 geo-aware
	// tool (ADR-088). Rule 16 — never hardcode the country list.
	let destinations: GeoSpaceOption[] = [];
	let disabled = false;
	try {
		destinations = (await apiList<GeoSpaceOption>(ctx, '/spaces?level=country')).items;
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) {
			disabled = true;
		}
		// Other errors: leave destinations empty; client handles errors.
	}

	// No initial stats/trend load — both require dest + visa selection which
	// is an interactive client-side action. Seeded as empty arrays.
	return {
		destinations,
		disabled,
		authed
	};
};
