import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { TrackSummary } from '$lib/types';

/**
 * Search results page — Phase 10 (ROADMAP-LEARN.md).
 * SSR: calls GET /learn/tracks?q=... with level filter, scoped to learning
 * tracks. On the Japa University host, uses /learn/university/catalog.
 * No-results state fetches suggested tracks (popular, limit 4).
 *
 * Ref: Section 54 (search), Section 36 (GET /learn/tracks), ADR-060 SSR/CSR boundary.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const isUniversity = event.locals.isUniversityHost;
	const q = event.url.searchParams.get('q') ?? '';
	const level = event.url.searchParams.get('level') ?? '';

	const catalogPath = isUniversity ? '/learn/university/catalog' : '/learn/tracks';

	// Build query string for the search request.
	const params = new URLSearchParams();
	if (q) params.set('q', q);
	if (level) params.set('level', level);
	params.set('limit', '24');
	const qs = params.toString() ? `?${params.toString()}` : '';

	let results: TrackSummary[] = [];
	let total = 0;
	try {
		const res = await apiList<TrackSummary>(ctx, `${catalogPath}${qs}`);
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
			const res = await apiList<TrackSummary>(ctx, `${catalogPath}?${sugParams.toString()}`);
			suggested = res.items;
		} catch {
			suggested = [];
		}
	}

	return {
		results,
		total,
		suggested,
		isUniversity,
		query: q,
		levelFilter: level
	};
};
