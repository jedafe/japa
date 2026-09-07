import type { PageServerLoad } from './$types';
import { apiContext, apiGetEnvelope, apiList } from '$lib/server/api';
import type { BlogSeries, ContentTypeMeta } from '$lib/types';
import type { GuideListItem } from '$lib/types/content';

/**
 * Guides hub — GET /content?type=guide with guide_category + cursor
 * pagination, plus tabs: Popular (sort=popular → view_count order, offset
 * pagination), Featured (generic content_featured junction), Series
 * (GET /blog/series?content_type=guide), and Roadmap (guides grouped by
 * journey_stage in canonical §87 order, fetched with page_size=100).
 * Category options come from GET /content-types (single source of truth).
 * Ref: japaconnect-engineering/sections/14-content-type-system.md
 */
const PAGE_SIZE = 24;
const ROADMAP_PAGE_SIZE = 100; // content.MaxPageSize — roadmap needs the full set to group by stage

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;
	const tab = event.url.searchParams.get('tab') ?? 'latest';
	const category = event.url.searchParams.get('category') ?? '';
	const cursor = event.url.searchParams.get('cursor') ?? '';
	const offset = parseInt(event.url.searchParams.get('offset') ?? '0', 10) || 0;

	const typesRes = await apiGetEnvelope<ContentTypeMeta[]>(ctx, '/content-types').catch(() => ({ data: [] }));
	const types = typesRes.data ?? [];
	const categories = types.find((t) => t.value === 'guide')?.categories ?? [];

	if (tab === 'series') {
		const { items, meta } = await apiList<BlogSeries>(
			ctx,
			`/blog/series?content_type=guide&page_size=${PAGE_SIZE}`
		).catch(() => ({ items: [] as BlogSeries[], meta: null }));
		return {
			items: [] as GuideListItem[],
			series: items,
			seriesTotal: meta?.total ?? 0,
			categories,
			category,
			tab,
			authed,
			pageSize: PAGE_SIZE,
			nextCursor: null as string | null,
			loadError: false
		};
	}

	const params = new URLSearchParams();
	params.set('type', 'guide');
	params.set('page_size', String(tab === 'roadmap' ? ROADMAP_PAGE_SIZE : PAGE_SIZE));
	if (tab === 'featured') {
		params.set('featured', '1');
	} else {
		if (category) params.set('guide_category', category);
		if (tab === 'popular') {
			params.set('sort', 'popular');
			if (offset > 0) params.set('page_offset', String(offset));
		} else if (tab === 'roadmap') {
			// Journey-context ranking — the API reads the resolved VisitorContext
			// (authed profile fields or the guest jc_journey_token cookie) and
			// boosts guides matching the visitor's stage/destination/visa/origin.
			params.set('sort', 'roadmap');
		} else if (cursor) {
			params.set('cursor', cursor);
		}
	}

	let items: GuideListItem[] = [];
	let loadError = false;
	try {
		const res = await apiGetEnvelope<GuideListItem[]>(ctx, `/content?${params.toString()}`);
		items = res.data ?? [];
	} catch {
		loadError = true;
	}

	// GET /content returns a bare list envelope. Latest uses a composite
	// `created_at|id` cursor; popular uses page_offset (view_count order has no
	// cursor); featured/roadmap are single-page views.
	const last = items[items.length - 1];
	const nextCursor =
		tab === 'popular'
			? (items.length >= PAGE_SIZE ? `offset:${offset + items.length}` : null)
			: tab === 'latest' && items.length >= PAGE_SIZE && last
				? `${last.created_at}|${last.id}`
				: null;

	return {
		items,
		series: [] as BlogSeries[],
		seriesTotal: 0,
		categories,
		category,
		tab,
		authed,
		pageSize: PAGE_SIZE,
		nextCursor,
		loadError
	};
};
