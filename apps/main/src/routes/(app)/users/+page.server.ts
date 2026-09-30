import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList, ApiError } from '$lib/server/api';
import type { PublicUser } from '$lib/types';

/**
 * Users directory at /users — public, SSR, indexable. Lists active,
 * search-indexable users with follower/following counts. Each card links to
 * the user profile at /u/[username]. Search filters by username or
 * display_name server-side. profile_type filter is optional.
 *
 * sort is whitelisted server-side: followers (default) | newest | oldest |
 * name. Unknown values degrade to the default ordering.
 *
 * Ref: japaconnect-engineering/sections/02-authentication-sessions.md
 *      japaconnect-engineering/sections/19-privacy-gdpr.md (search_indexable opt-out)
 *      ADR-058 (renders inside the app shell)
 */
const DEFAULT_PAGE_SIZE = 24;
const PAGE_SIZES = new Set([12, 24, 48, 96]);

const SORT_KEYS = new Set(['followers', 'newest', 'oldest', 'name']);

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const q = (event.url.searchParams.get('q') ?? '').trim();
	const profileType = (event.url.searchParams.get('profile_type') ?? '').trim();
	const sortParam = (event.url.searchParams.get('sort') ?? '').trim();
	const sort = SORT_KEYS.has(sortParam) ? sortParam : 'followers';
	const sizeParam = parseInt(event.url.searchParams.get('page_size') ?? '', 10);
	const pageSize = PAGE_SIZES.has(sizeParam) ? sizeParam : DEFAULT_PAGE_SIZE;
	const pageParam = parseInt(event.url.searchParams.get('page') ?? '0', 10);
	const page = isNaN(pageParam) || pageParam < 0 ? 0 : pageParam;
	const offset = page * pageSize;

	let users: PublicUser[] = [];
	let total = 0;
	let totalPages = 0;
	let loadError = false;
	let forbidden = false;

	try {
		const params = new URLSearchParams();
		params.set('page_size', String(pageSize));
		params.set('page_offset', String(offset));
		if (q) params.set('q', q);
		if (profileType) params.set('profile_type', profileType);
		if (sort !== 'followers') params.set('sort', sort);

		const result = await apiList<PublicUser>(ctx, `/users?${params.toString()}`);
		users = result.items;
		total = result.meta?.total ?? 0;
		totalPages = result.meta?.total_pages ?? 0;
	} catch (err) {
		if (err instanceof ApiError && err.status === 403) {
			forbidden = true;
		} else {
			loadError = true;
		}
	}

	// Out-of-range page (e.g. ?page=999 or stale links after the result set
	// shrank) renders an empty grid with a bogus "page N / M" label. Redirect
	// to the last valid page, preserving the active filters.
	if (!loadError && !forbidden && totalPages > 0 && page > totalPages - 1) {
		const params = new URLSearchParams();
		if (q) params.set('q', q);
		if (profileType) params.set('profile_type', profileType);
		if (sort !== 'followers') params.set('sort', sort);
		if (pageSize !== DEFAULT_PAGE_SIZE) params.set('page_size', String(pageSize));
		params.set('page', String(totalPages - 1));
		throw redirect(303, `/users?${params.toString()}`);
	}

	return { users, q, profileType, sort, page, pageSize, totalPages, total, forbidden, loadError };
};
