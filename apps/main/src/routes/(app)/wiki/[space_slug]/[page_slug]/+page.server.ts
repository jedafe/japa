// Ref: japaconnect-engineering/sections/80-destination-wiki.md
import type { PageServerLoad } from './$types';
import { error, redirect } from '@sveltejs/kit';
import { apiContext, apiGet, apiGetEnvelope, apiList, ApiError } from '$lib/server/api';
import type { WikiPage, WikiPageIndex, SpaceDetail } from '$lib/types';

type WikiApiCategory = {
	slug: string;
	label_key: string;
	label: string;
	/** §80 lazy template pages — sibling page links under this category,
	 *  {name} already substituted. Present only on ?space= requests. */
	page_templates?: { slug: string; label: string; position: number }[];
	/** Stub page slug for /wiki/{space}/{page_slug} links. */
	page_slug?: string;
	/** §80 directories — the stub page is the child-space listing. */
	directory?: boolean;
	/** §80 directories — true when the space has zero matching
	 *  descendants; the tab/card is hidden. */
	empty?: boolean;
	/** §80 category icon — admin-configured lucide name. */
	icon?: string;
};

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
		// Index, space detail, and the scoped category set all fail soft — the
		// article body is the essential payload. They only need space_slug, so
		// they fire alongside the page fetch rather than waiting for it.
		const pagePromise = apiGet<WikiPage>(
			ctx,
			`/wiki/${encodeURIComponent(spaceSlug)}/${encodeURIComponent(pageSlug)}`
		);
		const [page, indexRes, spaceDetail, catRes] = await Promise.all([
			pagePromise,
			apiList<WikiPageIndex>(ctx, `/wiki/${encodeURIComponent(spaceSlug)}`).catch(() => null),
			apiGet<SpaceDetail>(ctx, `/spaces/${encodeURIComponent(spaceSlug)}`).catch(() => null),
			apiGetEnvelope<WikiApiCategory[]>(
				ctx,
				`/wiki/categories?space=${encodeURIComponent(spaceSlug)}`
			).catch(() => null)
		]);
		// §43 crawl gate — stub/thin pages carry noindex,follow: out of the
		// index, but crawlers still follow their links into real content.
		if (!page?.indexable) {
			event.setHeaders({ 'x-robots-tag': 'noindex, follow' });
		}
		// §wiki-seo guest surface — a prose stub redirects guests to the
		// parent index (real content or the coming-soon state) instead of
		// erroring; members still see the stub as the contribution target.
		// Directory pages stay public: their rendered listing is content.
		if (!authed && page && !page.indexable && !page.directory) {
			redirect(303, `/wiki/${encodeURIComponent(spaceSlug)}`);
		}
		return {
			spaceSlug,
			pageSlug,
			page,
			// §80 §8: ?section={token} is the same page's section-filtered
			// view — "View more {section} prices" links land here.
			section: event.url.searchParams.get('section') ?? '',
			authed,
			userPermissions,
			loadError: false,
			removed: false,
			spaceDetail,
			// Guests see only indexable siblings — stub links dead-end at
			// the 404 gate above anyway.
			index: (indexRes?.items ?? []).filter((p) => authed || p.indexable !== false),
			apiCategories: catRes?.data ?? ([] as WikiApiCategory[]),
			wikiScope:
				catRes?.scope === 'university' || catRes?.scope === 'attraction'
					? catRes.scope
					: ('geo' as const)
		};
	} catch (e) {
		// redirect() throws — rethrow non-API failures (incl. the guest-stub
		// redirect above) rather than degrading them into loadError.
		if (!(e instanceof ApiError)) throw e;
		if (e.status === 404 || e.status === 410) {
			if (e.status === 410 || e.code === 'page_removed') {
				event.setHeaders({ 'x-robots-tag': 'noindex, nofollow' });
				return {
					spaceSlug,
					pageSlug,
					page: null,
					section: event.url.searchParams.get('section') ?? '',
					authed,
					userPermissions,
					loadError: true,
					removed: true,
					spaceDetail: null,
					index: [] as WikiPageIndex[],
					apiCategories: [] as WikiApiCategory[],
					wikiScope: 'geo' as const
				};
			}
			error(404, { code: 'wiki_page_not_found', message: 'Wiki page not found' });
		}
		event.setHeaders({ 'x-robots-tag': 'noindex, nofollow' });
		return {
			spaceSlug,
			pageSlug,
			page: null,
			section: event.url.searchParams.get('section') ?? '',
			authed,
			userPermissions,
			loadError: true,
			removed: false,
			spaceDetail: null,
			index: [] as WikiPageIndex[],
			apiCategories: [] as WikiApiCategory[],
			wikiScope: 'geo' as const
		};
	}
};
