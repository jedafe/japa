<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { navigating } from '$app/state';
	import { GraduationCap, Users, FileText, Heart, LayoutGrid, List, X } from 'lucide-svelte';
	import {
		EmptyState,
		LoadingSkeleton,
		PaginationControls,
		SearchInput,
		SelectField,
		DirectoryCard,
		SpaceJoinButton,
		ListPageBar,
		FilterToggle,
		type SearchSuggestion,
		type CardMetaItem
	} from '$lib/components/ui';
	import type { University } from '$lib/types';
	import type { PageData } from './$types';
	import { destinationMarkets, pinMarketsFirst } from '$lib/utils/geoMarkets';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('universities.title', 'Universities — {site}'));
	const description = $derived(
		t('universities.description', 'Explore universities relevant to the diaspora community.')
	);

	// Server-paginated slice for the current page (filter/sort/page are
	// server-side — the backend honors ?q=, ?country=, ?sort=, ?page=, ?limit=).
	const paged = $derived<University[]>(data.universities);
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
		{ value: 'members', label: t('universities.sort_members', 'Most members') },
		{ value: 'followers', label: t('universities.sort_followers', 'Most followers') },
		{ value: 'posts', label: t('universities.sort_posts', 'Most posts') }
	]);

	// Panel-active = explicit filters only: the geo-aware country default and
	// the always-visible search box don't count (they're not in the panel).
	const hasActiveFilters = $derived(
		!!countryParam || !!stateParam || !!cityParam || sortParam !== 'newest'
	);

	// Filter panel (ListPageBar + FilterToggle pattern — same as /blog, /guides).
	let showFilters = $state(false);
	$effect(() => {
		if (hasActiveFilters) showFilters = true;
	});
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
	<div class="border-b border-[var(--color-border)] px-4 py-4" style="margin-inline: max(1rem, calc((100% - 48rem)/2))">
		<div class="min-w-0">
			<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
				<GraduationCap size={20} /> {t('universities.heading', 'Universities')}
			</h1>
			<p class="mt-1 text-sm text-[var(--color-muted)]">{description}</p>
		</div>

		<form
			class="mt-3"
			onsubmit={(e) => {
				e.preventDefault();
				applySearch();
			}}
		>
			<SearchInput
				bind:value={qInput}
				placeholder={t('universities.search', 'Search universities…')}
				ariaLabel={t('universities.heading', 'Universities')}
				fetcher={uniSearchFetcher}
				onInput={queueApply}
				inputClass="focus:ring-2 focus:ring-[var(--color-primary)]"
			/>
		</form>
	</div>

	<!-- Filter bar: ListPageBar + FilterToggle pattern, same as /guides -->
	<ListPageBar>
		<span class="flex items-center px-4 text-sm text-[var(--color-muted)]" style="min-height: var(--nav-height)">
			{total} {t('universities.results', 'universities')}
		</span>
		{#snippet actions()}
			<FilterToggle bind:show={showFilters} active={hasActiveFilters} controls="universities-filter-panel" />
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

	{#if showFilters}
		<!-- Country + sort filters (moved into the toggle panel) -->
		<div id="universities-filter-panel" class="border-b border-[var(--color-border)] px-4 py-3" style="margin-inline: max(1rem, calc((100% - 48rem)/2))">
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
	{/if}

	<!-- Results -->
	<div class="mx-auto w-full max-w-3xl p-4">
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
							subtitle="{uni.geo_code ? `${uni.geo_code} · ` : ''}{t('meta.members', '{n} members').replace('{n}', String(uni.member_count))}"
							description={uni.description}
							imageUrl={uni.logo_url}
							fallback={uniMark(uni)}
							hueSeed={uni.id}
							meta={cardMeta}
						>
							{#snippet actions()}
								{#if data.user}
									<SpaceJoinButton
										entityId={uni.id}
										entitySlug={uni.slug}
										initiallyJoined={uni.is_member ?? false}
										initiallyPending={uni.is_pending ?? false}
										initiallyFollowing={uni.is_following ?? false}
										compact
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
								<th class="px-4 py-2 text-end font-medium">{t('universities.col_members', 'Members')}</th>
								<th class="px-4 py-2 text-end font-medium">{t('universities.col_posts', 'Posts')}</th>
								<th class="px-4 py-2 text-end font-medium">{t('universities.col_followers', 'Followers')}</th>
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
									<td class="px-4 py-2.5 text-end tabular-nums text-[var(--color-muted)]">{uni.member_count}</td>
									<td class="px-4 py-2.5 text-end tabular-nums text-[var(--color-muted)]">{uni.post_count}</td>
									<td class="px-4 py-2.5 text-end tabular-nums text-[var(--color-muted)]">{uni.follower_count}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}

			{#if totalPages > 1}
				<div class="mt-4 flex justify-center">
					<PaginationControls
						page={currentPage}
						totalPages={totalPages}
						onPageChange={goToPage}
					/>
				</div>
			{/if}
		{/if}
	</div>
</div>
