import type { PageServerLoad } from './$types';
import { apiContext, apiList, ApiError } from '$lib/server/api';
import type { PublicUser } from '$lib/types';

/**
 * Users directory at /users — public, SSR, indexable. Lists active,
 * search-indexable users with follower/following counts. Each card links to
 * the user profile at /u/[username]. Search filters by username or
 * display_name server-side. profile_type filter is optional.
 *
 * Ref: japaconnect-engineering/sections/02-authentication-sessions.md
 *      japaconnect-engineering/sections/19-privacy-gdpr.md (search_indexable opt-out)
 *      ADR-058 (renders inside the app shell)
 */
const PAGE_SIZE = 24;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const q = (event.url.searchParams.get('q') ?? '').trim();
	const profileType = (event.url.searchParams.get('profile_type') ?? '').trim();
	const pageParam = parseInt(event.url.searchParams.get('page') ?? '0', 10);
	const page = isNaN(pageParam) || pageParam < 0 ? 0 : pageParam;
	const offset = page * PAGE_SIZE;

	let users: PublicUser[] = [];
	let total = 0;
	let totalPages = 0;
	let loadError = false;
	let forbidden = false;

	try {
		const params = new URLSearchParams();
		params.set('page_size', String(PAGE_SIZE));
		params.set('page_offset', String(offset));
		if (q) params.set('q', q);
		if (profileType) params.set('profile_type', profileType);

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

	return { users, q, profileType, page, totalPages, total, pageSize: PAGE_SIZE, forbidden, loadError };
};
