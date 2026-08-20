import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { University, ListMeta, GeoCountry } from '$lib/types';

/**
 * Universities at /universities — public, SSR, indexable. Lists university
 * spaces (gen.Space rows filtered to the university space type).
 *
 * The backend ListUniversities handler honors ?country=, ?q=, ?sort=, ?page=,
 * ?limit= (Section 39 catalog). Server-side pagination + filtering + sorting
 * so the ~9,000-row seeded catalog never loads in full on one render. The
 * URL is the source of truth for filter / sort / page state (shareable,
 * back-button safe, SSR-rendered).
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

	// Forward filter / sort / page params to the backend. The backend does
	// server-side ILIKE search, geo_code filter, sort, and pagination.
	const params = new URLSearchParams();
	params.set('limit', String(PAGE_SIZE));
	const page = event.url.searchParams.get('page') ?? '1';
	params.set('page', page);
	const q = event.url.searchParams.get('q');
	if (q) params.set('q', q);
	const country = explicitCountry ?? defaultCountry;
	if (country) params.set('country', country);
	const sort = event.url.searchParams.get('sort');
	if (sort) params.set('sort', sort);

	let universities: University[] = [];
	let meta: ListMeta | undefined;
	try {
		const result = await apiList<University>(ctx, `/universities?${params.toString()}`);
		universities = result.items;
		meta = result.meta;
	} catch {
		universities = [];
	}

	// Canonical country list for the filter dropdown (ADR-005).
	let countries: GeoCountry[] = [];
	try {
		countries = (await apiList<GeoCountry>(ctx, '/geo/countries')).items;
	} catch {
		countries = [];
	}

	return {
		universities,
		meta,
		pageSize: PAGE_SIZE,
		countryDefault: countryDefault ?? null,
		countries
	};
};
