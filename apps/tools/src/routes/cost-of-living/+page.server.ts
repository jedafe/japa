import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiGet } from '$lib/server/api';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { ColCity, ColComparison } from '$lib/types';

/**
 * Cost of Living Calculator — §72, ADR-072.
 * SSR loads the city list. The comparison matrix is loaded when the user
 * selects cities (or from a shareable URL with city_ids param).
 *
 * API: GET /calculator/cost-of-living/cities — cities with data.
 *      GET /calculator/cost-of-living/compare?city_ids= — matrix + ratios.
 *
 * Ref: §72 (Cost of Living), ROADMAP-TOOLS 02.04.
 */
export const load: PageServerLoad = async (event) => {
	// Gate: skip all backend calls when col_calculator.enabled is false.
	// Ref: conventions/system-settings-catalog.md, Rule 3.
	const tool = getTool('cost-of-living')!;
	const settings = event.locals.siteSettings;
	if (!isToolEnabled(tool, settings)) {
		return { enabled: false, tool, cities: [], comparison: null, maxCities: 3, staleDays: 180 };
	}

	const ctx = apiContext(event);

	let cities: ColCity[] = [];
	try {
		const res = await apiList<ColCity>(ctx, '/calculator/cost-of-living/cities');
		cities = res.items;
	} catch {
		cities = [];
	}

	// If the URL has city_ids (shareable link), load the comparison directly.
	const cityIdsParam = event.url.searchParams.get('city_ids');
	let comparison: ColComparison | null = null;
	if (cityIdsParam) {
		try {
			comparison = await apiGet<ColComparison>(
				ctx,
				`/calculator/cost-of-living/compare?city_ids=${encodeURIComponent(cityIdsParam)}`
			);
		} catch {
			comparison = null;
		}
	}

	// Tool config from system_settings (Rule 3 / Rule 16 — never hardcode config
	// values). Reuses the public settings already fetched in hooks.server.ts
	// (event.locals.siteSettings) to avoid a redundant backend round-trip.
	// Ref: §72 (Cost of Living), conventions/system-settings-catalog.md.
	let maxCities = 3;
	let staleDays = 180;
	if (typeof settings['col_calculator.max_cities'] === 'number') {
		maxCities = settings['col_calculator.max_cities'] as number;
	}
	if (typeof settings['col_calculator.stale_days'] === 'number') {
		staleDays = settings['col_calculator.stale_days'] as number;
	}

	return { enabled: true, tool, cities, comparison, maxCities, staleDays };
};
