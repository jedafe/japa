import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { University } from '$lib/types';

/**
 * Universities at /universities — public, SSR, indexable. Lists university
 * spaces (gen.Space rows filtered to the university space type).
 *
 * L4 — ADR-064 / Section 39: defaults the country filter from the user's
 * ContentCountry preference (or GeoContext). An explicit ?country= query
 * param always overrides the default.
 *
 * Ref: japaconnect-engineering/sections/39-universities.md
 */
const PAGE_SIZE = 24;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	// Resolve the country filter: explicit query param > user preference > none.
	const explicitCountry = event.url.searchParams.get('country');
	const defaultCountry = event.locals.user?.content_country_preference ?? null;
	const country = explicitCountry ?? defaultCountry;

	let universities: University[] = [];
	try {
		const params = new URLSearchParams({ page_size: String(PAGE_SIZE) });
		if (country) params.set('country', country);
		universities = (await apiList<University>(ctx, `/universities?${params.toString()}`)).items;
	} catch {
		universities = [];
	}

	return { universities, pageSize: PAGE_SIZE, countryFilter: country ?? null };
};
