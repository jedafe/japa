<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { Layers, Users, MessageSquare, Plus, ShieldX, ChevronLeft, ChevronRight, X } from 'lucide-svelte';
	import { EmptyState, SpaceJoinButton, SearchInput, DirectoryCard, ListPageBar, FilterToggle, PageAction, SelectField, UrlTabs, type SearchSuggestion, type CardMetaItem, type UrlTab } from '$lib/components/ui';
	import Autocomplete, { type AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { api } from '$lib/api/client';
	import SpaceCreationModal from '$lib/components/spaces/SpaceCreationModal.svelte';
	import SpaceCombobox from '$lib/components/spaces/SpaceCombobox.svelte';
	import { decodeBase64Text } from '$lib/tiptap/serialize';
	import { pinMarketsFirst, servedMarkets } from '$lib/utils/geoMarkets';
	import { flagUrl } from '$lib/utils/flags';
	import type { Space } from '$lib/types';
	import type { PageData } from './$types';
import { safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	let showCreateModal = $state(false);

	const title = $derived(t('spaces.title', 'Spaces — {site}'));
	const description = $derived(
		t('spaces.description', 'Discover and join spaces — countries, states, cities, and topical communities.')
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'CollectionPage',
			name: 'Spaces',
			description,
			url: page.url.href
		})
	);

	let query = $state(untrack(() => data.q));
	let selectedType = $state(untrack(() => data.type));
	let selectedLevel = $state(untrack(() => data.level));
	let selectedSort = $state(untrack(() => data.sort));
	let selectedGeo = $state(untrack(() => data.geo));
	let selectedRegion = $state(untrack(() => data.region));
	let selectedSubregion = $state(untrack(() => data.subregion));
	let selectedParent = $state<{ id: string; name: string; slug: string } | null>(
		untrack(() => data.parent ? { id: data.parent, name: data.parentName || data.parent, slug: '' } : null)
	);
	// "My spaces" is a server-side scope (?joined=1) — the API returns only
	// spaces the viewer is an approved member of, with correct pagination
	// (audit fix: the chip used to filter just the current page client-side).
	let joinedOnly = $state(untrack(() => data.joined ?? false));
	const searchPlaceholder = $derived(t('spaces.search_placeholder', 'Search spaces…'));

	// Sync local filter state with the URL-driven server data on navigation
	// (same pattern as /sources — keeps controls honest on back/forward nav).
	$effect(() => {
		query = data.q;
		selectedType = data.type;
		selectedLevel = data.level;
		selectedSort = data.sort;
		selectedGeo = data.geo;
		selectedRegion = data.region;
		selectedSubregion = data.subregion;
		selectedParent = data.parent
			? { id: data.parent, name: data.parentName || data.parent, slug: '' }
			: null;
		joinedOnly = data.joined ?? false;
	});

	// Filter panel (ListPageBar + FilterToggle pattern — same as /blog, /sources).
	// `joined` is a scope tab now, not a filter, so it doesn't light the dot.
	const hasActiveFilters = $derived(
		!!selectedType || !!selectedLevel || !!selectedSort || !!selectedGeo.trim() ||
		!!selectedRegion || !!selectedSubregion || !!selectedParent
	);
	let showFilters = $state(false);
	$effect(() => {
		if (hasActiveFilters) showFilters = true;
	});

	const visibleSpaces = $derived(data.spaces);
	const visibleTotal = $derived(data.total);

	const sortOptions = $derived([
		{ value: '', label: t('spaces.sort_default', 'Default') },
		{ value: 'members', label: t('spaces.sort_members', 'Most members') },
		{ value: 'followers', label: t('spaces.sort_followers', 'Most followers') },
		{ value: 'posts', label: t('spaces.sort_posts', 'Most posts') },
		{ value: 'name', label: t('spaces.sort_name', 'Name') },
		{ value: 'newest', label: t('spaces.sort_newest', 'Newest') },
		{ value: 'activity', label: t('spaces.sort_activity', 'Most active') }
	]);

	// Rule 16: level list from system_settings; labels are i18n keys.
	function humanizeLevel(level: string): string {
		return level.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
	}

	// Type and level read as one axis to users: the geo types (Country/State/
	// City) already express the geo levels, and member-created spaces are all
	// level=custom (SpaceCreationModal hardcodes it). So a single "Type"
	// picker lists every space type plus the levels no type covers — a
	// `level:<l>` option still writes ?level=, keeping old URLs working.
	const typeCoveredLevels = $derived(
		new Set(data.spaceTypes.flatMap((st) => [st.slug, st.name.toLowerCase()]))
	);
	const kindOptions = $derived([
		{ value: '', label: t('common.all_types', 'All types') },
		// §80 — wiki-only types (attraction) live under /wiki, not /spaces;
		// offering them here filters to an empty list.
		...data.spaceTypes.filter((st) => !st.wiki_only).map((st) => ({ value: st.slug, label: st.name })),
		...(data.levels ?? [])
			.filter((l) => !typeCoveredLevels.has(l))
			.map((l) => ({
				value: `level:${l}`,
				label:
					l === 'custom'
						? t('spaces.member_created', 'Member-created')
						: t(`spaces.level_${l}`, humanizeLevel(l))
			}))
	]);
	// The picker's value: the type slug, or `level:<l>` for a level-only
	// option. A ?level= URL that a type already covers (e.g. level=country)
	// displays as that type — equivalent filter, cleaner presentation.
	const kindValue = $derived(
		selectedType
			? selectedType
			: selectedLevel
				? typeCoveredLevels.has(selectedLevel)
					? (data.spaceTypes.find(
							(st) => st.slug === selectedLevel || st.name.toLowerCase() === selectedLevel
						)?.slug ?? `level:${selectedLevel}`)
					: `level:${selectedLevel}`
				: ''
	);
	function onKindChange(v: string) {
		if (v.startsWith('level:')) {
			selectedType = '';
			selectedLevel = v.slice('level:'.length);
		} else {
			selectedType = v;
			selectedLevel = '';
		}
		onFilterChange();
	}

	const regionOptions = $derived([
		{ value: '', label: t('spaces.region_all', 'All regions') },
		...data.regions.map((r) => ({ value: r, label: r }))
	]);

	// Scope tabs — ?joined=1 switches between the full directory and the
	// viewer's memberships (guests only get the directory).
	const scopeTabs = $derived<UrlTab[]>([
		{ value: 'all', label: t('spaces.tab_all', 'All spaces') },
		{ value: 'joined', label: t('spaces.joined_only', 'My spaces') }
	]);

	// Country picker for ?geo= — filters the server-loaded /geo/countries
	// list client-side (same pattern as /scholarships), submits the geo_code.
	// The configured origin/destination markets (ADR-064, public settings)
	// are pinned first; every seeded country stays selectable.
	const geoPins = $derived(servedMarkets(data.siteSettings));
	const orderedCountries = $derived(pinMarketsFirst(data.countries, geoPins, (c) => c.geo_code));
	function toCountryItems(list: PageData['countries']): AutocompleteItem[] {
		return list.map((c) => ({ id: c.geo_code, label: c.name, sublabel: c.geo_code }));
	}
	function countryFetcher(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase().trim();
		const filtered = query === ''
			? orderedCountries
			: orderedCountries.filter(
					(c) => c.name.toLowerCase().includes(query) || c.geo_code.toLowerCase().includes(query)
				);
		return Promise.resolve(toCountryItems(filtered));
	}
	const geoSelectedLabel = $derived(
		data.countries.find((c) => c.geo_code === selectedGeo)?.name ?? (selectedGeo || null)
	);

	const followingSpaceIdSet = $derived(new Set(data.followingSpaceIds ?? []));

	function buildHref(pageNum: number): string {
		const params = new URLSearchParams();
		if (data.q) params.set('q', data.q);
		if (data.type) params.set('type', data.type);
		if (data.level) params.set('level', data.level);
		if (data.sort) params.set('sort', data.sort);
		if (data.geo) params.set('geo', data.geo);
		if (data.region) params.set('region', data.region);
		if (data.subregion) params.set('subregion', data.subregion);
		if (data.parent) params.set('parent', data.parent);
		if (data.joined) params.set('joined', '1');
		if (pageNum > 0) params.set('page', String(pageNum));
		const qs = params.toString();
		return qs ? `/spaces?${qs}` : '/spaces';
	}

	function onFilterChange() {
		const params = new URLSearchParams();
		if (query.trim()) params.set('q', query.trim());
		if (selectedType) params.set('type', selectedType);
		if (selectedLevel) params.set('level', selectedLevel);
		if (selectedSort) params.set('sort', selectedSort);
		if (selectedGeo.trim()) params.set('geo', selectedGeo.trim());
		if (selectedRegion) params.set('region', selectedRegion);
		if (selectedSubregion) params.set('subregion', selectedSubregion);
		if (selectedParent) params.set('parent', selectedParent.id);
		if (joinedOnly) params.set('joined', '1');
		const qs = params.toString();
		goto(qs ? `/spaces?${qs}` : '/spaces', { keepFocus: true });
	}

	// Clear keeps the search query and scope tab — only the panel resets.
	function clearFilters() {
		selectedType = '';
		selectedLevel = '';
		selectedSort = '';
		selectedGeo = '';
		selectedRegion = '';
		selectedSubregion = '';
		selectedParent = null;
		onFilterChange();
	}

	// Look up the space type name by ID from the data-driven type list.
	const spaceTypeMap = $derived(
		data.spaceTypes.reduce((m, t) => { m[t.id] = t.name; return m; }, {} as Record<string, string>)
	);

	// Extract a short description for space cards: prefer space.description,
	// fall back to the Wikipedia extract from wiki_content (base64-encoded JSON).
	function cardExcerpt(space: Space): string {
		if (space.description) return space.description;
		if (!space.wiki_content) return '';
		try {
			const json = typeof space.wiki_content === 'string'
				? JSON.parse(decodeBase64Text(space.wiki_content))
				: space.wiki_content;
			return json?.extract ?? '';
		} catch {
			return '';
		}
	}

	async function spaceSearchFetcher(q: string): Promise<SearchSuggestion[]> {
		const res = await api.get<{ results: { type: string; item: { id: string; name: string; excerpt?: string; url: string } }[] }>(
			`/spaces/search?q=${encodeURIComponent(q)}&limit=8`
		);
		if (res.error || !res.data) return [];
		return (res.data.results ?? []).map((r) => ({
			id: r.item.id,
			title: r.item.name,
			url: r.item.url,
			excerpt: r.item.excerpt,
			type: r.type
		}));
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	{@html safeJsonLdScript(jsonLd)}
</svelte:head>

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	<!-- Header + search (H1/description; Filter + Create controls sit on the heading row) -->
	<div class="border-b border-[var(--color-border)] px-4 py-4" style="margin-inline: var(--page-gutter)">
		<div class="flex items-start justify-between gap-3">
			<div class="min-w-0">
				<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
					<Layers size={20} /> {t('spaces.heading', 'Spaces')}
				</h1>
				<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
					{t('spaces.subtitle', 'Join spaces to connect with people who share your interests.')}
				</p>
			</div>
			<!-- Page controls on the heading row: Create only — the Filters
			     toggle lives on the ListPageBar (same pattern as /blog).
			     PageAction keeps its mobile FAB (portaled to <body>), so this
			     wrapper only carries the desktop layout. -->
			<div class="flex shrink-0 items-center gap-2 pt-0.5">
				{#if page.data.user}
					<PageAction
						onclick={() => (showCreateModal = true)}
						icon={Plus}
						label={t('spaces.create', 'Create')}
					/>
				{/if}
			</div>
		</div>

		<form class="mt-3" action="/spaces" method="get" role="search">
			<SearchInput
				bind:value={query}
				placeholder={searchPlaceholder}
				ariaLabel={t('spaces.heading', 'Spaces')}
				fetcher={spaceSearchFetcher}
			/>
			{#if selectedType}
				<input type="hidden" name="type" value={selectedType} />
			{/if}
			{#if selectedLevel}
				<input type="hidden" name="level" value={selectedLevel} />
			{/if}
			{#if selectedSort}
				<input type="hidden" name="sort" value={selectedSort} />
			{/if}
			{#if selectedGeo}
				<input type="hidden" name="geo" value={selectedGeo} />
			{/if}
			{#if selectedRegion}
				<input type="hidden" name="region" value={selectedRegion} />
			{/if}
			{#if selectedSubregion}
				<input type="hidden" name="subregion" value={selectedSubregion} />
			{/if}
			{#if selectedParent}
				<input type="hidden" name="parent" value={selectedParent.id} />
			{/if}
			{#if joinedOnly}
				<input type="hidden" name="joined" value="1" />
			{/if}
		</form>
	</div>

	<!-- Scope tabs (All / My spaces) + filter toggle — the ListPageBar/UrlTabs
	     pattern used by /blog and /sources. Guests see the plain count. -->
	<ListPageBar>
		{#if page.data.user}
			<UrlTabs
				tabs={scopeTabs}
				value={joinedOnly ? 'joined' : 'all'}
				onValueChange={(v) => { joinedOnly = v === 'joined'; onFilterChange(); }}
				class="!border-b-0 !bg-transparent"
			/>
		{:else}
			<span class="flex items-center px-4 text-sm text-[var(--color-muted)]" style="min-height: var(--nav-height)">
				{visibleTotal} {t('spaces.results', 'spaces')}
			</span>
		{/if}
		{#snippet actions()}
			{#if page.data.user}
				<span class="hidden text-xs text-[var(--color-muted)] sm:inline">
					{visibleTotal} {t('spaces.results', 'spaces')}
				</span>
			{/if}
			<FilterToggle bind:show={showFilters} active={hasActiveFilters} controls="spaces-filter-panel" />
		{/snippet}
	</ListPageBar>

	{#if showFilters}
		<!-- Filter panel — one uniform field grid (SelectField/Autocomplete),
		     same pattern as /sources and /blog. Type+level are merged into a
		     single picker; ?subregion= stays URL-only (no dedicated control). -->
		<div id="spaces-filter-panel" class="border-b border-[var(--color-border)] px-4 py-3" style="margin-inline: var(--page-gutter)">
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
				{#if kindOptions.length > 1}
					<div>
						<label for="spaces-filter-type" class="sr-only">{t('spaces.type_label', 'Type')}</label>
						<SelectField
							id="spaces-filter-type"
							value={kindValue}
							options={kindOptions}
							placeholder={t('common.all_types', 'All types')}
							onValueChange={onKindChange}
							class="w-full"
						/>
					</div>
				{/if}

				<div>
					<Autocomplete
						id="spaces-filter-country"
						placeholder={t('common.all_countries', 'All countries')}
						ariaLabel={t('spaces.country_label', 'Country')}
						emptyMessage={t('spaces.no_countries', 'No countries found')}
						fetcher={countryFetcher}
						initialFetcher={async () => toCountryItems(data.countries)}
						selectedId={selectedGeo || null}
						selectedLabel={geoSelectedLabel}
						onSelect={(item) => { selectedGeo = item.id; onFilterChange(); }}
						onClear={() => { selectedGeo = ''; onFilterChange(); }}
					/>
				</div>

				{#if regionOptions.length > 1}
					<div>
						<label for="spaces-filter-region" class="sr-only">{t('spaces.region_label', 'Region')}</label>
						<SelectField
							id="spaces-filter-region"
							value={selectedRegion}
							options={regionOptions}
							placeholder={t('spaces.region_all', 'All regions')}
							onValueChange={(v) => { selectedRegion = v; selectedSubregion = ''; onFilterChange(); }}
							class="w-full"
						/>
					</div>
				{/if}

				<div class="sm:col-span-2">
					<SpaceCombobox
						parentMode
						placeholder={t('spaces.parent_placeholder', 'Filter by parent space…')}
						selected={selectedParent}
						onSelect={(space) => { selectedParent = space; onFilterChange(); }}
						onRemove={() => { selectedParent = null; onFilterChange(); }}
					/>
				</div>

				<div>
					<label for="spaces-filter-sort" class="sr-only">{t('spaces.sort_label', 'Sort')}</label>
					<SelectField
						id="spaces-filter-sort"
						value={selectedSort}
						options={sortOptions}
						placeholder={t('spaces.sort_label', 'Sort')}
						onValueChange={(v) => { selectedSort = v; onFilterChange(); }}
						class="w-full"
					/>
				</div>
			</div>

			{#if hasActiveFilters}
				<div class="mt-3 flex justify-end">
					<button
						type="button"
						onclick={clearFilters}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<X size={14} /> {t('common.clear_filters', 'Clear filters')}
					</button>
				</div>
			{/if}
		</div>
	{/if}

	<div class="mx-auto w-full max-w-3xl p-4">
		{#if data.forbidden}
			<div class="flex flex-col items-center gap-3 py-16 text-center">
				<ShieldX size={40} class="text-[var(--color-muted)]" />
				<p class="text-sm font-medium text-[var(--color-text)]">
					{t('spaces.forbidden', 'You do not have permission to view this directory.')}
				</p>
			</div>
		{:else if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('spaces.load_error', 'Failed to load spaces. Please try again later.')}
			</p>
		{:else if visibleSpaces.length === 0}
			<EmptyState icon={Layers} title={t('spaces.empty', 'No spaces yet.')} />
		{:else}
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{#each visibleSpaces as space (space.id)}
					{@const cardMeta: CardMetaItem[] = [
						{ icon: Users, text: t('meta.members', '{n} members').replace('{n}', String(space.member_count)) },
						{ icon: MessageSquare, text: t('discover.posts', '{n} posts').replace('{n}', String(space.post_count)) }
					]}
					<DirectoryCard
						href="/spaces/{space.slug}"
						title={space.name}
						subtitle={spaceTypeMap[space.space_type_id] ?? 'Space'}
						description={cardExcerpt(space) || null}
						imageUrl={(space.level === 'country' && flagUrl(space.geo_code)) || space.logo_url}
						fallback={space.name.charAt(0).toUpperCase()}
						hueSeed={space.id}
						meta={cardMeta}
						entityType="space"
						entityId={space.id}
						entitySlug={space.slug}
						entityCount={space.member_count}
					>
						{#snippet actions()}
							{#if page.data.user}
								<SpaceJoinButton
									entityId={space.id}
									entitySlug={space.slug}
									initiallyJoined={space.is_member ?? false}
									initiallyPending={space.is_pending ?? false}
									initiallyFollowing={space.is_following ?? followingSpaceIdSet.has(space.id)}
									compact
								/>
							{/if}
						{/snippet}
					</DirectoryCard>
				{/each}
			</div>

			<!-- Pagination -->
			{#if data.totalPages > 1}
				<div class="mt-6 flex items-center justify-center gap-4">
					<a
						href={buildHref(data.page - 1)}
						class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] {data.page === 0 ? 'pointer-events-none opacity-40' : ''}"
					>
						<ChevronLeft class="rtl:-scale-x-100" size={16} /> {t('common.prev', 'Prev')}
					</a>
					<span class="text-xs text-[var(--color-muted)]">
						{data.page + 1} / {data.totalPages}
					</span>
					<a
						href={buildHref(data.page + 1)}
						class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] {data.page >= data.totalPages - 1 ? 'pointer-events-none opacity-40' : ''}"
					>
						{t('common.next', 'Next')} <ChevronRight class="rtl:-scale-x-100" size={16} />
					</a>
				</div>
			{/if}
		{/if}
	</div>
</div>

<SpaceCreationModal bind:open={showCreateModal} />
