<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md
	// Global wiki landing at /wiki — edge-driven destination chooser: the
	// drill-down levels come from the declared wiki_type_parents edges
	// (parent_type_ids on GET /wiki/types), not a hardcoded geo chain.
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
	import {
		wikiPrimaryChild,
		wikiSidelinedChildren
	} from '$lib/utils/wikiTypeChain';
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

	// svelte-ignore state_referenced_locally
	const allTypes: WikiTypeItem[] = data.types ?? [];
	// svelte-ignore state_referenced_locally
	const chain: WikiTypeItem[] = data.chain ?? [];
	const rootType = chain[0] ?? null;
	// spaces.region only exists on country rows — the continent picker is
	// rendered only when the drill chain roots at the country type.
	const showContinent = rootType?.space_type_slug === 'country';
	// Every chain level but the leaf gets a picker — the leaf type's spaces
	// land in the browse grid (picking a city means reading its wiki).
	const pickable = chain.slice(0, -1);

	let selectedContinent = $state<string | null>(null);
	// Selections aligned to `pickable`; null = that level not picked.
	let picks = $state<(WikiDestination | null)[]>(pickable.map(() => null));

	function toAutocompleteItem(d: WikiDestination): AutocompleteItem {
		return { id: d.slug, label: d.name, sublabel: d.geo_code ?? '' };
	}

	function typeLabel(wt: WikiTypeItem): string {
		return t(`wiki.type_${wt.space_type_slug}`, wt.label ?? wt.space_type_name);
	}

	async function searchDestinations(
		q: string,
		params: Record<string, string>
	): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase().trim();
		try {
			const usp = new URLSearchParams({ has_wiki: 'false', ...params });
			if (query) usp.set('q', query);
			const res = await api.get<WikiDestination[]>(`/wiki/destinations?${usp.toString()}`);
			let list = res.data ?? [];
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
			const res = await api.get<WikiDestination[]>(
				`/wiki/destinations?level=country&has_wiki=false`
			);
			const regions = Array.from(
				new Set((res.data ?? []).map((d) => d.region).filter((r): r is string => !!r))
			).sort();
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

	/** Picker fetcher for chain level i — root filters by region, deeper
	 *  levels filter by their parent's slug (?parent=, direct parent_id). */
	function levelFetcher(i: number): (q: string) => Promise<AutocompleteItem[]> {
		return (q: string) => {
			const lt = pickable[i];
			if (!lt) return Promise.resolve([]);
			if (i === 0) {
				return searchDestinations(q, {
					level: lt.space_type_slug,
					...(selectedContinent ? { region: selectedContinent } : {})
				});
			}
			const parent = picks[i - 1];
			if (!parent) return Promise.resolve([]);
			return searchDestinations(q, { level: lt.space_type_slug, parent: parent.slug });
		};
	}

	function onSelectLevel(i: number, item: AutocompleteItem) {
		const lt = pickable[i];
		picks[i] = {
			space_id: '',
			slug: item.id,
			name: item.label,
			geo_code: item.sublabel || undefined,
			level: lt.space_type_slug,
			page_count: 0
		};
		for (let j = i + 1; j < picks.length; j++) picks[j] = null;
		loadBrowse();
	}

	function onClearLevel(i: number) {
		picks[i] = null;
		for (let j = i + 1; j < picks.length; j++) picks[j] = null;
		loadBrowse();
	}

	function onSelectContinent(item: AutocompleteItem) {
		selectedContinent = item.id;
		for (let j = 0; j < picks.length; j++) picks[j] = null;
		loadBrowse();
	}

	function onClearContinent() {
		selectedContinent = null;
		for (let j = 0; j < picks.length; j++) picks[j] = null;
		loadBrowse();
	}

	// Deepest picked level — the browse grid lists that space's children of
	// the next chain type, or the space itself when the chain ends there.
	const deepestIdx = $derived(picks.reduce((acc, p, i) => (p ? i : acc), -1));
	const gridType = $derived.by<WikiTypeItem | null>(() => {
		if (deepestIdx < 0) return chain[0] ?? null;
		const selType = chain[deepestIdx];
		return wikiPrimaryChild(allTypes, selType) ?? null;
	});
	// Declared child types beyond the drill chain (e.g. universities under a
	// selected country) get their own scoped search pickers — ?within= so
	// junction-linked (space_parent_spaces) children resolve.
	const sidelinedTypes = $derived.by<WikiTypeItem[]>(() => {
		if (deepestIdx < 0) return [];
		const selType = chain[deepestIdx];
		if (!selType) return [];
		return wikiSidelinedChildren(allTypes, selType);
	});
	const deepestSpace = $derived(deepestIdx >= 0 ? picks[deepestIdx] : null);

	async function loadBrowse() {
		browseLoading = true;
		try {
			const params = new URLSearchParams();
			params.set('has_wiki', 'false');
			if (deepestSpace) {
				if (gridType) {
					params.set('level', gridType.space_type_slug);
					params.set('parent', deepestSpace.slug);
				} else {
					// Chain ends at the selection — show it as the result card.
					browseOptions = [deepestSpace];
					return;
				}
			} else if (rootType) {
				params.set('level', rootType.space_type_slug);
				if (selectedContinent) params.set('region', selectedContinent);
			} else {
				browseOptions = [];
				return;
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

	function pageCountLabel(count: number): string {
		return t(
			count === 1 ? 'wiki.destination_page_count_one' : 'wiki.destination_page_count',
			count === 1 ? '{count} guide' : '{count} guides'
		).replace('{count}', String(count));
	}

	// §80 Wiki Types: every enabled type outside the drill chain gets a
	// search + browse section — new types surface here with zero code.
	// svelte-ignore state_referenced_locally
	const extraTypes = data.extraTypes ?? [];
	// svelte-ignore state_referenced_locally
	const typeWikis = data.typeWikis ?? {};

	function typeSectionLabel(wt: WikiTypeItem): string {
		return t(`wiki.${wt.space_type_slug}_wikis`, t('wiki.type_wikis', '{type} wikis', { type: typeLabel(wt) }));
	}

	function typeSearchHint(wt: WikiTypeItem): string {
		return t(
			`wiki.${wt.space_type_slug}_search_hint`,
			t('wiki.type_search_hint', 'Search a {type}…', { type: typeLabel(wt).toLowerCase() })
		);
	}

	function searchTypeEntities(typeSlug: string, withinSlug?: string): (q: string) => Promise<AutocompleteItem[]> {
		return async (q: string) => {
			// Require a query — non-geo entity sets are too large to list.
			const query = q.trim();
			if (!query) return [];
			try {
				const params = new URLSearchParams({ level: typeSlug, has_wiki: 'false', q: query });
				if (withinSlug) params.set('within', withinSlug);
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
				{#if showContinent}
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
				{/if}
				{#each pickable as lt, i (lt.space_type_id)}
					<Autocomplete
						id="wiki-level-{lt.space_type_slug}"
						placeholder={typeLabel(lt)}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={levelFetcher(i)}
						initialFetcher={() => levelFetcher(i)('')}
						selectedId={picks[i]?.slug ?? null}
						selectedLabel={picks[i]?.name ?? null}
						onSelect={(item) => onSelectLevel(i, item)}
						onClear={() => onClearLevel(i)}
					/>
				{/each}
			</div>

			<!-- Declared child types off the drill chain (e.g. universities
			     under a country) get scoped search pickers once the parent
			     space is chosen. -->
			{#if deepestSpace && sidelinedTypes.length > 0}
				<div class="mt-3 grid grid-cols-1 gap-3 md:grid-cols-3">
					{#each sidelinedTypes as wt (wt.space_type_id)}
						<Autocomplete
							id="wiki-side-{wt.space_type_slug}"
							placeholder={t('wiki.type_within_hint', '{type} in {name}…', {
								type: typeLabel(wt),
								name: deepestSpace.name
							})}
							emptyMessage={t('common.no_results', 'No results found')}
							fetcher={searchTypeEntities(wt.space_type_slug, deepestSpace.slug)}
							selectedId={null}
							selectedLabel={null}
							onSelect={onSelectTypeEntity}
						/>
					{/each}
				</div>
			{/if}

			{#if browseLoading}
				<p class="mt-4 text-xs text-[var(--color-muted)]">{t('wiki.destination_loading', 'Loading destinations…')}</p>
			{/if}
		</div>

		<!-- §80 Wiki Types — one search + browse section per enabled type
		     outside the drill chain. Rendered above the (long) destination
		     grid so it stays visible. -->
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
					{gridType ? typeLabel(gridType) : t('wiki.browse_destinations', 'Browse destinations')}
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
