<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md
	// Global wiki landing at /wiki — the root type's grid (countries today)
	// plus a country-only search. Child-type exploration lives on each
	// parent's directory category pages (/wiki/{country}/states) — the
	// landing deliberately stays shallow.
	//
	// §80 Reader UX: sort (A–Z | most guides), server-side pagination with a
	// total count, destination logos/descriptions on the cards, and a
	// request-a-guide CTA pointing at the public feedback form.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import PaginationControls from '$lib/components/ui/PaginationControls.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import type { WikiDestination, WikiTypeItem } from '$lib/types';
	import { page } from '$app/state';
	import { BookOpen, Globe, MapPin, MessageSquarePlus, Table2 } from 'lucide-svelte';
	import { ListPageBar } from '$lib/components/ui';
	import { ExploreMapBlock, LandingRenderer } from '@japaconnect/landing-blocks';
	import { browser } from '$app/environment';
	import { apiBaseUrl } from '$lib/api/client';
	import type { PageData } from './$types';

	const fetchCtx = $derived(
		browser ? { fetch, apiBase: apiBaseUrl, surface: 'main' as const } : undefined
	);
	const exploreProps = { headline: '', subtitle: '', geo_source: 'api' as const, pin_limit: 10 };

	let { data }: { data: PageData } = $props();

	// §80 Reader UX: read straight off `data` so ?page/?sort navigations
	// (server re-load) update the grid without local bookkeeping.
	const destinations = $derived(data.destinations ?? []);
	const total = $derived(data.total ?? destinations.length);
	const currentPage = $derived(data.page ?? 1);
	const pageLimit = $derived(data.limit ?? 0);
	const sort = $derived(data.sort ?? 'name');
	const totalPages = $derived(pageLimit > 0 ? Math.max(1, Math.ceil(total / pageLimit)) : 1);

	const wikiEnabled = $derived(data.features?.['wiki.enabled'] !== false);
	// §80 community field ops — destination structured data is
	// community-editable when the structured_edits flag is on.
	const structuredEditsOn = $derived(
		wikiEnabled && data.features?.['wiki.structured_edits'] === true
	);

	const chain = $derived(data.chain ?? []);
	const rootType = $derived(chain[0] ?? null);

	function toAutocompleteItem(d: WikiDestination): AutocompleteItem {
		return { id: d.slug, label: d.name, sublabel: d.geo_code ?? '' };
	}

	function typeLabel(wt: WikiTypeItem): string {
		return t(`wiki.type_${wt.space_type_slug}`, wt.label ?? wt.space_type_name);
	}

	// Country search only — states/cities/universities are reached via
	// the parent directories, so the landing search mirrors the grid.
	async function searchAll(q: string): Promise<AutocompleteItem[]> {
		const query = q.trim();
		if (!query) return [];
		try {
			const usp = new URLSearchParams({
				has_wiki: 'false',
				q: query,
				...(rootType ? { level: rootType.space_type_slug } : {})
			});
			const res = await api.get<WikiDestination[]>(`/wiki/destinations?${usp.toString()}`);
			return (res.data ?? []).map(toAutocompleteItem);
		} catch {
			return [];
		}
	}

	function onSelectDestination(item: AutocompleteItem) {
		if (item.id) goto(`/wiki/${encodeURIComponent(item.id)}`);
	}

	function pageCountLabel(count: number): string {
		return t(
			count === 1 ? 'wiki.destination_page_count_one' : 'wiki.destination_page_count',
			count === 1 ? '{count} guide' : '{count} guides'
		).replace('{count}', String(count));
	}

	function destinationsCountLabel(n: number): string {
		return t(
			n === 1 ? 'wiki.destinations_count_one' : 'wiki.destinations_count',
			n === 1 ? '{count} destination' : '{count} destinations'
		).replace('{count}', String(n));
	}

	function gotoParams(next: { page?: number; sort?: string }) {
		const usp = new URLSearchParams();
		const p = next.page ?? currentPage;
		const s = next.sort ?? sort;
		if (p > 1) usp.set('page', String(p));
		if (s !== 'name') usp.set('sort', s);
		const qs = usp.toString();
		goto(qs ? `/wiki?${qs}` : '/wiki');
	}

	function onSortChange(v: string) {
		gotoParams({ page: 1, sort: v });
	}

	function onPageChange(p: number) {
		gotoParams({ page: p });
	}

	const sortOptions = $derived([
		{ value: 'name', label: t('wiki.sort_az', 'A–Z') },
		{ value: 'page_count', label: t('wiki.sort_most_guides', 'Most guides') }
	]);
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

