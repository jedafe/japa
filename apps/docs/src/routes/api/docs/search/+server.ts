import { json, error } from '@sveltejs/kit';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { RequestHandler } from './$types';
import type { DocSearchResult } from '$lib/types';

/**
 * Same-origin search proxy for the docs command palette.
 *
 * GET /api/docs/search?q=&audience=&limit=
 *
 * The backend /api/v1/docs/search is public but audience-filtered based on the
 * caller's Bearer token. Client JavaScript cannot read the HttpOnly jc_token
 * cookie, so this server endpoint forwards the request with the session token
 * and returns the raw backend envelope.
 */
export const GET: RequestHandler = async (event) => {
	const q = event.url.searchParams.get('q')?.trim();
	if (!q) {
		throw error(400, { code: 'missing_query', message: 'q is required' });
	}

	const params = new URLSearchParams({ q });
	const audience = event.url.searchParams.get('audience');
	if (audience) params.set('audience', audience);

	const limit = event.url.searchParams.get('limit') ?? '10';
	if (limit) params.set('limit', limit);

	try {
		const results = await apiGet<DocSearchResult[]>(apiContext(event), `/docs/search?${params}`);
		return json({ data: results });
	} catch (err) {
		if (err instanceof ApiError) {
			throw error(err.status, { code: err.code, message: err.message });
		}
		throw error(500, { code: 'internal_error', message: 'Search request failed' });
	}
};
