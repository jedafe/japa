// Ref: japaconnect-engineering/sections/80-destination-wiki.md
import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiGet, apiGetEnvelope } from '$lib/server/api';
import type { WikiRevision, WikiPageIndex, SpaceDetail } from '$lib/types';

type ListMeta = { total: number; page: number; limit: number };
type WikiApiCategory = { slug: string; label_key: string; label: string };

/**
 * Revision history for a wiki page at /wiki/{space_slug}/{page_slug}/history —
 * lists all revisions (pending/approved/rejected) via
 * GET /wiki/{space_slug}/{page_slug}/history. The space index, detail, and
 * scoped categories are fetched fail-soft so the route-level sidebar and
 * breadcrumb render identically to the article view.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const spaceSlug = event.params.space_slug;
	const pageSlug = event.params.page_slug;

	const page = Math.max(1, parseInt(event.url.searchParams.get('page') ?? '1', 10) || 1);
	const limit = Math.min(100, Math.max(1, parseInt(event.url.searchParams.get('limit') ?? '20', 10) || 20));

	try {
		const [revRes, indexRes, spaceDetail, catRes] = await Promise.all([
			apiList<WikiRevision>(
				ctx,
				`/wiki/${encodeURIComponent(spaceSlug)}/${encodeURIComponent(pageSlug)}/history?page=${page}&limit=${limit}`
			),
			apiList<WikiPageIndex>(ctx, `/wiki/${encodeURIComponent(spaceSlug)}`).catch(() => null),
			apiGet<SpaceDetail>(ctx, `/spaces/${encodeURIComponent(spaceSlug)}`).catch(() => null),
			apiGetEnvelope<WikiApiCategory[]>(
				ctx,
				`/wiki/categories?space=${encodeURIComponent(spaceSlug)}`
			).catch(() => null)
		]);
		return {
			spaceSlug,
			pageSlug,
			revisions: revRes.items,
			meta: revRes.meta ?? { total: revRes.items.length, page, limit },
			loadError: false,
			spaceDetail,
			index: indexRes?.items ?? [],
			apiCategories: catRes?.data ?? ([] as WikiApiCategory[]),
			wikiScope: catRes?.scope === 'university' ? ('university' as const) : ('geo' as const)
		};
	} catch {
		return {
			spaceSlug,
			pageSlug,
			revisions: [] as WikiRevision[],
			meta: { total: 0, page, limit },
			loadError: true,
			spaceDetail: null,
			index: [] as WikiPageIndex[],
			apiCategories: [] as WikiApiCategory[],
			wikiScope: 'geo' as const
		};
	}
};
