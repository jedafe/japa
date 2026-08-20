import { apiList } from './api';
import type { ApiContext } from './api';
import type { GeoCountry, GeoSpaceOption } from '$lib/types';

/**
 * Load the full 250 primary geo space countries from GET /geo/countries
 * and map them into the GeoSpaceOption shape used by tool selectors.
 *
 * Ref: §05 (Geo-First Space System), §78 (Tools Web Surface), ADR-088.
 */
export async function loadCountries(ctx: ApiContext): Promise<GeoSpaceOption[]> {
	const { items } = await apiList<GeoCountry>(ctx, '/geo/countries');
	return items.map((c) => ({
		id: c.id,
		name: c.name,
		slug: c.slug,
		code: c.geo_code ?? null
	}));
}
