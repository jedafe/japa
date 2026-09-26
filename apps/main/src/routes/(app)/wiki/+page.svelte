<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md
	// Global wiki landing at /wiki — geo-aware destination chooser and directory.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import type { WikiDestination, WikiTypeItem } from '$lib/types';
	import { page } from '$app/state';
	import { BookOpen, Globe, MapPin, GraduationCap, Table2 } from 'lucide-svelte';
	import { ListPageBar } from '$lib/components/ui';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// Server-rendered initial directory; intentionally static for SSR/SEO.
	// svelte-ignore state_referenced_locally
	const initialDestinations = data.destinations ?? [];
	let browseOptions = $state<WikiDestination[]>(initialDestinations);
	let browseLoading = $state(false);
	const wikiEnabled = $derived(data.features?.['wiki.enabled'] !== false);
	// §80 community field ops — destination structured data is
	// community-editable when the structured_edits flag is on.
	const structuredEditsOn = $derived(
		wikiEnabled && data.features?.['wiki.structured_edits'] === true
	);

	let selectedContinent = $state<string | null>(null);
	let selectedCountry = $state<WikiDestination | null>(null);
	let selectedState = $state<WikiDestination | null>(null);

	function toAutocompleteItem(d: WikiDestination): AutocompleteItem {
		return { id: d.slug, label: d.name, sublabel: d.geo_code ?? '' };
	}

	async function searchSpaces(q: string, levelFilter: 'country' | 'state' | 'city', parentSlug?: string, region?: string): Promise<AutocompleteItem[]> {
		const params = new URLSearchParams();
		params.set('has_wiki', 'false');
		params.set('level', levelFilter);
		if (parentSlug) params.set('parent', parentSlug);
		if (region) params.set('region', region);

		try {
			const res = await api.get<WikiDestination[]>(`/wiki/destinations?${params.toString()}`);
			let list = res.data ?? [];
			const query = q.toLowerCase().trim();
			if (query) {
				list = list.filter(
					(s) =>
						s.name.toLowerCase().includes(query) ||
						s.slug.toLowerCase().includes(query) ||
						(s.geo_code?.toLowerCase() ?? '').includes(query)
				);
			}
			return list.map(toAutocompleteItem);
		} catch {
			return [];
		}
	}

	async function allContinents(q: string): Promise<AutocompleteItem[]> {
		try {
			const res = await api.get<WikiDestination[]>(`/wiki/destinations?level=country&has_wiki=false`);
			const regions = Array.from(new Set((res.data ?? []).map((d) => d.region).filter((r): r is string => !!r))).sort();
			const query = q.toLowerCase().trim();
			const filtered = query ? regions.filter((r) => r.toLowerCase().includes(query)) : regions;
			return filtered.map((r) => ({ id: r, label: r, sublabel: '' }));
		} catch {
			return [];
		}
	}
	function allContinentsInitial(): Promise<AutocompleteItem[]> {
		return allContinents('');
	}

	function fetchCountries(q: string): Promise<AutocompleteItem[]> {
		return searchSpaces(q, 'country', undefined, selectedContinent ?? undefined);
	}
	function fetchCountriesInitial(): Promise<AutocompleteItem[]> {
		return fetchCountries('');
	}

	function fetchStates(q: string): Promise<AutocompleteItem[]> {
		if (!selectedCountry) return Promise.resolve([]);
		return searchSpaces(q, 'state', selectedCountry.slug);
	}
	function fetchStatesInitial(): Promise<AutocompleteItem[]> {
		return fetchStates('');
	}

	async function loadBrowse() {
		browseLoading = true;
		try {
			const params = new URLSearchParams();
			params.set('has_wiki', 'false');
			if (selectedState) {
				params.set('level', 'city');
				params.set('parent', selectedState.slug);
			} else if (selectedCountry) {
				params.set('level', 'state');
				params.set('parent', selectedCountry.slug);
			} else if (selectedContinent) {
				params.set('level', 'country');
				params.set('region', selectedContinent);
			} else {
				params.set('level', 'country');
			}
			const res = await api.get<WikiDestination[]>(`/wiki/destinations?${params.toString()}`);
			browseOptions = res.data ?? [];
		} catch {
			browseOptions = [];
		} finally {
			browseLoading = false;
		}
	}

	// No mount-time fetch: browseOptions is seeded from SSR data and every
	// chooser change calls loadBrowse() explicitly. (An $effect calling
	// loadBrowse here double-fetches on mount — the reads inside loadBrowse
	// are tracked — and again on each selection change.)

	function onSelectContinent(item: AutocompleteItem) {
		selectedContinent = item.id;
		selectedCountry = null;
		selectedState = null;
		loadBrowse();
	}

	function onClearContinent() {
		selectedContinent = null;
		selectedCountry = null;
		selectedState = null;
		loadBrowse();
	}

	function onSelectCountry(item: AutocompleteItem) {
		selectedCountry = { space_id: '', slug: item.id, name: item.label, geo_code: item.sublabel || undefined, level: 'country', page_count: 0 };
		selectedState = null;
		loadBrowse();
	}

	function onClearCountry() {
		selectedCountry = null;
		selectedState = null;
		loadBrowse();
	}

	function onSelectState(item: AutocompleteItem) {
		selectedState = { space_id: '', slug: item.id, name: item.label, geo_code: item.sublabel || undefined, level: 'state', page_count: 0 };
		loadBrowse();
	}

	function onClearState() {
		selectedState = null;
		loadBrowse();
	}

	function pageCountLabel(count: number): string {
		return t(
			count === 1 ? 'wiki.destination_page_count_one' : 'wiki.destination_page_count',
			count === 1 ? '{count} guide' : '{count} guides'
		).replace('{count}', String(count));
	}

	// §80 Wiki Types: every enabled non-geo wiki type gets a search + browse
	// section driven by GET /wiki/types, so newly enabled types surface here
	// with zero code. `?level=` matches the space type slug server-side.
	// svelte-ignore state_referenced_locally
	const extraTypes = data.extraTypes ?? [];
	// svelte-ignore state_referenced_locally
	const typeWikis = data.typeWikis ?? {};

	function typeLabel(wt: WikiTypeItem): string {
		return t(`wiki.type_${wt.space_type_slug}`, wt.label ?? wt.space_type_name);
	}

	function typeSectionLabel(wt: WikiTypeItem): string {
		return t(`wiki.${wt.space_type_slug}_wikis`, t('wiki.type_wikis', '{type} wikis', { type: typeLabel(wt) }));
	}

	function typeSearchHint(wt: WikiTypeItem): string {
		return t(
			`wiki.${wt.space_type_slug}_search_hint`,
			t('wiki.type_search_hint', 'Search a {type}…', { type: typeLabel(wt).toLowerCase() })
		);
	}

	function searchTypeEntities(typeSlug: string): (q: string) => Promise<AutocompleteItem[]> {
		return async (q: string) => {
			// Require a query — non-geo entity sets are too large to list.
			const query = q.trim();
			if (!query) return [];
			try {
				const params = new URLSearchParams({ level: typeSlug, has_wiki: 'false', q: query });
				const res = await api.get<WikiDestination[]>(`/wiki/destinations?${params.toString()}`);
				return (res.data ?? []).map(toAutocompleteItem);
			} catch {
				return [];
			}
		};
	}

	function onSelectTypeEntity(item: AutocompleteItem) {
		if (item.id) goto(`/wiki/${encodeURIComponent(item.id)}`);
	}
