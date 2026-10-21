import { env } from '$env/dynamic/public';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { ListMeta } from '$lib/types';
import type { ComparisonType, JCEComparisonSummary } from '$lib/types/jce';
import type { PageServerLoad } from './$types';

function parseListPageSize(): number {
	return parseInt(env.PUBLIC_JCE_COMPARISON_LIST_PAGE_SIZE ?? '12', 10) || 12;
}

function parseListMaxPageSize(): number {
	return parseInt(env.PUBLIC_JCE_COMPARISON_LIST_MAX_PAGE_SIZE ?? '100', 10) || 100;
}

function mapComparisonTypes(comparisons: JCEComparisonSummary[], types: ComparisonType[]): JCEComparisonSummary[] {
	const byID = new Map<string, ComparisonType>();
	for (const t of types) byID.set(t.id, t);
	return comparisons.map((c) => {
		const type = c.type_id ? byID.get(c.type_id) : undefined;
		if (type) {
			return { ...c, type_slug: type.slug, type_name: type.name };
		}
		return c;
	});
}

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const searchParams = event.url.searchParams;

	const defaultPageSize = parseListPageSize();
	const maxPageSize = parseListMaxPageSize();
	const page = Math.max(1, parseInt(searchParams.get('page') ?? '1', 10) || 1);
	const pageSize = Math.min(
		maxPageSize,
		Math.max(1, parseInt(searchParams.get('page_size') ?? String(defaultPageSize), 10) || defaultPageSize)
	);
	const sort = searchParams.get('sort') ?? '';
	const q = searchParams.get('q') ?? '';
	const filterType = searchParams.get('filter_type') ?? searchParams.get('type') ?? '';
	let filterDepth = searchParams.get('filter_depth') ?? searchParams.get('depth') ?? '';
	let filterStatus = searchParams.get('filter_status') ?? searchParams.get('status') ?? '';
	let filterVisibility = searchParams.get('filter_visibility') ?? searchParams.get('visibility') ?? '';
	const mine = searchParams.get('mine') === 'true';

	// Public /comparisons must only show published, public, detailed comparisons.
	if (!mine) {
		filterDepth = 'detailed';
		filterStatus = 'published';
		filterVisibility = 'public';
	}

	let types: ComparisonType[] = [];
	let loadError = '';
	let loadErrorParams: Record<string, string> = {};
	try {
		// page_size=100 — the default page (20) truncates the type list, which
		// silently drops badges and filter options for types beyond the cut.
		const res = await apiGet<ComparisonType[]>(ctx, '/jce/comparison-types?page_size=100');
		types = res ?? [];
	} catch (err) {
		types = [];
		if (err instanceof ApiError) {
			if (err.status === 404 && err.code === 'feature_disabled') {
				loadError = 'jce.comparisons.errors.feature_disabled';
			} else if (err.status === 404) {
				loadError = 'jce.comparisons.errors.types_not_found';
			} else {
				loadError = 'jce.comparisons.errors.load_types';
				loadErrorParams = { message: err.message };
			}
		} else if (err instanceof Error) {
			loadError = 'jce.comparisons.errors.api_unreachable';
			loadErrorParams = { message: err.message };
		} else {
			loadError = 'jce.comparisons.errors.unexpected';
		}
	}

	const params = new URLSearchParams();
	params.set('page', String(page));
	params.set('page_size', String(pageSize));
	if (sort) params.set('sort', sort);
	if (q) params.set('q', q);
	if (filterType) params.set('filter_type', filterType);
	if (filterDepth) params.set('filter_depth', filterDepth);
	if (filterStatus) params.set('filter_status', filterStatus);
	if (filterVisibility) params.set('filter_visibility', filterVisibility);
	if (mine) params.set('mine', 'true');

	let items: JCEComparisonSummary[] = [];
	let meta: ListMeta = { total: 0, page: 1, limit: pageSize };

	try {
		const res = await apiList<JCEComparisonSummary>(ctx, `/jce/comparisons?${params}`);
		items = mapComparisonTypes(res.items, types);
		meta = res.meta ?? meta;
	} catch (err) {
		if (!loadError) {
			if (err instanceof ApiError) {
				if (err.status === 403) {
					items = [];
				} else if (err.status === 404 && err.code === 'feature_disabled') {
					loadError = 'jce.comparisons.errors.feature_disabled';
				} else if (err.status === 404) {
					loadError = 'jce.comparisons.errors.not_found';
				} else {
					loadError = 'jce.comparisons.errors.load';
					loadErrorParams = { message: err.message };
				}
			} else if (err instanceof Error) {
				loadError = 'jce.comparisons.errors.api_unreachable';
				loadErrorParams = { message: err.message };
			} else {
				loadError = 'jce.comparisons.errors.unexpected';
			}
		}
	}

	return {
		user: event.locals.user,
		items,
		meta,
		types,
		defaultPageSize,
		maxPageSize,
		loadError,
		loadErrorParams,
		filters: {
			q,
			sort,
			filter_type: filterType,
			filter_depth: filterDepth,
			filter_status: filterStatus,
			filter_visibility: filterVisibility,
			page,
			page_size: pageSize,
			mine
		}
	};
};
