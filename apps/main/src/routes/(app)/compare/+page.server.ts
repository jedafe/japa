import { redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/public';
import { apiContext, apiGet, apiList, apiPost, ApiError, API_BASE } from '$lib/server/api';
import type { ComparisonProfile, ComparisonType, ComparisonTypeConfig, JCEComparisonSummary, ResolvedItem } from '$lib/types/jce';
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

function parseDefaultItemRefTypes(): string[] {
	const raw = env.PUBLIC_JCE_DEFAULT_ITEM_REF_TYPES;
	if (raw) {
		try {
			const parsed = JSON.parse(raw);
			if (Array.isArray(parsed) && parsed.every((v) => typeof v === 'string')) return parsed;
		} catch {
			// fall through
		}
	}
	return ['space'];
}

function parseDefaultPageSize(): number {
	return parseInt(env.PUBLIC_JCE_DEFAULT_PAGE_SIZE ?? '24', 10) || 24;
}

function parseDefaultPageSizeOptions(): number[] {
	const raw = env.PUBLIC_JCE_DEFAULT_PAGE_SIZE_OPTIONS;
	if (raw) {
		try {
			const parsed = JSON.parse(raw);
			if (Array.isArray(parsed) && parsed.every((v) => typeof v === 'number')) return parsed;
		} catch {
			// fall through
		}
	}
	return [12, 24, 48];
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
	let loadErrorParams: Record<string, string> = {};
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
				loadError = 'jce.compare.errors.load_types';
				loadErrorParams = { message: err.message };
			}
		} else if (err instanceof Error) {
			loadError = 'jce.compare.errors.api_unreachable';
			loadErrorParams = { message: err.message };
		} else {
			loadError = 'jce.compare.errors.unexpected';
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

	const configuredItemRefTypes = selectedType?.config?.item_ref_types as string[] | undefined;
	const defaultItemRefTypes = parseDefaultItemRefTypes();
	const itemRefTypes = configuredItemRefTypes ??
		(selectedType?.config?.default_item_ref_type
			? [String(selectedType.config.default_item_ref_type)]
			: defaultItemRefTypes);
	const allowNestedComparisons = Boolean(selectedType?.config?.allow_nested_comparisons);
	const allowedNestedTypes = (selectedType?.config?.allowed_nested_types as string[]) ?? [];

	const defaultPageSize = parseDefaultPageSize();
	const defaultPageSizeOptions = parseDefaultPageSizeOptions();
	const configuredPageSizeOptions = (selectedType?.config?.page_size_options as number[]) ?? [];
	const pageSizeOptions = configuredPageSizeOptions.length > 0 ? configuredPageSizeOptions : defaultPageSizeOptions;

	let source = searchParams.get('source') ?? '';
	if (!itemRefTypes.includes(source)) {
		source = selectedType?.config?.default_item_ref_type as string ?? itemRefTypes[0] ?? 'space';
	}

	const q = searchParams.get('q') ?? '';
	const page = Math.max(1, parseInt(searchParams.get('page') ?? '1', 10) || 1);
	const requestedPageSize = parseInt(searchParams.get('page_size') ?? '', 10) || 0;
	const pageSize = pageSizeOptions.includes(requestedPageSize)
		? requestedPageSize
		: (pageSizeOptions.includes(defaultPageSize) ? defaultPageSize : (pageSizeOptions[0] ?? defaultPageSize));
	// Fetch one extra item so the client can infer whether another page exists without a total count.
	const fetchLimit = pageSize + 1;
	const fetchOffset = (page - 1) * pageSize;
	const nestedType = searchParams.get('nested_type') ?? '';

	let scopeChain = (selectedType?.config?.scope_chain as string[]) ?? [];
	// Course search must be scoped to a university. Older type configs that do
	// not include a scope_chain still need this selector so users can choose one.
	if (source === 'course' && !scopeChain.includes('university')) {
		scopeChain = [...scopeChain, 'university'];
	}
	const scopeParamMapping = (selectedType?.config?.scope_param_mapping as Record<string, string> | undefined) ?? {};

	function defaultScopeParamKey(step: string): string {
		// Region/subregion are string filters; everything else uses the
		// conventional `{scope}_id` query parameter expected by the resolver.
		if (step === 'region' || step === 'subregion') return step;
		return `${step}_id`;
	}

	function scopeParamKey(step: string): string {
		return scopeParamMapping[step] ?? defaultScopeParamKey(step);
	}

	const scopeParams: Record<string, string> = {};
	for (const step of scopeChain) {
		const key = scopeParamKey(step);
		const v = searchParams.get(key);
		if (v) scopeParams[step] = v;
	}

	let items: ResolvedItem[] = [];
	let itemsTotal: number | null = null;
	if (selectedType) {
		// Course search requires a university scope. Don't call the resolver before
		// the user has selected one; the scope dropdown is the prompt.
		if (source === 'course' && !scopeParams['university']) {
			items = [];
		} else {
			try {
				const params = new URLSearchParams({ q });
				params.set('limit', String(fetchLimit));
				params.set('offset', String(fetchOffset));
				if (source === 'comparison' && allowNestedComparisons && allowedNestedTypes.length > 0) {
					params.set('nested_types', allowedNestedTypes.join(','));
				}
				if (source === 'comparison' && nestedType) {
					params.set('nested_type', nestedType);
				}
				for (const [step, value] of Object.entries(scopeParams)) {
					const key = scopeParamKey(step);
					params.set(key, value);
				}
				const contentType = selectedType?.config?.content_type as string | undefined;
				if (contentType) {
					params.set('content_type', contentType);
				}
				const res = await apiList<ResolvedItem>(ctx, `/jce/entities/${source}?${params.toString()}`);
				items = res.items ?? [];
				itemsTotal = res.meta?.total ?? null;
			} catch (err) {
				if (err instanceof ApiError && err.status !== 404) {
					loadError = loadError || 'jce.compare.errors.load_items';
					loadErrorParams = { type: selectedType.name, message: err.message };
				}
			}
		}
	}

	let preselectedItems: ResolvedItem[] = [];
	const preselectFailures: string[] = [];
	const preselect = searchParams.get('preselect') ?? '';
	if (preselect && selectedType) {
		const ids = preselect.split(',').filter(Boolean);
		for (const id of ids.slice(0, 4)) {
			try {
				const item = await apiGet<ResolvedItem>(ctx, `/jce/entities/${source}/${id}`);
				if (item) preselectedItems.push(item);
			} catch (err) {
				preselectFailures.push(id);
				console.warn(`compare preselect: could not resolve ${source}/${id}`, err);
			}
		}
	}
	if (preselectFailures.length > 0 && !loadError) {
		loadError = 'jce.compare.errors.preselect_failed';
		loadErrorParams = { ids: preselectFailures.join(', ') };
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
				loadError = loadError || 'jce.compare.errors.from_token';
				loadErrorParams = { message: err.message };
			} else if (err instanceof Error) {
				loadError = loadError || 'jce.compare.errors.api_unreachable';
				loadErrorParams = { message: err.message };
			}
		}
	}
	if (fromTokenSlug) {
		redirect(302, `/compare/${fromTokenSlug}`);
	}

	const mine = searchParams.get('mine') === 'true';

	let myComparisons: JCEComparisonSummary[] = [];
	let myMeta: ListMeta = { total: 0, page: 1, limit: defaultPageSize };
	if (event.locals.user) {
		try {
			const res = await apiList<JCEComparisonSummary>(ctx, `/jce/comparisons?mine=true&filter_depth=quick&page_size=${defaultPageSize}&sort=created_at`);
			myComparisons = mapComparisonTypes(res.items, types);
			myMeta = res.meta ?? myMeta;
		} catch (err) {
			if (err instanceof ApiError && err.status !== 404) {
				loadError = loadError || 'jce.compare.errors.load_mine';
				loadErrorParams = { message: err.message };
			}
		}
	}

	let profiles: ComparisonProfile[] = [];
	if (event.locals.user) {
		try {
			const res = await apiGet<ComparisonProfile[]>(ctx, '/jce/profiles');
			profiles = res ?? [];
		} catch (err) {
			// Feature may be disabled; ignore 404 feature_disabled.
			if (err instanceof ApiError && err.status !== 404) {
				loadError = loadError || 'jce.compare.errors.load_profiles';
				loadErrorParams = { message: err.message };
			}
		}
	}

	let defaultProfile = profiles.find((p) => p.is_default) ?? profiles[0] ?? null;

	return {
		types,
		selectedType: selectedType ?? null,
		source,
		itemRefTypes,
		allowNestedComparisons,
		allowedNestedTypes,
		pageSizeOptions,
		defaultPageSize,
		defaultPageSizeOptions,
		scopeChain,
		scopeParams,
		items,
		itemsTotal,
		preselectedItems,
		loadError,
		loadErrorParams,
		user: event.locals.user,
		mine,
		myComparisons,
		myMeta,
		profiles,
		defaultProfile,
		apiBase: API_BASE,
		mainOrigin: event.url.origin
	};
};
