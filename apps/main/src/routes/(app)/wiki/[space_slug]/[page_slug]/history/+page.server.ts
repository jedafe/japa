// Ref: japaconnect-engineering/sections/80-destination-wiki.md
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { WikiRevision } from '$lib/types';

type ListMeta = { total: number; page: number; limit: number };

/**
 * Revision history for a wiki page at /wiki/{space_slug}/{page_slug}/history —
 * lists all revisions (pending/approved/rejected) via
 * GET /wiki/{space_slug}/{page_slug}/history.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const spaceSlug = event.params.space_slug;
	const pageSlug = event.params.page_slug;

	const page = Math.max(1, parseInt(event.url.searchParams.get('page') ?? '1', 10) || 1);
	const limit = Math.min(100, Math.max(1, parseInt(event.url.searchParams.get('limit') ?? '20', 10) || 20));

	try {
		const { items, meta } = await apiList<WikiRevision>(
			ctx,
			`/wiki/${encodeURIComponent(spaceSlug)}/${encodeURIComponent(pageSlug)}/history?page=${page}&limit=${limit}`
		);
		return { spaceSlug, pageSlug, revisions: items, meta: meta ?? { total: items.length, page, limit }, loadError: false };
	} catch {
		return { spaceSlug, pageSlug, revisions: [] as WikiRevision[], meta: { total: 0, page, limit }, loadError: true };
	}
};
