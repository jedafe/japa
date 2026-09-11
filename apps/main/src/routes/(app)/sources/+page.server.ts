import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiGet } from '$lib/server/api';
import type { Source, SourceCollection } from '$lib/types/sources';

const PAGE_SIZE = 24;
const SORT_OPTIONS = ['popular', 'recent', 'trending'];

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	const q = event.url.searchParams.get('q') ?? '';
	const category = event.url.searchParams.get('category') ?? '';
	const sourceKind = event.url.searchParams.get('source_kind') ?? '';
	const rawSort = event.url.searchParams.get('sort') ?? 'popular';
	const sort = SORT_OPTIONS.includes(rawSort) ? rawSort : 'popular';

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

	const { items: sources, meta } = await apiList<Source>(ctx, `/sources?${params.toString()}`);

	const collections = await apiGet<SourceCollection[]>(ctx, '/sources/collections');

	return {
		sources,
		meta,
		collections,
		q,
		category,
		sourceKind,
		sort,
		page,
		pageSize
	};
};
