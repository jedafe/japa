import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiGet, ApiError } from '$lib/server/api';
import type { Space, SpaceType } from '$lib/types';

/**
 * Spaces index at /spaces — public (OptionalAuth), SSR. Lists every tenant
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
	const pageParam = parseInt(event.url.searchParams.get('page') ?? '0', 10);
	const page = isNaN(pageParam) || pageParam < 0 ? 0 : pageParam;

	// Rule 3 / Rule 16: configurable page size and level set from system_settings.
	let siteSettings: Record<string, unknown> = {};
	try {
		siteSettings = (await apiGet<Record<string, unknown>>(ctx, '/site/settings/public')) ?? {};
	} catch {
		siteSettings = {};
	}

	const levels = Array.isArray(siteSettings['spaces.levels'])
		? (siteSettings['spaces.levels'] as string[])
		: DEFAULT_LEVELS;
	const pageSize =
		typeof siteSettings['spaces.page_size'] === 'number'
			? Math.max(1, Math.round(siteSettings['spaces.page_size'] as number))
			: DEFAULT_PAGE_SIZE;
	const offset = page * pageSize;

	// Fetch space types first so we can resolve the slug → ID and render chips.
	let spaceTypes: SpaceType[] = [];
	try {
		spaceTypes = await apiGet<SpaceType[]>(ctx, '/space-types');
		if (!Array.isArray(spaceTypes)) spaceTypes = [];
	} catch {
		spaceTypes = [];
	}

	// Resolve the type slug (from the URL) to the space_type_id the backend
	// expects. If the slug doesn't match any type, the filter is ignored.
	let typeID = '';
	if (typeSlug) {
		const match = spaceTypes.find((t) => t.slug === typeSlug);
		if (match) typeID = match.id;
	}

	let spaces: Space[] = [];
	let total = 0;
	let totalPages = 0;
	let loadError = false;
	let forbidden = false;
	let followingSpaceIds: string[] = [];

	if (event.locals.user) {
		try {
			const following = await apiGet<{ entity_id: string }[]>(ctx, '/me/following?type=space&page_size=100');
			followingSpaceIds = (following ?? []).map((f) => f.entity_id);
		} catch {
			followingSpaceIds = [];
		}
	}

	try {
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

		const result = await apiList<Space>(ctx, `/spaces?${params.toString()}`);
		spaces = result.items;
		total = result.meta?.total ?? 0;
		totalPages = result.meta?.total_pages ?? 0;
	} catch (err) {
		if (err instanceof ApiError && err.status === 403) {
			forbidden = true;
		} else {
			loadError = true;
		}
	}

	// Fetch region and subregion options for the directory filters.
	let regions: string[] = [];
	try {
		regions = await apiGet<string[]>(ctx, '/spaces/regions');
		if (!Array.isArray(regions)) regions = [];
	} catch {
		regions = [];
	}

	let subregions: string[] = [];
	if (regionFilter) {
		try {
			subregions = await apiGet<string[]>(ctx, `/spaces/subregions?region=${encodeURIComponent(regionFilter)}`);
			if (!Array.isArray(subregions)) subregions = [];
		} catch {
			subregions = [];
		}
	}

	// Try to resolve the parent filter name so the UI can display a chip.
	let parentName = '';
	if (parentFilter) {
		try {
			const parents = await apiGet<{ id: string; name: string }[]>(ctx, '/spaces/suggest-parents');
			parentName = parents?.find((p) => p.id === parentFilter)?.name ?? '';
		} catch {
			parentName = '';
		}
	}

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
		parentName,
		regions,
		subregions,
		page,
		totalPages,
		total,
		pageSize,
		levels,
		forbidden,
		loadError,
		followingSpaceIds
	};
};
