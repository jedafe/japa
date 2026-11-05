import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiGet, ApiError } from '$lib/server/api';
import type { Space, SpaceType, GeoCountry, GeoState, GeoCity, SpaceDetail } from '$lib/types';

/**
 * Spaces index at /spaces — public (OptionalAuth), SSR. Lists every
 * space the caller can see, with member/post counts. Each card links to the
 * space hub at /spaces/[slug]. Search filters by name or slug. The type
 * filter uses the space type SLUG in the URL (?type=university) for clean,
 * readable links; the slug is resolved to the space_type_id here before
 * calling the backend API.
 *
 * Ref: japaconnect-engineering/sections/05-geo-first-space-system.md
 *      conventions/system-settings-catalog.md (spaces.* settings)
 *      ADR-058 (renders inside the app shell)
 */
const DEFAULT_PAGE_SIZE = 24;
const DEFAULT_LEVELS = ['country', 'state', 'city', 'custom'];

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const q = (event.url.searchParams.get('q') ?? '').trim();
	const typeSlug = (event.url.searchParams.get('type') ?? '').trim();
	const levelFilter = (event.url.searchParams.get('level') ?? '').trim();
	const sort = (event.url.searchParams.get('sort') ?? '').trim();
	const geoFilter = (event.url.searchParams.get('geo') ?? '').trim();
	const regionFilter = (event.url.searchParams.get('region') ?? '').trim();
	const subregionFilter = (event.url.searchParams.get('subregion') ?? '').trim();
	const parentFilter = (event.url.searchParams.get('parent') ?? '').trim();
	// Universities-style geo cascade (filter parity with /universities):
	// ?country= is an ISO geo_code; ?state=/?city= are primary-space slugs
	// resolved to a parent-space id — the deepest selection wins.
	const countryParam = (event.url.searchParams.get('country') ?? '').trim();
	const stateParam = event.url.searchParams.get('state') ?? '';
	const cityParam = event.url.searchParams.get('city') ?? '';
	// joined=1 — server-side "My spaces" filter (audit fix: the chip used to
	// filter only the current page client-side against the capped /me/spaces
	// list, which is both incomplete and wrong for super-admins).
	const joinedOnly = event.url.searchParams.get('joined') === '1' && !!event.locals.user;
	const pageParam = parseInt(event.url.searchParams.get('page') ?? '0', 10);
	const page = isNaN(pageParam) || pageParam < 0 ? 0 : pageParam;

	// Rule 3 / Rule 16: configurable page size and level set from system_settings.
	// Settings + space types are shared catalogs; fetch them concurrently —
	// alongside the geo cascade's option lists and slug→id resolutions.
	const effectiveCountry = countryParam || geoFilter;
	// Parent filter resolves the deepest selection (city > state); the cities
	// option list needs the STATE id, so resolve state and city separately —
	// a single city||state resolution returns the city id and the cities
	// endpoint 404s on /geo/states/{city-id}/cities.
	const parentSlug = cityParam || stateParam;
	const [siteSettingsRes, spaceTypesRes, countries, parentDetail, stateDetail] = await Promise.all([
		apiGet<Record<string, unknown>>(ctx, '/site/settings/public')
			.then((s): Record<string, unknown> => s ?? {})
			.catch((): Record<string, unknown> => ({})),
		apiGet<SpaceType[]>(ctx, '/space-types')
			.then((t) => (Array.isArray(t) ? t : []))
			.catch(() => [] as SpaceType[]),
		apiList<GeoCountry>(ctx, '/geo/countries')
			.then((r) => r.items)
			.catch(() => [] as GeoCountry[]),
		parentSlug
			? apiGet<SpaceDetail>(ctx, `/spaces/${encodeURIComponent(parentSlug)}`).catch(() => null)
			: Promise.resolve(null),
		stateParam
			? apiGet<SpaceDetail>(ctx, `/spaces/${encodeURIComponent(stateParam)}`).catch(() => null)
			: Promise.resolve(null)
	]);
	const siteSettings = siteSettingsRes;
	const spaceTypes = spaceTypesRes;

	const levels = Array.isArray(siteSettings['spaces.levels'])
		? (siteSettings['spaces.levels'] as string[])
		: DEFAULT_LEVELS;
	const pageSize =
		typeof siteSettings['spaces.page_size'] === 'number'
			? Math.max(1, Math.round(siteSettings['spaces.page_size'] as number))
			: DEFAULT_PAGE_SIZE;
	const offset = page * pageSize;

	// Resolve the type slug (from the URL) to the space_type_id the backend
	// expects. If the slug doesn't match any type, the filter is ignored.
	let typeID = '';
	if (typeSlug) {
		const match = spaceTypes.find((t) => t.slug === typeSlug);
		if (match) typeID = match.id;
	}

	const params = new URLSearchParams();
	params.set('page_size', String(pageSize));
	params.set('page_offset', String(offset));
	// §05 amendment — the directory lists member-created spaces only. Primary
	// (geo + platform-seeded) spaces are reachable via /wiki, /universities,
	// direct URLs, and search — never this listing.
	params.set('member_created', '1');
	if (q) params.set('q', q);
	if (typeID) params.set('type', typeID);
	if (levelFilter) params.set('level', levelFilter);
	if (sort) params.set('sort', sort);
	if (geoFilter) params.set('geo', geoFilter);
	if (regionFilter) params.set('region', regionFilter);
	if (subregionFilter) params.set('subregion', subregionFilter);
	// Geo cascade → ?within= (deepest selection wins: city > state >
	// country). The hierarchical closure matches spaces linked to the geo
	// subtree through space_parent_spaces/parent_id — ?parent= only ever
	// matched DIRECT junction children, so a space linked to a city was
	// missed when filtering by its state. ?geo= is the fallback when the
	// country isn't a resolvable space slug, and ?parent=<id> still works
	// for hand-crafted links when no cascade selection exists.
	const withinSlug =
		cityParam ||
		stateParam ||
		countries.find((c) => c.geo_code === countryParam)?.slug ||
		'';
	if (withinSlug) params.set('within', withinSlug);
	else if (countryParam) params.set('geo', countryParam);
	if (parentFilter && !withinSlug) params.set('parent', parentFilter);
	if (joinedOnly) params.set('joined', '1');


	// Geo cascade option lists — states for the effective country, cities for
	// the selected state (same fetch pattern as /universities).
	// Cities options need the STATE id — ?state= resolves to it directly;
	// a city-only URL surfaces it via the city space's parent_id.
	const stateSpaceId = stateParam
		? (stateDetail?.space?.id ?? null)
		: (parentDetail?.space?.parent_id ?? null);
	const statesPromise = effectiveCountry
		? apiList<GeoState>(ctx, `/geo/countries/${encodeURIComponent(effectiveCountry)}/states`)
				.then((r) => r.items)
				.catch(() => [] as GeoState[])
		: Promise.resolve([] as GeoState[]);
	const citiesPromise =
		(stateParam || cityParam) && stateSpaceId
			? apiList<GeoCity>(ctx, `/geo/states/${stateSpaceId}/cities`)
					.then((r) => r.items)
					.catch(() => [] as GeoCity[])
			: Promise.resolve([] as GeoCity[]);

	const spacesPromise = apiList<Space>(ctx, `/spaces?${params.toString()}`).then(
		(result) => ({
			items: result.items,
			total: result.meta?.total ?? 0,
			totalPages: result.meta?.total_pages ?? 0,
			forbidden: false,
			loadError: false
		}),
		(err: unknown) => ({
			items: [] as Space[],
			total: 0,
			totalPages: 0,
			forbidden: err instanceof ApiError && err.status === 403,
			loadError: !(err instanceof ApiError && err.status === 403)
		})
	);

	// Try to resolve the parent filter name so the UI can display a chip.
	const parentNamePromise = parentFilter
		? apiGet<{ id: string; name: string }[]>(ctx, '/spaces/suggest-parents')
				.then((parents) => parents?.find((p) => p.id === parentFilter)?.name ?? '')
				.catch(() => '')
		: Promise.resolve('');

	const [spacesResult, parentName, states, cities] = await Promise.all([
		spacesPromise,
		parentNamePromise,
		statesPromise,
		citiesPromise
	]);

	const spaces = spacesResult.items;
	const total = spacesResult.total;
	const totalPages = spacesResult.totalPages;
	const loadError = spacesResult.loadError;
	const forbidden = spacesResult.forbidden;

	// Staff gate for the inline guide editor (same as /sources).
	const isSuperAdmin = (event.locals.user?.groups ?? []).some(
		(g) => g === 'Super Admin' || g === 'super_admin'
	);
	let canManage = isSuperAdmin;
	if (event.locals.user && !canManage) {
		try {
			const perms = await apiGet<{ permissions: string[] }>(ctx, '/me/permissions');
			canManage = (perms?.permissions ?? []).some((p) => p.startsWith('admin.settings'));
		} catch {
			canManage = false;
		}
	}

	return {
		canManage,
		spaces,
		spaceTypes,
		q,
		type: typeSlug,
		level: levelFilter,
		sort,
		geo: geoFilter,
		region: regionFilter,
		subregion: subregionFilter,
		parent: parentFilter,
		// Geo cascade (universities-style filter fields) — selections plus
		// the option lists that feed the autocompletes.
		country: countryParam,
		state: stateParam,
		city: cityParam,
		countries,
		states,
		cities,
		joined: joinedOnly,
		parentName,
		page,
		totalPages,
		total,
		pageSize,
		levels,
		forbidden,
		loadError,
		followingSpaceIds: [] as string[]
	};
};
