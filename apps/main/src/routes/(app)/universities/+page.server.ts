import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { University } from '$lib/types';

/**
 * Universities at /universities — public, SSR, indexable. Lists university
 * spaces (gen.Space rows filtered to the university space type).
 *
 * The backend ListUniversities handler returns the full university catalog in
 * a single call (no server-side paging/filter). The catalog is small, so
 * search / country filter / sort / pagination are derived client-side from
 * URL search params (shareable, back-button safe, SSR-rendered).
 *
 * L4 — ADR-064 / Section 39: defaults the country filter from the user's
 * ContentCountry preference (or GeoContext). An explicit ?country= query
 * param always overrides the default.
 *
 * Ref: japaconnect-engineering/sections/39-universities.md
 */
const PAGE_SIZE = 12;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	// Resolve the default country filter: explicit query param > user preference > none.
	// Used only to pre-select the country dropdown on first render.
	const explicitCountry = event.url.searchParams.get('country');
	const defaultCountry = event.locals.user?.content_country_preference ?? null;
	const countryDefault = explicitCountry ?? defaultCountry;

	let universities: University[] = [];
	try {
		universities = (await apiList<University>(ctx, `/universities`)).items;
	} catch {
		universities = [];
	}

	return {
		universities,
		pageSize: PAGE_SIZE,
		countryDefault: countryDefault ?? null
	};
};
