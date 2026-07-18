import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { FeedItem } from '$lib/types';

/**
 * Root timeline. Authenticated users get their primary feed; guests get the
 * public explore feed (the documented fallback until a Page Builder homepage is
 * assigned to the `homepage` route — ADR-032/058).
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;
	const path = authed ? '/feed?limit=20' : '/explore?limit=20';

	try {
		const { items } = await apiList<FeedItem>(ctx, path);
		return { items, authed, loadError: false };
	} catch {
		return { items: [] as FeedItem[], authed, loadError: true };
	}
};
