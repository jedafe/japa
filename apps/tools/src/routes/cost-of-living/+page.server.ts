import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiGet } from '$lib/server/api';
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

	return { cities, comparison };
};
