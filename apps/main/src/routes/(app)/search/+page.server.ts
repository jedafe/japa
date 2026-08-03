import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { SearchResult } from '$lib/types/search';

/**
 * Search results — GET /api/v1/search?q=...&type=...&limit=20 (auth required).
 * Ref: japaconnect-engineering/sections/13-search-meilisearch.md
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const q = event.url.searchParams.get('q') ?? '';
	const type = event.url.searchParams.get('type') ?? '';
	const limit = Number(event.url.searchParams.get('limit') ?? '20');

	if (!event.locals.user) {
		throw redirect(302, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	if (!q.trim()) {
		return { query: '', results: [] as SearchResult[], authed: true };
	}

	try {
		const path = `/search?q=${encodeURIComponent(q)}&limit=${limit}${type ? `&type=${type}` : ''}`;
		// Backend returns { data: { results: SearchResult[] } }. apiGet
		// unwraps the outer `data` envelope, leaving { results: [...] }.
		const res = await apiGet<{ results: SearchResult[] }>(ctx, path);
		return { query: q, results: res?.results ?? [], authed: true };
	} catch {
		return { query: q, results: [] as SearchResult[], authed: true, loadError: true };
	}
};