</script>

<svelte:head>
	<title>{t('wiki.heading', 'Wiki')} — JapaConnect</title>
	<meta name="description" content={t('wiki.landing_description', 'Explore destination wikis')} />
	<meta property="og:title" content={t('wiki.heading', 'Wiki')} />
	<meta property="og:description" content={t('wiki.landing_description', 'Explore destination wikis')} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<link rel="canonical" href={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-4xl">
	<ListPageBar title={t('wiki.heading', 'Wiki')} />

	<div class="p-6">
	<p class="text-sm text-[var(--color-muted)]">
		{t('wiki.landing_description', 'Choose a destination to explore community-maintained guides.')}
	</p>
	{#if structuredEditsOn}
		<p class="mt-2 flex items-center gap-1.5 text-xs text-[var(--color-muted)]">
			<Table2 size={13} />
			{t('wiki.community_data_note', 'Guides and destination data are community-maintained — suggest edits on any wiki page.')}
		</p>
	{/if}

	{#if !wikiEnabled}
		<div class="mt-8">
			<EmptyState
				icon={BookOpen}
				title={t('wiki.coming_soon', 'Wiki is not enabled for this destination')}
				body={t('wiki.coming_soon_body', 'Wiki is currently disabled. Please check back later.')}
			/>
		</div>
	{:else}
		<div class="mt-8 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<div class="mb-4 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
				<Globe size={16} />
				{t('wiki.destination_label', 'Destination')}
			</div>

			<div class="grid grid-cols-1 gap-3 md:grid-cols-3">
				<Autocomplete
					id="wiki-continent"
					placeholder={t('wiki.continent_label', 'Continent')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={allContinents}
					initialFetcher={allContinentsInitial}
					selectedId={selectedContinent}
					selectedLabel={selectedContinent}
					onSelect={onSelectContinent}
					onClear={onClearContinent}
				/>
				<Autocomplete
					id="wiki-country"
					placeholder={t('wiki.country_label', 'Country')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchCountries}
					initialFetcher={fetchCountriesInitial}
					selectedId={selectedCountry?.slug ?? null}
					selectedLabel={selectedCountry?.name ?? null}
					onSelect={onSelectCountry}
					onClear={onClearCountry}
				/>
				<Autocomplete
					id="wiki-state"
					placeholder={t('wiki.state_label', 'State')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchStates}
					initialFetcher={fetchStatesInitial}
					selectedId={selectedState?.slug ?? null}
					selectedLabel={selectedState?.name ?? null}
					onSelect={onSelectState}
					onClear={onClearState}
				/>
			</div>

			{#if browseLoading}
				<p class="mt-4 text-xs text-[var(--color-muted)]">{t('wiki.destination_loading', 'Loading destinations…')}</p>
			{/if}
		</div>

		<!-- §80 Wiki Types — one search + browse section per enabled non-geo
		     wiki type (university today; new types appear without code). Rendered
		     above the (long) destination grid so it stays visible. -->
		{#each extraTypes as wt (wt.space_type_slug)}
			{@const wikis = typeWikis[wt.space_type_slug] ?? []}
			<div class="mt-6 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<div class="mb-4 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
					<GraduationCap size={16} />
					{typeSectionLabel(wt)}
				</div>
				<div class="w-full max-w-sm">
					<Autocomplete
						id="wiki-type-{wt.space_type_slug}"
						placeholder={typeSearchHint(wt)}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={searchTypeEntities(wt.space_type_slug)}
						selectedId={null}
						selectedLabel={null}
						onSelect={onSelectTypeEntity}
					/>
				</div>
			</div>

			{#if wikis.length > 0}
				<div class="mt-6">
					<h2 class="mb-3 text-sm font-semibold uppercase tracking-wide text-[var(--color-muted)]">
						{typeSectionLabel(wt)}
					</h2>
					<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
						{#each wikis as w (w.slug)}
							<a
								href="/wiki/{encodeURIComponent(w.slug)}"
								class="flex flex-col gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3 transition-colors hover:border-[var(--color-primary)] hover:bg-[var(--color-surface-subtle)]"
								data-sveltekit-preload-data="hover"
							>
								<span class="text-sm font-medium text-[var(--color-text)]">{w.name}</span>
								<span class="text-xs text-[var(--color-muted)]">{pageCountLabel(w.page_count)}</span>
							</a>
						{/each}
					</div>
				</div>
			{/if}
		{/each}

		<div class="mt-6">
			{#if browseLoading}
				<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
					{#each Array(6) as _}
						<div class="h-20 animate-pulse rounded-[var(--radius)] bg-[var(--color-surface-subtle)]"></div>
					{/each}
				</div>
			{:else if browseOptions.length === 0}
				<div class="mt-2">
					<EmptyState
						icon={MapPin}
						title={t('wiki.no_destinations', 'No wikis yet')}
						body={t('wiki.no_destinations_body', 'No destination wikis are available right now.')}
					/>
				</div>
			{:else}
				<h2 class="mb-3 text-sm font-semibold uppercase tracking-wide text-[var(--color-muted)]">
					{t('wiki.browse_destinations', 'Browse destinations')}
				</h2>
				<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
					{#each browseOptions as dest (dest.slug)}
						<a
							href="/wiki/{encodeURIComponent(dest.slug)}"
							class="flex flex-col gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3 transition-colors hover:border-[var(--color-primary)] hover:bg-[var(--color-surface-subtle)]"
						>
							<span class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
								{dest.name}
								{#if dest.geo_code}
									<span class="rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10px] uppercase text-[var(--color-muted)]">{dest.geo_code}</span>
								{/if}
							</span>
							<span class="text-xs text-[var(--color-muted)]">{pageCountLabel(dest.page_count)}</span>
						</a>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
	</div>
</div>
