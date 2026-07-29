import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { resolveSort, PAGE_SIZE } from '$lib/feed/endpoint';
import type { FeedItem } from '$lib/types';

/**
 * Explore timeline at /explore — the ADR-058 §5 fallback for unauthenticated
 * guests when no homepage Page Builder landing is assigned, and the "Explore"
 * tab destination for authenticated users.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;
	const sort = resolveSort(event.url.searchParams.get('sort'));

	try {
		const { items } = await apiList<FeedItem>(ctx, `/explore?sort=${sort}&page_size=${PAGE_SIZE}&page_offset=0`);
		return { items, sort, authed, pageSize: PAGE_SIZE, loadError: false };
	} catch {
		return { items: [] as FeedItem[], sort, authed, pageSize: PAGE_SIZE, loadError: true };
	}
};
