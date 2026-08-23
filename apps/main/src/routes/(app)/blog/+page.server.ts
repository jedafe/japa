import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { ContentArticle } from '$lib/types/content';
import type { BlogSeries } from '$lib/types';

/**
 * Blog listing — GET /api/v1/blog (public, tenant-scoped).
 * Ref: japaconnect-engineering/sections/14-content-type-system.md
 * Rendered INSIDE the app shell with Ghost.org magazine card design.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features?.['feature.blog']) {
		throw error(404, { code: 'feature_disabled', message: 'Blog is not available' });
	}

	const ctx = apiContext(event);
	const page = Number(event.url.searchParams.get('page') ?? '1');
	const pageSize = 12;
	const tab = event.url.searchParams.get('tab') ?? 'latest';

	try {
		if (tab === 'series') {
			const { items } = await apiList<BlogSeries>(ctx, '/blog/series');
			return { items: [] as ContentArticle[], meta: null, series: items, page, pageSize, tab, loadError: false };
		}
		const pageOffset = (page - 1) * pageSize;
		const blogRes =
			tab === 'featured'
				? await apiList<ContentArticle>(
						ctx,
						`/blog/featured?page_size=${pageSize}&page_offset=${pageOffset}`
					)
				: await apiList<ContentArticle>(
						ctx,
						`/blog?page_size=${pageSize}&page_offset=${pageOffset}`
					);
		const { items, meta } = blogRes;
		return { items, meta, series: [] as BlogSeries[], page, pageSize, tab, loadError: false };
	} catch {
		return { items: [] as ContentArticle[], meta: null, series: [] as BlogSeries[], page, pageSize, tab, loadError: true };
	}
};
