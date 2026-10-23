import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { attachAuthorUsernames } from '$lib/server/users';
import type { ContentArticle } from '$lib/types/content';
import type { BlogSeries } from '$lib/types';

/**
 * Blog listing — GET /api/v1/blog (public, platform-scoped).
 * Ref: japaconnect-engineering/sections/14-content-type-system.md
 * Supports ?tab=latest|top|featured|series, ?sort=latest|top, ?tag=slug,
 * ?space_id=uuid, ?author_id=uuid and ?series_id=uuid per §24.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features?.['feature.blog']) {
		throw error(404, { code: 'feature_disabled', message: 'Blog is not available' });
	}

	const ctx = apiContext(event);
	// Clamp junk page values (?page=abc / 0 / -3) to page 1 instead of
	// forwarding NaN/negative offsets that 400 the backend request.
	const rawPage = Number(event.url.searchParams.get('page') ?? '1');
	const page = Number.isFinite(rawPage) && rawPage >= 1 ? Math.floor(rawPage) : 1;
	const pageSize = 12;
	const sortParam = event.url.searchParams.get('sort') ?? '';
	const tabParam = event.url.searchParams.get('tab');
	// tab and sort are one control in the UI: tab=top is shorthand for
	// sort=top on the main list. An explicit ?sort=top without a tab selects
	// the Top tab; an explicit tab always wins, so a contradictory
	// ?tab=latest&sort=top can't show top-sorted rows under "Latest".
	const tab = tabParam ?? (sortParam === 'top' ? 'top' : 'latest');
	const sort = tab === 'top' ? 'top' : 'latest';
	const tag = event.url.searchParams.get('tag') ?? '';
	const spaceId = event.url.searchParams.get('space_id') ?? '';
	const authorId = event.url.searchParams.get('author_id') ?? '';
	const seriesId = event.url.searchParams.get('series_id') ?? '';

	try {
		if (tab === 'series') {
			const q = event.url.searchParams.get('q') ?? '';
			const pageOffset = (page - 1) * pageSize;
			const params = new URLSearchParams();
			if (q) params.set('q', q);
			// Scope the directory to series containing published blog posts —
			// series are cross-type (§14), so unfiltered would list story/guide
			// series on the Blog tab. Same filter /guides and /stories apply.
			params.set('content_type', 'blog_post');
			params.set('page_size', String(pageSize));
			params.set('page_offset', String(pageOffset));
			const { items, meta } = await apiList<BlogSeries>(ctx, `/series?${params.toString()}`);
			return { items: [] as ContentArticle[], meta, series: items, page, pageSize, tab, tag, spaceId, authorId, seriesId, sort, q, loadError: false };
		}
		const pageOffset = (page - 1) * pageSize;
		const params = new URLSearchParams();
		params.set('page_size', String(pageSize));
		params.set('page_offset', String(pageOffset));
		params.set('sort', sort);

		let blogRes;
		if (tab === 'featured') {
			// /blog/featured honors only page_size/page_offset — filter
			// params are silently ignored, so don't forward them (they'd
			// imply a filtered result the endpoint never applied).
			blogRes = await apiList<ContentArticle>(ctx, `/blog/featured?${params.toString()}`);
		} else {
			if (tag) params.set('tag', tag);
			if (spaceId) params.set('space_id', spaceId);
			if (authorId) params.set('author_id', authorId);
			if (seriesId) params.set('series_id', seriesId);
			blogRes = await apiList<ContentArticle>(ctx, `/blog?${params.toString()}`);
		}
		const { items, meta } = blogRes;
		await attachAuthorUsernames(ctx, items);
		return { items, meta, series: [] as BlogSeries[], page, pageSize, tab, tag, spaceId, authorId, seriesId, sort, q: '', loadError: false };
	} catch {
		return { items: [] as ContentArticle[], meta: null, series: [] as BlogSeries[], page, pageSize, tab, tag, spaceId, authorId, seriesId, sort, q: '', loadError: true };
	}
};
