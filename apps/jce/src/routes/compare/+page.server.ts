import { redirect } from '@sveltejs/kit';
import { apiContext, apiGet, apiList, apiPost, ApiError, API_V1 } from '$lib/server/api';
import type { ComparisonType, ComparisonTypeConfig, JCEComparisonSummary, ResolvedItem } from '$lib/types/jce';
import type { ListMeta } from '$lib/types';
import type { PageServerLoad } from './$types';

function parseConfig(raw: unknown): ComparisonTypeConfig | null {
	if (!raw) return null;
	if (typeof raw === 'object') return raw as ComparisonTypeConfig;
	if (typeof raw !== 'string') return null;
	const decoded = Buffer.isBuffer(raw) ? raw : Buffer.from(raw, 'base64').toString('utf-8');
	try {
		return JSON.parse(decoded) as ComparisonTypeConfig;
	} catch {
		return null;
	}
}

function enrichTypes(types: ComparisonType[]): ComparisonType[] {
	return types.map((t) => ({
		...t,
		config: parseConfig(t.config as unknown) ?? undefined
	}));
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

	let types: ComparisonType[] = [];
	let loadError = '';
	try {
		const res = await apiGet<ComparisonType[]>(ctx, '/jce/comparison-types');
		types = enrichTypes(res ?? []);
	} catch (err) {
		if (err instanceof ApiError) {
			if (err.status === 403) {
				types = [];
			} else if (err.status === 404 && err.code === 'feature_disabled') {
				loadError = 'jce.compare.errors.feature_disabled';
			} else if (err.status === 404) {
				loadError = 'jce.compare.errors.types_not_found';
			} else {
				loadError = `Failed to load comparison types: ${err.message}`;
			}
		} else if (err instanceof Error) {
			loadError = `Cannot reach the backend API: ${err.message}. Make sure the Go server is running.`;
		} else {
			loadError = 'An unexpected error occurred while loading comparison types.';
		}
	}

	let selectedType: ComparisonType | undefined;
	const typeSlug = searchParams.get('type') ?? '';
	if (typeSlug) {
		selectedType = types.find((t) => t.slug === typeSlug);
	}
	if (!selectedType && types.length > 0) {
		// Default to the type with the most existing comparisons (heuristic for a populated initial view).
		selectedType = [...types].sort((a, b) => (b.item_count ?? 0) - (a.item_count ?? 0))[0];
	}

	const itemRefTypes = (selectedType?.config?.item_ref_types as string[]) ?? ['space'];
	const allowNestedComparisons = Boolean(selectedType?.config?.allow_nested_comparisons);
	const allowedNestedTypes = (selectedType?.config?.allowed_nested_types as string[]) ?? [];

	let source = searchParams.get('source') ?? '';
	if (!itemRefTypes.includes(source)) {
		source = selectedType?.config?.default_item_ref_type as string ?? itemRefTypes[0] ?? 'space';
	}

	let items: ResolvedItem[] = [];
	if (selectedType) {
		try {
			let endpoint = `/jce/entities/${source}?q=`;
			if (source === 'comparison' && allowNestedComparisons && allowedNestedTypes.length > 0) {
				endpoint += `&nested_types=${encodeURIComponent(allowedNestedTypes.join(','))}`;
			}
			const res = await apiGet<ResolvedItem[]>(ctx, endpoint);
			items = res ?? [];
		} catch (err) {
			if (err instanceof ApiError && err.status !== 404) {
				loadError = loadError || `Failed to load ${selectedType.name} items: ${err.message}`;
			}
		}
	}

	let preselectedItems: ResolvedItem[] = [];
	const preselect = searchParams.get('preselect') ?? '';
	if (preselect && selectedType) {
		const ids = preselect.split(',').filter(Boolean);
		for (const id of ids.slice(0, 4)) {
			try {
				const item = await apiGet<ResolvedItem>(ctx, `/jce/entities/${source}/${id}`);
				if (item) preselectedItems.push(item);
			} catch {
				// Ignore resolution failures for malformed preselect values.
			}
		}
	}

	const compareToken = searchParams.get('compare_token') ?? '';

	// Guest selections that survive login are consumed server-side and the
	// user is redirected to the new quick comparison.
	let fromTokenSlug = '';
	if (compareToken && event.locals.user && selectedType) {
		try {
			const cmp = await apiPost<{ slug: string }>(ctx, '/jce/comparisons/from-token', {
				token: compareToken,
				type_id: selectedType.id
			});
			if (cmp?.slug) {
				fromTokenSlug = cmp.slug;
			}
		} catch (err) {
			if (err instanceof ApiError) {
				loadError = loadError || `Could not restore your selection: ${err.message}`;
			} else if (err instanceof Error) {
				loadError = loadError || err.message;
			}
		}
	}
	if (fromTokenSlug) {
		redirect(302, `/compare/${fromTokenSlug}`);
	}

	const mine = searchParams.get('mine') === 'true';

	let myComparisons: JCEComparisonSummary[] = [];
	let myMeta: ListMeta = { total: 0, page: 1, limit: 12 };
	if (event.locals.user) {
		try {
			const res = await apiList<JCEComparisonSummary>(ctx, '/jce/comparisons?mine=true&filter_depth=quick&page_size=12&sort=created_at');
			myComparisons = mapComparisonTypes(res.items, types);
			myMeta = res.meta ?? myMeta;
		} catch (err) {
			if (err instanceof ApiError && err.status !== 404) {
				loadError = loadError || `Failed to load your comparisons: ${err.message}`;
			}
		}
	}

	return {
		types,
		selectedType: selectedType ?? null,
		source,
		itemRefTypes,
		allowNestedComparisons,
		allowedNestedTypes,
		items,
		preselectedItems,
		loadError,
		user: event.locals.user,
		mine,
		myComparisons,
		myMeta,
		apiBase: API_V1,
		mainOrigin: (event.locals.siteSettings?.['jce.main_origin'] as string) ?? 'http://localhost:3000'
	};
};
