import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiGetOrNull } from '$lib/server/api';
import type { TrackSummary, ReviewAggregate } from '$lib/types';

/**
 * Course Catalog — SSR public content only (ADR-060 SSR/CSR boundary).
 * Fetches the marketplace catalog from /learn/university/catalog.
 *
 * Enrollment state is NEVER fetched here — it is CSR-only via /me/enrollments.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const q = event.url.searchParams.get('q') ?? '';
	const level = event.url.searchParams.get('level') ?? '';

	// Build query string for filtering.
	const params = new URLSearchParams();
	if (q) params.set('q', q);
	if (level) params.set('level', level);
	const qs = params.toString() ? `?${params.toString()}` : '';

	let tracks: TrackSummary[] = [];
	try {
		const res = await apiList<TrackSummary>(ctx, `/learn/university/catalog${qs}`);
		tracks = res.items;
	} catch {
		tracks = [];
	}

	return {
		tracks,
		query: q,
		levelFilter: level
	};
};
