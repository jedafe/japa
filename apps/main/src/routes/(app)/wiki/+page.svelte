<script lang="ts">
	import SeoHead from '$lib/seo/SeoHead.svelte';
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md
	// Global wiki landing at /wiki — the root type's grid (countries today)
	// plus a country-only search. Child-type exploration lives on each
	// parent's directory category pages (/wiki/{country}/states) — the
	// landing deliberately stays shallow.
	//
	// §80 Reader UX: sort (A–Z | most guides), server-side pagination with a
	// total count, destination logos/descriptions on the cards, and a
	// request-a-guide CTA pointing at the public feedback form.
	import { t, siteName} from '$lib/i18n';
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
	import { flagUrl, flagEmoji } from '$lib/utils/flags';
	import { ExploreMapBlock, LandingRenderer } from '@japaconnect/landing-blocks';
	import { browser } from '$app/environment';
	import { apiBaseUrl } from '$lib/api/client';
	import type { PageData } from './$types';

	const fetchCtx = $derived(
		browser ? { fetch, apiBase: apiBaseUrl, surface: 'main' as const } : undefined
	);
	// Full-width hero: toolbar off, legend/stats stacked horizontally below
	// the map so it spans the full content column.
	const exploreProps = {
		headline: '',
		subtitle: '',
		geo_source: 'api' as const,
		pin_limit: 10,
		show_toolbar: false,
		aside_position: 'bottom' as const,
	};

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
	// The grid is the flag gallery — inherently country-level, so it stays
	// on countries even when the chain roots at a hub type (region); the
	// root type's spaces render as the browse row above the grid instead.
	const galleryType = $derived(
		chain.find((t) => t.space_type_slug === 'country') ?? chain[1] ?? chain[0] ?? null
	);
	const hubs = $derived(data.hubDestinations ?? []);
	const activeRegion = $derived(data.region ?? '');
	// The region hub matching ?region= — the filtered gallery links out to
	// the canonical listing at /wiki/{region}/countries rather than
	// duplicating it as a second surface.
	const activeHub = $derived(hubs.find((h) => h.name === activeRegion) ?? null);

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
				...(galleryType ? { level: galleryType.space_type_slug } : {})
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
		if (activeRegion) usp.set('region', activeRegion);
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

<SeoHead title={`${t('wiki.heading', 'Wiki')} — ${siteName()}`} description={t('wiki.landing_description', 'Explore destination wikis')} ogTitle={t('wiki.heading', 'Wiki')} />

