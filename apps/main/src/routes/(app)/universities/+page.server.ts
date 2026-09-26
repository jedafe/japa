import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { University, ListMeta, GeoCountry, GeoState, GeoCity, SpaceDetail } from '$lib/types';

/**
 * Universities at /universities — public, SSR, indexable. Lists university
 * spaces (gen.Space rows filtered to the university space type).
 *
 * The backend ListUniversities handler honors ?country=, ?state=, ?city=,
 * ?q=, ?sort=, ?page=, ?limit= (Section 39 catalog). Server-side pagination +
 * filtering + sorting so the ~9,000-row seeded catalog never loads in full on
 * one render. The URL is the source of truth for filter / sort / page state
 * (shareable, back-button safe, SSR-rendered).
 *
 * L4 — ADR-064 / Section 39: defaults the country filter from the user's
 * ContentCountry preference (or GeoContext). An explicit ?country= query
 * param always overrides the default.
 *
 * State/city cascade: ?state= and ?city= are primary-space slugs forwarded
 * to the backend `within` filter (space_parent_spaces hierarchy). The option
 * lists are loaded server-side from the canonical geo endpoints — states for
 * the effective country, cities for the selected state.
 *
 * Ref: japaconnect-engineering/sections/39-universities.md
 */
const PAGE_SIZE = 12;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	// Resolve the default country filter: explicit query param > user preference > none.
	// Used only to pre-select the country dropdown on first render.
	const explicitCountry = event.url.searchParams.get('country');
	const stateParam = event.url.searchParams.get('state') ?? '';
	const cityParam = event.url.searchParams.get('city') ?? '';
	const defaultCountry = event.locals.user?.content_country_preference ?? null;

	// Canonical country list for the filter dropdown (ADR-005). Loaded first
	// because a state selection also implies its parent country — resolved via
	// the countries' ids — on hand-crafted URLs like ?state=lagos.
	let countries: GeoCountry[] = [];
	try {
		countries = (await apiList<GeoCountry>(ctx, '/geo/countries')).items;
	} catch {
		countries = [];
	}

	// Resolve the state space when ?state= is set: its id feeds the cities
	// endpoint and its parent_id lets a state-only URL imply the country,
	// which wins over the user-preference default.
	let effectiveCountry = explicitCountry ?? defaultCountry;
	let stateSpaceId: string | null = null;
	if (stateParam) {
		try {
			const detail = await apiGet<SpaceDetail>(ctx, `/spaces/${encodeURIComponent(stateParam)}`);
			stateSpaceId = detail.space.id;
			if (!explicitCountry && detail.space.parent_id) {
				const parent = countries.find((c) => c.id === detail.space.parent_id);
				if (parent) effectiveCountry = parent.geo_code;
			}
		} catch {
			// Unknown slug — the backend within filter simply matches nothing.
		}
	}

	// City-only URLs (?city= without ?state=): the city space's parent_id is
	// the state UUID and its geo_code the country ISO, so the whole chain
	// resolves server-side — the state/city dropdowns render populated and
	// selected instead of dead.
	let resolvedStateSlug: string | null = null;
	if (cityParam && !stateParam) {
		try {
			const detail = await apiGet<SpaceDetail>(ctx, `/spaces/${encodeURIComponent(cityParam)}`);
			if (detail.space.level === 'city' && detail.space.parent_id) {
				stateSpaceId = detail.space.parent_id;
				if (!explicitCountry && detail.space.geo_code) {
					const parent = countries.find((c) => c.geo_code === detail.space.geo_code);
					if (parent) effectiveCountry = parent.geo_code;
				}
			}
		} catch {
			// Unknown slug — the backend within filter simply matches nothing.
		}
	}

	// Forward filter / sort / page params to the backend. The backend does
	// server-side ILIKE search, geo_code filter, within filter, sort, and
	// pagination.
	const params = new URLSearchParams();
	params.set('limit', String(PAGE_SIZE));
	const page = event.url.searchParams.get('page') ?? '1';
	params.set('page', page);
	const q = event.url.searchParams.get('q');
	if (q) params.set('q', q);
	if (effectiveCountry) params.set('country', effectiveCountry);
	if (stateParam) params.set('state', stateParam);
	if (cityParam) params.set('city', cityParam);
	const sort = event.url.searchParams.get('sort');
	if (sort) params.set('sort', sort);

	// State options for the effective country, and city options for the
	// selected state (its UUID is required by the cities endpoint).
	let states: GeoState[] = [];
	let cities: GeoCity[] = [];
	if (effectiveCountry) {
		try {
			states = (await apiList<GeoState>(ctx, `/geo/countries/${encodeURIComponent(effectiveCountry)}/states`)).items;
		} catch {
			states = [];
		}
	}
	const selectedStateId = states.find((s) => s.slug === stateParam)?.id ?? stateSpaceId;
	if (!stateParam && stateSpaceId) {
		// City-only URL — the resolved parent state's slug preselects the
		// state dropdown (and lets the next UI change emit a canonical URL).
		resolvedStateSlug = states.find((s) => s.id === stateSpaceId)?.slug ?? null;
	}
	if ((stateParam || cityParam) && selectedStateId) {
		try {
			cities = (await apiList<GeoCity>(ctx, `/geo/states/${selectedStateId}/cities`)).items;
		} catch {
			cities = [];
		}
	}

	let universities: University[] = [];
	let meta: ListMeta | undefined;
	try {
		const result = await apiList<University>(ctx, `/universities?${params.toString()}`);
		universities = result.items;
		meta = result.meta;
	} catch {
		universities = [];
	}

	return {
		universities,
		meta,
		pageSize: PAGE_SIZE,
		countryDefault: effectiveCountry ?? null,
		/** Parent state slug resolved for city-only URLs (?city= without
		 *  ?state=) — preselects the state dropdown. */
		stateDefault: resolvedStateSlug,
		countries,
		states,
		cities
	};
};
