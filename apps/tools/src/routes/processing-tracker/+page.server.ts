import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { GeoSpaceOption, ProcessingStat, ProcessingTrendPoint } from '$lib/types';

/**
 * Visa Processing Time Tracker — §93 (reuses §83 visa timeline backend).
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
	// Gate: skip all backend calls when visa_timeline.enabled is false.
	// Ref: conventions/system-settings-catalog.md, Rule 3.
	const tool = getTool('processing-tracker')!;
	const settings = event.locals.siteSettings;
	if (!isToolEnabled(tool, settings)) {
		return {
			enabled: false,
			tool,
			destinations: [],
			stats: [],
			trend: [],
			user: event.locals.user
		};
	}

	const ctx = apiContext(event);

	// Destination country list — the primary data axis for every V2 geo-aware
	// tool (ADR-088). Rule 16 — never hardcode the country list.
	let destinations: GeoSpaceOption[] = [];
	try {
		destinations = (await apiList<GeoSpaceOption>(ctx, '/spaces?level=country')).items;
	} catch {
		destinations = [];
	}

	// No initial stats/trend load — both require dest + visa selection which
	// is an interactive client-side action. Seeded as empty arrays.
	return {
		enabled: true,
		tool,
		destinations,
		stats: [] as ProcessingStat[],
		trend: [] as ProcessingTrendPoint[],
		user: event.locals.user
	};
};
