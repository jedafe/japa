import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { SearchResponse, SearchResult, ResultGroup } from '$lib/types/search';

/**
 * Search results — GET /api/v1/search?q=...&type=...&limit=20 (§54).
 * Public SSR: guests can search published content (Phase 10.01). The backend
 * /search route uses OptionalAuth so unauthenticated requests are served.
 *
 * §54 parameters passed through: q, type, limit, page, sort, content_type,
 * space_id, tag_slug, geo. The backend returns grouped results (ADR-017)
 * when no type filter is active, or a flat results list otherwise.
 * Ref: japaconnect-engineering/sections/13-search-meilisearch.md
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const q = event.url.searchParams.get('q') ?? '';
	const type = event.url.searchParams.get('type') ?? '';
	const limit = event.url.searchParams.get('limit') ?? '20';
	const page = event.url.searchParams.get('page') ?? '';
	const sort = event.url.searchParams.get('sort') ?? '';
	const contentType = event.url.searchParams.get('content_type') ?? '';
	const spaceId = event.url.searchParams.get('space_id') ?? '';
	const tagSlug = event.url.searchParams.get('tag_slug') ?? '';
	const geo = event.url.searchParams.get('geo') ?? '';
	const authed = !!event.locals.user;

	if (!q.trim()) {
		return { query: '', results: [] as SearchResult[], groups: [] as ResultGroup[], authed };
	}

	try {
		const params = new URLSearchParams();
		params.set('q', q);
		params.set('limit', limit);
		if (type) params.set('type', type);
		if (page) params.set('page', page);
		if (sort) params.set('sort', sort);
		if (contentType) params.set('content_type', contentType);
		if (spaceId) params.set('space_id', spaceId);
		if (tagSlug) params.set('tag_slug', tagSlug);
		if (geo) params.set('geo', geo);

		const path = `/search?${params.toString()}`;
		// Backend returns { data: { results, groups, total, took_ms } }.
		// apiGet unwraps the outer `data` envelope.
		const res = await apiGet<SearchResponse>(ctx, path);
		return {
			query: q,
			results: res?.results ?? [],
			groups: res?.groups ?? [],
			authed
		};
	} catch {
		return { query: q, results: [] as SearchResult[], groups: [] as ResultGroup[], authed, loadError: true };
	}
};
