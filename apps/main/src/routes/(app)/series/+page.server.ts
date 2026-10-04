import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { BlogSeries } from '$lib/types';

/**
 * Series directory at /series — public, SSR, indexable. Series are
 * cross-type (§14): any content_type may belong, so the directory lives at
 * the top level, not under /blog. Supports:
 *   ?tab=all|mine        — "mine" scopes to the signed-in author's series
 *   ?q=                  — title/slug search
 *   ?content_type=       — series containing a published item of that type
 *   ?sort=latest|oldest|title
 *   ?page=               — offset pagination
 * Filter option lists come from GET /content-types (Rule 16).
 *
 * Ref: japaconnect-engineering/sections/14-content-type-system.md
 *      japaconnect-engineering/sections/24-blog-extension.md
 */
const PAGE_SIZE = 12;

interface ContentTypeOption {
	label: string;
	value: string;
}

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features?.['feature.blog']) {
		throw error(404, { code: 'feature_disabled', message: 'Series is not available' });
	}

	const ctx = apiContext(event);
	const user = event.locals.user;
	const tab = event.url.searchParams.get('tab') === 'mine' ? 'mine' : 'all';
	const q = (event.url.searchParams.get('q') ?? '').trim();
	const contentType = (event.url.searchParams.get('content_type') ?? '').trim();
	const sortParam = event.url.searchParams.get('sort') ?? 'latest';
	const sort = sortParam === 'oldest' || sortParam === 'title' ? sortParam : 'latest';
	const pageRaw = Number(event.url.searchParams.get('page') ?? '1');
	const page = Number.isFinite(pageRaw) && pageRaw > 0 ? Math.floor(pageRaw) : 1;
	const pageOffset = (page - 1) * PAGE_SIZE;

	const params = new URLSearchParams();
	if (q) params.set('q', q);
	if (contentType) params.set('content_type', contentType);
	if (sort !== 'latest') params.set('sort', sort);
	params.set('page_size', String(PAGE_SIZE));
	params.set('page_offset', String(pageOffset));
	if (tab === 'mine' && user) params.set('author_id', user.id);

	// Guests on the Mine tab skip the list fetch entirely — the page renders
	// a sign-in empty state instead of an unscoped directory.
	const mineNeedsAuth = tab === 'mine' && !user;

	const [seriesResult, typesResult] = await Promise.all([
		mineNeedsAuth
			? Promise.resolve<{ items: BlogSeries[]; meta?: { total?: number } } | null>({
					items: [],
					meta: { total: 0 }
				})
			: apiList<BlogSeries>(ctx, `/series?${params.toString()}`).catch(() => null),
		apiList<ContentTypeOption>(ctx, '/content-types').catch(() => null)
	]);

	return {
		series: seriesResult?.items ?? [],
		meta: seriesResult?.meta ?? null,
		contentTypes: typesResult?.items ?? [],
		tab,
		q,
		contentType,
		sort,
		page,
		pageSize: PAGE_SIZE,
		authed: !!user,
		loadError: !mineNeedsAuth && seriesResult === null
	};
};
