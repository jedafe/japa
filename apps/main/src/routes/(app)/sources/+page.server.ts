import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiGet } from '$lib/server/api';
import type { Source, SourceCollection, SourceMeta } from '$lib/types/sources';

const PAGE_SIZE = 24;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	const q = event.url.searchParams.get('q') ?? '';
	const category = event.url.searchParams.get('category') ?? '';
	const sourceKind = event.url.searchParams.get('source_kind') ?? '';

	const sourceMeta = await apiGet<SourceMeta>(ctx, '/sources/meta').catch(
		(): SourceMeta => ({
			categories: [],
			source_types: [],
			sorts: [],
			item_sorts: []
		})
	);

	const validSorts = new Set((sourceMeta.sorts ?? []).map((s) => s.value));
	const defaultSort = sourceMeta.sorts?.[0]?.value ?? 'popular';
	const rawSort = event.url.searchParams.get('sort') ?? defaultSort;
	const sort = validSorts.has(rawSort) ? rawSort : defaultSort;

	const pageSize = PAGE_SIZE;
	// Normalize ?page= — parseInt yields NaN for junk input and admits
	// non-positive values, which previously rendered "Page NaN of N".
	const rawPage = Number(event.url.searchParams.get('page') ?? '1');
	const page = Number.isInteger(rawPage) && rawPage >= 1 ? rawPage : 1;
	const pageOffset = (page - 1) * pageSize;

	const params = new URLSearchParams();
	if (q) params.set('q', q);
	if (category) params.set('category', category);
	if (sourceKind) params.set('source_kind', sourceKind);
	params.set('sort', sort);
	params.set('page_size', String(pageSize));
	params.set('page_offset', String(pageOffset));

	// Curated collections only render on the default directory view — the
	// first page with no search or filters active (sources-directory v2
	// plan, step 4). A non-default sort is excluded too: the seed collections
	// ("Recently added", "Trending") would just duplicate the sorted list.
	const isDirectoryHome = !q && !category && !sourceKind && page === 1 && sort === defaultSort;

	const [listResult, collections] = await Promise.all([
		apiList<Source>(ctx, `/sources?${params.toString()}`),
		isDirectoryHome
			? apiGet<SourceCollection[]>(ctx, '/sources/collections').catch(() => [] as SourceCollection[])
			: Promise.resolve([] as SourceCollection[])
	]);
	const { items: sources, meta } = listResult;

	// Canonicalize out-of-range pages (e.g. ?page=99 with one page of
	// results) — otherwise the UI renders "Page 99 of 1" over an empty
	// list. Redirect to the last valid page (or drop the param when the
	// result set fits on page 1).
	const lastPage = Math.max(1, Math.ceil((meta?.total ?? 0) / pageSize));
	if (page > lastPage) {
		const url = new URL(event.url);
		if (lastPage > 1) url.searchParams.set('page', String(lastPage));
		else url.searchParams.delete('page');
		redirect(303, `${url.pathname}${url.search}`);
	}

	return {
		sources,
		meta,
		sourceMeta,
		collections,
		q,
		category,
		sourceKind,
		sort,
		page,
		pageSize
	};
};
