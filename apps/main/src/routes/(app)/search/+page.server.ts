import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { SearchResult } from '$lib/types/search';

/**
 * Search results — GET /api/v1/search?q=...&type=...&limit=20.
 * Public SSR: guests can search published content (Phase 10.01). The backend
 * /search route uses OptionalAuth so unauthenticated requests are served.
 * Ref: japaconnect-engineering/sections/13-search-meilisearch.md
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const q = event.url.searchParams.get('q') ?? '';
	const type = event.url.searchParams.get('type') ?? '';
	const limit = Number(event.url.searchParams.get('limit') ?? '20');
	const authed = !!event.locals.user;

	if (!q.trim()) {
		return { query: '', results: [] as SearchResult[], authed };
	}

	try {
		const path = `/search?q=${encodeURIComponent(q)}&limit=${limit}${type ? `&type=${type}` : ''}`;
		// Backend returns { data: { results: SearchResult[] } }. apiGet
		// unwraps the outer `data` envelope, leaving { results: [...] }.
		const res = await apiGet<{ results: SearchResult[] }>(ctx, path);
		return { query: q, results: res?.results ?? [], authed };
	} catch {
		return { query: q, results: [] as SearchResult[], authed, loadError: true };
	}
};
