import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiGet } from '$lib/server/api';
import type { Source, SourceCollection, SourceMeta } from '$lib/types/sources';

const PAGE_SIZE = 24;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	const q = event.url.searchParams.get('q') ?? '';
	const category = event.url.searchParams.get('category') ?? '';
	const sourceKind = event.url.searchParams.get('source_kind') ?? '';

	const sourceMeta = await apiGet<SourceMeta>(ctx, '/sources/meta').catch(() => ({
		categories: [],
		source_types: [],
		sorts: [],
		item_sorts: []
	}));

	const validSorts = new Set((sourceMeta.sorts ?? []).map((s) => s.value));
	const rawSort = event.url.searchParams.get('sort') ?? 'popular';
	const sort = validSorts.has(rawSort) ? rawSort : (sourceMeta.sorts?.[0]?.value ?? 'popular');

	const pageSize = PAGE_SIZE;
	const page = parseInt(event.url.searchParams.get('page') ?? '1', 10);
	const pageOffset = Math.max(0, page - 1) * pageSize;

	const params = new URLSearchParams();
	if (q) params.set('q', q);
	if (category) params.set('category', category);
	if (sourceKind) params.set('source_kind', sourceKind);
	params.set('sort', sort);
	params.set('page_size', String(pageSize));
	params.set('page_offset', String(pageOffset));

	const [{ items: sources, meta }, collections] = await Promise.all([
		apiList<Source>(ctx, `/sources?${params.toString()}`),
		apiGet<SourceCollection[]>(ctx, '/sources/collections')
	]);

	return {
		sources,
		meta,
		collections,
		sourceMeta,
		q,
		category,
		sourceKind,
		sort,
		page,
		pageSize
	};
};
