import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { ContentArticle } from '$lib/types/content';

/**
 * Blog listing — GET /api/v1/blog (public, tenant-scoped).
 * Ref: japaconnect-engineering/sections/14-content-type-system.md
 * Rendered INSIDE the app shell with Ghost.org magazine card design.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const page = Number(event.url.searchParams.get('page') ?? '1');
	const pageSize = 12;
	const tab = event.url.searchParams.get('tab') ?? 'latest';

	try {
		const path = tab === 'featured'
			? `/blog/featured?page_size=${pageSize}&page_offset=${(page - 1) * pageSize}`
			: `/blog?page_size=${pageSize}&page_offset=${(page - 1) * pageSize}`;
		const { items, meta } = await apiList<ContentArticle>(ctx, path);
		return { items, meta, page, pageSize, tab, loadError: false };
	} catch {
		return { items: [] as ContentArticle[], meta: null, page, pageSize, tab, loadError: true };
	}
};
