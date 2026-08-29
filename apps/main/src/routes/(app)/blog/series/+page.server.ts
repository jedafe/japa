import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { BlogSeries } from '$lib/types';

/**
 * Blog series listing at /blog/series — public, SSR, indexable.
 * Supports ?q= and ?page= for search and pagination (Rule 5).
 *
 * Ref: japaconnect-engineering/sections/24-blog-extension.md
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features?.['feature.blog']) {
		throw error(404, { code: 'feature_disabled', message: 'Blog is not available' });
	}

	const ctx = apiContext(event);
	const q = event.url.searchParams.get('q') ?? '';
	const page = Math.max(1, Number(event.url.searchParams.get('page') ?? '1'));
	const pageSize = 12;
	const pageOffset = (page - 1) * pageSize;

	const params = new URLSearchParams();
	if (q) params.set('q', q);
	params.set('page_size', String(pageSize));
	params.set('page_offset', String(pageOffset));

	try {
		const { items, meta } = await apiList<BlogSeries>(ctx, `/blog/series?${params.toString()}`);
		return { series: items, meta, page, pageSize, q, loadError: false };
	} catch {
		return { series: [] as BlogSeries[], meta: null, page, pageSize, q, loadError: true };
	}
};
