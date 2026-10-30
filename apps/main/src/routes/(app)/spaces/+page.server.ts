import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiGet, ApiError } from '$lib/server/api';
import type { GeoCountry, Space, SpaceType } from '$lib/types';

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
	// joined=1 — server-side "My spaces" filter (audit fix: the chip used to
	// filter only the current page client-side against the capped /me/spaces
	// list, which is both incomplete and wrong for super-admins).
	const joinedOnly = event.url.searchParams.get('joined') === '1' && !!event.locals.user;
	const pageParam = parseInt(event.url.searchParams.get('page') ?? '0', 10);
	const page = isNaN(pageParam) || pageParam < 0 ? 0 : pageParam;

	// Rule 3 / Rule 16: configurable page size and level set from system_settings.
	// Settings + space types are shared catalogs; fetch them concurrently.
	const [siteSettingsRes, spaceTypesRes] = await Promise.all([
		apiGet<Record<string, unknown>>(ctx, '/site/settings/public')
			.then((s): Record<string, unknown> => s ?? {})
			.catch((): Record<string, unknown> => ({})),
		apiGet<SpaceType[]>(ctx, '/space-types')
			.then((t) => (Array.isArray(t) ? t : []))
			.catch(() => [] as SpaceType[])
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
	if (q) params.set('q', q);
	if (typeID) params.set('type', typeID);
	if (levelFilter) params.set('level', levelFilter);
	if (sort) params.set('sort', sort);
	if (geoFilter) params.set('geo', geoFilter);
	if (regionFilter) params.set('region', regionFilter);
	if (subregionFilter) params.set('subregion', subregionFilter);
	if (parentFilter) params.set('parent', parentFilter);
	if (joinedOnly) params.set('joined', '1');


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

	// Country list powers the ?geo= filter picker (was a free-text code input).
	const countriesPromise = apiGet<GeoCountry[]>(ctx, '/geo/countries')
		.then((c) => (Array.isArray(c) ? c : []))
		.catch(() => [] as GeoCountry[]);

	// Fetch region options for the directory filters. ?subregion= remains a
	// valid URL-only param (passed through above) — no dedicated UI control.
	const regionsPromise = apiGet<string[]>(ctx, '/spaces/regions')
		.then((r) => (Array.isArray(r) ? r : []))
		.catch(() => [] as string[]);

	// Try to resolve the parent filter name so the UI can display a chip.
	const parentNamePromise = parentFilter
		? apiGet<{ id: string; name: string }[]>(ctx, '/spaces/suggest-parents')
				.then((parents) => parents?.find((p) => p.id === parentFilter)?.name ?? '')
				.catch(() => '')
		: Promise.resolve('');

	const [spacesResult, countries, regions, parentName] = await Promise.all([
		spacesPromise,
		countriesPromise,
		regionsPromise,
		parentNamePromise
	]);

	const spaces = spacesResult.items;
	const total = spacesResult.total;
	const totalPages = spacesResult.totalPages;
	const loadError = spacesResult.loadError;
	const forbidden = spacesResult.forbidden;

	return {
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
		joined: joinedOnly,
		parentName,
		countries,
		regions,
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
