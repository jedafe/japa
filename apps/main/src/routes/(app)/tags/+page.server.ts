import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { TagDetail } from '$lib/types';

/**
 * Tags index at /tags — public, SSR, indexable. Lists every tag with
 * follower counts; each card links to the tag hub at /tags/[slug]. Search
 * filters by name or slug server-side. Sort filter is optional.
 *
 * Ref: japaconnect-engineering/sections/08-tags.md
 *      ADR-058 (renders inside the app shell; not a documented tab surface)
 */
const PAGE_SIZE = 24;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const q = (event.url.searchParams.get('q') ?? '').trim();
	const sort = (event.url.searchParams.get('sort') ?? '').trim();
	// The API's `page` param is 1-based per the route catalog (page=1 is the
	// first page); the UI keeps a 0-based index internally.
	const pageParam = parseInt(event.url.searchParams.get('page') ?? '1', 10);
	const page = isNaN(pageParam) || pageParam < 1 ? 0 : pageParam - 1;
	const offset = page * PAGE_SIZE;

	let tags: TagDetail[] = [];
	let total = 0;
	let totalPages = 0;
	let loadError = false;
	let forbidden = false;

	try {
		const params = new URLSearchParams();
		params.set('page_size', String(PAGE_SIZE));
		params.set('page_offset', String(offset));
		if (q) params.set('q', q);
		if (sort) params.set('sort', sort);

		const result = await apiList<TagDetail>(ctx, `/tags?${params.toString()}`);
		tags = result.items;
		total = result.meta?.total ?? 0;
		totalPages = result.meta?.total_pages ?? 0;
	} catch (err) {
		if (err instanceof ApiError && err.status === 403) {
			forbidden = true;
		} else {
			loadError = true;
		}
	}

	// Tags the viewer already follows — used to hydrate each card's
	// FollowButton so followed tags render "Following" instead of "Follow".
	// Auth-only endpoint; guests get an empty set. Best-effort: a failure
	// degrades to all-buttons-Follow rather than blocking the directory.
	let followedTagSlugs: string[] = [];
	if (event.locals.user) {
		try {
			const rows = await apiGet<{ entity_slug?: string; is_following?: boolean }[]>(
				ctx,
				'/me/followed-tags'
			);
			followedTagSlugs = (rows ?? [])
				.filter((r) => r.is_following !== false && r.entity_slug)
				.map((r) => r.entity_slug as string);
		} catch {
			followedTagSlugs = [];
		}
	}

	return {
		tags,
		q,
		sort,
		page,
		totalPages,
		total,
		pageSize: PAGE_SIZE,
		forbidden,
		loadError,
		followedTagSlugs
	};
};
