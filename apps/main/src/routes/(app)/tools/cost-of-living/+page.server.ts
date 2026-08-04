import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import { ApiError } from '$lib/server/api';
import type { ColCity, ColCitiesListResponse } from '$lib/types';

/**
 * Cost of Living Calculator at /tools/cost-of-living (Section 72, ADR-072).
 * SSR-fetches the city list so the selector renders without a client
 * round-trip. The comparison matrix is fetched client-side when the user
 * selects cities (shareable via ?city_ids= query param).
 *
 * Ref: japaconnect-engineering/sections/72-cost-of-living.md
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	let cities: ColCity[] = [];
	let disabled = false;
	try {
		const res = await apiGet<ColCitiesListResponse>(ctx, '/calculator/cost-of-living/cities?page=1&limit=200');
		cities = res.cities ?? [];
	} catch (e) {
		if (e instanceof ApiError && (e.code === 'col_calculator_disabled' || e.status === 404)) {
			disabled = true;
		}
	}

	// Pre-select from shareable URL ?city_ids=a,b,c
	const cityIdsParam = event.url.searchParams.get('city_ids');
	const preselected = cityIdsParam ? cityIdsParam.split(',').filter(Boolean) : [];

	return { cities, disabled, authed, preselected };
};
