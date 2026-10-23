import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGetEnvelope } from '$lib/server/api';
import { attachAuthorUsernames } from '$lib/server/users';
import type { GuideListItem } from '$lib/types/content';

/**
 * Lists hub at /lists — GET /content?type=list with cursor pagination,
 * modeled on /guides minus categories/series/roadmap (lists carry no
 * category taxonomy). Tabs: Latest (composite created_at|id cursor),
 * Popular (sort=popular → view_count order, offset pagination), Featured
 * (generic content_featured junction, single page). GET /content returns a
 * bare list envelope — no meta.total.
 * Ref: japaconnect-engineering/sections/14-content-type-system.md (List amendment)
 */
const PAGE_SIZE = 24;

export const load: PageServerLoad = async (event) => {
	// list.enabled is a feature flag (unified rollout — the legacy public
	// system_settings key is deprecated and no longer updated). Gate on the
	// flags map like /stories (story.enabled), not siteSettings.
	const { features } = await event.parent();
	if (features?.['list.enabled'] !== true) {
		throw error(404, { code: 'feature_disabled', message: 'Lists are not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;
	// Unknown tab values fall back to 'latest' — passing a raw ?tab= through
	// to the page would leave UrlTabs with no selected tab while the fetch
	// already served latest items.
	const rawTab = event.url.searchParams.get('tab') ?? 'latest';
	const tab = rawTab === 'popular' || rawTab === 'featured' ? rawTab : 'latest';
	const cursor = event.url.searchParams.get('cursor') ?? '';
	const offset = parseInt(event.url.searchParams.get('offset') ?? '0', 10) || 0;

	const params = new URLSearchParams();
	params.set('type', 'list');
	params.set('page_size', String(PAGE_SIZE));
	if (tab === 'featured') {
		params.set('featured', '1');
	} else if (tab === 'popular') {
		params.set('sort', 'popular');
		if (offset > 0) params.set('page_offset', String(offset));
	} else if (cursor) {
		params.set('cursor', cursor);
	}

	let items: GuideListItem[] = [];
	let loadError = false;
	try {
		const res = await apiGetEnvelope<GuideListItem[]>(ctx, `/content?${params.toString()}`);
		items = res.data ?? [];
		await attachAuthorUsernames(ctx, items);
	} catch {
		loadError = true;
	}

	// Latest uses a composite `created_at|id` cursor; popular uses
	// page_offset (view_count order has no cursor); featured is a
	// single-page view.
	const last = items[items.length - 1];
	const nextCursor =
		tab === 'popular'
			? items.length >= PAGE_SIZE
				? `offset:${offset + items.length}`
				: null
			: tab === 'latest' && items.length >= PAGE_SIZE && last
				? `${last.created_at}|${last.id}`
				: null;

	return { items, tab, authed, pageSize: PAGE_SIZE, nextCursor, loadError };
};
