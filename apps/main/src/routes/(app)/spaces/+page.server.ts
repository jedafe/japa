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
 * Ref: japaconnect-engineering/sections/22-spaces.md
 *      ADR-058 (renders inside the app shell)
 */
const PAGE_SIZE = 24;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const q = (event.url.searchParams.get('q') ?? '').trim();
	const typeSlug = (event.url.searchParams.get('type') ?? '').trim();
	const pageParam = parseInt(event.url.searchParams.get('page') ?? '0', 10);
	const page = isNaN(pageParam) || pageParam < 0 ? 0 : pageParam;
	const offset = page * PAGE_SIZE;

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

	try {
		const params = new URLSearchParams();
		params.set('page_size', String(PAGE_SIZE));
		params.set('page_offset', String(offset));
		if (q) params.set('q', q);
		if (typeID) params.set('type', typeID);

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

	return {
		spaces,
		spaceTypes,
		q,
		type: typeSlug,
		page,
		totalPages,
		total,
		pageSize: PAGE_SIZE,
		forbidden,
		loadError
	};
};
