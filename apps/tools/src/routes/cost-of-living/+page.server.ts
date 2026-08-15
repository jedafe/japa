import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { ColCity, ColCitiesListResponse, ColComparisonMatrix } from '$lib/types';

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
	if (!isToolEnabled(tool, event.locals.features)) {
		return { enabled: false, tool, cities: [], matrix: null, preselected: [], maxCities: 3, staleDays: 180 };
	}

	const ctx = apiContext(event);

	let cities: ColCity[] = [];
	try {
		const res = await apiGet<ColCitiesListResponse>(ctx, '/calculator/cost-of-living/cities?page=1&limit=200');
		cities = res.cities ?? [];
	} catch {
		cities = [];
	}

	// If the URL has city_ids (shareable link), load the comparison directly.
	const cityIdsParam = event.url.searchParams.get('city_ids');
	const preselected = cityIdsParam ? cityIdsParam.split(',').filter(Boolean) : [];
	let matrix: ColComparisonMatrix | null = null;
	if (cityIdsParam) {
		try {
			matrix = await apiGet<ColComparisonMatrix>(
				ctx,
				`/calculator/cost-of-living/compare?city_ids=${encodeURIComponent(cityIdsParam)}`
			);
		} catch {
			matrix = null;
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

	return { enabled: true, tool, cities, matrix, preselected, maxCities, staleDays };
};
