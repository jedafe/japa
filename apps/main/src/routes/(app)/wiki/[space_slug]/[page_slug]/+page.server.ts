// Ref: japaconnect-engineering/sections/80-destination-wiki.md
import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { apiContext, apiGet, apiGetEnvelope, apiList, ApiError } from '$lib/server/api';
import type { WikiPage, WikiPageIndex, SpaceDetail } from '$lib/types';

type WikiApiCategory = { slug: string; label_key: string; label: string };

/**
 * Wiki page view at /wiki/{space_slug}/{page_slug} — fetches the current
 * approved revision's content via GET /wiki/{space_slug}/{page_slug}, plus
 * the space index + scoped categories so the guide tab strip and sidebar
 * chrome render identically to the destination index.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const spaceSlug = event.params.space_slug;
	const pageSlug = event.params.page_slug;
	const authed = !!event.locals.user;
	const userPermissions = event.locals.user?.permissions ?? [];

	try {
		const page = await apiGet<WikiPage>(
			ctx,
			`/wiki/${encodeURIComponent(spaceSlug)}/${encodeURIComponent(pageSlug)}`
		);
		// Index, space detail, and the scoped category set all fail soft — the
		// article body is the essential payload.
		const [indexRes, spaceDetail, catRes] = await Promise.all([
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
			page,
			authed,
			userPermissions,
			loadError: false,
			spaceDetail,
			index: indexRes?.items ?? [],
			apiCategories: catRes?.data ?? ([] as WikiApiCategory[]),
			wikiScope: catRes?.scope === 'university' ? ('university' as const) : ('geo' as const)
		};
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) {
			error(404, { code: 'wiki_page_not_found', message: 'Wiki page not found' });
		}
		return {
			spaceSlug,
			pageSlug,
			page: null,
			authed,
			userPermissions,
			loadError: true,
			spaceDetail: null,
			index: [] as WikiPageIndex[],
			apiCategories: [] as WikiApiCategory[],
			wikiScope: 'geo' as const
		};
	}
};
