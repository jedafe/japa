// Ref: japaconnect-engineering/sections/80-destination-wiki.md
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { WikiPageIndex } from '$lib/types';

/**
 * Wiki index for a space at /wiki/{space_slug} — lists all wiki pages for the
 * space, grouped by category on the client. The backend returns the page
 * summaries (no content) via GET /wiki/{space_slug}.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const spaceSlug = event.params.space_slug;

	try {
		const { items } = await apiList<WikiPageIndex>(ctx, `/wiki/${encodeURIComponent(spaceSlug)}`);
		return { spaceSlug, index: items, loadError: false };
	} catch {
		return { spaceSlug, index: [] as WikiPageIndex[], loadError: true };
	}
};
