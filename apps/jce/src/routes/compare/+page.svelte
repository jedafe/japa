<script lang="ts">
	import { page } from '$app/state';
	import { goto, replaceState, invalidateAll } from '$app/navigation';
	import {
		createQuickComparison,
		createGuestSelection,
		deleteComparison,
		promoteComparison,
		previewScore,
		listScoreFactors
	} from '$lib/api/jce';
	import type { ComparisonProfile, ComparisonProfileInput, ComparisonType, ResolvedItem, JCEComparisonSummary, ScoreResult } from '$lib/types/jce';
	import ProfileDrawer from '$lib/components/compare/ProfileDrawer.svelte';
	import ScopeChain from '$lib/components/compare/ScopeChain.svelte';
	import SmartToggle from '$lib/components/compare/SmartToggle.svelte';
	import JceSelect from '$lib/components/ui/JceSelect.svelte';
	import JceTabs from '$lib/components/ui/JceTabs.svelte';
	import ScoreBadge from '$lib/components/compare/ScoreBadge.svelte';
	import ScoreExplanation from '$lib/components/compare/ScoreExplanation.svelte';
	import type { PageData } from './$types';
	import { i18n, t } from '$lib/i18n';
	import {
		GitCompare,
		Search,
		X,
		ArrowRight,
		Trash2,
		ExternalLink,
		Sparkles,
		Check,
		User,
		GraduationCap,
		MapPin,
		Briefcase,
		Languages,
		Globe,
		BookOpen,
		Landmark,
		Wallet
	} from 'lucide-svelte';

	let { data }: { data: PageData } = $props();

	let types = $derived(data.types ?? []);
	let selectedType = $derived(data.selectedType ?? null);
	let profileAllowedGoals = $derived(
		((selectedType?.config as Record<string, unknown> | undefined)?.allowed_goals as
			| { key: string; label: string; default_weight?: number }[]
			| undefined) ?? []
	);
	let source = $derived(data.source ?? 'space');
	let itemRefTypes = $derived(data.itemRefTypes ?? []);
	let allowNestedComparisons = $derived(data.allowNestedComparisons ?? false);
	let allowedNestedTypes = $derived(data.allowedNestedTypes ?? []);
	let allItems = $derived(data.items ?? []);
	let itemsTotal = $derived(data.itemsTotal ?? null);
	let preselectedItems = $derived(data.preselectedItems ?? []);
	let loadError = $derived(data.loadError ?? '');
	let loadErrorParams = $derived(data.loadErrorParams ?? {});
	let user = $derived(data.user ?? null);
	let mainOrigin = $derived(data.mainOrigin);
	let mine = $derived(data.mine ?? false);
	let myComparisons = $derived(data.myComparisons ?? []);

	let profiles = $derived(data.profiles ?? []);
	let activeProfile = $state<ComparisonProfile | null>(null);
	$effect.pre(() => {
		if (activeProfile === null) {
			activeProfile = data.defaultProfile ?? null;
		}
	});

	let smartMode = $state<string>('none');
	let smartModes = $derived.by(() => {
		const cfg = selectedType?.config as Record<string, unknown> | undefined;
		const smart = cfg?.smart_modes as Record<string, unknown> | undefined;
		return smart ? Object.keys(smart) : [];
	});
	let profileScoreFactors = $state<{ key: string; label: string }[]>([]);
	$effect(() => {
		const typeID = selectedType?.id;
		if (!typeID || !user) {
			profileScoreFactors = [];
			return;
		}
		let cancelled = false;
		listScoreFactors(typeID)
			.then((factors) => {
				if (cancelled) return;
				profileScoreFactors = factors.map((f) => ({ key: f.factor_key, label: f.label }));
			})
			.catch(() => {
				if (cancelled) return;
				profileScoreFactors = [];
			});
		return () => {
			cancelled = true;
		};
	});
	let profileOpen = $state(false);

	let previewResult = $state<ScoreResult | null>(null);
	let previewing = $state(false);
	let previewError = $state('');
	let previewTimeout: ReturnType<typeof setTimeout> | null = null;

	let previewTop = $derived.by(() => {
		if (!previewResult?.scores?.length) return null;
		return [...previewResult.scores].sort((a, b) => b.japa_score - a.japa_score)[0];
	});

	// Snapshot the initial URL params; client-side filter state is updated manually.
	const searchParams = page.url.searchParams;
	// Page-size and sort options come from the comparison type config (server-side,
	// with env defaults applied) and fall back to the env defaults.
	let pageSizeOptions = $derived(data.pageSizeOptions ?? []);
	let defaultPageSize = $derived(data.defaultPageSize);
	let defaultPageSizeOptions = $derived(data.defaultPageSizeOptions);
	let sortValues = $derived.by(() => {
		const configured = selectedType?.config?.sort_options;
		return Array.isArray(configured) ? configured : [];
	});

	const initialQuery = searchParams.get('q') ?? '';
	const initialSort = searchParams.get('sort') ?? 'title';
	const initialPage = parseInt(searchParams.get('page') ?? '1', 10) || 1;
	const initialNestedType = searchParams.get('nested_type') ?? '';

	let query = $state(initialQuery);
	let sortBy = $state(initialSort);
	let pageSize = $state(0);
	$effect.pre(() => {
		const next =
			parseInt(page.url.searchParams.get('page_size') ?? String(defaultPageSize), 10) || defaultPageSize;
		if (next !== pageSize) pageSize = next;
	});
	let currentPage = $state(initialPage);
	let nestedType = $state(initialNestedType);
	let selectedItems = $state<ResolvedItem[]>([]);
	let searchTimeout: ReturnType<typeof setTimeout> | null = null;

	let scopeSelections = $state<Record<string, string>>({});
	$effect(() => {
		// Sync local scope state with the server-loaded page data on every navigation.
		scopeSelections = Object.fromEntries(
			Object.entries(data.scopeParams ?? {}).filter(([key]) => (data.scopeChain ?? []).includes(key))
		);
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

	const maxItems = $derived((selectedType?.config?.max_item_count as number) ?? 4);

	let sortOptions = $derived(
		sortValues.map((value) => ({ value, label: t(`jce.compare.sort.${value}`) }))
	);

	$effect.pre(() => {
		if (selectedItems.length === 0 && preselectedItems.length > 0) {
			selectedItems = [...preselectedItems];
		}
	});

	$effect(() => {
		if (loadError) {
			error = loadError;
			errorParams = loadErrorParams;
		}
	});

	function selectType(nextType: ComparisonType) {
		selectedItems = [];
		query = '';
		sortBy = 'title';
		scopeSelections = {};
		const nextOptions = (nextType.config?.page_size_options as number[] | undefined) ?? defaultPageSizeOptions;
		pageSize = nextOptions.includes(defaultPageSize) ? defaultPageSize : (nextOptions[0] ?? defaultPageSize);
		currentPage = 1;
		nestedType = '';
		const url = new URL(page.url);
		url.searchParams.set('type', nextType.slug);
		url.searchParams.delete('mine');
		url.searchParams.delete('q');
		url.searchParams.delete('sort');
		url.searchParams.delete('page_size');
		url.searchParams.delete('page');
		url.searchParams.delete('source');
		url.searchParams.delete('nested_type');
		for (const step of data.scopeChain ?? []) {
			url.searchParams.delete(scopeParamFor(step));
		}
		url.searchParams.set('page', '1');
		goto(url, { replaceState: true });
	}

	function selectSource(next: string) {
		selectedItems = [];
		query = '';
		sortBy = 'title';
		scopeSelections = {};
		pageSize = pageSizeOptions.includes(defaultPageSize) ? defaultPageSize : (pageSizeOptions[0] ?? defaultPageSize);
		currentPage = 1;
		nestedType = '';
		const url = new URL(page.url);
		url.searchParams.set('source', next);
		url.searchParams.delete('nested_type');
		url.searchParams.delete('q');
		url.searchParams.delete('sort');
		url.searchParams.delete('page_size');
		url.searchParams.delete('page');
		for (const step of data.scopeChain ?? []) {
			url.searchParams.delete(scopeParamFor(step));
		}
		url.searchParams.set('page', '1');
		goto(url, { replaceState: true });
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

		// Reset downstream selections so we don't keep stale IDs.
		const idx = selectedType?.config?.scope_chain?.indexOf(step) ?? -1;
		if (idx >= 0 && selectedType?.config?.scope_chain) {
			for (const next of selectedType.config.scope_chain.slice(idx + 1)) {
				delete scopeSelections[next];
			}
		}
		scopeSelections = { ...scopeSelections };

		currentPage = 1;
		const url = new URL(page.url);
		const param = scopeParamFor(step);
		if (value) {
			url.searchParams.set(param, value);
		} else {
			url.searchParams.delete(param);
		}
		// Also remove downstream params.
		if (idx >= 0 && selectedType?.config?.scope_chain) {
			for (const next of selectedType.config.scope_chain.slice(idx + 1)) {
				url.searchParams.delete(scopeParamFor(next));
			}
		}
		url.searchParams.set('page', '1');
		goto(url, { replaceState: true });
	}

	function clearScope() {
		scopeSelections = {};
		currentPage = 1;
		const url = new URL(page.url);
		for (const step of data.scopeChain ?? []) {
			url.searchParams.delete(scopeParamFor(step));
		}
		url.searchParams.set('page', '1');
		goto(url, { replaceState: true });
	}

	// updateURL is for client-side-only state (sort) that does not need a server reload.
	function updateURL(updates: Record<string, string | null>) {
		// Use the live browser URL so rapid client-side updates don't race the SvelteKit page store.
		const url = new URL(window.location.href);
		for (const [k, v] of Object.entries(updates)) {
			if (v === null || v === '') url.searchParams.delete(k);
			else url.searchParams.set(k, v);
		}
		replaceState(url, {});
	}

	// navigateURL triggers a SvelteKit client-side navigation so the +page.server.ts load re-runs
	// with the updated query (search, page, page-size, nested-type filters, etc.).
	function navigateURL(updates: Record<string, string | null>) {
		const url = new URL(window.location.href);
		for (const [k, v] of Object.entries(updates)) {
			if (v === null || v === '') url.searchParams.delete(k);
			else url.searchParams.set(k, v);
		}
		goto(url, { replaceState: true, keepFocus: true });
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
		updateURL({ sort, page: null });
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
		const url = new URL(page.url);
		if (isMine) url.searchParams.set('mine', 'true');
		else url.searchParams.delete('mine');
		url.searchParams.delete('page');
		goto(url, { replaceState: true });
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
		if (meta.level) chips.push(String(meta.level));
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

	// The server already applies pagination (limit/offset) and returns one extra item so we
	// can infer hasNext without a total count. We only need to cap the visible list at pageSize.
	function paginatedItems(items: ResolvedItem[]): ResolvedItem[] {
		return items.slice(0, pageSize);
	}

	function isSelected(item: ResolvedItem) {
		return selectedItems.some((s) => s.id === item.id && s.type === item.type);
	}

	function toggleItem(item: ResolvedItem) {
		if (isSelected(item)) {
			selectedItems = selectedItems.filter((s) => s.id !== item.id);
		} else if (selectedItems.length < maxItems) {
			selectedItems = [...selectedItems, item];
		}
	}

	function removeSelected(item: ResolvedItem) {
		selectedItems = selectedItems.filter((s) => s.id !== item.id);
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

	function sourceLabel(value: string) {
		return t(`jce.compare.sources.${value}.label`, value);
	}

	function sourcePlural(value: string) {
		return t(`jce.compare.sources.${value}.plural`, value);
	}

	function schedulePreview() {
		if (previewTimeout) clearTimeout(previewTimeout);
		if (!selectedType || !user || selectedItems.length < 2) {
			previewResult = null;
			return;
		}
		previewTimeout = setTimeout(async () => {
			previewing = true;
			previewError = '';
			try {
				const type = selectedType;
				const refs = selectedItems.map((i) => ({
					type: i.type,
					id: i.id,
					title: i.title,
					slug: (i.metadata?.slug as string) ?? ''
				}));
				const profileInput: ComparisonProfileInput | undefined = activeProfile
					? {
							title: activeProfile.title,
							source_space_id: activeProfile.source_space_id,
							target_space_ids: activeProfile.target_space_ids,
							goals: activeProfile.goals,
							preferences: activeProfile.preferences,
							demographics: activeProfile.demographics,
							is_default: activeProfile.is_default,
							is_template: activeProfile.is_template
					  }
					: undefined;
				previewResult = await previewScore({
					type_id: type.id,
					smart_mode: smartMode,
					item_refs: refs,
					profile: profileInput
				});
			} catch (e) {
				previewError = e instanceof Error ? e.message : t('jce.compare.errors.unknown');
				previewResult = null;
			} finally {
				previewing = false;
			}
		}, 400);
	}

	$effect(() => {
		// Track dependencies for live preview.
		const _ = selectedItems.length + selectedItems.map((i) => i.id).join(',');
		const _m = smartMode;
		const _p = activeProfile?.id;
		if (!selectedType || !user || selectedItems.length < 2) {
			previewResult = null;
			return;
		}
		schedulePreview();
		return () => {
			if (previewTimeout) clearTimeout(previewTimeout);
		};
	});

	async function create() {
		if (!selectedType || selectedItems.length < 2) return;
		const type = selectedType;
		creating = true;
		error = '';
		try {
			const title = selectedItems.map((i) => i.title).join(t('jce.compare.title_separator'));
			const input = {
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
				smart_mode: smartMode,
				config: { profile_id: activeProfile?.id ?? null }
			};
			const cmp = await createQuickComparison(input);
			if (cmp?.slug) goto(`/compare/${cmp.slug}`);
		} catch (e) {
			error = 'jce.compare.errors.create_failed';
			errorParams = { message: e instanceof Error ? e.message : t('jce.compare.errors.unknown') };
		} finally {
			creating = false;
		}
	}

	function navigateToMain(path: string) {
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
		const itemRefs = selectedItems.map((i) => ({
			type: i.type,
			id: i.id,
			title: i.title,
			slug: (i.metadata?.slug as string) ?? ''
		}));
		const token = await createGuestSelection({ type_slug: type.slug, item_refs: itemRefs });
		if (token) {
			const returnUrl = encodeURIComponent(`/compare?type=${encodeURIComponent(type.slug)}&compare_token=${token}`);
			// Ref: sections/02-authentication-sessions.md (redirect after anonymous selection token)
			navigateToMain(`/login?redirect=${returnUrl}`);
		}
	}

	async function removeComparison(id: string) {
		deleting = id;
		try {
			await deleteComparison(id);
			goto(page.url, { invalidateAll: true });
		} catch (e) {
			error = 'jce.compare.errors.delete_failed';
			errorParams = { message: e instanceof Error ? e.message : t('jce.compare.errors.unknown') };
		} finally {
			deleting = null;
		}
	}

	async function promote(id: string) {
		promoting = id;
		try {
			const cmp = await promoteComparison(id);
			if (cmp?.id) {
				// Ref: sections/30-generic-comparison-system.md (JCE promotion to detailed composer)
				navigateToMain(`/create?type=comparison&draft=${cmp.id}`);
			}
		} catch (e) {
			error = 'jce.compare.errors.promote_failed';
			errorParams = { message: e instanceof Error ? e.message : t('jce.compare.errors.unknown') };
		} finally {
			promoting = null;
		}
	}

	function formatDate(iso?: string) {
		if (!iso) return '';
		return new Date(iso).toLocaleDateString(i18n.locale, { year: 'numeric', month: 'short', day: 'numeric' });
	}

	let visibleItems = $derived(filteredItems(allItems));
	let pageCount = $derived.by(() => {
		if (itemsTotal !== null) return Math.max(1, Math.ceil(itemsTotal / pageSize));
		return currentPage + (visibleItems.length > pageSize ? 1 : 0);
	});
	let effectivePage = $derived(Math.min(currentPage, pageCount));
	let pagedItems = $derived(paginatedItems(visibleItems));
	let Icon = $derived(sourceIcon(source));
	let nestedTypeOptions = $derived([
		{ value: '', label: t('jce.compare.all_nested_types') },
		...allowedNestedTypes.map((s) => {
			const foundType = types.find((x) => x.slug === s);
			return { value: s, label: foundType?.name ?? sourceLabel(s) };
		})
	]);

	let tabOptions = $derived([
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

<div class="max-w-6xl mx-auto p-4 sm:p-6 space-y-6">
	<div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
		<h1 class="text-2xl sm:text-3xl font-bold" style="color: var(--color-text)">
			<GitCompare size={28} class="inline-block mr-2" /> {t('jce.compare.heading')}
		</h1>
	</div>

	{#if user}
		<JceTabs
			tabs={tabOptions}
			value={mine ? 'mine' : 'new'}
			onValueChange={(value) => setTab(value === 'mine')}
		/>
	{/if}

	{#if error}
		<div class="rounded-[var(--radius-button)] p-4 text-sm" role="alert" style="background-color: rgba(239,68,68,0.1); color: #ef4444; border: 1px solid rgba(239,68,68,0.2)">
			{t(error, undefined, errorParams)}
		</div>
	{/if}

	{#if mine}
		<div class="space-y-4">
			{#if myComparisons.length === 0}
				<div class="rounded-[var(--radius-card)] border p-8 text-center" style="background-color: var(--color-surface); border-color: var(--color-border)">
					<p style="color: var(--color-muted)">{t('jce.compare.empty_mine_body')}</p>
					<button
						type="button"
						onclick={() => setTab(false)}
						class="mt-4 inline-flex items-center gap-2 px-4 py-2 rounded-[var(--radius-button)] text-sm font-medium"
						style="background-color: var(--color-bg); border: 1px solid var(--color-border); color: var(--color-text)"
					>
						<ArrowRight size={16} /> {t('jce.compare.empty_create_one')}
					</button>
				</div>
			{:else}
				<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
					{#each myComparisons as item (item.id)}
						<article
							class="rounded-[var(--radius-card)] border p-4 space-y-3"
							style="background-color: var(--color-surface); border-color: var(--color-border)"
						>
							<div>
								<a href="/compare/{item.slug}" class="block font-semibold" style="color: var(--color-text)">{item.title}</a>
								<div class="flex flex-wrap items-center gap-2 text-xs mt-2" style="color: var(--color-muted)">
									{#if item.type_name}
										<span class="rounded px-2 py-0.5" style="background-color: var(--color-bg)">{item.type_name}</span>
									{/if}
									<span class="rounded px-2 py-0.5" style="background-color: var(--color-bg)">{item.depth}</span>
								</div>
								<p class="text-xs mt-1" style="color: var(--color-muted)">{formatDate(item.created_at)}</p>
							</div>

							<div class="flex flex-wrap items-center gap-2 pt-3 border-t" style="border-color: var(--color-border)">
								<a
									href="/compare/{item.slug}"
									class="inline-flex items-center gap-1 px-3 py-1.5 rounded-[var(--radius-button)] text-xs font-medium"
									style="background-color: var(--color-bg); color: var(--color-text)"
								>
									<ExternalLink size={12} /> {t('jce.common.open')}
								</a>
								<button
									type="button"
									disabled={promoting === item.id}
									onclick={() => promote(item.id)}
									class="inline-flex items-center gap-1 px-3 py-1.5 rounded-[var(--radius-button)] text-xs font-medium disabled:opacity-50"
									style="background-color: var(--color-bg); color: var(--color-text)"
								>
									<Sparkles size={12} /> {promoting === item.id ? t('jce.common.promoting') : t('jce.common.promote')}
								</button>
								<button
									type="button"
									disabled={deleting === item.id}
									onclick={() => removeComparison(item.id)}
									class="inline-flex items-center gap-1 px-3 py-1.5 rounded-[var(--radius-button)] text-xs font-medium disabled:opacity-50"
									style="background-color: rgba(239,68,68,0.1); color: #ef4444"
								>
									<Trash2 size={12} /> {deleting === item.id ? t('jce.common.deleting') : t('jce.common.delete')}
								</button>
							</div>
						</article>
					{/each}
				</div>
			{/if}
		</div>
	{:else}
		<!-- Type selector -->
		<section class="space-y-3">
			<h2 class="text-sm font-semibold" style="color: var(--color-text)">{t('jce.compare.choose_type')}</h2>
			<div class="flex flex-wrap gap-2">
				{#each types as cmpType (cmpType.id)}
					<button
						type="button"
						class="px-4 py-2 rounded-[var(--radius-button)] border text-sm font-medium transition-colors"
						style="border-color: var(--color-border); color: var(--color-text); background-color: {selectedType?.id === cmpType.id ? 'var(--color-bg)' : 'var(--color-surface)'}"
						onclick={() => selectType(cmpType)}
					>
						{cmpType.name}
					</button>
				{/each}
			</div>
		</section>

		{#if selectedType && itemRefTypes.length > 1}
			<!-- Source selector (e.g. Country vs existing comparison) -->
			<section class="space-y-3">
				<h2 class="text-sm font-semibold" style="color: var(--color-text)">{t('jce.compare.select_source')}</h2>
				<div class="flex flex-wrap gap-2">
					{#each itemRefTypes as refType (refType)}
						<button
							type="button"
							class="px-3 py-1.5 rounded-[var(--radius-button)] border text-sm font-medium transition-colors"
							style="border-color: var(--color-border); color: var(--color-text); background-color: {source === refType ? 'var(--color-bg)' : 'var(--color-surface)'}"
							onclick={() => selectSource(refType)}
						>
							{sourceLabel(refType)}
						</button>
					{/each}
				</div>
			</section>
		{/if}

		{#if selectedType}
			{#if source !== 'comparison'}
				<!-- Selected panel -->
				<section
					class="rounded-[var(--radius-card)] border p-4 space-y-3"
					style="background-color: var(--color-surface); border-color: var(--color-border)"
				>
					<div class="flex items-center justify-between gap-4 flex-wrap">
					<div>
						<h2 class="font-semibold" style="color: var(--color-text)">
							{t('jce.compare.selected_count', 'Selected {count}/{max}', {
								count: selectedItems.length,
								max: maxItems
							})}
						</h2>
						<p class="text-sm" style="color: var(--color-muted)">
							{t('jce.compare.pick_items', 'Pick 2–{max} {label} items to compare.', {
								max: maxItems,
								label: sourceLabel(source)
							})}
						</p>
					</div>
					{#if selectedItems.length >= 2}
						{#if user}
							<button
								type="button"
								disabled={creating}
								onclick={create}
								class="inline-flex items-center gap-2 px-5 py-2 rounded-[var(--radius-button)] text-sm font-medium disabled:opacity-50"
								style="background-color: var(--color-bg); border: 1px solid var(--color-border); color: var(--color-text)"
							>
								{creating ? t('jce.compare.creating') : t('jce.compare.compare_now')}
								<ArrowRight size={16} />
							</button>
						{:else}
							<button
								type="button"
								onclick={saveGuestSelectionAndLogin}
								class="inline-flex items-center gap-2 px-5 py-2 rounded-[var(--radius-button)] text-sm font-medium"
								style="background-color: var(--color-bg); border: 1px solid var(--color-border); color: var(--color-text)"
							>
								{t('jce.compare.login_to_compare')}
							</button>
						{/if}
					{/if}
				</div>

				{#if user}
					<div class="flex items-center gap-3 flex-wrap">
						<div class="flex items-center gap-2">
							<span class="text-xs font-medium" style="color: var(--color-muted)">
								{t('jce.smart_mode.title')}
							</span>
							<SmartToggle bind:mode={smartMode} modes={smartModes} />
						</div>
						<button
							type="button"
							onclick={() => (profileOpen = true)}
							class="inline-flex items-center gap-1.5 rounded-[var(--radius-button)] border px-3 py-1.5 text-xs font-medium"
							style="border-color: var(--color-border); background-color: var(--color-bg); color: var(--color-text)"
						>
							<User size={14} />
							{activeProfile?.title ?? t('jce.profile.title')}
						</button>
					</div>

					{#if previewing || previewError || previewResult}
						<div
							class="rounded-[var(--radius-card)] border p-3 text-sm space-y-2"
							style="background-color: var(--color-bg); border-color: var(--color-border); color: var(--color-text)"
						>
							{#if previewing}
								<span style="color: var(--color-muted)">{t('jce.compare.score.loading')}</span>
							{:else if previewError}
								<span style="color: #ef4444">{previewError}</span>
							{:else if previewResult && previewTop}
								<div class="font-medium" style="color: var(--color-text)">
									{t('jce.score.best_match')}: {previewTop.item_title}
								</div>
								<div class="flex flex-wrap gap-3">
									{#each previewResult.scores as score (score.item_id)}
										<div class="flex items-center gap-2">
											<ScoreBadge {score} best={score.item_id === previewTop.item_id} />
											<span style="color: var(--color-text)">{score.item_title}</span>
										</div>
									{/each}
								</div>
							{/if}
						</div>
					{/if}
				{/if}

				{#if selectedItems.length > 0}
					<div class="flex flex-wrap gap-2">
						{#each selectedItems as item (item.id)}
							<span
								class="inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-sm"
								style="background-color: var(--color-bg); border-color: var(--color-border); color: var(--color-text)"
							>
								<Icon size={12} />
								{item.title}
								<button
									type="button"
									onclick={() => removeSelected(item)}
									class="rounded-full p-0.5"
									style="color: var(--color-muted)"
								>
									<X size={13} />
								</button>
							</span>
						{/each}
					</div>
				{/if}
			</section>
			{/if}

			{#if user}
				<ProfileDrawer
					bind:open={profileOpen}
					{profiles}
					bind:activeProfile
					scoreFactors={profileScoreFactors}
					allowedGoals={profileAllowedGoals}
					onsave={async (p: ComparisonProfile) => {
						activeProfile = p;
						profileOpen = false;
						try {
							await invalidateAll();
						} catch (e) {
							console.error('Failed to refresh profiles:', e);
							error = 'jce.compare.errors.load_profiles';
							errorParams = { message: e instanceof Error ? e.message : t('jce.compare.errors.unknown') };
						}
					}}
				/>
			{/if}

			{#if data.scopeChain?.length}
				<div class="flex flex-col gap-2">
					<div class="flex items-center justify-between gap-3">
						<span class="text-sm font-medium" style="color: var(--color-text)">
							{t('jce.compare.scope.title')}
						</span>
						<button
							type="button"
							onclick={clearScope}
							class="text-xs"
							style="color: var(--color-muted)"
						>
							{t('jce.common.clear')}
						</button>
					</div>
					<ScopeChain chain={data.scopeChain} selections={scopeSelections} onChange={setScopeSelection} />
				</div>
			{/if}

			<!-- Controls -->
			<div class="flex flex-col sm:flex-row gap-3 items-start sm:items-center justify-between">
				<div class="relative w-full sm:w-72">
					<Search size={16} class="absolute left-3 top-1/2 -translate-y-1/2" style="color: var(--color-muted)" />
					<input
						type="text"
						value={query}
						oninput={(e) => setSearch(e.currentTarget.value)}
						placeholder={t('jce.compare.search_placeholder', 'Search {label}...', { label: sourcePlural(source) })}
						class="w-full rounded-[var(--radius-button)] border bg-[var(--color-surface)] pl-9 pr-4 py-2 text-sm outline-none"
						style="border-color: var(--color-border); color: var(--color-text)"
					/>
				</div>

				<div class="flex gap-2 items-center">
					{#if source === 'comparison' && allowNestedComparisons && nestedTypeOptions.length > 1}
						<JceSelect
							options={nestedTypeOptions}
							value={nestedType}
							onchange={(value) => setNestedType(value)}
							class="w-auto"
						/>
					{/if}
					<JceSelect
						options={sortOptions}
						value={sortBy}
						onchange={(value) => setSort(value)}
						class="w-auto"
					/>

					<JceSelect
						options={pageSizeSelectOptions}
						value={String(pageSize)}
						onchange={(value) => setPageSize(parseInt(value, 10))}
						class="w-auto"
					/>
				</div>
			</div>

			<!-- Results count -->
			<p class="text-sm" style="color: var(--color-muted)">
				{t('jce.compare.available_count', '{count} {label} available', {
					count: String(itemsTotal ?? pagedItems.length),
					label: sourcePlural(source)
				})}
			</p>

			<!-- Card grid -->
			{#if pagedItems.length === 0}
				<div class="rounded-[var(--radius-card)] border p-8 text-center" style="background-color: var(--color-surface); border-color: var(--color-border)">
					<Icon size={40} class="mx-auto mb-3" style="color: var(--color-muted)" />
					<p style="color: var(--color-muted)">
						{t('jce.compare.no_match', 'No {label} match your search.', { label: sourcePlural(source) })}
					</p>
				</div>
			{:else}
				<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
					{#each pagedItems as item (item.id)}
						{@const comparisonSlug = (item.metadata?.slug as string) ?? ''}
						{#if source === 'comparison'}
							<a
								href="/comparisons/{comparisonSlug || item.id}"
								class="rounded-[var(--radius-card)] border p-4 text-left transition-colors flex flex-col gap-3 hover:border-[var(--color-text)]"
								style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-text)"
							>
								<div class="flex items-start justify-between gap-3">
									<span
										class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius-button)]"
										style="background-color: var(--color-bg); color: var(--color-muted)"
									>
										<Icon size={20} />
									</span>
									<span class="inline-flex items-center gap-1 text-xs font-medium" style="color: var(--color-text)">
										{t('jce.common.view')}
										<ArrowRight size={14} />
									</span>
								</div>

								<div class="min-w-0 flex-1">
									<p class="font-medium truncate" style="color: var(--color-text)">{item.title}</p>
									{#if item.description}
										<p class="text-sm line-clamp-2 mt-1" style="color: var(--color-muted)">{item.description}</p>
									{/if}
									{#if getMetaChips(item).length > 0}
										<div class="flex flex-wrap gap-2 mt-2 text-xs" style="color: var(--color-muted)">
											{#each getMetaChips(item) as chip (chip)}
												<span class="rounded px-2 py-0.5" style="background-color: var(--color-bg)">{chip}</span>
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
								class="rounded-[var(--radius-card)] border p-4 text-left transition-colors flex flex-col gap-3 disabled:opacity-40"
								style="border-color: {selected ? 'var(--color-text)' : 'var(--color-border)'}; background-color: {selected ? 'var(--color-bg)' : 'var(--color-surface)'}; color: var(--color-text)"
								aria-pressed={selected}
							>
								<div class="flex items-start justify-between gap-3">
									<span
										class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius-button)]"
										style="background-color: {selected ? 'var(--color-surface)' : 'var(--color-bg)'}; color: var(--color-muted)"
									>
										{#if selected}
											<Check size={20} style="color: var(--color-text)" />
										{:else}
											<Icon size={20} />
										{/if}
									</span>
									{#if selected}
										<span class="text-xs font-medium" style="color: var(--color-text)">{t('jce.compare.selected')}</span>
									{/if}
								</div>

								<div class="min-w-0 flex-1">
									<p class="font-medium truncate" style="color: var(--color-text)">{item.title}</p>
									{#if item.description}
										<p class="text-sm line-clamp-2 mt-1" style="color: var(--color-muted)">{item.description}</p>
									{/if}
									{#if getMetaChips(item).length > 0}
										<div class="flex flex-wrap gap-2 mt-2 text-xs" style="color: var(--color-muted)">
											{#each getMetaChips(item) as chip (chip)}
												<span class="rounded px-2 py-0.5" style="background-color: var(--color-bg)">{chip}</span>
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
					<div class="flex items-center justify-between pt-4">
						<p class="text-sm" style="color: var(--color-muted)">
							{t('jce.common.page_of', 'Page {current} of {total}')
								.replace('{current}', String(effectivePage))
								.replace('{total}', String(pageCount))}
						</p>
						<div class="flex gap-2">
							<button
								type="button"
								disabled={effectivePage <= 1}
								onclick={() => setPage(effectivePage - 1)}
								class="px-3 py-1.5 rounded-[var(--radius-button)] text-sm font-medium disabled:opacity-50"
								style="background-color: var(--color-bg); border: 1px solid var(--color-border); color: var(--color-text)"
							>
								{t('jce.common.previous')}
							</button>
							<button
								type="button"
								disabled={effectivePage >= pageCount}
								onclick={() => setPage(effectivePage + 1)}
								class="px-3 py-1.5 rounded-[var(--radius-button)] text-sm font-medium disabled:opacity-50"
								style="background-color: var(--color-bg); border: 1px solid var(--color-border); color: var(--color-text)"
							>
								{t('jce.common.next')}
							</button>
						</div>
					</div>
				{/if}
			{/if}
		{:else}
			<div class="rounded-[var(--radius-card)] border p-8 text-center" style="background-color: var(--color-surface); border-color: var(--color-border)">
				<p style="color: var(--color-muted)">{t('jce.compare.no_type_selected')}</p>
			</div>
		{/if}
	{/if}
</div>
