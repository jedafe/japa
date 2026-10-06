import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiGet } from '$lib/server/api';
import type { TrackSummary, CatalogOption, TrackCategory, CatalogOptionsResponse } from '$lib/types';

const DEFAULT_LIMIT = 24;

/**
 * Course Catalog — SSR public content only (ADR-060 SSR/CSR boundary).
 * Fetches the marketplace catalog from /academy/catalog.
 *
 * Enrollment state is NEVER fetched here — it is CSR-only via /me/enrollments.
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

	// Build query string for filtering.
	const params = new URLSearchParams();
	if (q) params.set('q', q);
	if (level) params.set('level', level);
	if (category) params.set('category', category);
	if (language) params.set('language', language);
	if (sort) params.set('sort', sort);
	if (isFree) params.set('is_free', 'true');
	params.set('limit', String(limit));
	if (offset > 0) params.set('offset', String(offset));
	const qs = `?${params.toString()}`;

	let tracks: TrackSummary[] = [];
	let total = 0;
	try {
		const res = await apiList<TrackSummary>(ctx, `/academy/catalog${qs}`);
		tracks = res.items;
		total = res.meta?.total ?? res.items.length;
	} catch {
		tracks = [];
		total = 0;
	}

	// Rule 16: option lists must come from the backend, not hardcoded constants.
	let difficultyLevels: CatalogOption[] = [];
	let categories: TrackCategory[] = [];
	let sortOptions: CatalogOption[] = [];
	let pageSizes: CatalogOption[] = [];
	let languages: string[] = [];
	try {
		const [opts, cats] = await Promise.all([
			apiGet<CatalogOptionsResponse>(ctx, '/academy/catalog/options'),
			apiGet<TrackCategory[]>(ctx, '/academy/categories')
		]);
		difficultyLevels = opts.difficulty_levels ?? [];
		sortOptions = opts.sort_options ?? [];
		pageSizes = opts.page_sizes ?? [];
		languages = opts.languages ?? [];
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
