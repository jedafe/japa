<script lang="ts">
	// Ref: japaconnect-engineering/sections/79-community-story-wall.md
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { BookOpen, PenSquare, ArrowRight, Loader2, X } from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { ListPageBar, UrlTabs, type UrlTab, FilterToggle, PageAction } from '$lib/components/ui';
	import { staticAutocompleteFetcher, autocompleteItems } from '$lib/autocomplete';
	import type { FeedItem, Story } from '$lib/types';
	import type { GuideListItem } from '$lib/types/content';
	import ContentGridCard from '$lib/components/feed/ContentGridCard.svelte';
	import { destinationMarkets, originMarkets, pinMarketsFirst } from '$lib/utils/geoMarkets';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('stories.list_title', 'Community Stories — {site}'));
	const description = $derived(
		t(
			'stories.list_description',
			'Real relocation stories from the Nigerian diaspora — challenges, resources, and advice.'
		)
	);

	// Accumulated stories for offset-based "load more" pagination. When the
	// API's pagination.total is present it is authoritative — the
	// length>=pageSize heuristic would show "Load more" on a last page that
	// happens to be exactly pageSize items.
	const computeHasMore = (count: number, total: number | undefined): boolean =>
		total !== undefined ? count < total : count >= data.pageSize;

	let stories = $state<Story[]>(untrack(() => data.stories));
	let loadingMore = $state(false);
	let hasMore = $state(untrack(() => computeHasMore(data.stories.length, data.total)));

	// Sync from server data on navigation (sort/filter changes re-run load).
	$effect(() => {
		stories = data.stories;
		hasMore = computeHasMore(data.stories.length, data.total);
	});

	// Local filter input state, seeded from URL params.
	let originInput = $state(untrack(() => data.filters.origin));
	let destInput = $state(untrack(() => data.filters.dest));
	let visaInput = $state(untrack(() => data.filters.visa));
	let professionInput = $state(untrack(() => data.filters.profession));

	$effect(() => {
		originInput = data.filters.origin;
		destInput = data.filters.dest;
		visaInput = data.filters.visa;
		professionInput = data.filters.profession;
	});

	function buildParams(overrides: Record<string, string> = {}): URLSearchParams {
		const params = new URLSearchParams();
		const sort = overrides.sort ?? data.sort;
		const origin = overrides.origin ?? originInput;
		const dest = overrides.dest ?? destInput;
		const visa = overrides.visa ?? visaInput;
		const profession = overrides.profession ?? professionInput;
		if (sort && sort !== 'top') params.set('sort', sort);
		if (origin) params.set('origin', origin);
		if (dest) params.set('dest', dest);
		if (visa) params.set('visa', visa);
		if (profession) params.set('profession', profession);
		return params;
	}

	function navigate(params: URLSearchParams) {
		goto(`/stories${params.toString() ? `?${params.toString()}` : ''}`, { keepFocus: true });
	}

	function setSort(value: string) {
		navigate(buildParams({ sort: value }));
	}

	function applyFilters(e?: SubmitEvent) {
		e?.preventDefault();
		navigate(buildParams());
	}

	function clearFilters() {
		originInput = '';
		destInput = '';
		visaInput = '';
		professionInput = '';
		const params = new URLSearchParams();
		if (data.sort && data.sort !== 'top') params.set('sort', data.sort);
		navigate(params);
	}

	const hasActiveFilters = $derived(
		!!data.filters.origin || !!data.filters.dest || !!data.filters.visa || !!data.filters.profession
	);

	let showFilters = $state(false);
	$effect(() => {
		if (hasActiveFilters) showFilters = true;
	});

	const tabs = $derived<UrlTab[]>([
		{ value: 'top', label: t('stories.sort_top', 'Top') },
		{ value: 'latest', label: t('stories.sort_latest', 'Latest') },
		{ value: 'featured', label: t('stories.tab_featured', 'Featured') },
		{ value: 'series', label: t('stories.tab_series', 'Series') }
	]);

	const activeTab = $derived(data.tab || data.sort);

	function switchTab(value: string) {
		if (value === 'featured' || value === 'series') {
			// Curated views — filters don't apply; keep the URL minimal.
			goto(`/stories?tab=${value}`, { keepFocus: true });
			return;
		}
		setSort(value);
	}

	function storyToFeedItem(s: Story): FeedItem {
		return {
			id: s.id,
			title: s.title,
			slug: s.slug,
			content_type: 'story',
			author_id: s.author_id ?? '',
			author_name: s.author_display_name ?? s.author_name ?? s.author_username ?? '',
			author_username: s.author_username,
			author_avatar_url: s.author_avatar,
			excerpt: s.biggest_challenge ?? undefined,
			created_at: s.created_at,
			published_at: s.published_at,
			view_count: s.view_count,
			comment_count: s.comment_count,
			reaction_count: s.reaction_count,
			bookmark_count: s.bookmark_count,
			vote_score: 0,
			cover_image_url: s.featured_image_url ?? s.header_image_url ?? null,
			source_country: s.origin_space_name,
			target_country: s.destination_space_name,
			visa_route: s.visa_category
		};
	}

	function featuredToFeedItem(g: GuideListItem): FeedItem {
		return {
			id: g.id,
			title: g.title,
			slug: g.slug,
			content_type: 'story',
			author_id: g.author_id ?? '',
			author_name: g.author_display_name ?? g.author_username ?? '',
			author_username: g.author_username,
			author_avatar_url: g.author_avatar_url,
			excerpt: g.purpose ?? undefined,
			created_at: g.created_at,
			published_at: g.published_at ?? undefined,
			view_count: g.view_count,
			comment_count: g.comment_count,
			reaction_count: g.reaction_count,
			bookmark_count: g.bookmark_count,
			vote_score: g.reshare_count ?? g.share_count ?? 0,
			reading_time_minutes: g.read_time_minutes,
			cover_image_url: g.featured_image_url ?? g.header_image_url ?? null,
			source_country: g.source_country,
			target_country: g.target_country,
			visa_route: g.visa_route
		};
	}

	const feedItems = $derived(stories.map(storyToFeedItem));
	const featuredItems = $derived(data.featured.map(featuredToFeedItem));

	async function loadMore() {
		if (loadingMore || !hasMore) return;
		loadingMore = true;
		// Paginate with the *applied* filters (data.filters/data.sort), not the
		// pending form inputs — otherwise typing into a field without pressing
		// Apply would silently change the result set mid-pagination.
		const params = new URLSearchParams();
		if (data.sort && data.sort !== 'top') params.set('sort', data.sort);
		if (data.filters.origin) params.set('origin', data.filters.origin);
		if (data.filters.dest) params.set('dest', data.filters.dest);
		if (data.filters.visa) params.set('visa', data.filters.visa);
		if (data.filters.profession) params.set('profession', data.filters.profession);
		params.set('page_size', String(data.pageSize));
		params.set('page_offset', String(stories.length));
		const res = await api.getEnvelope<Story[]>(`/stories?${params.toString()}`);
		loadingMore = false;
		if (res.data) {
			stories = [...stories, ...res.data];
			const total = (res.envelope?.pagination as { total?: number } | undefined)?.total;
			hasMore = computeHasMore(stories.length, total ?? data.total);
		}
	}

	function fetchOriginCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('stories.filter_origin_all', 'All origins');
		const query = q.toLowerCase().trim();
		const allItem: AutocompleteItem = { id: '', label: allLabel };
		if (query === '') {
			return Promise.resolve([allItem, ...originCountries.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }))]);
		}
		const matchesAll = allLabel.toLowerCase().includes(query);
		const filtered = originCountries.filter(
			(c) => c.name.toLowerCase().includes(query) || c.geo_code.toLowerCase().includes(query)
		);
		const items = filtered.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }));
		return Promise.resolve(matchesAll ? [allItem, ...items] : items);
	}
	function fetchOriginCountriesInitial(): Promise<AutocompleteItem[]> {
		return fetchOriginCountries('');
	}
	const selectedOriginLabel = $derived(
		originInput ? (data.countries ?? []).find((c) => c.id === originInput)?.name ?? null : null
	);

	// ADR-064 curated markets (public settings) — pinned first in the
	// country pickers; every seeded country stays selectable.
	const destCountries = $derived(
		pinMarketsFirst(data.countries ?? [], destinationMarkets(data.siteSettings), (c) => c.geo_code)
	);
	const originCountries = $derived(
		pinMarketsFirst(data.countries ?? [], originMarkets(data.siteSettings), (c) => c.geo_code)
	);

	function fetchDestinationCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('stories.filter_dest_all', 'All destinations');
		const query = q.toLowerCase().trim();
		const allItem: AutocompleteItem = { id: '', label: allLabel };
		if (query === '') {
			return Promise.resolve([allItem, ...destCountries.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }))]);
		}
		const matchesAll = allLabel.toLowerCase().includes(query);
		const filtered = destCountries.filter(
			(c) => c.name.toLowerCase().includes(query) || c.geo_code.toLowerCase().includes(query)
		);
		const items = filtered.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }));
		return Promise.resolve(matchesAll ? [allItem, ...items] : items);
	}
	function fetchDestinationCountriesInitial(): Promise<AutocompleteItem[]> {
		return fetchDestinationCountries('');
	}
	const selectedDestinationLabel = $derived(
		destInput ? (data.countries ?? []).find((c) => c.id === destInput)?.name ?? null : null
	);

	// Visa/profession are free-text server filters — suggest values observed
	// in the loaded corpus, and allow arbitrary typed input via allowFreeText.
	const visaFetcher = $derived(
		staticAutocompleteFetcher(autocompleteItems(data.stories.map((s) => s.visa_category)))
	);
	const professionFetcher = $derived(
		staticAutocompleteFetcher(autocompleteItems(data.stories.map((s) => s.profession)))
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	<h1 class="sr-only">{t('stories.heading', 'Community Stories')}</h1>
	<ListPageBar>
		<UrlTabs {tabs} value={activeTab} onValueChange={(v) => switchTab(v)} class="!border-b-0 !bg-transparent" />
		{#snippet actions()}
			{#if data.tab !== 'featured' && data.tab !== 'series'}
				<FilterToggle bind:show={showFilters} active={hasActiveFilters} controls="stories-filter-panel" />
			{/if}
			<PageAction
				href={data.authed ? '/studio/create?type=story' : '/login?redirect=/studio%2Fcreate%3Ftype%3Dstory'}
				icon={PenSquare}
				label={t('stories.share', 'Share Your Story')}
			/>
		{/snippet}
	</ListPageBar>

	<!-- Story list -->
	<div class="mx-auto w-full max-w-3xl p-4">
		{#if showFilters && data.tab !== 'featured' && data.tab !== 'series'}
			<!-- Filters (autocomplete for all four: origin, dest, visa, profession) -->
			<div id="stories-filter-panel" class="mb-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
		<form onsubmit={applyFilters} class="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
			<Autocomplete
				placeholder={t('stories.filter_origin_all', 'All origins')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={fetchOriginCountries}
				initialFetcher={fetchOriginCountriesInitial}
				selectedId={originInput || null}
				selectedLabel={selectedOriginLabel}
				onSelect={(item) => { originInput = item.id; applyFilters(); }}
				onClear={() => { originInput = ''; applyFilters(); }}
			/>
			<Autocomplete
				placeholder={t('stories.filter_dest_all', 'All destinations')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={fetchDestinationCountries}
				initialFetcher={fetchDestinationCountriesInitial}
				selectedId={destInput || null}
				selectedLabel={selectedDestinationLabel}
				onSelect={(item) => { destInput = item.id; applyFilters(); }}
				onClear={() => { destInput = ''; applyFilters(); }}
			/>
			<Autocomplete
				placeholder={t('stories.filter_visa', 'Visa type')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={visaFetcher}
				initialFetcher={visaFetcher}
				selectedId={visaInput || null}
				selectedLabel={visaInput || null}
				allowFreeText
				onSelect={(item) => { visaInput = item.id; applyFilters(); }}
				onClear={() => { visaInput = ''; applyFilters(); }}
			/>
			<Autocomplete
				placeholder={t('stories.filter_profession', 'Profession')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={professionFetcher}
				initialFetcher={professionFetcher}
				selectedId={professionInput || null}
				selectedLabel={professionInput || null}
				allowFreeText
				onSelect={(item) => { professionInput = item.id; applyFilters(); }}
				onClear={() => { professionInput = ''; applyFilters(); }}
			/>
		</form>

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

		{#if data.tab === 'series'}
			{#if data.series.length === 0}
				<EmptyState icon={BookOpen} title={t('blog_series.empty', 'No series yet')} />
			{:else}
				<div class="grid gap-3 sm:grid-cols-2">
					{#each data.series as series (series.id)}
						<a
							href="/series/{series.slug}"
							class="group flex flex-col rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							<div class="flex items-start gap-3">
								{#if series.cover_image_url}
									<img
										src={series.cover_image_url}
										alt=""
										class="h-10 w-10 shrink-0 rounded-[var(--radius)] object-cover"
									/>
								{:else}
									<span
										class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
									>
										<BookOpen size={20} />
									</span>
								{/if}
								<div class="min-w-0 flex-1">
									<h2 class="truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]">
										{series.title}
									</h2>
									{#if series.status}
										<span class="mt-0.5 inline-block text-xs capitalize text-[var(--color-muted)]">
											{series.status}
										</span>
									{/if}
								</div>
							</div>
							{#if series.description}
								<p class="mt-3 line-clamp-2 flex-1 text-sm text-[var(--color-text-subtle)]">
									{series.description}
								</p>
							{/if}
							<div class="mt-3 flex items-center justify-end text-xs text-[var(--color-text-subtle)]">
								<span class="flex items-center gap-1 group-hover:text-[var(--color-primary)]">
									{t('blog_series.view', 'View series')} <ArrowRight class="rtl:-scale-x-100" size={14} />
								</span>
							</div>
						</a>
					{/each}
				</div>
			{/if}
		{:else if data.tab === 'featured'}
			{#if data.loadError}
				<EmptyState
					icon={BookOpen}
					title={t('stories.load_error', 'Could not load stories')}
					body={t('stories.load_error_body', 'Please try again later.')}
				/>
			{:else if featuredItems.length === 0}
				<EmptyState
					icon={BookOpen}
					title={t('stories.featured_empty', 'No featured stories yet')}
					body={t('stories.featured_empty_body', 'Curators have not featured any stories yet.')}
				/>
			{:else}
				<div class="grid gap-3.5 sm:grid-cols-2">
					{#each featuredItems as item (item.id)}
						<ContentGridCard {item} />
					{/each}
				</div>
			{/if}
		{:else if data.loadError}
			<EmptyState
				icon={BookOpen}
				title={t('stories.load_error', 'Could not load stories')}
				body={t('stories.load_error_body', 'Please try again later.')}
			/>
		{:else if stories.length === 0}
			<EmptyState
				icon={BookOpen}
				title={t('stories.empty', 'No stories yet')}
				body={t('stories.empty_body', 'Be the first to share your relocation journey.')}
				action={data.authed
					? { label: t('stories.share', 'Share Your Story'), href: '/studio/create?type=story' }
					: { label: t('stories.share', 'Share Your Story'), href: '/login?redirect=/studio%2Fcreate%3Ftype%3Dstory' }}
			/>
		{:else}
			<div class="grid gap-3.5 sm:grid-cols-2">
				{#each feedItems as item (item.id)}
					<ContentGridCard {item} />
				{/each}
			</div>

			{#if hasMore}
				<div class="mt-4 flex justify-center">
					<button
						type="button"
						onclick={loadMore}
						disabled={loadingMore}
						class="inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>
						{#if loadingMore}
							<Loader2 size={16} class="animate-spin" />
							{t('stories.loading', 'Loading…')}
						{:else}
							{t('stories.load_more', 'Load more')}
						{/if}
					</button>
				</div>
			{/if}
		{/if}
	</div>
</div>
