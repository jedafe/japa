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
		return { enabled: false, tool, cities: [], matrix: null, preselected: [], preselectedBaseline: null, maxCities: 3, staleDays: 180 };
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
	// baseline_id names the ratio reference city (§72 amendment:
	// user-selectable baseline).
	const cityIdsParam = event.url.searchParams.get('city_ids');
	const baselineParam = event.url.searchParams.get('baseline_id');
	// Dedupe + keep only UUID-shaped values so crafted URLs
	// (?city_ids=a,a or ?city_ids=not-a-uuid) can't produce duplicate keyed
	// rows client-side or trip the compare endpoint's 400s.
	const UUID_RE = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;
	const preselected = cityIdsParam
		? [...new Set(cityIdsParam.split(',').filter((id) => UUID_RE.test(id.trim())))]
		: [];
	const preselectedBaseline =
		baselineParam && UUID_RE.test(baselineParam.trim()) ? baselineParam.trim() : null;
	let matrix: ColComparisonMatrix | null = null;
	if (preselected.length > 0) {
		try {
			const params = new URLSearchParams({ city_ids: preselected.join(',') });
			if (preselectedBaseline) params.set('baseline_id', preselectedBaseline);
			matrix = await apiGet<ColComparisonMatrix>(
				ctx,
				`/calculator/cost-of-living/compare?${params.toString()}`
			);
		} catch {
			matrix = null;
		}
	}

	// Tool config from system_settings (Rule 3 / Rule 16 — never hardcode config
	// values). Reuses the public settings already fetched in hooks.server.ts
	// (event.locals.siteSettings) to avoid a redundant backend round-trip.
	// Ref: §72 (Cost of Living), conventions/system-settings-catalog.md.
	// Public settings serialise ints as JSON strings — coerce leniently.
	const configuredMax = Number(settings['col_calculator.max_cities']);
	const maxCities =
		Number.isInteger(configuredMax) && configuredMax > 0 ? configuredMax : 3;
	const configuredStale = Number(settings['col_calculator.stale_days']);
	const staleDays =
		Number.isInteger(configuredStale) && configuredStale > 0 ? configuredStale : 180;

	return { enabled: true, tool, cities, matrix, preselected, preselectedBaseline, maxCities, staleDays };
};
