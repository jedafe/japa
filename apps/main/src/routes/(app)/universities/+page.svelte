<script lang="ts">
	import { untrack } from 'svelte';
	import { enhance } from '$app/forms';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { navigating } from '$app/state';
	import { Filter, GraduationCap, Users, FileText, Heart, LayoutGrid, List, X, Search, Globe, ChevronRight, GitCompare, Loader2 } from 'lucide-svelte';
	import {
		EmptyState,
		EntityHero,
		LoadingSkeleton,
		PaginationControls,
		SearchInput,
		SelectField,
		DirectoryCard,
		SpaceJoinButton,
		ListPageBar,
		type SearchSuggestion,
		type CardMetaItem
	} from '$lib/components/ui';
	import { GuideSlideOver, Modal, type GuideContent } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import HeroArtUniversities from '$lib/components/heroes/HeroArtUniversities.svelte';
	import { formatMetric } from '$lib/components/hub/charts/format';
	import type { University } from '$lib/types';
	import type { PageData, ActionData } from './$types';
	import { destinationMarkets, pinMarketsFirst } from '$lib/utils/geoMarkets';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data, form }: { data: PageData; form?: ActionData } = $props();

	const title = $derived(t('universities.title', 'Universities — {site}'));
	const description = $derived(
		t('universities.description', 'Explore universities relevant to the diaspora community.')
	);

	// Server-paginated slice for the current page (filter/sort/page are
	// server-side — the backend honors ?q=, ?country=, ?sort=, ?page=, ?limit=).
	// Load more — client-side appends (replaces pagination); resets when the
	// SSR list changes (filter/sort/search navigation reloads data).
	let allUnis = $state<University[]>(untrack(() => data.universities));
	let loadingMore = $state(false);
	let uniItems = $state<University[]>(untrack(() => data.universities));
	$effect(() => {
		const fresh = data.universities;
		untrack(() => {
			allUnis = fresh;
			uniItems = fresh;
		});
	});
	const paged = $derived<University[]>(allUnis);

	async function loadMore() {
		if (loadingMore || allUnis.length >= total) return;
		loadingMore = true;
		try {
			const params = new URLSearchParams(page.url.searchParams);
			params.set('page', String(Math.floor(allUnis.length / data.pageSize) + 1));
			params.set('limit', String(data.pageSize));
			const res = await api.get<{ items?: University[]; data?: University[] } | University[]>(
				`/universities?${params.toString()}`
			);
			const payload = (res as { data?: unknown }).data ?? res;
			const next: University[] = Array.isArray(payload) ? payload : ((payload as { items?: University[] }).items ?? []);
			const seen = new Set(allUnis.map((u) => u.id));
			allUnis = [...allUnis, ...next.filter((u) => !seen.has(u.id))];
		} finally {
			loadingMore = false;
		}
	}

	// Compare selection — same pattern as /jobs: up to 4 unis -> JCE compare.
	let selectedUniSlugs = $state<Set<string>>(new Set());
	let savingJCE = $state(false);
	const canCompare = $derived(selectedUniSlugs.size >= 2 && selectedUniSlugs.size <= 4);
	const selectedUnis = $derived<University[]>(allUnis.filter((u) => selectedUniSlugs.has(u.slug)));

	function toggleUni(slug: string) {
		const next = new Set(selectedUniSlugs);
		if (next.has(slug)) next.delete(slug);
		else if (next.size < 4) next.add(slug);
		selectedUniSlugs = next;
	}
	const total = $derived(data.meta?.total ?? data.universities.length);
	const totalPages = $derived(data.meta?.total_pages ?? 1);
	const currentPage = $derived(data.meta?.page ?? (parseInt(page.url.searchParams.get('page') ?? '1', 10) || 1));

	// URL is the source of truth for filter / sort / page state. The country
	// filter falls back to the geo-aware default (ADR-064) resolved server-side
	// when no explicit ?country= param is present — "a UK visitor sees UK
	// universities first". An explicit param always wins.
	const qParam = $derived(page.url.searchParams.get('q') ?? '');
	const countryParam = $derived(page.url.searchParams.get('country') ?? '');
	const stateParam = $derived(page.url.searchParams.get('state') ?? '');
	const cityParam = $derived(page.url.searchParams.get('city') ?? '');
	const effectiveCountry = $derived(countryParam || data.countryDefault || '');
	const sortParam = $derived(page.url.searchParams.get('sort') ?? 'newest');

	// Country options are sourced from the canonical /geo/countries list
	// loaded server-side (ADR-005). The dropdown now shows all 250 primary
	// geo-space countries and filters client-side via Autocomplete.
	const allLabel = t('universities.all_countries', 'All countries');
	const allStatesLabel = t('universities.all_states', 'All states');
	const allCitiesLabel = t('universities.all_cities', 'All cities');

	const sortOptions = $derived([
		{ value: 'newest', label: t('universities.sort_newest', 'Newest') },
		{ value: 'name', label: t('universities.sort_name', 'Name (A–Z)') },
		// §05 amendment — universities are follow-only primary spaces; the
		// meaningful count is followers, not (auto-joined) members.
		{ value: 'followers', label: t('universities.sort_followers', 'Most followers') },
		{ value: 'posts', label: t('universities.sort_posts', 'Most posts') }
	]);

	// Panel-active = explicit filters only: the geo-aware country default and
	// the always-visible search box don't count (they're not in the panel).
	const hasActiveFilters = $derived(
		!!countryParam || !!stateParam || !!cityParam || sortParam !== 'newest'
	);

	// Filter panel (ListPageBar + FilterToggle pattern — same as /blog, /guides).
	let filtersModalOpen = $state(false);
	let searchModalOpen = $state(false);
	const showingStart = $derived(total === 0 ? 0 : (currentPage - 1) * data.pageSize + 1);
	const showingEnd = $derived(Math.min(currentPage * data.pageSize, total));

	// University avatar text — seeded initials can be long codes (e.g.
	// "VERMONTLAW"); anything over 5 chars overflows the badge, so fall back
	// to the first letter of the name.
	const uniMark = (uni: University) =>
		uni.initials && uni.initials.length <= 5 ? uni.initials : uni.name.charAt(0).toUpperCase();

	// Form input state — kept in sync with URL params so back/forward navigation
	// restores the visible field values.
	let qInput = $state(page.url.searchParams.get('q') ?? '');
	let countryInput = $state(page.url.searchParams.get('country') ?? untrack(() => data.countryDefault) ?? '');
	let stateInput = $state(page.url.searchParams.get('state') ?? untrack(() => data.stateDefault) ?? '');
	let cityInput = $state(page.url.searchParams.get('city') ?? '');
	let viewMode = $state<'grid' | 'list'>('grid');

	// Sync local form state whenever the URL params change (back/forward, clear).
	$effect(() => {
		qInput = page.url.searchParams.get('q') ?? '';
		countryInput = page.url.searchParams.get('country') ?? data.countryDefault ?? '';
		// ?? data.stateDefault surfaces the server-resolved parent state on
		// city-only URLs; it is null unless ?city= is present, so clearing the
		// state filter can't resurrect it.
		stateInput = page.url.searchParams.get('state') ?? data.stateDefault ?? '';
		cityInput = page.url.searchParams.get('city') ?? '';
	});

	// Build a URLSearchParams from the current filter state, resetting to page 1.
	function buildParams(overrides: Record<string, string> = {}): URLSearchParams {
		const params = new URLSearchParams();
		const q = overrides.q ?? qInput;
		const country = overrides.country ?? countryInput;
		const state = overrides.state ?? stateInput;
		const city = overrides.city ?? cityInput;
		const sort = overrides.sort ?? sortParam;
		if (q) params.set('q', q);
		if (country) params.set('country', country);
		if (state) params.set('state', state);
		if (city) params.set('city', city);
		if (sort && sort !== 'newest') params.set('sort', sort);
		return params;
	}

	function applySearch() {
		const params = buildParams();
		goto(`/universities${params.toString() ? `?${params.toString()}` : ''}`, { keepFocus: true, noScroll: true });
	}

	// Debounced auto-apply for the free-text search field.
	let qTimer: ReturnType<typeof setTimeout> | null = null;
	function queueApply() {
		if (qTimer) clearTimeout(qTimer);
		qTimer = setTimeout(applySearch, 400);
	}

	function setSort(value: string) {
		const params = buildParams({ sort: value });
		goto(`/universities${params.toString() ? `?${params.toString()}` : ''}`, { keepFocus: true });
	}

	function setCountry(value: string) {
		countryInput = value;
		// A new country invalidates the old state/city selections.
		stateInput = '';
		cityInput = '';
		const params = buildParams({ country: value, state: '', city: '' });
		goto(`/universities${params.toString() ? `?${params.toString()}` : ''}`, { keepFocus: true });
	}

	function setState(value: string) {
		stateInput = value;
		// A new state invalidates the old city selection.
		cityInput = '';
		const params = buildParams({ state: value, city: '' });
		goto(`/universities${params.toString() ? `?${params.toString()}` : ''}`, { keepFocus: true });
	}

	function setCity(value: string) {
		cityInput = value;
		const params = buildParams({ city: value });
		goto(`/universities${params.toString() ? `?${params.toString()}` : ''}`, { keepFocus: true });
	}

	function clearFilters() {
		qInput = '';
		countryInput = '';
		stateInput = '';
		cityInput = '';
		// Sort lives in the filter panel too — clear it along with country.
		goto('/universities');
	}

	function goToPage(p: number) {
		const params = buildParams();
		if (p > 1) params.set('page', String(p));
		goto(`/universities${params.toString() ? `?${params.toString()}` : ''}`);
	}

	// Autocomplete fetcher — filters the already-loaded page slice (no API call).
	// The full search is performed server-side on submit; this only suggests
	// matches from the current page for quick navigation.
	async function uniSearchFetcher(query: string): Promise<SearchSuggestion[]> {
		const q = query.trim().toLowerCase();
		if (!q) return [];
		return paged
			.filter(
				(u) =>
					u.name.toLowerCase().includes(q) ||
					(u.initials?.toLowerCase().includes(q) ?? false)
			)
			.slice(0, 8)
			.map((u) => ({
				id: u.id,
				title: u.initials ? `${u.initials} — ${u.name}` : u.name,
				url: `/universities/${u.slug}`,
				excerpt: u.geo_code ?? undefined,
				type: t('universities.suggestion_type', 'University')
			}));
	}

	// JSON-LD: ItemList of EducationalOrganization for the current page view.
	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'ItemList',
			itemListElement: paged.map((uni, i) => ({
				'@type': 'ListItem',
				position: showingStart + i,
				item: {
					'@type': 'EducationalOrganization',
					name: uni.name,
					url: `${page.url.origin}/universities/${uni.slug}`
				}
			}))
		})
	);

	// Autocomplete fetchers — filter the canonical /geo/countries list client-side.
	// The "All countries" option (id: '') is always included at the top; each
	// item is keyed by the ISO geo_code used by the university catalog.
	// ADR-064 curated markets (public settings) — pinned first; every
	// seeded country stays selectable (pin-first, not restricted).
	const uniCountries = $derived(
		pinMarketsFirst(data.countries, destinationMarkets(data.siteSettings), (c) => c.geo_code)
	);

	function fetchCountries(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase().trim();
		const allItem: AutocompleteItem = { id: '', label: allLabel };
		if (query === '') {
			return Promise.resolve([allItem, ...uniCountries.map((c) => ({ id: c.geo_code, label: c.name, sublabel: c.geo_code }))]);
		}
		const matchesAll = allLabel.toLowerCase().includes(query);
		const filtered = uniCountries.filter(
			(c) => c.name.toLowerCase().includes(query) || c.geo_code.toLowerCase().includes(query)
		);
		const items = filtered.map((c) => ({ id: c.geo_code, label: c.name, sublabel: c.geo_code }));
		return Promise.resolve(matchesAll ? [allItem, ...items] : items);
	}
	function fetchCountriesInitial(): Promise<AutocompleteItem[]> {
		return fetchCountries('');
	}
	const selectedCountryLabel = $derived(
		countryInput ? data.countries.find((c) => c.geo_code === countryInput)?.name ?? countryInput : allLabel
	);

	// State/city autocompletes — cascade off the country selection. Options are
	// primary-space slugs (the backend `within` filter resolves descendants via
	// space_parent_spaces). Each list is loaded server-side for the current
	// parent selection.
	function fetchStates(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase().trim();
		const allItem: AutocompleteItem = { id: '', label: allStatesLabel };
		const items = data.states.map((s) => ({ id: s.slug, label: s.name }));
		if (query === '') return Promise.resolve([allItem, ...items]);
		const filtered = items.filter((s) => s.label.toLowerCase().includes(query));
		return Promise.resolve(allStatesLabel.toLowerCase().includes(query) ? [allItem, ...filtered] : filtered);
	}
	function fetchStatesInitial(): Promise<AutocompleteItem[]> {
		return fetchStates('');
	}
	const selectedStateLabel = $derived(
		stateInput ? data.states.find((s) => s.slug === stateInput)?.name ?? stateInput : allStatesLabel
	);

	function fetchCities(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase().trim();
		const allItem: AutocompleteItem = { id: '', label: allCitiesLabel };
		const items = data.cities.map((c) => ({ id: c.slug, label: c.name }));
		if (query === '') return Promise.resolve([allItem, ...items]);
		const filtered = items.filter((c) => c.label.toLowerCase().includes(query));
		return Promise.resolve(allCitiesLabel.toLowerCase().includes(query) ? [allItem, ...filtered] : filtered);
	}
	function fetchCitiesInitial(): Promise<AutocompleteItem[]> {
		return fetchCities('');
	}
	const selectedCityLabel = $derived(
		cityInput ? data.cities.find((c) => c.slug === cityInput)?.name ?? cityInput : allCitiesLabel
	);


	// "How it works" slide-over — markdown panel + CTA repeater persisted to
	// public system settings (universities.guide) via PUT /admin/settings/general.
	let guideOpen = $state(false);
	let guideOverride = $state<GuideContent | null>(null);
	const siteSettings = $derived((page.data.siteSettings ?? {}) as Record<string, unknown>);
	const storedGuide = $derived.by((): Partial<GuideContent> => {
		const raw = siteSettings['universities.guide'];
		if (raw && typeof raw === 'object' && !Array.isArray(raw)) return raw as Partial<GuideContent>;
		return {};
	});
	const effectiveGuide = $derived<GuideContent>({
		title: guideOverride?.title ?? storedGuide.title ?? t('common.how_it_works', 'How it works'),
		body: guideOverride?.body ?? storedGuide.body ?? t('universities.guide_body', ''),
		ctas: guideOverride?.ctas ?? storedGuide.ctas ?? [{ title: t('guide_panel.read_full_guide', 'Read the full guide'), url: '/guides' }]
	});

	async function saveGuide(v: GuideContent): Promise<string | null> {
		const res = await api.put('/admin/settings/general', { universities_guide: v });
		if (res.error) return res.error.message || t('common.error', 'Something went wrong');
		guideOverride = v;
		return null;
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	{@html safeJsonLdScript(jsonLd)}
</svelte:head>

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	<!-- Header — EntityHero directory panel -->
	<EntityHero
		class="mt-4"
		icon={GraduationCap}
		accent="var(--color-primary)"
		heading={t('universities.heading', 'Universities')}
		description={t('universities.hero_desc', 'Explore universities relevant to the diaspora — compare programs, follow updates, and read community reviews.')}
		crumbs={[
			{ label: t('common.home', 'Home'), href: '/' },
			{ label: t('universities.heading', 'Universities') }
		]}
		stats={[
			{ icon: GraduationCap, value: formatMetric(total), label: t('universities.stats_unis', 'Universities') },
			{ icon: Globe, value: formatMetric(data.countries.length), label: t('universities.stats_countries', 'Countries') }
		]}
	>
		{#snippet art()}
			<HeroArtUniversities accent="var(--color-primary)" />
		{/snippet}
		{#snippet actions(c)}
			<div class="grid w-full grid-cols-2 gap-3 sm:flex sm:items-center">
			<a href="#uni-results" class="{c.primary} flex-1 justify-center whitespace-nowrap">
				<Search size={15} /> {t('universities.find', 'Find universities')}
			</a>
			<a href="/compare" class="{c.secondary} flex-1 justify-center whitespace-nowrap">
				<GitCompare size={15} /> {t('universities.compare', 'Compare')}
			</a>
			<button type="button" onclick={() => (guideOpen = true)} class="{c.ghost} flex-1 justify-center whitespace-nowrap">
				{t('common.how_it_works', 'How it works')}
				<ChevronRight size={14} class="rtl:-scale-x-100" />
				</button>
			</div>
		{/snippet}
	</EntityHero>


	<!-- Filter bar: ListPageBar + FilterToggle pattern, same as /guides -->
	<ListPageBar>
		<span class="flex items-center px-4 text-sm text-[var(--color-muted)]" style="min-height: var(--nav-height)">
			{total} {t('universities.results', 'universities')}
		</span>
		{#snippet actions()}
			<button type="button" onclick={() => (filtersModalOpen = true)} class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]">
				{t('common.filters', 'Filters')}
				{#if hasActiveFilters}<span class="ms-0.5 h-2 w-2 rounded-full bg-[var(--color-primary)]" aria-hidden="true"></span>{/if}
				<Filter size={16} />
			</button>
			<button type="button" onclick={() => (searchModalOpen = true)} class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]">
				{t('common.search', 'Search')}
				{#if qInput.trim()}<span class="ms-0.5 h-2 w-2 rounded-full bg-[var(--color-primary)]" aria-hidden="true"></span>{/if}
				<Search size={16} />
			</button>
			<!-- Grid / list view toggle -->
			<div class="flex shrink-0 items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] p-0.5">
				<button
					type="button"
					onclick={() => (viewMode = 'grid')}
					class="inline-flex h-7 w-7 items-center justify-center rounded-[calc(var(--radius)-2px)] transition-colors {viewMode ===
					'grid'
						? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]'
						: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
					aria-label={t('universities.view_grid', 'Grid view')}
					aria-pressed={viewMode === 'grid'}
				>
					<LayoutGrid size={15} />
				</button>
				<button
					type="button"
					onclick={() => (viewMode = 'list')}
					class="inline-flex h-7 w-7 items-center justify-center rounded-[calc(var(--radius)-2px)] transition-colors {viewMode ===
					'list'
						? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]'
						: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
					aria-label={t('universities.view_list', 'List view')}
					aria-pressed={viewMode === 'list'}
				>
					<List size={15} />
				</button>
			</div>
		{/snippet}
	</ListPageBar>

	<Modal bind:open={searchModalOpen} title={t('common.search', 'Search')} icon={Search} size="sm">
		{#snippet children()}
			<form onsubmit={(e) => { e.preventDefault(); searchModalOpen = false; applySearch(); }} role="search">
				<SearchInput
					bind:value={qInput}
					placeholder={t('universities.search', 'Search universities…')}
					ariaLabel={t('universities.heading', 'Universities')}
					fetcher={uniSearchFetcher}
				/>
			</form>
		{/snippet}
	</Modal>

	<Modal bind:open={filtersModalOpen} title={t('common.filters', 'Filters')} icon={Filter} size="lg">
		{#snippet children()}
		<div>

			<div class="flex flex-wrap gap-2">
				<div class="w-56">
					<Autocomplete
						placeholder={t('universities.all_countries', 'All countries')}
						ariaLabel={t('universities.country_filter', 'Filter by country')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchCountries}
						initialFetcher={fetchCountriesInitial}
						selectedId={countryInput || null}
						selectedLabel={selectedCountryLabel}
						onSelect={(item) => setCountry(item.id)}
						onClear={() => setCountry('')}
					/>
				</div>
				<div class="w-56 {effectiveCountry ? '' : 'pointer-events-none opacity-50'}">
					<Autocomplete
						placeholder={effectiveCountry
							? t('universities.all_states', 'All states')
							: t('universities.select_country_first', 'Select a country first')}
						ariaLabel={t('universities.state_filter', 'Filter by state')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchStates}
						initialFetcher={fetchStatesInitial}
						selectedId={stateInput || null}
						selectedLabel={selectedStateLabel}
						onSelect={(item) => setState(item.id)}
						onClear={() => setState('')}
					/>
				</div>
				<div class="w-56 {stateInput || cityInput ? '' : 'pointer-events-none opacity-50'}">
					<Autocomplete
						placeholder={stateInput || cityInput
							? t('universities.all_cities', 'All cities')
							: t('universities.select_state_first', 'Select a state first')}
						ariaLabel={t('universities.city_filter', 'Filter by city')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchCities}
						initialFetcher={fetchCitiesInitial}
						selectedId={cityInput || null}
						selectedLabel={selectedCityLabel}
						onSelect={(item) => setCity(item.id)}
						onClear={() => setCity('')}
					/>
				</div>
				<div class="w-44">
					<SelectField
						value={sortParam}
						options={sortOptions}
						placeholder={t('universities.sort_newest', 'Newest')}
						onValueChange={setSort}
					/>
				</div>
			</div>

			{#if hasActiveFilters}
				<div class="mt-2">
					<button
						type="button"
						onclick={clearFilters}
						class="inline-flex items-center gap-1 text-xs text-[var(--color-muted)] underline hover:text-[var(--color-text)]"
					>
						<X size={12} /> {t('common.clear_filters', 'Clear filters')}
					</button>
				</div>
			{/if}
		</div>
		{/snippet}
	</Modal>

	<!-- Results -->
	<div id="uni-results" class="mx-auto w-full max-w-3xl p-4">
		{#if navigating.to}
			<LoadingSkeleton variant="card" />
		{:else if total === 0}
			<EmptyState
				icon={GraduationCap}
				title={t('universities.empty', 'No universities found')}
				body={t('universities.empty_body', 'Try adjusting your search or filters.')}
			/>
		{:else}
			<p class="mb-3 text-xs text-[var(--color-muted)]">
				{t('common.showing', 'Showing {start}-{end} of {total}')
					.replace('{start}', String(showingStart))
					.replace('{end}', String(showingEnd))
					.replace('{total}', String(total))}
			</p>

			{#if form?.error}
				<div class="mb-4 rounded-[var(--radius)] border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">
					{form.message || t('jobs.jce_error', 'Could not create comparison')}
				</div>
			{/if}
			{#if selectedUniSlugs.size > 0}
				<form method="POST" use:enhance={() => { savingJCE = true; return async ({ update }) => { savingJCE = false; await update(); }; }} class="mb-4 flex flex-wrap items-center justify-between gap-3">
					<p class="text-sm text-[var(--color-muted)]">{t('jobs.selected', 'Selected')}: {selectedUniSlugs.size} / 4</p>
					{#each selectedUnis as uni (uni.slug)}<input type="hidden" name="uni_slug" value={uni.slug} />{/each}
					<button
						type="submit"
						disabled={!canCompare || !data.user}
						title={data.user ? t('jobs.compare_in_jce_hint', 'Open in JCE comparison') : t('jobs.sign_in_to_compare', 'Sign in to compare')}
						class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed"
					>
						{#if savingJCE}<Loader2 size={16} class="animate-spin" />{:else}<GitCompare size={16} />{/if}
						{t('jobs.compare_in_jce', 'Compare in JCE')}
					</button>
				</form>
			{/if}

			{#if viewMode === 'grid'}
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
					{#each paged as uni (uni.id)}
						{@const cardMeta: CardMetaItem[] = [
							{ icon: FileText, text: t('discover.posts', '{n} posts').replace('{n}', String(uni.post_count)) },
							{ icon: Heart, text: t('profile.followers', '{n} followers').replace('{n}', String(uni.follower_count)) }
						]}
						<DirectoryCard
							href="/universities/{uni.slug}"
							title={uni.name}
							subtitle="{uni.geo_code ? `${uni.geo_code} · ` : ''}{t('meta.followers', '{n} followers').replace('{n}', String(uni.follower_count))}"
							description={uni.description}
							imageUrl={uni.logo_url}
							fallback={uniMark(uni)}
							hueSeed={uni.id}
							meta={cardMeta}
						>
							{#snippet actions()}
								<input
									type="checkbox"
									checked={selectedUniSlugs.has(uni.slug)}
									onchange={() => toggleUni(uni.slug)}
									disabled={selectedUniSlugs.size >= 4 && !selectedUniSlugs.has(uni.slug)}
									title={t('jobs.compare_in_jce_hint', 'Select up to 4 to compare side by side.')}
									aria-label={t('jobs.compare_in_jce_hint', 'Select up to 4 to compare side by side.')}
									onclick={(e) => e.stopPropagation()}
									class="accent-[var(--color-primary)]"
								/>
								{#if data.user}
									<SpaceJoinButton
										entityId={uni.id}
										entitySlug={uni.slug}
										initiallyJoined={uni.is_member ?? false}
										initiallyPending={uni.is_pending ?? false}
										initiallyFollowing={uni.is_following ?? false}
										compact
									followOnly
									/>
								{/if}
							{/snippet}
						</DirectoryCard>
					{/each}
				</div>
			{:else}
				<div class="overflow-x-auto rounded-[var(--radius-card)] border border-[var(--color-border)]">
					<table class="w-full text-start text-sm">
						<thead class="bg-[var(--color-surface-subtle)] text-xs uppercase text-[var(--color-muted)]">
							<tr>
								<th class="px-4 py-2 font-medium">{t('universities.col_name', 'Name')}</th>
								<th class="px-4 py-2 text-center font-medium">{t('universities.col_initials', 'Initials')}</th>
								<th class="px-4 py-2 text-end font-medium">{t('universities.col_posts', 'Posts')}</th>
								<th class="px-4 py-2 text-end font-medium">{t('universities.col_followers', 'Followers')}</th>
								<th class="w-10 px-2 py-2 font-medium" aria-label={t('jobs.compare_in_jce_hint', 'Compare')}></th>
							</tr>
						</thead>
						<tbody>
							{#each paged as uni (uni.id)}
								<tr class="border-t border-[var(--color-border)] transition-colors hover:bg-[var(--color-surface-subtle)]">
									<td class="px-4 py-2.5">
										<a
											href="/universities/{uni.slug}"
											class="flex items-center gap-2 text-[var(--color-text)] hover:underline"
										>
											{#if uni.logo_url}
												<img
													src={uni.logo_url}
													alt=""
													class="h-7 w-7 shrink-0 rounded-[var(--radius)] object-cover"
												/>
											{:else}
												<span
													class="grid h-7 w-7 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)] text-xs font-semibold"
												>
													{uniMark(uni)}
												</span>
											{/if}
											<span class="truncate">{uni.name}</span>
										</a>
									</td>
									<td class="px-4 py-2.5 text-center">
										{#if uni.initials}
											<span class="rounded bg-[var(--color-primary)]/10 px-1.5 py-0.5 text-xs font-semibold text-[var(--color-primary)]"
												>{uni.initials}</span>
										{/if}
									</td>
									<td class="px-4 py-2.5 text-end tabular-nums text-[var(--color-muted)]">{uni.post_count}</td>
									<td class="px-4 py-2.5 text-end tabular-nums text-[var(--color-muted)]">{uni.follower_count}</td>
									<td class="px-2 py-2.5 text-center">
										<input
											type="checkbox"
											checked={selectedUniSlugs.has(uni.slug)}
											onchange={() => toggleUni(uni.slug)}
											disabled={selectedUniSlugs.size >= 4 && !selectedUniSlugs.has(uni.slug)}
											aria-label={t('jobs.compare_in_jce_hint', 'Compare')}
											class="accent-[var(--color-primary)]"
										/>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}

			{#if allUnis.length < total}
				<div class="mt-4 flex justify-center">
					<button
						type="button"
						onclick={loadMore}
						disabled={loadingMore}
						class="inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>
						{#if loadingMore}<Loader2 size={16} class="animate-spin" />{/if}
						{t('common.load_more', 'Load more')}
					</button>
				</div>
			{/if}
		{/if}
	</div>
</div>

<!-- "How it works" slide-over — GuideSlideOver; admins edit inline (universities.guide). -->
<GuideSlideOver
	bind:open={guideOpen}
	title={effectiveGuide.title}
	body={effectiveGuide.body}
	ctas={effectiveGuide.ctas}
	canManage={data.canManage}
	onSave={saveGuide}
/>
