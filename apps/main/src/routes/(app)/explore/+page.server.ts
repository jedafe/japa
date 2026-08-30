import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { feedPath, resolveSort, PAGE_SIZE } from '$lib/feed/endpoint';
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
	// Forward the explicit ?geo= override or the user's content_country_preference
	// so the SSR feed matches the client-side load-more (ADR-064).
	const geo =
		event.url.searchParams.get('geo') ??
		event.locals.user?.content_country_preference ??
		undefined;

	try {
		const { items } = await apiList<FeedItem>(
			ctx,
			feedPath('explore', sort, PAGE_SIZE + 1, 0, geo)
		);
		const hasMore = items.length > PAGE_SIZE;
		return { items: items.slice(0, PAGE_SIZE), hasMore, sort, authed, pageSize: PAGE_SIZE, loadError: false };
	} catch {
		return { items: [] as FeedItem[], sort, authed, pageSize: PAGE_SIZE, loadError: true, hasMore: false };
	}
};
