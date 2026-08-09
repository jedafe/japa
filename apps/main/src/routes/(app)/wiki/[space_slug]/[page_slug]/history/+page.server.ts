// Ref: japaconnect-engineering/sections/80-destination-wiki.md
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { WikiRevision } from '$lib/types';

/**
 * Revision history for a wiki page at /wiki/{space_slug}/{page_slug}/history —
 * lists all revisions (pending/approved/rejected) via
 * GET /wiki/{space_slug}/{page_slug}/history.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const spaceSlug = event.params.space_slug;
	const pageSlug = event.params.page_slug;

	try {
		const { items } = await apiList<WikiRevision>(
			ctx,
			`/wiki/${encodeURIComponent(spaceSlug)}/${encodeURIComponent(pageSlug)}/history`
		);
		return { spaceSlug, pageSlug, revisions: items, loadError: false };
	} catch {
		return { spaceSlug, pageSlug, revisions: [] as WikiRevision[], loadError: true };
	}
};
