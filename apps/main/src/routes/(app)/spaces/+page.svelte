<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { Filter, Layers, Users, MessageSquare, Plus, ShieldX, ChevronLeft, ChevronRight, X, Search, Globe } from 'lucide-svelte';
	import { EmptyState, EntityHero, SpaceJoinButton, SearchInput, DirectoryCard, ListPageBar, SelectField, UrlTabs, type SearchSuggestion, type CardMetaItem, type UrlTab } from '$lib/components/ui';
	import { GuideSlideOver, Modal, type GuideContent } from '$lib/components/ui';
	import HeroArtSpaces from '$lib/components/heroes/HeroArtSpaces.svelte';
	import { formatMetric } from '$lib/components/hub/charts/format';
	import { api } from '$lib/api/client';
	import SpaceCreationModal from '$lib/components/spaces/SpaceCreationModal.svelte';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { decodeBase64Text } from '$lib/tiptap/serialize';
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
	let selectedSort = $state(untrack(() => data.sort));
	// Geo cascade (universities-style filter fields): country is a geo_code,
	// state/city are primary-space slugs — the server maps them onto
	// ?geo= / ?parent= respectively.
	let selectedCountry = $state(untrack(() => data.country ?? ''));
	let selectedState = $state(untrack(() => data.state ?? ''));
	let selectedCity = $state(untrack(() => data.city ?? ''));
	// "My spaces" is a server-side scope (?joined=1) — the API returns only
	// spaces the viewer is an approved member of, with correct pagination
	// (audit fix: the chip used to filter just the current page client-side).
	let joinedOnly = $state(untrack(() => data.joined ?? false));
	const searchPlaceholder = $derived(t('spaces.search_placeholder', 'Search spaces…'));

	// Sync local filter state with the URL-driven server data on navigation
	// (same pattern as /sources — keeps controls honest on back/forward nav).
	$effect(() => {
		query = data.q;
		selectedSort = data.sort;
		selectedCountry = data.country ?? '';
		selectedState = data.state ?? '';
		selectedCity = data.city ?? '';
		joinedOnly = data.joined ?? false;
	});

	// Filter panel (ListPageBar + FilterToggle pattern — same as /blog, /sources).
	// `joined` is a scope tab now, not a filter, so it doesn't light the dot.
	const hasActiveFilters = $derived(
		!!selectedCountry || !!selectedState || !!selectedCity || !!selectedSort ||
		// Legacy URL params still resolve server-side — light the dot so the
		// clear control appears on hand-crafted links too.
		!!data.type || !!data.level || !!data.geo || !!data.region ||
		!!data.subregion || !!data.parent
	);
	let filtersModalOpen = $state(false);
	let searchModalOpen = $state(false);

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

	// Geo-cascade autocompletes — same fetch pattern as /universities:
	// countries from the canonical /geo/countries list, states/cities
	// preloaded server-side for the current selections.
	const allLabel = t('universities.all_countries', 'All countries');
	const allStatesLabel = t('universities.all_states', 'All states');
	const allCitiesLabel = t('universities.all_cities', 'All cities');

	function fetchCountries(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase().trim();
		const allItem: AutocompleteItem = { id: '', label: allLabel };
		const items = (data.countries ?? []).map((c) => ({ id: c.geo_code, label: c.name, sublabel: c.geo_code }));
		if (query === '') return Promise.resolve([allItem, ...items]);
		const filtered = items.filter(
			(c) => c.label.toLowerCase().includes(query) || c.id.toLowerCase().includes(query)
		);
		return Promise.resolve(allLabel.toLowerCase().includes(query) ? [allItem, ...filtered] : filtered);
	}
	const fetchCountriesInitial = () => fetchCountries('');
	const selectedCountryLabel = $derived(
		selectedCountry ? data.countries?.find((c) => c.geo_code === selectedCountry)?.name ?? selectedCountry : allLabel
	);

	function fetchStates(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase().trim();
		const allItem: AutocompleteItem = { id: '', label: allStatesLabel };
		const items = (data.states ?? []).map((s) => ({ id: s.slug, label: s.name }));
		if (query === '') return Promise.resolve([allItem, ...items]);
		const filtered = items.filter((s) => s.label.toLowerCase().includes(query));
		return Promise.resolve(allStatesLabel.toLowerCase().includes(query) ? [allItem, ...filtered] : filtered);
	}
	const fetchStatesInitial = () => fetchStates('');
	const selectedStateLabel = $derived(
		selectedState ? data.states?.find((s) => s.slug === selectedState)?.name ?? selectedState : allStatesLabel
	);

	function fetchCities(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase().trim();
		const allItem: AutocompleteItem = { id: '', label: allCitiesLabel };
		const items = (data.cities ?? []).map((c) => ({ id: c.slug, label: c.name }));
		if (query === '') return Promise.resolve([allItem, ...items]);
		const filtered = items.filter((c) => c.label.toLowerCase().includes(query));
		return Promise.resolve(allCitiesLabel.toLowerCase().includes(query) ? [allItem, ...filtered] : filtered);
	}
	const fetchCitiesInitial = () => fetchCities('');
	const selectedCityLabel = $derived(
		selectedCity ? data.cities?.find((c) => c.slug === selectedCity)?.name ?? selectedCity : allCitiesLabel
	);

	// Scope tabs — ?joined=1 switches between the full directory and the
	// viewer's memberships (guests only get the directory).
	const scopeTabs = $derived<UrlTab[]>([
		{ value: 'all', label: t('spaces.tab_all', 'All spaces') },
		{ value: 'joined', label: t('spaces.joined_only', 'My spaces') }
	]);

	const followingSpaceIdSet = $derived(new Set(data.followingSpaceIds ?? []));

	function buildHref(pageNum: number): string {
		const params = new URLSearchParams();
		if (data.q) params.set('q', data.q);
		if (data.country) params.set('country', data.country);
		if (data.state) params.set('state', data.state);
		if (data.city) params.set('city', data.city);
		if (data.sort) params.set('sort', data.sort);
		if (data.joined) params.set('joined', '1');
		if (pageNum > 0) params.set('page', String(pageNum));
		const qs = params.toString();
		return qs ? `/spaces?${qs}` : '/spaces';
	}

	function onFilterChange() {
		const params = new URLSearchParams();
		if (query.trim()) params.set('q', query.trim());
		if (selectedCountry) params.set('country', selectedCountry);
		if (selectedState) params.set('state', selectedState);
		if (selectedCity) params.set('city', selectedCity);
		if (selectedSort) params.set('sort', selectedSort);
		if (joinedOnly) params.set('joined', '1');
		const qs = params.toString();
		goto(qs ? `/spaces?${qs}` : '/spaces', { keepFocus: true });
	}

	// Clear keeps the search query and scope tab — only the panel resets.
	function clearFilters() {
		selectedCountry = '';
		selectedState = '';
		selectedCity = '';
		selectedSort = '';
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

	// Suggestions are scoped to member-created spaces — the same
	// member_created=1 filter the directory list uses (§05 amendment):
	// primary/geo spaces are reachable via /wiki and /universities, not here.
	async function spaceSearchFetcher(q: string): Promise<SearchSuggestion[]> {
		const res = await api.get<{ items: { id: string; name: string; slug: string; description?: string }[] }>(
			`/spaces?q=${encodeURIComponent(q)}&member_created=1&page_size=8`
		);
		if (res.error || !res.data) return [];
		return (res.data.items ?? []).map((s) => ({
			id: s.id,
			title: s.name,
			url: `/spaces/${s.slug}`,
			excerpt: s.description,
			type: 'space'
		}));
	}

	// "How it works" slide-over — markdown panel + CTA repeater persisted to
	// public system settings (spaces.guide) via PUT /admin/settings/general.
	let guideOpen = $state(false);
	let guideOverride = $state<GuideContent | null>(null);
	const siteSettings = $derived((page.data.siteSettings ?? {}) as Record<string, unknown>);
	const storedGuide = $derived.by((): Partial<GuideContent> => {
		const raw = siteSettings['spaces.guide'];
		if (raw && typeof raw === 'object' && !Array.isArray(raw)) return raw as Partial<GuideContent>;
		return {};
	});
	const effectiveGuide = $derived<GuideContent>({
		title: guideOverride?.title ?? storedGuide.title ?? t('common.how_it_works', 'How it works'),
		body: guideOverride?.body ?? storedGuide.body ?? t('spaces.guide_body', ''),
		ctas: guideOverride?.ctas ?? storedGuide.ctas ?? [{ title: t('guide_panel.read_full_guide', 'Read the full guide'), url: '/guides' }]
	});

	async function saveGuide(v: GuideContent): Promise<string | null> {
		const res = await api.put('/admin/settings/general', { spaces_guide: v });
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
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	{@html safeJsonLdScript(jsonLd)}
</svelte:head>

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	<!-- Header — EntityHero directory panel -->
	<EntityHero
		class="mt-4"
		icon={Layers}
		accent="var(--color-primary)"
		heading={t('spaces.heading', 'Spaces')}
		description={t('spaces.hero_desc', 'Join countries, cities, and topical communities — find your people wherever the journey takes you.')}
		crumbs={[
			{ label: t('common.home', 'Home'), href: '/' },
			{ label: t('spaces.heading', 'Spaces') }
		]}
		stats={[
			{ icon: Layers, value: formatMetric(data.total), label: t('spaces.stats_spaces', 'Spaces') },
			{ icon: Globe, value: formatMetric(data.countries.length), label: t('spaces.stats_countries', 'Countries') }
		]}
	>
		{#snippet art()}
			<HeroArtSpaces accent="var(--color-primary)" />
		{/snippet}
		{#snippet actions(c)}
			<div class="grid w-full grid-cols-2 gap-3 sm:flex sm:items-center">
			<a href="#spaces-results" class="{c.primary} flex-1 justify-center whitespace-nowrap">
				<Search size={15} /> {t('spaces.find', 'Find a space')}
			</a>
			{#if page.data.user}
				<button type="button" onclick={() => (showCreateModal = true)} class="{c.secondary} flex-1 justify-center whitespace-nowrap">
					<Plus size={15} /> {t('spaces.create_space', 'Create a space')}
				</button>
			{/if}
			<button type="button" onclick={() => (guideOpen = true)} class="{c.ghost} flex-1 justify-center whitespace-nowrap">
				{t('common.how_it_works', 'How it works')}
				<ChevronRight size={14} class="rtl:-scale-x-100" />
				</button>
			</div>
		{/snippet}
	</EntityHero>

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
			<button type="button" onclick={() => (filtersModalOpen = true)} class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]">
				{t('common.filters', 'Filters')}
				{#if hasActiveFilters}<span class="ms-0.5 h-2 w-2 rounded-full bg-[var(--color-primary)]" aria-hidden="true"></span>{/if}
				<Filter size={16} />
			</button>
			<button type="button" onclick={() => (searchModalOpen = true)} class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]">
				{t('common.search', 'Search')}
				{#if query.trim()}<span class="ms-0.5 h-2 w-2 rounded-full bg-[var(--color-primary)]" aria-hidden="true"></span>{/if}
				<Search size={16} />
			</button>
		{/snippet}
	</ListPageBar>

	<Modal bind:open={searchModalOpen} title={t('common.search', 'Search')} icon={Search} size="sm">
		{#snippet children()}
			<form action="/spaces" method="get" role="search" onsubmit={() => (searchModalOpen = false)}>
				<SearchInput
					bind:value={query}
					placeholder={searchPlaceholder}
					ariaLabel={t('spaces.heading', 'Spaces')}
					fetcher={spaceSearchFetcher}
				/>
				{#if selectedCountry}<input type="hidden" name="country" value={selectedCountry} />{/if}
				{#if selectedState}<input type="hidden" name="state" value={selectedState} />{/if}
				{#if selectedCity}<input type="hidden" name="city" value={selectedCity} />{/if}
				{#if selectedSort}<input type="hidden" name="sort" value={selectedSort} />{/if}
				{#if joinedOnly}<input type="hidden" name="joined" value="1" />{/if}
			</form>
		{/snippet}
	</Modal>

	<Modal bind:open={filtersModalOpen} title={t('common.filters', 'Filters')} icon={Filter} size="lg">
		{#snippet children()}
		<div>
		<!-- Filter panel — one uniform field grid (SelectField/Autocomplete),
		     same pattern as /sources and /blog. Type+level are merged into a
		     single picker; ?subregion= stays URL-only (no dedicated control). -->
		<div id="spaces-filter-panel" class="border-b border-[var(--color-border)] px-4 py-3" style="margin-inline: var(--page-gutter)">
			<!-- Universities-style geo cascade (filter parity with /universities):
			     country → ?geo=, state/city → ?parent= (deepest wins) + sort. -->
			<div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
				<div>
					<Autocomplete
						placeholder={t('universities.all_countries', 'All countries')}
						ariaLabel={t('universities.country_filter', 'Filter by country')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchCountries}
						initialFetcher={fetchCountriesInitial}
						selectedId={selectedCountry || null}
						selectedLabel={selectedCountryLabel}
						onSelect={(item) => { selectedCountry = item.id; selectedState = ''; selectedCity = ''; onFilterChange(); }}
						onClear={() => { selectedCountry = ''; selectedState = ''; selectedCity = ''; onFilterChange(); }}
					/>
				</div>
				<div class="{selectedCountry ? '' : 'pointer-events-none opacity-50'}">
					<Autocomplete
						placeholder={selectedCountry
							? t('universities.all_states', 'All states')
							: t('universities.select_country_first', 'Select a country first')}
						ariaLabel={t('universities.state_filter', 'Filter by state')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchStates}
						initialFetcher={fetchStatesInitial}
						selectedId={selectedState || null}
						selectedLabel={selectedStateLabel}
						onSelect={(item) => { selectedState = item.id; selectedCity = ''; onFilterChange(); }}
						onClear={() => { selectedState = ''; selectedCity = ''; onFilterChange(); }}
					/>
				</div>
				<div class="{selectedState || selectedCity ? '' : 'pointer-events-none opacity-50'}">
					<Autocomplete
						placeholder={selectedState || selectedCity
							? t('universities.all_cities', 'All cities')
							: t('universities.select_state_first', 'Select a state first')}
						ariaLabel={t('universities.city_filter', 'Filter by city')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchCities}
						initialFetcher={fetchCitiesInitial}
						selectedId={selectedCity || null}
						selectedLabel={selectedCityLabel}
						onSelect={(item) => { selectedCity = item.id; onFilterChange(); }}
						onClear={() => { selectedCity = ''; onFilterChange(); }}
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
		</div>
		{/snippet}
	</Modal>

	<div id="spaces-results" class="mx-auto w-full max-w-3xl p-4">
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
									followOnly={!!space.is_primary}
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

<!-- "How it works" slide-over — GuideSlideOver; admins edit inline (spaces.guide). -->
<GuideSlideOver
	bind:open={guideOpen}
	title={effectiveGuide.title}
	body={effectiveGuide.body}
	ctas={effectiveGuide.ctas}
	canManage={data.canManage}
	onSave={saveGuide}
/>
