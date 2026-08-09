// Ref: japaconnect-engineering/sections/80-destination-wiki.md
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { WikiPage } from '$lib/types';

/**
 * Wiki page view at /wiki/{space_slug}/{page_slug} — fetches the current
 * approved revision's content via GET /wiki/{space_slug}/{page_slug}.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const spaceSlug = event.params.space_slug;
	const pageSlug = event.params.page_slug;

	try {
		const page = await apiGet<WikiPage>(
			ctx,
			`/wiki/${encodeURIComponent(spaceSlug)}/${encodeURIComponent(pageSlug)}`
		);
		return { spaceSlug, pageSlug, page, loadError: false };
	} catch {
		return { spaceSlug, pageSlug, page: null, loadError: true };
	}
};
