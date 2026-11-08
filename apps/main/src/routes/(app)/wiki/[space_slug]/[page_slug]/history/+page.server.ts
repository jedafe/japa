// Ref: japaconnect-engineering/sections/80-destination-wiki.md
import { fail } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiList, apiGet, apiGetEnvelope, apiDelete, ApiError } from '#lib/server/api.js';
import type { WikiRevision, WikiPageIndex, SpaceDetail } from '#lib/types/index.js';

type ListMeta = { total: number; page: number; limit: number };
type WikiApiCategory = {
	slug: string;
	label_key: string;
	label: string;
	/** Stub page slug for /wiki/{space}/{page_slug} links. */
	page_slug?: string;
	/** §80 directories — the stub page is the child-space listing. */
	directory?: boolean;
};

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
			canClear: event.locals.user?.permissions?.includes('wiki.manage') ?? false,
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
			canClear: event.locals.user?.permissions?.includes('wiki.manage') ?? false,
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

export const actions: Actions = {
	/**
	 * Clear a page's resolved revision history (§44 amendment 5 parity —
	 * wiki.manage only, pending submissions are never touched). The slug is
	 * resolved server-side to the page id — the client never supplies it.
	 */
	clear: async (event) => {
		if (!(event.locals.user?.permissions?.includes('wiki.manage') ?? false)) {
			return fail(403, { error: 'forbidden' });
		}

		const ctx = apiContext(event);
		let pageId: string;
		try {
			const detail = await apiGet<{ id: string }>(
				ctx,
				`/wiki/${encodeURIComponent(event.params.space_slug)}/${encodeURIComponent(event.params.page_slug)}`
			);
			pageId = detail.id;
		} catch {
			return fail(404, { error: 'page_not_found' });
		}

		try {
			await apiDelete(ctx, `/admin/wiki/${pageId}/revisions`);
		} catch (err) {
			if (err instanceof ApiError) return fail(err.status, { error: err.code });
			return fail(500, { error: 'internal_error' });
		}

		return { cleared: true };
	}
};
