<script lang="ts">
	import { page } from '$app/state';
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { goto, afterNavigate } from '$app/navigation';
	import type { Component } from 'svelte';
	import { onDestroy } from 'svelte';
	import {
		createQuickComparison,
		createGuestSelection,
		deleteComparison,
		promoteComparison
	} from '#lib/api/jce.js';
	import { compareEntities, type CompareEntitiesResult } from '#lib/ai/compareApi.js';
	import type { ComparisonType, ResolvedItem, JCEComparisonSummary } from '#lib/types/jce.js';
	import StepHeader from '#lib/components/compare/StepHeader.svelte';
	import ScopeChain from '#lib/components/compare/ScopeChain.svelte';
	import CompareTray from '#lib/components/compare/CompareTray.svelte';
	import AiQuickPanel from '#lib/components/compare/AiQuickPanel.svelte';
	import FieldPoolPicker from '#lib/components/compare/FieldPoolPicker.svelte';
	import { groupTypesByFamily } from '#lib/components/compare/compareFamilies.js';
	import CompareCategoryCard from '#lib/components/compare/CompareCategoryCard.svelte';
	import ComparisonVsCard from '#lib/components/compare/ComparisonVsCard.svelte';
	import { badgeFor } from '#lib/components/compare/compareBadge.js';
	import JceSelect from '#lib/components/ui/JceSelect.svelte';
	import { EmptyState, EntityHero, ListPageBar, Modal, PaginationControls, UrlTabs, GuideSlideOver, type GuideContent, type IconComponent, type UrlTab } from '#lib/components/ui/index.js';

	import {
		missingRequiredScope,
		sourceOptions
	} from '#lib/components/compare/compareFlow.js';
	import type { PageData } from './$types';
	import { api } from '#lib/api/client.js';
	import HeroArtCompare from '#lib/components/heroes/HeroArtCompare.svelte';
	import { i18n, t, siteName} from '#lib/i18n/index.js';
	import {
		GitCompare,
		ChevronDown,
		ChevronRight,
		LayoutGrid,
		Search,
		ArrowRight,
		Trash2,
		ExternalLink,
		Sparkles,
		Check,
		GraduationCap,
		MapPin,
		Briefcase,
		Languages,
		Globe,
		BookOpen,
		Landmark,
		Wallet,
		MessageCircle,
		SlidersHorizontal
	} from 'lucide-svelte';

	let { data }: { data: PageData } = $props();

	// "How it works" slide-over — editable via compare.guide (admins).
	let guideOpen = $state(false);
	const fallbackGuide: GuideContent = {
		title: t('common.how_it_works', 'How it works'),
		body: t('compare.guide_body', ''),
		ctas: []
	};
	let guideContent = $state<GuideContent | null>(null);
	const effectiveGuide = $derived(guideContent ?? fallbackGuide);
	async function saveGuide(v: GuideContent): Promise<string | null> {
		try {
			const res = await api.put<{ compare_guide?: GuideContent }>('/admin/settings/general', { compare_guide: v });
			if (res.data?.compare_guide) guideContent = res.data.compare_guide;
			else guideContent = v;
			return null;
		} catch (e) {
			return e instanceof Error ? e.message : 'save_failed';
		}
	}

	let types = $derived(data.types ?? []);
	let selectedType = $derived(data.selectedType ?? null);
	// Hero breadcrumb — the workspace and "mine" views trail back to the
	// /compare landing; the plain landing keeps its unlinked current crumb.
	let heroCrumbs = $derived.by(() => {
		const base = [
			{ label: t('common.home', 'Home'), href: '/' },
			{ label: t('jce.compare.heading'), href: '/compare' }
		];
		if (data.mine) return [...base, { label: t('jce.compare.tab_mine', 'My comparisons') }];
		if (data.selectedType) return [...base, { label: data.selectedType.name }];
		return [
			{ label: t('common.home', 'Home'), href: '/' },
			{ label: t('jce.compare.heading') }
		];
	});
	let source = $derived(data.source ?? 'space');
	let itemRefTypes = $derived(data.itemRefTypes ?? []);
	let allowNestedComparisons = $derived(data.allowNestedComparisons ?? false);
	let allowedNestedTypes = $derived(data.allowedNestedTypes ?? []);
	let allItems = $derived(data.items ?? []);
	let itemsTotal = $derived(data.itemsTotal ?? null);
	let preselectedItems = $derived(data.preselectedItems ?? []);
	// Sub-spec 6: dimension pool selection — defaults to the pool's
	// is_default groups (today's Overview-only behavior).
	let fieldPool = $derived(data.fieldPool ?? { groups: [] });
	// Rail only makes sense when at least one pool group carries fields —
	// e.g. product types whose bound groups are still unpopulated.
	let hasFieldPool = $derived(fieldPool.groups.some((g) => g.fields.length > 0));
	let selectedFieldIds = $state<Set<string>>(new Set());
	let lastPoolTypeId = $state('');


	$effect(() => {
		const tid = selectedType?.id ?? '';
		if (tid === lastPoolTypeId) return;
		lastPoolTypeId = tid;
		const init = new Set<string>();
		for (const g of fieldPool.groups) {
			if (g.is_default) for (const f of g.fields) if (f.comparable) init.add(f.id);
		}
		selectedFieldIds = init;
	});
	let loadError = $derived(data.loadError ?? '');
	let loadErrorParams = $derived(data.loadErrorParams ?? {});
	let user = $derived(data.user ?? null);
	let mainOrigin = $derived(data.mainOrigin);
	let mine = $derived(data.mine ?? false);
	let myComparisons = $derived(data.myComparisons ?? []);
	let myMeta = $derived(data.myMeta ?? null);

	// Snapshot the initial URL params; client-side filter state is updated manually.
	const searchParams = page.url.searchParams;
	// Page-size and sort options come from the comparison type config (server-side,
	// with env defaults applied) and fall back to the env defaults.
	let pageSizeOptions = $derived(data.pageSizeOptions ?? []);
	let defaultPageSize = $derived(data.defaultPageSize);
	let defaultPageSizeOptions = $derived(data.defaultPageSizeOptions);
	let sortValues = $derived.by(() => {
		const configured = data.sortOptions;
		return Array.isArray(configured) ? configured : [];
	});

	const initialQuery = searchParams.get('q') ?? '';
	const initialSort = searchParams.get('sort') ?? 'title';
	const initialPage = parseInt(searchParams.get('page') ?? '1', 10) || 1;
	const initialNestedType = searchParams.get('nested_type') ?? '';

	let query = $state(initialQuery);
	let sortBy = $state(initialSort);
	// SSR-safe initial value — mirrors the $effect.pre correction below so the
	// first render already shows items instead of an empty grid.
	function initialPageSize() {
		const requested = parseInt(searchParams.get('page_size') ?? '', 10) || 0;
		if (pageSizeOptions.includes(requested)) return requested;
		if (pageSizeOptions.includes(defaultPageSize)) return defaultPageSize;
		return pageSizeOptions[0] ?? defaultPageSize;
	}
	let pageSize = $state(initialPageSize());
	$effect.pre(() => {
		const next =
			parseInt(page.url.searchParams.get('page_size') ?? String(defaultPageSize), 10) || defaultPageSize;
		if (next !== pageSize) pageSize = next;
	});
	let currentPage = $state(initialPage);
	let nestedType = $state(initialNestedType);
	let selectedItems = $state<ResolvedItem[]>([]);
	let searchTimeout: ReturnType<typeof setTimeout> | null = null;
	onDestroy(() => {
		if (searchTimeout) clearTimeout(searchTimeout);
	});

	// SSR-safe initial value — same computation as the sync effect below so the
	// narrow summary chip renders correctly on first paint.
	function initialScope() {
		return Object.fromEntries(
			Object.entries(data.scopeParams ?? {}).filter(([key]) => (data.scopeChain ?? []).includes(key))
		);
	}
	let scopeSelections = $state<Record<string, string>>(initialScope());
	$effect(() => {
		// Sync local scope state with the server-loaded page data on every navigation.
		scopeSelections = initialScope();
	});

	$effect(() => {
		// Mirror the URL on SPA navigation (back/forward, redirects): these
		// controls are init-once snapshots — without resync the grid shows the
		// new page's items while the controls claim the old sort/search/page.
		const sp = page.url.searchParams;
		currentPage = parseInt(sp.get('page') ?? '1', 10) || 1;
		sortBy = sp.get('sort') ?? 'title';
		query = sp.get('q') ?? '';
		nestedType = sp.get('nested_type') ?? '';
	});

	$effect(() => {
		if (!pageSizeOptions.includes(pageSize)) {
			pageSize = pageSizeOptions[0] ?? defaultPageSize;
		}
		if (!sortValues.includes(sortBy)) {
			sortBy = sortValues[0] ?? 'title';
		}
	});
	let creating = $state(false);
	let deleting = $state<string | null>(null);
	let promoting = $state<string | null>(null);
	let error = $state('');
	let errorParams = $state<Record<string, string>>({});

	// §30 AI Quick Compare — POST /api/v1/ai/compare-entities (§103 task 7.07).
	// Visible when feature.jce.ai_quick is on and the user carries agent.use.
	// The form inputs live in AiQuickPanel; it calls back via runAiQuick(opts).
	let aiQuickOpen = $state(false);
	let aiQuickLoading = $state(false);
	let aiQuickResult = $state<CompareEntitiesResult | null>(null);
	let aiQuickError = $state('');
	let aiQuickEnabled = $derived(
		Boolean(
			(data as { features?: Record<string, boolean> }).features?.['feature.jce.ai_quick'] &&
				data.canAiQuick
		)
	);

	const maxItems = $derived((selectedType?.config?.max_item_count as number) ?? 4);

	let sortOptions = $derived(
		sortValues.map((value) => ({ value, label: t(`jce.compare.sort.${value}`, value) }))
	);

	// Type switches (sibling picker row, category tile, back/forward between
	// two ?type= URLs) must drop the tray — create() posts the new type_id
	// and stale picks would mint a wrong-typed comparison. Landing detours
	// keep the tray (tid only toggles ''→id), and this .pre runs before the
	// preselect effect below so a same-nav ?preselect can restage into the
	// emptied tray.
	let lastItemsTypeId = $state('');
	$effect.pre(() => {
		const tid = selectedType?.id ?? '';
		if (tid === lastItemsTypeId) return;
		const typeSwitch = Boolean(lastItemsTypeId && tid);
		lastItemsTypeId = tid;
		if (typeSwitch) {
			selectedItems = [];
			scopeLabels = {};
		}
	});

	let lastPreselect = $state<ResolvedItem[] | null>(null);
	$effect.pre(() => {
		if (selectedItems.length === 0 && preselectedItems.length > 0 &&
				lastPreselect !== preselectedItems) {
			lastPreselect = preselectedItems;
			// Cap server pre-selections at maxItems so a crafted URL cannot
			// stage more items than the tray allows.
			selectedItems = [...preselectedItems.slice(0, Math.max(0, maxItems))];
		}
	});

	$effect(() => {
		if (loadError) {
			error = loadError;
			errorParams = loadErrorParams;
		}
	});

	let popularComparisons = $derived(data.popularComparisons ?? []);

	// Landing discovery: family cards derive from the type catalog; the
	// trending strip is seeded server-side by popularity and re-fetches only
	// when the visitor switches to "Trending".
	let familyGroups = $derived(groupTypesByFamily(types));
	let trendingSort = $state<'popularity' | 'trending'>('popularity');
	let trendingItems = $state<JCEComparisonSummary[] | null>(null); // null = use server seed
	let trendingLoading = $state(false);

	async function setTrendingSort(next: 'popularity' | 'trending') {
		if (next === trendingSort) return;
		trendingSort = next;
		if (next === 'popularity') {
			trendingItems = null;
			return;
		}
		trendingLoading = true;
		try {
			const res = await api.get<JCEComparisonSummary[]>(
				'/jce/comparisons?filter_depth=detailed&filter_status=published&filter_visibility=public&page_size=9&sort=trending'
			);
			trendingItems = res.error ? [] : (res.data ?? []);
		} catch {
			trendingItems = []; // hide the strip rather than strand it on the seed
		} finally {
			trendingLoading = false;
		}
	}
	let trending = $derived(trendingItems ?? popularComparisons);

	// Reseed when the server payload changes (SPA nav back to the landing) —
	// otherwise a stale "Trending" fetch would shadow the fresh seed.
	$effect(() => {
		data.popularComparisons;
		trendingSort = 'popularity';
		trendingItems = null;
	});

	// Card subtitle: up to three member type names (+N overflow), else
	// item_ref_types labels — kept to one line so tiles stay icon-sized.
	function familySubtitle(members: ComparisonType[]): string {
		if (members.length > 1) {
			const names = members.slice(0, 3).map((m) => m.name);
			const extra = members.length - names.length;
			return extra > 0
				? `${names.join(', ')} ${t('jce.compare.landing.more_types', '+{n} more', { n: extra })}`
				: names.join(', ');
		}
		const refs = ((members[0]?.config?.item_ref_types as string[] | undefined) ?? [])
			.filter((r) => r !== 'comparison');
		return refs.map(sourcePlural).join(', ');
	}

	// Unified category tiles — family cards first, then ungrouped types. Every
	// tile opens the picker workspace (?type=): a family enters through its
	// first member type and the in-workspace sibling switcher covers the rest.
	// The grid shows exactly two rows (4 tiles mobile / 8 desktop); the "All
	// categories" dialog lists everything in a scrollable view.
	interface CategoryTile {
		icon: Component;
		tint: string;
		name: string;
		subtitle: string;
		count?: number;
		href: string;
	}
	const CATEGORY_TILES_DESKTOP = 8;
	const CATEGORY_TILES_MOBILE = 4;
	let categoryTiles = $derived<CategoryTile[]>([
		...familyGroups.groups.map((g) => ({
			icon: g.family.icon,
			tint: g.family.tint,
			name: t(`jce.compare.family.${g.family.key}`, g.family.key),
			subtitle: familySubtitle(g.types),
			href: `/compare?type=${g.types[0].slug}`
		})),
		...familyGroups.ungrouped.map((cmpType) => ({
			icon: typeIcon(cmpType),
			tint: 'var(--color-primary)',
			name: cmpType.name,
			subtitle: cmpType.description ?? '',
			count: cmpType.item_count,
			href: `/compare?type=${cmpType.slug}`
		}))
	]);
	let visibleTiles = $derived(categoryTiles.slice(0, CATEGORY_TILES_DESKTOP));
	// Type picker scope: the workspace's "category" is the selected type's
	// family — picking a category tile enters the family through its first
	// type, and the picker lists just that family's types.
	let pickerGroup = $derived.by(() => {
		const family = selectedType?.config?.family;
		if (typeof family !== 'string' || !family) return null;
		return familyGroups.groups.find((g) => g.family.key === family) ?? null;
	});
	let pickerTypes = $derived(pickerGroup ? pickerGroup.types : familyGroups.ungrouped);
	let categoriesModalOpen = $state(false);
	let sortModalOpen = $state(false);
	let typeModalOpen = $state(false);
	// Tile clicks inside the modal are plain anchors — a successful
	// client-side navigation must dismiss it or it lingers over the new
	// type's workspace.
	afterNavigate(() => {
		categoriesModalOpen = false;
		typeModalOpen = false;
	});

	let landingSearch = $state('');
	function submitLandingSearch() {
		if (!landingSearch.trim()) return;
		goto(`/comparisons?q=${encodeURIComponent(landingSearch.trim())}`);
	}
	let scopeLabels = $state<Record<string, string>>({});
	let showNarrow = $derived((data.scopeChain ?? []).length > 0);
	let missingScope = $derived(
		missingRequiredScope(source, data.scopeChain ?? [], scopeSelections)
	);
	let requiredScopeMissing = $derived(missingScope !== null);
	let isNestedSource = $derived(source === 'comparison');
	let refTypeOptions = $derived(sourceOptions(itemRefTypes, sourceLabel));
	let trayHint = $derived(
		selectedItems.length >= maxItems
			? t('jce.compare.tray.hint_max', undefined, { max: String(maxItems) })
			: ''
	);
	let selectSummary = $derived(
		t('jce.compare.step.select', undefined, {
			count: String(selectedItems.length),
			max: String(maxItems)
		})
	);

	function selectSource(next: string) {
		selectedItems = [];
		query = '';
		sortBy = 'title';
		scopeSelections = {};
		scopeLabels = {};
		pageSize = pageSizeOptions.includes(defaultPageSize) ? defaultPageSize : (pageSizeOptions[0] ?? defaultPageSize);
		currentPage = 1;
		nestedType = '';
		const url = new URL(page.url.href);
		url.searchParams.set('source', next);
		url.searchParams.delete('preselect');
		url.searchParams.delete('nested_type');
		url.searchParams.delete('q');
		url.searchParams.delete('sort');
		url.searchParams.delete('page_size');
		url.searchParams.delete('page');
		for (const step of data.scopeChain ?? []) {
			url.searchParams.delete(scopeParamFor(step));
		}
		url.searchParams.set('page', '1');
		goto(url, { replace: true });
	}

	function setNestedType(next: string) {
		nestedType = next;
		currentPage = 1;
		navigateURL({ nested_type: next, page: null });
	}

	function scopeParamFor(step: string): string {
		const mapping = (selectedType?.config?.scope_param_mapping as Record<string, string> | undefined) ?? {};
		if (mapping[step]) return mapping[step];
		// Region/subregion are string filters; everything else uses the
		// conventional `{scope}_id` query parameter expected by the resolver.
		if (step === 'region' || step === 'subregion') return step;
		return `${step}_id`;
	}

	function setScopeSelection(step: string, value: string) {
		const old = scopeSelections[step];
		if (old === value) return;
		scopeSelections = { ...scopeSelections, [step]: value };

		// Reset downstream selections so we don't keep stale IDs. Use the
		// effective chain (data.scopeChain — post augmentation/self-step drop),
		// not the raw config chain, so appended steps (e.g. university for
		// course sources) reset correctly too.
		const effectiveChain = data.scopeChain ?? [];
		const idx = effectiveChain.indexOf(step);
		if (idx >= 0) {
			for (const next of effectiveChain.slice(idx + 1)) {
				delete scopeSelections[next];
			}
		}
		scopeSelections = { ...scopeSelections };

		currentPage = 1;
		const url = new URL(page.url.href);
		const param = scopeParamFor(step);
		if (value) {
			url.searchParams.set(param, value);
		} else {
			url.searchParams.delete(param);
		}
		// Also remove downstream params.
		if (idx >= 0) {
			for (const next of effectiveChain.slice(idx + 1)) {
				url.searchParams.delete(scopeParamFor(next));
			}
		}
		url.searchParams.set('page', '1');
		goto(url, { replace: true });
	}

	function backToLanding() {
		const url = new URL(page.url.href);
		url.searchParams.delete('type');
		url.searchParams.delete('preselect');
		url.searchParams.delete('source');
		url.searchParams.delete('nested_type');
		url.searchParams.delete('q');
		url.searchParams.delete('sort');
		url.searchParams.delete('page_size');
		url.searchParams.delete('page');
		for (const step of data.scopeChain ?? []) {
			url.searchParams.delete(scopeParamFor(step));
		}
		goto(url, { replace: true });
	}


	// navigateURL triggers a SvelteKit client-side navigation so the +page.server.ts load re-runs
	// with the updated query (search, page, page-size, nested-type filters, etc.).
	function navigateURL(updates: Record<string, string | null>) {
		const url = new URL(window.location.href);
		for (const [k, v] of Object.entries(updates)) {
			if (v === null || v === '') url.searchParams.delete(k);
			else url.searchParams.set(k, v);
		}
		goto(url, { replace: true, reset: false });
	}

	function setSearch(q: string) {
		query = q;
		currentPage = 1;
		if (searchTimeout) clearTimeout(searchTimeout);
		searchTimeout = setTimeout(() => {
			navigateURL({ q, page: null });
		}, 300);
	}

	function setSort(sort: string) {
		sortBy = sort;
		currentPage = 1;
		navigateURL({ sort, page: null });
	}

	function setPageSize(size: number) {
		pageSize = size;
		currentPage = 1;
		navigateURL({ page_size: String(size), page: null });
	}

	function setPage(p: number) {
		currentPage = p;
		navigateURL({ page: String(p) });
	}

	function setTab(isMine: boolean) {
		const url = new URL(page.url.href);
		if (isMine) url.searchParams.set('mine', 'true');
		else url.searchParams.delete('mine');
		url.searchParams.delete('page');
		goto(url, { replace: true });
	}

	function filteredItems(items: ResolvedItem[]): ResolvedItem[] {
		let out = items;
		// Search is now server-side (q is passed to /jce/entities). Only nested-type is still
		// filtered client-side as a guard for resolvers that may not honor the nested_type filter.
		if (source === 'comparison' && nestedType) {
			out = out.filter((it) => (it.metadata?.type_slug as string | undefined) === nestedType);
		}

		switch (sortBy) {
			case 'title':
				out = [...out].sort((a, b) => a.title.localeCompare(b.title));
				break;
			case 'title_desc':
				out = [...out].sort((a, b) => b.title.localeCompare(a.title));
				break;
			case 'members':
				out = [...out].sort((a, b) => getMemberCount(b) - getMemberCount(a));
				break;
			case 'newest':
				out = [...out].sort((a, b) => (getCreatedAt(b) ?? '').localeCompare(getCreatedAt(a) ?? ''));
				break;
		}
		return out;
	}

	function getMemberCount(item: ResolvedItem) {
		const meta = item.metadata as Record<string, unknown> | undefined;
		if (!meta) return 0;
		if (typeof meta.total_enrolled === 'number') return meta.total_enrolled;
		if (typeof meta.member_count === 'number') return meta.member_count;
		if (typeof meta.subscriber_count === 'number') return meta.subscriber_count;
		return 0;
	}

	function getCreatedAt(item: ResolvedItem) {
		const meta = item.metadata as Record<string, unknown> | undefined;
		if (!meta) return '';
		if (typeof meta.created_at === 'string') return meta.created_at;
		return '';
	}

	function getMetaChips(item: ResolvedItem): string[] {
		const chips: string[] = [];
		const meta = item.metadata as Record<string, unknown> | undefined;
		if (!meta) return chips;
		if (meta.geo_code) chips.push(String(meta.geo_code));
		if (meta.level && meta.level !== 'custom') chips.push(String(meta.level));
		if (meta.country_code) chips.push(String(meta.country_code));
		if (typeof meta.member_count === 'number' && meta.member_count > 0) {
			chips.push(t('jce.common.members_count', '{n} members', { n: meta.member_count }));
		}
		if (typeof meta.total_enrolled === 'number' && meta.total_enrolled > 0) {
			chips.push(t('jce.common.enrolled_count', '{n} enrolled', { n: meta.total_enrolled }));
		}
		if (source === 'comparison' && meta.type_name) chips.push(String(meta.type_name));
		return chips;
	}

	// The server applies pagination (limit/offset) and returns one extra item so we
	// can infer hasNext without a total count. When the whole matching set was
	// loaded (≤ MaxSearchLimit), the client paginates the sorted superset.
	function paginatedItems(items: ResolvedItem[]): ResolvedItem[] {
		if (allLoaded) return items.slice((currentPage - 1) * pageSize, currentPage * pageSize);
		return items.slice(0, pageSize);
	}

	function sameItem(a: ResolvedItem, b: ResolvedItem) {
		return a.id === b.id && a.type === b.type;
	}

	function isSelected(item: ResolvedItem) {
		return selectedItems.some((s) => sameItem(s, item));
	}

	function toggleItem(item: ResolvedItem) {
		if (isSelected(item)) {
			selectedItems = selectedItems.filter((s) => !sameItem(s, item));
		} else if (selectedItems.length < maxItems) {
			selectedItems = [...selectedItems, item];
		}
	}

	function removeSelected(item: ResolvedItem) {
		selectedItems = selectedItems.filter((s) => !sameItem(s, item));
	}

	function sourceIcon(value?: string) {
		const key = value?.toLowerCase() ?? '';
		const iconName = t(`jce.compare.sources.${key}.icon`, 'MapPin');
		switch (iconName) {
			case 'GitCompare': return GitCompare;
			case 'GraduationCap': return GraduationCap;
			case 'BookOpen': return BookOpen;
			case 'Globe': return Globe;
			case 'Briefcase': return Briefcase;
			case 'Languages': return Languages;
			case 'Landmark': return Landmark;
			case 'Wallet': return Wallet;
		}
		return MapPin;
	}

	// lucide-svelte ships legacy class-component typings (SvelteComponentTyped);
	// the compare components take Svelte 5 `Component` props, so cast at the seam.
	function asComponent(icon: ReturnType<typeof sourceIcon>): Component {
		return icon as unknown as Component;
	}

	// Type card icon = icon of the first non-'comparison' item_ref_type
	// (falls back to the comparison icon → GitCompare).
	function typeIcon(cmpType: ComparisonType): Component {
		const refTypes = (cmpType.config?.item_ref_types as string[] | undefined) ?? [];
		const primary = refTypes.find((r) => r !== 'comparison') ?? 'comparison';
		return asComponent(sourceIcon(primary));
	}

	function sourceLabel(value: string) {
		return t(`jce.compare.sources.${value}.label`, value);
	}

	function sourcePlural(value: string) {
		return t(`jce.compare.sources.${value}.plural`, value);
	}

	async function create() {
		if (!selectedType || selectedItems.length < 2) return;
		const type = selectedType;
		// C2 pre-flight: the backend caps guest/minted selections at the
		// type's max_item_count — fail locally with the max_items key
		// instead of a round-trip.
		const maxItems = type.config?.max_item_count ?? 0;
		if (maxItems > 0 && selectedItems.length > maxItems) {
			error = 'jce.compare.errors.max_items';
			errorParams = { max: String(maxItems), count: String(selectedItems.length) };
			return;
		}
		creating = true;
		error = '';
		const title = selectedItems.map((i) => i.title).join(t('jce.compare.title_separator'));
		const res = await createQuickComparison({
			title,
			type_id: type.id,
			depth: 'quick' as const,
			status: 'draft' as const,
			visibility: 'private' as const,
			items: selectedItems.map((i) => ({
				title: i.title,
				description: i.description ?? '',
				type_id: type.id,
				item_ref: { type: i.type, id: i.id, title: i.title, slug: (i.metadata?.slug as string) ?? '' }
			})),
			dimensions: [],
			...(selectedFieldIds.size > 0 ? { field_ids: [...selectedFieldIds] } : {})
		});
		creating = false;
		if (res.error) {
			error = res.error.code === 'daily_limit' ? 'jce.compare.errors.daily_limit' : 'jce.compare.errors.create_failed';
			errorParams = { message: res.error.message };
			return;
		}
		const slug = res.data?.slug;
		if (slug) goto(`/compare/${slug}`);
	}

	async function runAiQuick(opts: { focus: string; fields: string; createPost: boolean }) {
		if (!selectedType || selectedItems.length < 2 || aiQuickLoading) return;
		aiQuickLoading = true;
		aiQuickError = '';
		aiQuickResult = null;
		const fields = opts.fields
			.split(',')
			.map((f) => f.trim())
			.filter(Boolean);
		const res = await compareEntities({
			entity_type: selectedType.slug,
			entity_ids: selectedItems.map((i) => i.title),
			...(fields.length > 0 ? { comparison_fields: fields } : {}),
			...(opts.focus.trim() ? { focus: opts.focus.trim() } : {}),
			...(user?.current_geo_space_id ? { geo_space_id: user.current_geo_space_id } : {}),
			create_post: opts.createPost
		});
		aiQuickLoading = false;
		if (res.error || !res.data) {
			aiQuickError = res.error?.message ?? t('jce.compare.ai_quick.errors.unknown');
		} else {
			aiQuickResult = res.data;
		}
	}

	function navigateToMain(path: string) {
		// C11: allowlist the redirect target — same-origin app paths only
		// (a single leading slash, never //evil or a scheme).
		if (!/^\/[^/]/.test(path) && path !== '/') return;
		const target = mainOrigin ?? page.url.origin;
		if (target === page.url.origin) {
			goto(path);
		} else {
			window.location.href = `${target}${path}`;
		}
	}

	async function saveGuestSelectionAndLogin() {
		if (!selectedType || selectedItems.length < 2) return;
		const type = selectedType;
		// C2 pre-flight on token redeem: minting caps at max_item_count, so
		// check before asking the guest to log in.
		const maxItems = type.config?.max_item_count ?? 0;
		if (maxItems > 0 && selectedItems.length > maxItems) {
			error = 'jce.compare.errors.max_items';
			errorParams = { max: String(maxItems), count: String(selectedItems.length) };
			return;
		}
		const itemRefs = selectedItems.map((i) => ({
			type: i.type,
			id: i.id,
			title: i.title,
			slug: (i.metadata?.slug as string) ?? ''
		}));
		const token = await createGuestSelection({ type_slug: type.slug, item_refs: itemRefs });
		if (token) {
			const fieldsParam = selectedFieldIds.size > 0 ? `&fields=${[...selectedFieldIds].join(',')}` : '';
			const returnUrl = encodeURIComponent(`/compare?type=${encodeURIComponent(type.slug)}&compare_token=${token}${fieldsParam}`);
			// Ref: sections/02-authentication-sessions.md (redirect after anonymous selection token)
			navigateToMain(`/login?redirect=${returnUrl}`);
		} else {
			error = 'jce.compare.errors.save_selection';
			errorParams = {};
		}
	}

	async function removeComparison(id: string) {
		deleting = id;
		const res = await deleteComparison(id);
		deleting = null;
		if (res.error) {
			error = 'jce.compare.errors.delete_failed';
			errorParams = { message: res.error.message };
			return;
		}
		goto(page.url.href, { invalidateAll: true });
	}

	async function promote(id: string) {
		promoting = id;
		const res = await promoteComparison(id);
		promoting = null;
		if (res.error) {
			error = res.error.code === 'nested_not_allowed' ? 'jce.compare.errors.nested_not_allowed' : 'jce.compare.errors.promote_failed';
			errorParams = { message: res.error.message };
			return;
		}
		const cmpId = res.data?.id;
		if (cmpId) {
			// Ref: sections/30-generic-comparison-system.md (JCE promotion to detailed composer)
			navigateToMain(`/create?type=comparison&draft=${cmpId}`);
		}
	}

	function formatDate(iso?: string) {
		if (!iso) return '';
		return new Date(iso).toLocaleDateString(i18n.locale, { year: 'numeric', month: 'short', day: 'numeric' });
	}

	let allLoaded = $derived(data.allItemsLoaded === true);
	let visibleItems = $derived(filteredItems(allItems));
	let pageCount = $derived.by(() => {
		if (itemsTotal !== null) return Math.max(1, Math.ceil(itemsTotal / pageSize));
		return currentPage + (visibleItems.length > pageSize ? 1 : 0);
	});
	let effectivePage = $derived(Math.min(currentPage, pageCount));
	let pagedItems = $derived(paginatedItems(visibleItems));
	let Icon = $derived(asComponent(sourceIcon(source)));
	let typeNames = $derived((data.typeNames ?? {}) as Record<string, string>);
	let nestedTypeOptions = $derived([
		{ value: '', label: t('jce.compare.all_nested_types') },
		...allowedNestedTypes.map((s) => ({
			value: s,
			label: typeNames[s] ?? sourceLabel(s)
		}))
	]);

	let tabOptions = $derived<UrlTab[]>([
		{ value: 'new', label: t('jce.compare.tab_new') },
		{ value: 'mine', label: t('jce.compare.tab_mine') }
	]);

	let pageSizeSelectOptions = $derived(
		pageSizeOptions.map((size) => ({
			value: String(size),
			label: `${size} ${t('jce.common.per_page')}`
		}))
	);
