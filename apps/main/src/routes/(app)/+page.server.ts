import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { feedPath, resolveTab, resolveSort, PAGE_SIZE } from '$lib/feed/endpoint';
import type { FeedItem } from '$lib/types';

/**
 * Home timeline. Tab + sort come from the URL (?tab=&sort=), so switching them
 * is a client-side navigation that re-runs this loader. Guests are pinned to
 * the public explore feed; authed users default to For You.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;
	const tab = resolveTab(event.url.searchParams.get('tab'), authed);
	const sort = resolveSort(event.url.searchParams.get('sort'));

	try {
		const { items } = await apiList<FeedItem>(ctx, feedPath(tab, sort, PAGE_SIZE, 0));
		return { items, tab, sort, authed, pageSize: PAGE_SIZE, loadError: false };
	} catch {
		return { items: [] as FeedItem[], tab, sort, authed, pageSize: PAGE_SIZE, loadError: true };
	}
};
