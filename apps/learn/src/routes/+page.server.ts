import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiGet } from '$lib/server/api';
import type { TrackSummary, CatalogOption, TrackCategory } from '$lib/types';

const DEFAULT_LIMIT = 20;

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
	const category = event.url.searchParams.get('category') ?? '';
	const isFree = event.url.searchParams.get('is_free') === 'true';
	const offset = Math.max(parseInt(event.url.searchParams.get('offset') ?? '0', 10) || 0, 0);

	// Build query string for filtering.
	const params = new URLSearchParams();
	if (q) params.set('q', q);
	if (level) params.set('level', level);
	if (category) params.set('category', category);
	if (isFree) params.set('is_free', 'true');
	params.set('limit', String(DEFAULT_LIMIT));
	if (offset > 0) params.set('offset', String(offset));
	const qs = `?${params.toString()}`;

	let tracks: TrackSummary[] = [];
	let total = 0;
	try {
		const res = await apiList<TrackSummary>(ctx, `/learn/university/catalog${qs}`);
		tracks = res.items;
		total = res.meta?.total ?? res.items.length;
	} catch {
		tracks = [];
		total = 0;
	}

	// Rule 16: option lists must come from the backend, not hardcoded constants.
	let difficultyLevels: CatalogOption[] = [];
	let categories: TrackCategory[] = [];
	try {
		const [opts, cats] = await Promise.all([
			apiGet<{ difficulty_levels: CatalogOption[] }>(ctx, '/learn/university/catalog/options'),
			apiGet<TrackCategory[]>(ctx, '/learn/university/categories')
		]);
		difficultyLevels = opts.difficulty_levels ?? [];
		categories = cats ?? [];
	} catch {
		difficultyLevels = difficultyLevels.length ? difficultyLevels : [];
		categories = categories.length ? categories : [];
	}

	return {
		tracks,
		total,
		query: q,
		levelFilter: level,
		categoryFilter: category,
		isFreeFilter: isFree,
		offset,
		limit: DEFAULT_LIMIT,
		difficultyLevels,
		categories
	};
};
