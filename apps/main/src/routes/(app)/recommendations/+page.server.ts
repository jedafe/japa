import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { RecommendationsResponse } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	const geoOverride = event.url.searchParams.get('geo')?.toUpperCase() ?? null;
	const path = geoOverride ? `/recommendations?limit=6&geo=${geoOverride}` : '/recommendations?limit=6';

	const recommendations = await apiGet<RecommendationsResponse>(ctx, path).catch(() => null);

	return { recommendations, geoOverride };
};