<!-- The map leads on every device — the title/description block stays
     hidden (the route's <svelte:head> still carries the page title). -->
<div class="mx-auto hidden w-full max-w-4xl px-4 pt-3 sm:px-6">
	<ListPageBar title={t('wiki.heading', 'Wiki')} />
	<p class="text-sm text-[var(--color-muted)]">
		{t('wiki.landing_description', 'Choose a destination to explore community-maintained guides.')}
	</p>
	{#if structuredEditsOn}
		<p class="mt-2 flex items-start gap-1.5 text-xs text-[var(--color-muted)]">
			<Table2 size={13} class="mt-0.5 shrink-0" />
			<span>{t('wiki.community_data_note', 'Guides and destination data are community-maintained — suggest edits on any wiki page.')}</span>
		</p>
	{/if}
</div>

<!-- §16/§80 — hero shares the same 4xl column width as the body below;
     the shell's right rail is still suppressed on this route. -->
<div class="wiki-hero mx-auto w-full max-w-4xl pt-1">
	{#if data.builderBlocks?.length}
		<!-- Builder-managed hero: a published Page Builder page with slug
		     "wiki" takes over this slot; else the built-in explorer map. -->
		<LandingRenderer blocks={data.builderBlocks} {fetchCtx} geoCountry={data.contentCountry} />
	{:else}
		<ExploreMapBlock props={exploreProps} {fetchCtx} />
	{/if}
</div>

<div class="mx-auto w-full max-w-4xl" style="--page-gutter: 1rem">
	<div class="px-4 py-4 sm:px-6">
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
				<div class="mb-4 flex items-center gap-2 text-xs font-medium text-[var(--color-text)] sm:text-sm">
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
					<h2 class="text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)] sm:text-sm">
						<!-- Mobile shows only the count, e.g. "250 destinations". -->
						<span class="hidden sm:inline">
							{galleryType ? typeLabel(galleryType) : t('wiki.browse_destinations', 'Browse destinations')}
						</span>
						<span class="font-normal normal-case tracking-normal sm:ms-2">
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
				{#if hubs.length > 0}
					<!-- Region filter chips — ?region= narrows the country
					     gallery in place; the region spaces still carry their own
					     wiki index (/wiki/africa …) for the breadcrumb hierarchy. -->
					<nav
						aria-label={t('wiki.browse_by_region', 'Browse by region')}
						class="mb-4 flex flex-wrap gap-1.5"
					>
						{#if activeRegion}
							<a
								href="/wiki"
								class="inline-flex items-center gap-1.5 rounded-full border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-muted)] transition-colors hover:text-[var(--color-primary)]"
							>
								{t('wiki.region_all', 'All regions')}
							</a>
						{/if}
						{#each hubs as hub (hub.slug)}
							{@const active = hub.name === activeRegion}
							<a
								href={active ? '/wiki' : `/wiki?region=${encodeURIComponent(hub.name)}`}
								aria-current={active ? 'true' : undefined}
								class="inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-medium transition-colors {active
									? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
									: 'border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text)] hover:border-[var(--color-primary)]/60 hover:text-[var(--color-primary)]'}"
							>
								<Globe size={13} class={active ? '' : 'text-[var(--color-muted)]'} />
								{hub.name}
							</a>
						{/each}
						{#if activeHub}
							<a
								href="/wiki/{encodeURIComponent(activeHub.slug)}"
								class="inline-flex items-center gap-1 px-2 py-1.5 text-xs font-medium text-[var(--color-primary)] transition-colors hover:underline"
							>
								{t('wiki.region_hub_link', 'Open the {name} wiki →', {
									name: activeHub.name
								})}
							</a>
						{/if}
					</nav>
				{/if}
				<!-- Flag gallery cards — same pattern as the radar country
				     gallery: bundled flag as the card background, name overlaid
				     on a gradient. -->
				<!-- 2 columns mobile, 3 on pad (md) and desktop. -->
				<div class="grid grid-cols-2 gap-2 md:grid-cols-3">
					{#each destinations as dest (dest.slug)}
						{@const bg = flagUrl(dest.geo_code)}
						<a
							href="/wiki/{encodeURIComponent(dest.slug)}"
							title={dest.name}
							class="group relative block aspect-[4/3] overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] [content-visibility:auto] transition-colors hover:border-[var(--color-primary)]/60 focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary)]"
						>
							{#if bg}
								<img
									src={bg}
									alt=""
									loading="lazy"
									decoding="async"
									class="absolute inset-0 h-full w-full object-cover motion-safe:transition-transform motion-safe:duration-300 motion-safe:group-hover:scale-105"
								/>
							{:else}
								<div
									class="absolute inset-0 flex items-center justify-center bg-[var(--color-surface-subtle)]"
									aria-hidden="true"
								>
									{#if flagEmoji(dest.geo_code)}
										<span class="text-4xl">{flagEmoji(dest.geo_code)}</span>
									{:else}
										<Globe size={32} class="text-[var(--color-muted)]" />
									{/if}
								</div>
							{/if}
							<span
								class="absolute inset-0 flex flex-col justify-end bg-gradient-to-t from-black/75 via-black/25 to-transparent p-2"
							>
								<span class="flex items-center gap-1 text-xs font-semibold leading-tight text-white [text-shadow:0_1px_2px_rgb(0_0_0/0.6)]">
									<span class="min-w-0 truncate">{dest.name}</span>
									{#if dest.geo_code}
										<span class="shrink-0 rounded bg-black/40 px-1 py-0.5 text-[9px] uppercase text-white/80">{dest.geo_code}</span>
									{/if}
								</span>
								<span class="mt-0.5 text-[10px] font-normal text-white/80 tabular-nums">{pageCountLabel(dest.page_count)}</span>
							</span>
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
				<p class="text-xs font-medium text-[var(--color-text)] sm:text-sm">
					{t('wiki.request_guide_title', "Can't find what you need?")}
				</p>
				<p class="mt-0.5 text-[11px] text-[var(--color-muted)] sm:text-xs">
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

<style>
	/* Tighten the gap above the map — the block's default section padding
	   (py-10) is meant for standalone landing sections, not an in-app page. */
	.wiki-hero :global(.explore-map) {
		padding-top: 0.5rem;
		padding-bottom: 1rem;
	}
</style>
