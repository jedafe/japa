import { error, redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/public';
import { apiContext, apiGet, apiList, apiPost, ApiError, API_BASE } from '$lib/server/api';
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

function parseDefaultSortOptions(): string[] {
	const raw = env.PUBLIC_JCE_DEFAULT_SORT_OPTIONS;
	if (raw) {
		try {
			const parsed = JSON.parse(raw);
			if (Array.isArray(parsed) && parsed.every((v) => typeof v === 'string') && parsed.length > 0)
				return parsed;
		} catch {
			// fall through
		}
	}
	return ['title', 'title_desc'];
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
	const { features } = await event.parent();
	const ctx = apiContext(event);
	const searchParams = event.url.searchParams;

	let types: ComparisonType[] = [];
	// Full type list retained for labeling existing comparisons — a user's
	// quick comparison may reference a system-managed type (e.g.
	// cost_of_living) that the picker must not offer (§30 can_create).
	let allTypes: ComparisonType[] = [];
	let loadError = '';
	let loadErrorParams: Record<string, string> = {};
	try {
		const res = await apiGet<ComparisonType[]>(ctx, '/jce/comparison-types?page_size=100');
		allTypes = enrichTypes(res ?? []);
		types = allTypes.filter((t) => t.can_create !== false);
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
	if (typeSlug === 'mentors' && features['mentorship.comparison'] !== true) {
		throw error(404, { code: 'feature_disabled', message: 'Mentorship comparison is not available' });
	}
	if (typeSlug) {
		selectedType = types.find((t) => t.slug === typeSlug);
	}
	// An unknown type slug normalizes to the landing page rather than
	// lingering in the URL — scope/preselect params are meaningless
	// without a resolved type, so only `mine` is preserved.
	if (typeSlug && !selectedType) {
		throw redirect(307, searchParams.get('mine') === 'true' ? '/compare?mine=true' : '/compare');
	}
	// No ?type= (or an unknown slug) → landing mode: the page renders the
	// type-card grid + popular comparisons instead of a picker workspace.

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
	const configuredSortOptions = (selectedType?.config?.sort_options as string[]) ?? [];
	const sortOptions = configuredSortOptions.length > 0 ? configuredSortOptions : parseDefaultSortOptions();

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
	const sort = searchParams.get('sort') ?? '';

	let scopeChain = (selectedType?.config?.scope_chain as string[]) ?? [];
	// Course search must be scoped to a university. Older type configs that do
	// not include a scope_chain still need this selector so users can choose one.
	if (source === 'course' && !scopeChain.includes('university')) {
		scopeChain = [...scopeChain, 'university'];
	}
	// A chain whose leaf step matches the entity being listed is
	// self-referential — filtering cities down to a single city, or states to
	// a single state, leaves nothing to compare. Only the parent steps are
	// meaningful, so the trailing self-step is dropped (e.g. the cities type
	// shows country + state, not city).
	const selfScopeStep: Record<string, string> = {
		country: 'country',
		state: 'state',
		city: 'city',
		col_city: 'city',
		university: 'university'
	};
	if (scopeChain[scopeChain.length - 1] === selfScopeStep[source]) {
		scopeChain = scopeChain.slice(0, -1);
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

	const compareToken = searchParams.get('compare_token') ?? '';
	const mine = searchParams.get('mine') === 'true';
	const preselect = searchParams.get('preselect') ?? '';

	// A preselect token is `id` or `type:id` — the latter resolves through the
	// named resolver so nested comparisons restore mixed item types.
	function resolvePreselectRef(token: string): string {
		const sep = token.indexOf(':');
		if (sep <= 0) return `${source}/${token}`;
		return `${token.slice(0, sep)}/${token.slice(sep + 1)}`;
	}

	// Guest selections that survive login are consumed server-side and the
	// user is redirected to the new quick comparison. Runs before the read
	// wave so a successful consume doesn't pay for fetches the redirect
	// discards. The error is captured and applied after the wave so the
	// original load-error priority (items → preselect → from_token → mine)
	// is preserved.
	let fromTokenSlug = '';
	let fromTokenErr: { key: string; params: Record<string, string> } | null = null;
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
				fromTokenErr = { key: 'jce.compare.errors.from_token', params: { message: err.message } };
			} else if (err instanceof Error) {
				fromTokenErr = { key: 'jce.compare.errors.api_unreachable', params: { message: err.message } };
			}
		}
	}
	if (fromTokenSlug) {
		redirect(302, `/compare/${fromTokenSlug}`);
	}

	// Read wave — entity items, pre-selection hydration, the user's quick
	// comparisons, the landing-mode discovery strip, and the AI Quick
	// Compare permission are all independent once the type is resolved.
	const itemsPromise = (async () => {
		if (!selectedType) return { items: [] as ResolvedItem[], itemsTotal: null as number | null, error: null as Record<string, string> | null };
		// Course search requires a university scope. Don't call the resolver
		// before the user has selected one; the scope dropdown is the prompt.
		if (source === 'course' && !scopeParams['university']) {
			return { items: [] as ResolvedItem[], itemsTotal: null as number | null, error: null as Record<string, string> | null };
		}
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
			// Kind filtering (JCE ↔ wiki mapping): push the type's subtype
			// into the resolver's native filter when it names a known
			// discriminator param — e.g. subtype 'student' on a visa_route
			// source passes ?visa_category=student server-side.
			const subtypeParam = selectedType?.config?.subtype;
			if (typeof subtypeParam === 'string' && subtypeParam) {
				if (source === 'visa_route') params.set('visa_category', subtypeParam);
				if (source === 'organization' || source === 'bank') params.set('org_type', subtypeParam);
			}
			if (sort) params.set('sort', sort);
			const res = await apiList<ResolvedItem>(ctx, `/jce/entities/${source}?${params.toString()}`);
			let items = res.items ?? [];
			let total = res.meta?.total ?? null;
			let allItemsLoaded = false;
			// Resolver ORDER BY coverage is partial (space-backed types only);
			// bounded result sets are fetched whole so the client sorts the
			// complete set correctly for every resolver.
			const FULL_LOAD_CAP = 1000;
			if (total !== null && total <= FULL_LOAD_CAP) {
				if (items.length >= total) {
					allItemsLoaded = true;
				} else {
					params.set('limit', String(total));
					params.set('offset', '0');
					const full = await apiList<ResolvedItem>(ctx, `/jce/entities/${source}?${params.toString()}`);
					if (full.items) {
						items = full.items;
						allItemsLoaded = items.length >= total;
						total = full.meta?.total ?? total;
					}
				}
			}
			// Kind filtering (JCE ↔ wiki mapping): a type config.subtype
			// narrows items to the matching discriminator value — e.g.
			// subtype 'student' keeps visa routes with visa_category=student.
			const subtype = selectedType?.config?.subtype;
			if (typeof subtype === 'string' && subtype) {
				const DISC_KEYS = ['visa_category', 'org_type', 'route_kind', 'kind', 'category'];
				const keyed = items.filter((it) =>
					DISC_KEYS.some((k) => it.metadata?.[k] === subtype)
				);
				if (keyed.length > 0) items = keyed;
			}
			return {
				items,
				itemsTotal: total as number | null,
				error: null as Record<string, string> | null,
				allItemsLoaded
			};
		} catch (err) {
			if (err instanceof ApiError && err.status !== 404) {
				return {
					items: [] as ResolvedItem[],
					itemsTotal: null as number | null,
					error: { type: selectedType.name, message: err.message }
				};
			}
			return { items: [] as ResolvedItem[], itemsTotal: null as number | null, error: null as Record<string, string> | null };
		}
	})();

	const preselectPromise = (async () => {
		if (!preselect || !selectedType) {
			return { items: [] as ResolvedItem[], failures: [] as string[] };
		}
		// Cap pre-selections at the type's max_item_count so a crafted URL
		// cannot stage more items than the tray (and create) allows.
		const preselectCap =
			typeof selectedType.config?.max_item_count === 'number' && selectedType.config.max_item_count > 0
				? selectedType.config.max_item_count
				: 4;
		// Tokens may carry a `type:id` prefix (nested comparisons mix item_ref
		// types); bare ids resolve against the active source.
		const ids = preselect.split(',').filter(Boolean).slice(0, preselectCap);
		const results = await Promise.all(
			ids.map((token) =>
				apiGet<ResolvedItem>(ctx, `/jce/entities/${resolvePreselectRef(token)}`)
					.then((item) => ({ token, item: item ?? null, failed: false }))
					.catch((err) => {
						console.warn(`compare preselect: could not resolve ${resolvePreselectRef(token)}`, err);
						return { token, item: null, failed: true };
					})
			)
		);
		return {
			items: results.flatMap((r) => (r.item ? [r.item] : [])),
			failures: results.flatMap((r) => (r.failed ? [r.token] : []))
		};
	})();

	const minePromise = (async () => {
		if (!event.locals.user || !mine) {
			return { comparisons: [] as JCEComparisonSummary[], meta: null as ListMeta | null, failed: false, message: '' };
		}
		try {
			const res = await apiList<JCEComparisonSummary>(ctx, `/jce/comparisons?mine=true&filter_depth=quick&page_size=${defaultPageSize}&page_offset=${(page - 1) * defaultPageSize}&sort=created_at`);
			return { comparisons: res.items, meta: res.meta ?? null, failed: false, message: '' };
		} catch (err) {
			return {
				comparisons: [] as JCEComparisonSummary[],
				meta: null,
				failed: err instanceof ApiError && err.status !== 404,
				message: err instanceof Error ? err.message : ''
			};
		}
	})();

	// Landing-mode discovery strip — published public detailed comparisons.
	// Only fetched when the picker is in landing mode (no ?type, not the
	// "mine" tab). Best-effort: failure hides the strip.
	const popularPromise =
		!typeSlug && !mine
			? apiList<JCEComparisonSummary>(
					ctx,
					'/jce/comparisons?filter_depth=detailed&filter_status=published&filter_visibility=public&page_size=6&sort=popularity'
				)
					.then((res) => res.items)
					.catch(() => [] as JCEComparisonSummary[])
			: Promise.resolve([] as JCEComparisonSummary[]);

	// §30 AI Quick Compare — the POST /ai/compare-entities endpoint is gated
	// by agent.use. /me/permissions only returns explicitly resolved nodes,
	// so super_admin (which bypasses RBAC at middleware) is checked via groups.
	// Best-effort: a failed lookup degrades to false, never blocks the page.
	const canAiQuickPromise = (async () => {
		if (!event.locals.user) return false;
		const isSuperAdmin = (event.locals.user.groups ?? []).some(
			(g) => g === 'Super Admin' || g === 'super_admin'
		);
		if (isSuperAdmin) return true;
		try {
			const permRes = await apiGet<{ permissions: string[] }>(ctx, '/me/permissions');
			return (permRes?.permissions ?? []).includes('agent.use');
		} catch {
			return false;
		}
	})();

	const [itemsRes, preselectRes, mineRes, popularItems, canAiQuick] = await Promise.all([
		itemsPromise,
		preselectPromise,
		minePromise,
		popularPromise,
		canAiQuickPromise
	]);

	const items = itemsRes.items;
	const itemsTotal = itemsRes.itemsTotal;
	const preselectedItems = preselectRes.items;
	const preselectFailures = preselectRes.failures;
	const myComparisons = mapComparisonTypes(mineRes.comparisons, allTypes);
	const myMeta: ListMeta = mineRes.meta ?? { total: 0, page: 1, limit: defaultPageSize };
	const popularComparisons = mapComparisonTypes(popularItems, allTypes);

	// Apply load errors in the original priority order.
	if (itemsRes.error && !loadError) {
		loadError = 'jce.compare.errors.load_items';
		loadErrorParams = itemsRes.error;
	}
	if (preselectFailures.length > 0 && !loadError) {
		loadError = 'jce.compare.errors.preselect_failed';
		loadErrorParams = { ids: preselectFailures.join(', ') };
	}
	if (fromTokenErr && !loadError) {
		loadError = fromTokenErr.key;
		loadErrorParams = fromTokenErr.params;
	}
	if (mineRes.failed && !loadError) {
		loadError = 'jce.compare.errors.load_mine';
		loadErrorParams = { message: mineRes.message };
	}

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
		sortOptions,
		typeNames: Object.fromEntries(allTypes.map((t) => [t.slug, t.name])),
		items,
		itemsTotal,
		allItemsLoaded: (itemsRes as { allItemsLoaded?: boolean }).allItemsLoaded === true,
		preselectedItems,
		loadError,
		loadErrorParams,
		user: event.locals.user,
		mine,
		myComparisons,
		myMeta,
		popularComparisons,
		canAiQuick,
		apiBase: API_BASE,
		mainOrigin: event.url.origin
	};
};
