import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { TrackSummary } from '$lib/types';

/**
 * Search results page — Phase 10 (ROADMAP-LEARN.md).
 * SSR: calls GET /learn/university/catalog?q=... with level filter.
 * No-results state fetches suggested tracks (popular, limit 4).
 *
 * Ref: Section 54 (search), ADR-060 SSR/CSR boundary.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const q = event.url.searchParams.get('q') ?? '';
	const level = event.url.searchParams.get('level') ?? '';

	// Build query string for the search request.
	const params = new URLSearchParams();
	if (q) params.set('q', q);
	if (level) params.set('level', level);
	params.set('limit', '24');
	const qs = params.toString() ? `?${params.toString()}` : '';

	let results: TrackSummary[] = [];
	let total = 0;
	try {
		const res = await apiList<TrackSummary>(ctx, `/learn/university/catalog${qs}`);
		results = res.items;
		total = res.meta?.total ?? res.items.length;
	} catch {
		results = [];
	}

	// No-results: fetch suggested tracks (popular, no query filter).
	let suggested: TrackSummary[] = [];
	if (results.length === 0) {
		try {
			const sugParams = new URLSearchParams();
			sugParams.set('sort', 'popular');
			sugParams.set('limit', '4');
			const res = await apiList<TrackSummary>(ctx, `/learn/university/catalog?${sugParams.toString()}`);
			suggested = res.items;
		} catch {
			suggested = [];
		}
	}

	return {
		results,
		total,
		suggested,
		query: q,
		levelFilter: level
	};
};