<div class="mx-auto w-full max-w-4xl" style="--page-gutter: 1rem">
	<ListPageBar title={t('wiki.heading', 'Wiki')} />

	<div class="p-4">
	<p class="text-sm text-[var(--color-muted)]">
		{t('wiki.landing_description', 'Choose a destination to explore community-maintained guides.')}
	</p>
	{#if structuredEditsOn}
		<p class="mt-2 flex items-center gap-1.5 text-xs text-[var(--color-muted)]">
			<Table2 size={13} />
			{t('wiki.community_data_note', 'Guides and destination data are community-maintained — suggest edits on any wiki page.')}
		</p>
	{/if}

	{#if data.builderBlocks?.length}
		<!-- Builder-managed hero: a published Page Builder page with slug
		     "wiki" takes over this slot; else the built-in explorer map. -->
		<LandingRenderer blocks={data.builderBlocks} {fetchCtx} geoCountry={data.contentCountry} />
	{:else}
		<ExploreMapBlock props={exploreProps} {fetchCtx} />
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
		<!-- The search earns its slot once the grid outgrows one screenful —
		     with ≤12 destinations every item is already visible. -->
		{#if total > 12}
			<div class="mt-8 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<div class="mb-4 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
					<Globe size={16} />
					{t('wiki.destination_label', 'Destination')}
				</div>
				<Autocomplete
					id="wiki-destination-search"
					placeholder={t('wiki.destination_search_hint', 'Search a country…')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={searchAll}
					selectedId={null}
					selectedLabel={null}
					onSelect={onSelectDestination}
				/>
			</div>
		{/if}

		<div class="mt-6">
			{#if destinations.length === 0 && total === 0}
				<div class="mt-2">
					<EmptyState
						icon={MapPin}
						title={t('wiki.no_destinations', 'No wikis yet')}
						body={t('wiki.no_destinations_body', 'No destination wikis are available right now.')}
					/>
				</div>
			{:else}
				<div class="mb-3 flex items-center justify-between gap-3">
					<h2 class="text-sm font-semibold uppercase tracking-wide text-[var(--color-muted)]">
						{rootType ? typeLabel(rootType) : t('wiki.browse_destinations', 'Browse destinations')}
						<span class="ml-2 font-normal normal-case tracking-normal">
							{destinationsCountLabel(total)}
						</span>
					</h2>
					<div class="w-40 shrink-0">
						<AutocompleteSelect
							value={sort}
							options={sortOptions}
							onValueChange={onSortChange}
							id="wiki-destination-sort"
							ariaLabel={t('wiki.sort_label', 'Sort destinations')}
						/>
					</div>
				</div>
				<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
					{#each destinations as dest (dest.slug)}
						<a
							href="/wiki/{encodeURIComponent(dest.slug)}"
							class="flex flex-col gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3 transition-colors hover:border-[var(--color-primary)] hover:bg-[var(--color-surface-subtle)]"
						>
							<span class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
								{#if dest.logo_url}
									<img
										src={dest.logo_url}
										alt=""
										class="h-5 w-5 shrink-0 rounded-sm object-cover"
										loading="lazy"
									/>
								{:else}
									<Globe size={15} class="shrink-0 text-[var(--color-muted)]" />
								{/if}
								{dest.name}
								{#if dest.geo_code}
									<span class="rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10px] uppercase text-[var(--color-muted)]">{dest.geo_code}</span>
								{/if}
							</span>
							{#if dest.description}
								<span class="line-clamp-2 text-xs text-[var(--color-text-subtle)]">{dest.description}</span>
							{/if}
							<span class="text-xs text-[var(--color-muted)]">{pageCountLabel(dest.page_count)}</span>
						</a>
					{/each}
				</div>

				{#if totalPages > 1}
					<div class="mt-4 flex justify-center">
						<PaginationControls
							page={currentPage}
							totalPages={totalPages}
							onPageChange={onPageChange}
						/>
					</div>
				{/if}
			{/if}
		</div>

		<div class="mt-6 flex items-center justify-between gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<div>
				<p class="text-sm font-medium text-[var(--color-text)]">
					{t('wiki.request_guide_title', "Can't find what you need?")}
				</p>
				<p class="mt-0.5 text-xs text-[var(--color-muted)]">
					{t('wiki.request_guide_body', 'Tell us which destination or topic to cover next.')}
				</p>
			</div>
			<a
				href="/feedback"
				class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)] hover:text-[var(--color-primary)]"
			>
				<MessageSquarePlus size={15} />
				{t('wiki.request_guide_cta', 'Request a guide')}
			</a>
		</div>
	{/if}
	</div>
</div>
