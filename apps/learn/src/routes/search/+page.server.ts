import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiGet } from '$lib/server/api';
import type { TrackSummary, CatalogOption, TrackCategory, CatalogOptionsResponse } from '$lib/types';

const DEFAULT_LIMIT = 24;

/**
 * Search results page — Phase 10 (ROADMAP-LEARN.md).
 * SSR: calls GET /learn/university/catalog?q=... with level, category,
 * language, sort and pagination.
 * No-results state fetches suggested tracks (popular, limit 4).
 *
 * Ref: Section 54 (search), ADR-060 SSR/CSR boundary.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const q = event.url.searchParams.get('q') ?? '';
	const level = event.url.searchParams.get('level') ?? '';
	const category = event.url.searchParams.get('category') ?? '';
	const language = event.url.searchParams.get('language') ?? '';
	const sort = event.url.searchParams.get('sort') ?? '';
	const isFree = event.url.searchParams.get('is_free') === 'true';
	const offset = Math.max(parseInt(event.url.searchParams.get('offset') ?? '0', 10) || 0, 0);
	const limit = Math.max(parseInt(event.url.searchParams.get('limit') ?? String(DEFAULT_LIMIT), 10) || DEFAULT_LIMIT, 1);

	// Build query string for the search request.
	const params = new URLSearchParams();
	if (q) params.set('q', q);
	if (level) params.set('level', level);
	if (category) params.set('category', category);
	if (language) params.set('language', language);
	if (sort) params.set('sort', sort);
	if (isFree) params.set('is_free', 'true');
	params.set('limit', String(limit));
	if (offset > 0) params.set('offset', String(offset));
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

	// Rule 16: option lists must come from the backend, not hardcoded constants.
	let difficultyLevels: CatalogOption[] = [];
	let categories: TrackCategory[] = [];
	let sortOptions: CatalogOption[] = [];
	let pageSizes: CatalogOption[] = [];
	let languages: string[] = [];
	try {
		const [opts, cats] = await Promise.all([
			apiGet<CatalogOptionsResponse>(ctx, '/learn/university/catalog/options'),
			apiGet<TrackCategory[]>(ctx, '/learn/university/categories')
		]);
		difficultyLevels = opts.difficulty_levels ?? [];
		sortOptions = opts.sort_options ?? [];
		pageSizes = opts.page_sizes ?? [];
		languages = opts.languages ?? [];
		categories = cats ?? [];
	} catch {
		difficultyLevels = [];
		categories = [];
	}

	return {
		results,
		total,
		suggested,
		query: q,
		levelFilter: level,
		categoryFilter: category,
		languageFilter: language,
		sortFilter: sort,
		isFreeFilter: isFree,
		offset,
		limit,
		difficultyLevels,
		categories,
		sortOptions,
		pageSizes,
		languages
	};
};