</script>

<SeoHead title={`${t('jce.compare.heading')} — ${siteName()}`} description={t('jce.compare.description', 'Compare destinations, services, schools, and organisations side by side — features, costs, and community insights.')} />

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 72rem)/2))">
	<h1 class="sr-only">{t('jce.compare.heading')}</h1>

	<!-- Header — EntityHero directory panel -->
	<EntityHero
		icon={GitCompare}
		heading={t('jce.compare.heading')}
		description={t('compare.hero_desc', 'Pick what to compare — universities, jobs, visa routes and more — and see them side by side.')}
		accent="var(--color-primary)"
		crumbs={heroCrumbs}
		class="mt-4"
	>
		{#snippet actions(c)}
			<div class="grid w-full grid-cols-2 gap-2 sm:flex">
				<a href="/comparisons" class="{c.secondary} flex-1 justify-center whitespace-nowrap">
					<GitCompare size={15} /> {t('jce.comparisons.heading', 'Comparisons')}
				</a>
				{#if user && !mine}
					<a href="/compare?mine=true" class="{c.secondary} flex-1 justify-center whitespace-nowrap">
						{t('jce.compare.tab_mine', 'My comparisons')}
					</a>
				{/if}
				<button type="button" onclick={() => (guideOpen = true)} class="{c.ghost} flex-1 justify-center whitespace-nowrap">
					{t('common.how_it_works', 'How it works')}
					<ChevronRight size={14} class="rtl:-scale-x-100" />
				</button>
			</div>
		{/snippet}
		{#snippet art()}
			<HeroArtCompare accent="var(--color-primary)" />
		{/snippet}
	</EntityHero>

	{#if user && (!selectedType || mine)}
		<ListPageBar>
			<UrlTabs
				tabs={tabOptions}
				value={mine ? 'mine' : 'new'}
				onValueChange={(value) => setTab(value === 'mine')}
				class="!border-b-0 !bg-transparent"
			/>
		</ListPageBar>
	{/if}

	<div class="mx-auto w-full max-w-6xl space-y-6 p-4">
	{#if error}
		<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
			{t(error, undefined, errorParams)}
		</p>
	{/if}

	{#if mine}
		<div class="space-y-4">
			{#if myComparisons.length === 0}
				<EmptyState icon={GitCompare} title={t('jce.compare.empty_mine_body')}>
					<button
						type="button"
						onclick={() => setTab(false)}
						class="mt-1 inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<ArrowRight class="rtl:-scale-x-100" size={16} /> {t('jce.compare.empty_create_one')}
					</button>
				</EmptyState>
			{:else}
				<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
					{#each myComparisons as item (item.id)}
						<article
							class="space-y-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
						>
							<div>
								<a href="/compare/{item.slug}" class="block font-semibold text-[var(--color-text)]">{item.title}</a>
								<div class="mt-2 flex flex-wrap items-center gap-2 text-xs text-[var(--color-muted)]">
									{#if item.type_name}
										<span class="rounded bg-[var(--color-surface-subtle)] px-2 py-0.5">{item.type_name}</span>
									{/if}
									<span class="rounded bg-[var(--color-surface-subtle)] px-2 py-0.5">{t(`jce.compare.depth.${item.depth}`, item.depth)}</span>
								</div>
								<p class="mt-1 text-xs text-[var(--color-muted)]">{formatDate(item.created_at)}</p>
							</div>

							<div class="flex flex-wrap items-center gap-2 border-t border-[var(--color-border)] pt-3">
								<a
									href="/compare/{item.slug}"
									class="inline-flex items-center gap-1 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-border)]"
								>
									<ExternalLink class="rtl:-scale-x-100" size={12} /> {t('jce.common.open')}
								</a>
								<button
									type="button"
									disabled={promoting === item.id}
									onclick={() => promote(item.id)}
									class="inline-flex items-center gap-1 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-border)] disabled:opacity-50"
								>
									<Sparkles size={12} /> {promoting === item.id ? t('jce.common.promoting') : t('jce.common.promote')}
								</button>
								<button
									type="button"
									disabled={deleting === item.id}
									onclick={() => removeComparison(item.id)}
									class="inline-flex items-center gap-1 rounded-[var(--radius)] bg-red-500/10 px-3 py-1.5 text-xs font-medium text-red-500 transition-colors hover:bg-red-500/20 disabled:opacity-50"
								>
									<Trash2 size={12} /> {deleting === item.id ? t('jce.common.deleting') : t('jce.common.delete')}
								</button>
							</div>
						</article>
					{/each}
				</div>
				{#if myMeta && (myMeta.total ?? 0) > defaultPageSize}
					<PaginationControls
						page={currentPage}
						totalPages={Math.max(1, Math.ceil((myMeta.total ?? 0) / defaultPageSize))}
						onPageChange={setPage}
					/>
				{/if}
			{/if}
		</div>
	{:else if !selectedType}
		<!-- Landing: search panel → category grid → trending → AI help -->
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 sm:p-5">
			<div class="mb-3 flex items-center justify-between gap-3">
				<h2 class="text-base font-semibold text-[var(--color-text)]">
					{t('jce.compare.choose_type')}
				</h2>
				{#if categoryTiles.length > 0}
					<button
						type="button"
						onclick={() => (categoriesModalOpen = true)}
						class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						{t('jce.compare.landing.all_categories', 'All categories')}
						<ChevronDown size={14} class="text-[var(--color-muted)]" />
					</button>
				{/if}
			</div>
			<form
				class="flex flex-col gap-3 sm:flex-row"
				onsubmit={(e) => {
					e.preventDefault();
					submitLandingSearch();
				}}
			>
				<div class="relative flex-1">
					<Search size={16} class="absolute start-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" />
					<input
						type="text"
						bind:value={landingSearch}
						placeholder={t('jce.compare.landing.search_placeholder', 'Search countries, universities, visa routes, jobs...')}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-2.5 ps-9 pe-4 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
					/>
				</div>
				<div class="flex items-center gap-2">
					{#each [
						{ v: 'popularity', label: t('jce.compare.landing.sort_popular', 'Popular') },
						{ v: 'trending', label: t('jce.compare.landing.sort_trending', 'Trending') }
					] as const as pill (pill.v)}
						<button
							type="button"
							onclick={() => setTrendingSort(pill.v)}
							aria-pressed={trendingSort === pill.v}
							class="rounded-[var(--radius)] border px-3 py-2 text-sm font-medium transition-colors
								{trendingSort === pill.v
									? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary-text)]'
									: 'border-[var(--color-border)] text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
						>{pill.label}</button>
					{/each}
				</div>
			</form>

			<div class="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
				{#each visibleTiles as tile, i (tile.href)}
					<CompareCategoryCard
						icon={tile.icon}
						tint={tile.tint}
						name={tile.name}
						subtitle={tile.subtitle}
						count={tile.count}
						href={tile.href}
						class={i >= CATEGORY_TILES_MOBILE ? 'hidden sm:flex' : ''}
					/>
				{/each}
			</div>
		</section>

		{#if trending.length > 0}
			<section class="space-y-3">
				<div class="flex items-center justify-between">
					<h2 class="text-sm font-semibold text-[var(--color-text)]">
						{t('jce.compare.landing.trending', 'Trending comparisons')}
						{#if trendingLoading}
							<span class="ms-2 text-xs font-normal text-[var(--color-muted)]">…</span>
						{/if}
					</h2>
					<a href="/comparisons" class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]">
						{t('jce.compare.landing.view_all', 'View all comparisons')}
						<span aria-hidden="true" class="rtl:-scale-x-100">→</span>
					</a>
				</div>
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
					{#each trending as item, i (item.id)}
						<ComparisonVsCard
							summary={item}
							badge={trendingSort === 'popularity' ? badgeFor(item, { popularityIndex: i }) : badgeFor(item)}
							authed={Boolean(user)}
						/>
					{/each}
				</div>
			</section>
		{/if}

		{#if aiQuickEnabled}
			<section class="flex items-center gap-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<span class="grid h-12 w-12 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary-text)]">
					<MessageCircle size={22} />
				</span>
				<div class="min-w-0 flex-1">
					<p class="font-semibold text-[var(--color-text)]">{t('jce.compare.landing.ai_help_title', 'Need help comparing?')}</p>
					<p class="text-sm text-[var(--color-muted)]">{t('jce.compare.landing.ai_help_body', 'I can help you find the right options and answer your questions.')}</p>
				</div>
				<button
					type="button"
					onclick={() => (aiQuickOpen = true)}
					class="shrink-0 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)]"
				>{t('jce.compare.landing.ai_help_cta', 'Chat with Japa AI')}</button>
			</section>
		{/if}
	{:else}
		<!-- Workspace: step header → unified filter row → grid → sticky tray -->
		<StepHeader
			typeName={selectedType.name}
			typeIcon={typeIcon(selectedType)}
			browseOnly={isNestedSource}
			{selectSummary}
			onChangeType={backToLanding}
			onAllCategories={() => (categoriesModalOpen = true)}
			onPickType={() => (typeModalOpen = true)}
		/>

		<!-- Workspace body: field pool rail on the start side (mockup's
			"popular themes"), items grid in the main column -->
		<div class="grid gap-4 {hasFieldPool ? 'lg:grid-cols-[15rem_minmax(0,1fr)]' : ''}">
			{#if hasFieldPool}
				<aside class="order-last lg:order-first lg:sticky lg:top-4 lg:self-start">
					<FieldPoolPicker groups={fieldPool.groups} bind:selected={selectedFieldIds} maxHeight="calc(100vh - 10rem)" />
				</aside>
			{/if}
			<div class="min-w-0 space-y-4">
		<!-- Unified filter row: narrow-by-location fields → search → source switch + sort/display modal -->
		<div class="flex flex-col gap-3 sm:flex-row sm:items-center">
			{#if showNarrow}
				<div class="min-w-0 sm:flex-[2]">
					<ScopeChain
						chain={data.scopeChain ?? []}
						selections={scopeSelections}
						bind:labels={scopeLabels}
						onChange={setScopeSelection}
						modal
					/>
				</div>
			{/if}
			<div class="relative w-full sm:flex-1">
				<Search size={16} class="absolute start-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" />
				<input
					type="text"
					value={query}
					oninput={(e) => setSearch(e.currentTarget.value)}
					placeholder={t('jce.compare.search_placeholder', 'Search {label}...', { label: sourcePlural(source) })}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-2 ps-9 pe-4 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
			</div>

			<div class="flex flex-wrap items-center gap-2">
				{#if itemRefTypes.length > 1}
					<JceSelect
						options={refTypeOptions}
						value={source}
						onchange={(value) => selectSource(value)}
						class="w-full sm:w-auto"
						modal
					/>
				{/if}

				<button
					type="button"
					onclick={() => (sortModalOpen = true)}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					{t('jce.compare.filters.sort_display')}
				</button>
			</div>
		</div>

		<!-- Results count -->
		<p class="text-sm text-[var(--color-muted)]">
			{t('jce.compare.available_count', '{count} {label} available', {
				count: String(itemsTotal ?? pagedItems.length),
				label: sourcePlural(source)
			})}
		</p>

		<!-- Card grid -->
		{#if requiredScopeMissing}
			<EmptyState
				icon={Icon as unknown as IconComponent}
				title={t('jce.compare.narrow.required_prompt', undefined, {
					step: missingScope ? sourceLabel(missingScope) : '',
					label: sourcePlural(source)
				})}
			/>
		{:else if pagedItems.length === 0}
			<EmptyState
				icon={Icon as unknown as IconComponent}
				title={t('jce.compare.no_match', 'No {label} match your search.', { label: sourcePlural(source) })}
			/>
		{:else}
			<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
				{#each pagedItems as item (item.id)}
					{@const comparisonSlug = (item.metadata?.slug as string) ?? ''}
					{#if source === 'comparison'}
						<a
							href="/comparisons/{comparisonSlug || item.id}"
							class="flex flex-col gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-start text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							<div class="flex items-start justify-between gap-3">
								<span
									class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-muted)]"
								>
									<Icon size={20} />
								</span>
								<span class="inline-flex items-center gap-1 text-xs font-medium text-[var(--color-text)]">
									{t('jce.common.view')}
									<ArrowRight class="rtl:-scale-x-100" size={14} />
								</span>
							</div>

							<div class="min-w-0 flex-1">
								<p class="truncate font-medium text-[var(--color-text)]">{item.title}</p>
								{#if item.description}
									<p class="mt-1 line-clamp-2 text-sm text-[var(--color-muted)]">{item.description}</p>
								{/if}
								{#if getMetaChips(item).length > 0}
									<div class="mt-2 flex flex-wrap gap-2 text-xs text-[var(--color-muted)]">
										{#each getMetaChips(item) as chip (chip)}
											<span class="rounded bg-[var(--color-surface-subtle)] px-2 py-0.5">{chip}</span>
										{/each}
									</div>
								{/if}
							</div>
						</a>
					{:else}
						{@const selected = isSelected(item)}
						{@const canSelect = !selected && selectedItems.length < maxItems}
						<button
							type="button"
							onclick={() => toggleItem(item)}
							disabled={!selected && !canSelect}
							class="flex flex-col gap-3 rounded-[var(--radius-card)] border p-4 text-start text-[var(--color-text)] transition-colors disabled:opacity-40 {selected ? 'border-[var(--color-text)] bg-[var(--color-surface-subtle)]' : 'border-[var(--color-border)] bg-[var(--color-surface)] hover:bg-[var(--color-surface-subtle)]'}"
							aria-pressed={selected}
						>
							<div class="flex items-start justify-between gap-3">
								<span
									class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] {selected ? 'bg-[var(--color-surface)]' : 'bg-[var(--color-surface-subtle)]'}"
								>
									{#if selected}
										<Check size={20} class="text-[var(--color-text)]" />
									{:else}
										<Icon size={20} />
									{/if}
								</span>
								{#if selected}
									<span class="text-xs font-medium text-[var(--color-text)]">{t('jce.compare.selected')}</span>
								{/if}
							</div>

							<div class="min-w-0 flex-1">
								<p class="truncate font-medium text-[var(--color-text)]">{item.title}</p>
								{#if item.description}
									<p class="mt-1 line-clamp-2 text-sm text-[var(--color-muted)]">{item.description}</p>
								{/if}
								{#if getMetaChips(item).length > 0}
									<div class="mt-2 flex flex-wrap gap-2 text-xs text-[var(--color-muted)]">
										{#each getMetaChips(item) as chip (chip)}
											<span class="rounded bg-[var(--color-surface-subtle)] px-2 py-0.5">{chip}</span>
										{/each}
									</div>
								{/if}
							</div>
						</button>
					{/if}
				{/each}
			</div>

			<!-- Pagination -->
			{#if pageCount > 1}
				<PaginationControls page={effectivePage} totalPages={pageCount} onPageChange={setPage} />
			{/if}
		{/if}
			</div>
		</div>

		<!-- Sticky zone: AI quick panel + compare tray (hidden in nested browse mode) -->
		{#if !isNestedSource}
			<div class="sticky bottom-20 z-40 -mx-4 space-y-2 bg-[var(--color-bg)] sm:bottom-0">
				{#if aiQuickOpen && aiQuickEnabled}
					<div class="px-4">
						<AiQuickPanel
							loading={aiQuickLoading}
							result={aiQuickResult}
							error={aiQuickError}
							onRun={runAiQuick}
							onClose={() => (aiQuickOpen = false)}
						/>
					</div>
				{/if}
				<CompareTray
					items={selectedItems}
					{maxItems}
					authed={!!user}
					{creating}
					aiEnabled={aiQuickEnabled}
					aiActive={aiQuickOpen}
					hint={trayHint}
					itemLabel={sourcePlural(source)}
					itemIcon={Icon}
					onRemove={removeSelected}
					onCompare={create}
					onLogin={saveGuestSelectionAndLogin}
					onToggleAi={() => (aiQuickOpen = !aiQuickOpen)}
				/>
			</div>
		{/if}
	{/if}
	</div>
</div>

<Modal
	bind:open={categoriesModalOpen}
	title={t('jce.compare.landing.all_categories', 'All categories')}
	icon={LayoutGrid}
	size="max-w-2xl"
>
	{#snippet children()}
		<div class="grid max-h-[65vh] grid-cols-1 gap-3 overflow-y-auto sm:grid-cols-2">
			{#each categoryTiles as tile (tile.href)}
				<CompareCategoryCard
					icon={tile.icon}
					tint={tile.tint}
					name={tile.name}
					subtitle={tile.subtitle}
					count={tile.count}
					href={tile.href}
				/>
			{/each}
		</div>
	{/snippet}
</Modal>

{#snippet typeRow(cmpType: ComparisonType)}
	{@const TIcon = typeIcon(cmpType)}
	<a
		href="/compare?type={cmpType.slug}"
		aria-current={cmpType.slug === selectedType?.slug ? 'page' : undefined}
		class="flex items-center gap-2 rounded-[var(--radius)] px-2 py-1.5 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]
			{cmpType.slug === selectedType?.slug ? 'bg-[var(--color-surface-subtle)] font-medium' : ''}"
	>
		<TIcon size={14} class="shrink-0 text-[var(--color-muted)]" />
		<span class="min-w-0 flex-1 truncate">{cmpType.name}</span>
		{#if cmpType.slug === selectedType?.slug}
			<Check size={14} class="shrink-0 text-[var(--color-primary)]" />
		{/if}
	</a>
{/snippet}

<Modal
	bind:open={typeModalOpen}
	title={t('jce.compare.step.switch_type', 'Switch type')}
	icon={GitCompare}
	size="max-w-md"
>
	{#snippet children()}
		<div class="max-h-[65vh] overflow-y-auto">
			{#if pickerGroup}
				{@const FamIcon = pickerGroup.family.icon}
				<div class="mb-1 flex items-center gap-1.5 px-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
					<FamIcon size={12} />
					{t(`jce.compare.family.${pickerGroup.family.key}`, pickerGroup.family.key)}
				</div>
			{/if}
			<div class="space-y-1">
				{#each pickerTypes as cmpType (cmpType.slug)}
					{@render typeRow(cmpType)}
				{:else}
					<p class="px-2 py-3 text-sm text-[var(--color-muted)]">
						{t('jce.compare.no_types_in_category', 'No other types in this category.')}
					</p>
				{/each}
			</div>
			<button
				type="button"
				onclick={() => {
					typeModalOpen = false;
					categoriesModalOpen = true;
				}}
				class="mt-3 flex w-full items-center justify-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				{t('jce.compare.landing.all_categories', 'All categories')}
			</button>
		</div>
	{/snippet}
</Modal>

<Modal
	bind:open={sortModalOpen}
	title={t('jce.compare.filters.sort_display')}
	icon={SlidersHorizontal}
	size="max-w-md"
>
	{#snippet children()}
		<div class="space-y-3">
			<div class="space-y-1.5">
				<span class="text-xs font-medium text-[var(--color-muted)]">{t('jce.comparisons.sort_placeholder')}</span>
				<JceSelect options={sortOptions} value={sortBy} onchange={(v) => setSort(v)} class="w-full" modal />
			</div>
			{#if isNestedSource && allowNestedComparisons && nestedTypeOptions.length > 1}
				<div class="space-y-1.5">
					<span class="text-xs font-medium text-[var(--color-muted)]">{t('jce.compare.sources.comparison.label')}</span>
					<JceSelect options={nestedTypeOptions} value={nestedType} onchange={(v) => setNestedType(v)} class="w-full" modal />
				</div>
			{/if}
			<div class="space-y-1.5">
				<span class="text-xs font-medium text-[var(--color-muted)]">{t('jce.common.per_page')}</span>
				<JceSelect options={pageSizeSelectOptions} value={String(pageSize)} onchange={(v) => setPageSize(parseInt(v, 10))} class="w-full" modal />
			</div>
		</div>
	{/snippet}
</Modal>

<GuideSlideOver
	bind:open={guideOpen}
	title={effectiveGuide.title}
	body={effectiveGuide.body}
	ctas={effectiveGuide.ctas}
	canManage={data.canManage}
	onSave={saveGuide}
/>
