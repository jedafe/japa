<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { Layers, Search, Users, MessageSquare, Plus, ShieldX, ChevronLeft, ChevronRight, ArrowUpDown, X } from 'lucide-svelte';
	import { EmptyState, SpaceJoinButton, SearchInput, DirectoryCard, ListPageBar, FilterToggle, PageAction, type SearchSuggestion, type CardMetaItem } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import SpaceCreationModal from '$lib/components/spaces/SpaceCreationModal.svelte';
	import SpaceCombobox from '$lib/components/spaces/SpaceCombobox.svelte';
	import { decodeBase64Text } from '$lib/tiptap/serialize';
	import type { Space } from '$lib/types';
	import type { PageData } from './$types';
import { safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	let showCreateModal = $state(false);

	const title = $derived(t('spaces.title', 'Spaces — JapaConnect'));
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
	let availableSubregions = $state<string[]>(untrack(() => data.subregions ?? []));
	// "My spaces" is a server-side filter (?joined=1) — the API returns only
	// spaces the viewer is an approved member of, with correct pagination
	// (audit fix: the chip used to filter just the current page client-side).
	let joinedOnly = $state(untrack(() => data.joined ?? false));
	const searchPlaceholder = $derived(t('spaces.search_placeholder', 'Search spaces…'));

	// Filter panel (ListPageBar + FilterToggle pattern — same as /blog, /guides).
	const hasActiveFilters = $derived(
		!!selectedType || !!selectedLevel || !!selectedSort || !!selectedGeo.trim() ||
		!!selectedRegion || !!selectedSubregion || !!selectedParent || joinedOnly
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
	const levelOptions = $derived([
		{ value: '', label: t('spaces.level_all', 'All levels') },
		...(data.levels ?? ['country', 'state', 'city', 'custom']).map((level) => ({
			value: level,
			label: t(`spaces.level_${level}`, humanizeLevel(level))
		}))
	]);
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
		goto(qs ? `/spaces?${qs}` : '/spaces');
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

	async function fetchSubregions(region: string) {
		if (!region) {
			availableSubregions = [];
			return;
		}
		const res = await api.get<string[]>(`/spaces/subregions?region=${encodeURIComponent(region)}`);
		availableSubregions = res.data ?? [];
		if (!availableSubregions.includes(selectedSubregion)) {
			selectedSubregion = '';
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

<div class="mx-auto w-full max-w-full">
	<!-- Header + search (H1/description; Filter + Create controls sit on the heading row) -->
	<div class="border-b border-[var(--color-border)] px-4 py-4">
		<div class="flex items-start justify-between gap-3">
			<div class="min-w-0">
				<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
					<Layers size={20} /> {t('spaces.heading', 'Spaces')}
				</h1>
				<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
					{t('spaces.subtitle', 'Join spaces to connect with people who share your interests.')}
				</p>
			</div>
			<!-- Page controls on the heading row: Filters toggle + Create.
			     PageAction keeps its mobile FAB (portaled to <body>), so this
			     wrapper only carries the desktop layout. -->
			<div class="flex shrink-0 items-center gap-2 pt-0.5">
				<FilterToggle bind:show={showFilters} active={hasActiveFilters} controls="spaces-filter-panel" />
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
		</form>
	</div>

	<!-- Results count bar — the Filter/Create controls live on the heading row above. -->
	<ListPageBar>
		<span class="flex items-center px-4 text-sm text-[var(--color-muted)]" style="min-height: var(--nav-height)">
			{visibleTotal} {t('spaces.results', 'spaces')}
		</span>
	</ListPageBar>

	{#if showFilters}
		<div id="spaces-filter-panel" class="border-b border-[var(--color-border)] px-4 py-3">
		<!-- Space type filter chips (data-driven from GET /space-types) -->
		{#if data.spaceTypes.length > 0}
			<div class="mt-3 flex flex-wrap gap-2">
				<button
					type="button"
					onclick={() => { selectedType = ''; onFilterChange(); }}
					class="rounded-full border px-3 py-1 text-xs transition-colors {selectedType === ''
						? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
						: 'border-[var(--color-border)] text-[var(--color-muted)] hover:border-[var(--color-primary)]/40'}"
				>
					All types
				</button>
				{#each data.spaceTypes as st (st.id)}
					<button
						type="button"
						onclick={() => { selectedType = st.slug; onFilterChange(); }}
						class="rounded-full border px-3 py-1 text-xs transition-colors {selectedType === st.slug
							? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
							: 'border-[var(--color-border)] text-[var(--color-muted)] hover:border-[var(--color-primary)]/40'}"
					>
						{st.name}
					</button>
				{/each}
			</div>
		{/if}

		<!-- Level filter chips (country/state/city/custom) -->
		<div class="mt-2 flex flex-wrap gap-2">
			{#each levelOptions as opt (opt.value)}
				<button
					type="button"
					onclick={() => { selectedLevel = opt.value; onFilterChange(); }}
					class="rounded-full border px-3 py-1 text-xs transition-colors {selectedLevel === opt.value
						? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
						: 'border-[var(--color-border)] text-[var(--color-muted)] hover:border-[var(--color-primary)]/40'}"
				>
					{opt.label}
				</button>
			{/each}
			{#if page.data.user}
				<button
					type="button"
					onclick={() => { joinedOnly = !joinedOnly; onFilterChange(); }}
					class="rounded-full border px-3 py-1 text-xs transition-colors {joinedOnly
						? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
						: 'border-[var(--color-border)] text-[var(--color-muted)] hover:border-[var(--color-primary)]/40'}"
				>
					{t('spaces.joined_only', 'My spaces')}
				</button>
			{/if}
		</div>

		<!-- Sort selector -->
		<div class="mt-3 flex flex-wrap items-center gap-3">
			<div class="flex items-center gap-2">
				<ArrowUpDown size={16} class="text-[var(--color-muted)]" />
				<select
					bind:value={selectedSort}
					onchange={() => onFilterChange()}
					class="appearance-none rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
					aria-label={t('spaces.sort_label', 'Sort')}
				>
					{#each sortOptions as opt (opt.value)}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>

			<!-- Geo code filter -->
			<div class="flex items-center gap-2">
				<span class="text-sm text-[var(--color-muted)]">{t('spaces.geo_label', 'Geo:')}</span>
				<input
					type="text"
					bind:value={selectedGeo}
					placeholder={t('spaces.geo_placeholder', 'e.g. NG, US')}
					class="w-28 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
					onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); onFilterChange(); } }}
				/>
				{#if selectedGeo}
					<button type="button" onclick={() => { selectedGeo = ''; onFilterChange(); }} class="text-[var(--color-muted)] hover:text-[var(--color-text)]">
						<X size={14} />
					</button>
				{/if}
			</div>

			<!-- Region / subregion filters -->
			<div class="flex items-center gap-2">
				<select
					bind:value={selectedRegion}
					onchange={() => { selectedSubregion = ''; fetchSubregions(selectedRegion); onFilterChange(); }}
					class="appearance-none rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
				>
					<option value="">{t('spaces.region_all', 'All regions')}</option>
					{#each data.regions as r (r)}
						<option value={r}>{r}</option>
					{/each}
				</select>
				{#if selectedRegion}
					<select
						bind:value={selectedSubregion}
						onchange={() => onFilterChange()}
						class="appearance-none rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
					>
						<option value="">{t('spaces.subregion_all', 'All subregions')}</option>
						{#each availableSubregions as sr (sr)}
							<option value={sr}>{sr}</option>
						{/each}
					</select>
				{/if}
			</div>

			<!-- Parent filter -->
			<div class="min-w-[14rem] max-w-xs flex-1">
				<SpaceCombobox
					parentMode
					placeholder={t('spaces.parent_placeholder', 'Filter by parent space…')}
					selected={selectedParent}
					onSelect={(space) => { selectedParent = space; onFilterChange(); }}
					onRemove={() => { selectedParent = null; onFilterChange(); }}
				/>
			</div>
		</div>
		</div>
	{/if}

	<div class="p-4">
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
			<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
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
						imageUrl={space.logo_url}
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
						<ChevronLeft size={16} /> {t('common.prev', 'Prev')}
					</a>
					<span class="text-xs text-[var(--color-muted)]">
						{data.page + 1} / {data.totalPages}
					</span>
					<a
						href={buildHref(data.page + 1)}
						class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] {data.page >= data.totalPages - 1 ? 'pointer-events-none opacity-40' : ''}"
					>
						{t('common.next', 'Next')} <ChevronRight size={16} />
					</a>
				</div>
			{/if}
		{/if}
	</div>
</div>

<SpaceCreationModal bind:open={showCreateModal} />
