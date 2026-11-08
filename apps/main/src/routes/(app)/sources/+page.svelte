<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '#lib/i18n/index.js';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Rss, Plus, Search, ChevronRight, ChevronDown, Tag, Filter } from 'lucide-svelte';
	import {
		EntityHero,
		GuideSlideOver,
		ListPageBar,
		Modal,
		SearchInput,
		SelectField,
		UrlTabs,
		type GuideContent,
		type SearchSuggestion,
		type UrlTab
	} from '#lib/components/ui/index.js';
	import HeroArtSources from '#lib/components/heroes/HeroArtSources.svelte';
	import { formatMetric } from '#lib/components/hub/charts/format.js';
	import SourceCard from '#lib/components/source/SourceCard.svelte';
	import ProposeSourceModal from '#lib/components/source/ProposeSourceModal.svelte';
	import { sourceCategoryOptions, sourceCategoryLabel } from '#lib/sources.js';
	import { requireAuthOrRedirect } from '#lib/auth/guest.js';
	import { api } from '#lib/api/client.js';
	import type { PageData } from './$types';
	import type { Source } from '#lib/types/sources.js';

	let { data }: { data: PageData } = $props();

	// Load more — client-side appends (replaces Previous/Next pagination);
	// resets when the SSR list changes (filter/sort/search navigation).
	let allSources = $state<Source[]>(untrack(() => data.sources));
	let loadingMore = $state(false);
	let moreError = $state(false);
	$effect(() => {
		const fresh = data.sources;
		untrack(() => {
			allSources = fresh;
			moreError = false;
		});
	});

	const total = $derived(data.meta?.total ?? 0);
	const pageSize = $derived(data.pageSize);

	const title = $derived(t('sources.title', 'Sources — {site}'));
	const description = $derived(
		t(
			'sources.description',
			'Browse trusted sources for scholarships, jobs, remittance, universities, and more.'
		)
	);

	// Header state — mirrors /tags header.
	let proposeOpen = $state(false);

	let query = $state(untrack(() => data.q));
	let selectedSort = $state(untrack(() => data.sort));
	let selectedCategory = $state(untrack(() => data.category));
	let selectedSourceKind = $state(untrack(() => data.sourceKind));

	$effect(() => {
		query = data.q;
		selectedSort = data.sort;
		selectedCategory = data.category;
		selectedSourceKind = data.sourceKind;
	});

	// Sort labels are localized via sources.sort.* keys; the backend label is
	// the fallback for sorts added without an i18n entry yet.
	const sortTabs = $derived<UrlTab[]>(
		(data.sourceMeta?.sorts ?? []).map((s) => ({
			value: s.value,
			label: t(`sources.sort.${s.value}`, s.label)
		}))
	);

	// Same canonical category set as the propose form (curated catalog ∪
	// full extraction taxonomy) — see $lib/sources.
	const categoryOptions = $derived(sourceCategoryOptions(data.sourceMeta));
	// ?source_kind= filters the publisher kind (publisher/institution/
	// individual/aggregator) — NOT the crawl mechanism values in
	// source_types (rss/video/api/static), which belong to the proposal
	// form. Using source_types here always produced an empty directory.
	const sourceKindOptions = $derived(data.sourceMeta?.source_kinds ?? []);

	const categorySelectOptions = $derived([
		{ value: '', label: t('sources.category.all', 'All categories') },
		...categoryOptions.map((c) => ({
			value: c,
			label: sourceCategoryLabel(c)
		}))
	]);

	const sourceKindSelectOptions = $derived([
		{ value: '', label: t('sources.source_kind.all', 'All source types') },
		...sourceKindOptions.map((k) => ({
			value: k.value,
			label: t(`sources.kind.${k.value}`, k.label)
		}))
	]);

	// Active-filter dot for the modal-held controls (category / source kind).
	const hasActiveFilters = $derived(!!selectedCategory || !!selectedSourceKind);

	// Right-side bar controls — each opens a modal rather than taking bar space.
	let filtersModalOpen = $state(false);
	let searchModalOpen = $state(false);

	// "How it works" slide-over — title/intro/CTAs live in a public
	// system-settings JSON blob (sources.guide) so staff can edit them
	// inline (PUT /admin/settings/general → {sources_guide}).
	let guideOpen = $state(false);
	let guideOverride = $state<GuideContent | null>(null);

	const siteSettings = $derived(
		(page.data.siteSettings ?? {}) as Record<string, unknown>
	);
	const storedGuide = $derived.by((): Partial<GuideContent> => {
		const raw = siteSettings['sources.guide'];
		if (raw && typeof raw === 'object' && !Array.isArray(raw)) return raw as Partial<GuideContent>;
		return {};
	});
	const effectiveGuide = $derived<GuideContent>({
		title: guideOverride?.title ?? storedGuide.title ?? t('common.how_it_works', 'How it works'),
		body: guideOverride?.body ?? storedGuide.body ?? t('sources.guide_body', ''),
		ctas: guideOverride?.ctas ?? storedGuide.ctas ?? [{ title: t('sources.guide_cta_title', 'Read the full guide'), url: '/guides' }]
	});

	async function saveGuide(v: GuideContent): Promise<string | null> {
		const res = await api.put('/admin/settings/general', { sources_guide: v });
		if (res.error) return res.error.message || t('common.error', 'Something went wrong');
		guideOverride = v;
		return null;
	}

	const searchPlaceholder = $derived(t('sources.search_placeholder', 'Search sources…'));

	function onFilterChange() {
		const params = new URLSearchParams();
		if (query.trim()) params.set('q', query.trim());
		if (selectedSort) params.set('sort', selectedSort);
		if (selectedCategory) params.set('category', selectedCategory);
		if (selectedSourceKind) params.set('source_kind', selectedSourceKind);
		const qs = params.toString();
		goto(qs ? `/sources?${qs}` : '/sources', { reset: false });
	}

	async function sourceSearchFetcher(q: string): Promise<SearchSuggestion[]> {
		const res = await api.get<Source[]>(`/sources?q=${encodeURIComponent(q)}&page_size=8`);
		if (res.error || !res.data) return [];
		return res.data.map((s) => ({
			id: s.id,
			title: s.name,
			url: `/sources/${s.slug}`,
			excerpt: s.domain,
			type: 'source'
		}));
	}

	async function loadMore() {
		if (loadingMore || allSources.length >= total) return;
		loadingMore = true;
		moreError = false;
		try {
			const params = new URLSearchParams();
			if (data.q) params.set('q', data.q);
			if (data.category) params.set('category', data.category);
			if (data.sourceKind) params.set('source_kind', data.sourceKind);
			params.set('sort', data.sort);
			params.set('page_size', String(pageSize));
			params.set('page_offset', String(allSources.length));
			const res = await api.get<{ items?: Source[]; data?: Source[] } | Source[]>(
				`/sources?${params.toString()}`
			);
			const payload = (res as { data?: unknown }).data ?? res;
			const next: Source[] = Array.isArray(payload)
				? payload
				: ((payload as { items?: Source[] }).items ?? []);
			const seen = new Set(allSources.map((s) => s.slug));
			allSources = [...allSources, ...next.filter((s) => !seen.has(s.slug))];
		} catch {
			moreError = true;
		} finally {
			loadingMore = false;
		}
	}

</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<!-- Section 26 — self-referencing canonical on the clean path (never the
	     filtered query-string form) so filter/sort hits consolidate. -->
	<link rel="canonical" href="{page.url.origin}/sources" />
	<meta property="og:url" content="{page.url.origin}/sources" />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
</svelte:head>

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	<!-- Header — EntityHero directory panel -->
	<EntityHero
		class="mt-4"
		icon={Rss}
		accent="var(--color-primary)"
		heading={t('sources.heading', 'Sources')}
		description={t('sources.hero_desc', 'Browse trusted sources for scholarships, jobs, remittance, universities, and more.')}
		crumbs={[
			{ label: t('common.home', 'Home'), href: '/' },
			{ label: t('sources.heading', 'Sources') }
		]}
		stats={[
			{ icon: Rss, value: formatMetric(data.meta?.total ?? 0), label: t('sources.stats_sources', 'Sources') },
			...(data.sourceMeta?.categories?.length
				? [{ icon: Tag, value: formatMetric(data.sourceMeta.categories.length), label: t('sources.stats_categories', 'Categories') }]
				: [])
		]}
	>
		{#snippet art()}
			<HeroArtSources accent="var(--color-primary)" />
		{/snippet}
		{#snippet actions(c)}
			<!-- Full-width CTA row — the three actions share the hero's width
			     evenly and stay on one line at desktop. -->
			<div class="grid w-full grid-cols-2 gap-3 sm:flex sm:items-center">
				<a href="#sources-results" class="{c.primary} flex-1 justify-center whitespace-nowrap">
					<Search size={15} /> {t('sources.browse', 'Browse sources')}
				</a>
				<button
					type="button"
					onclick={async () => {
						if (!(await requireAuthOrRedirect())) return;
						proposeOpen = true;
					}}
					class="{c.secondary} flex-1 justify-center whitespace-nowrap"
				>
					<Plus size={15} /> {t('sources.propose', 'Propose a source')}
				</button>
				<button type="button" onclick={() => (guideOpen = true)} class="{c.ghost} col-span-2 flex-1 justify-center whitespace-nowrap sm:col-span-1">
					{t('common.how_it_works', 'How it works')}
					<ChevronRight size={14} class="rtl:-scale-x-100" />
				</button>
			</div>
		{/snippet}
	</EntityHero>

	<!-- Sort tabs on the left; Filters + Search buttons on the right —
	     one ListPageBar row (the /org, /users, /jobs pattern). -->
	<ListPageBar>
		{#if sortTabs.length > 0}
			<UrlTabs
				tabs={sortTabs}
				value={selectedSort}
				onValueChange={(v) => {
					selectedSort = v;
					onFilterChange();
				}}
				class="!border-b-0 !bg-transparent"
			/>
		{/if}
		{#snippet actions()}
			{#if categoryOptions.length > 0 || sourceKindOptions.length > 0}
				<button
					type="button"
					onclick={() => (filtersModalOpen = true)}
					class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					{t('common.filters', 'Filters')}
					{#if hasActiveFilters}
						<span class="ms-0.5 h-2 w-2 rounded-full bg-[var(--color-primary)]" aria-hidden="true"></span>
					{/if}
					<Filter size={16} />
				</button>
			{/if}
			<button
				type="button"
				onclick={() => (searchModalOpen = true)}
				class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				{t('common.search', 'Search')}
				{#if query.trim()}
					<span class="ms-0.5 h-2 w-2 rounded-full bg-[var(--color-primary)]" aria-hidden="true"></span>
				{/if}
				<Search size={16} />
			</button>
		{/snippet}
	</ListPageBar>

	<!-- Filters modal — category + source kind dropdowns. -->
	{#if categoryOptions.length > 0 || sourceKindOptions.length > 0}
		<Modal
			bind:open={filtersModalOpen}
			title={t('common.filters', 'Filters')}
			icon={Filter}
			size="lg"
		>
			{#snippet children()}
				<div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
					{#if categoryOptions.length > 0}
						<div>
							<label for="sources-category" class="sr-only">
								{t('sources.category_label', 'Category')}
							</label>
							<SelectField
								id="sources-category"
								value={selectedCategory}
								options={categorySelectOptions}
								placeholder={t('sources.category.all', 'All categories')}
								onValueChange={(v) => {
									selectedCategory = v;
									onFilterChange();
								}}
								class="w-full"
							/>
						</div>
					{/if}

					{#if sourceKindOptions.length > 0}
						<div>
							<label for="sources-source-kind" class="sr-only">
								{t('sources.source_type_label', 'Source type')}
							</label>
							<SelectField
								id="sources-source-kind"
								value={selectedSourceKind}
								options={sourceKindSelectOptions}
								placeholder={t('sources.source_kind.all', 'All source types')}
								onValueChange={(v) => {
									selectedSourceKind = v;
									onFilterChange();
								}}
								class="w-full"
							/>
						</div>
					{/if}
				</div>
				{#if hasActiveFilters}
					<button
						type="button"
						onclick={() => { selectedCategory = ''; selectedSourceKind = ''; onFilterChange(); }}
						class="mt-3 text-xs text-[var(--color-muted)] underline hover:text-[var(--color-text)]"
					>
						{t('common.clear_filters', 'Clear filters')}
					</button>
				{/if}
			{/snippet}
		</Modal>
	{/if}

	<!-- Search modal — GET form preserving the active filters. -->
	<Modal bind:open={searchModalOpen} title={t('common.search', 'Search')} icon={Search} size="sm">
		{#snippet children()}
			<form action="/sources" method="get" role="search" onsubmit={() => (searchModalOpen = false)}>
				<SearchInput
					bind:value={query}
					placeholder={searchPlaceholder}
					ariaLabel={t('sources.heading', 'Sources')}
					fetcher={sourceSearchFetcher}
				/>
				{#if selectedSort}
					<input type="hidden" name="sort" value={selectedSort} />
				{/if}
				{#if selectedCategory}
					<input type="hidden" name="category" value={selectedCategory} />
				{/if}
				{#if selectedSourceKind}
					<input type="hidden" name="source_kind" value={selectedSourceKind} />
				{/if}
			</form>
		{/snippet}
	</Modal>

	<div id="sources-results" class="mx-auto w-full max-w-3xl p-4">
		<!-- NOTE: the curated-collections sections (GET /sources/collections)
			were intentionally removed from this page — the directory is just the
			sorted grid below. Do not re-add collection rails here; the API
			endpoint still exists for other consumers. -->
		{#if data.sources.length === 0}
			<p class="py-8 text-center text-sm text-[var(--color-text-subtle)]">
				{t('sources.empty', 'No sources found.')}
			</p>
		{:else}
			<div class="grid gap-3 grid-cols-1 sm:grid-cols-2">
				{#each allSources as source (source.slug)}
					<SourceCard {source} />
				{/each}
			</div>
		{/if}

		{#if total > 0}
			<div class="mt-4 flex justify-center">
				<button
					type="button"
					onclick={loadMore}
					disabled={loadingMore || allSources.length >= total}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
				>
					{loadingMore ? t('common.loading', 'Loading…') : t('common.load_more', 'Load more')}
					<ChevronDown size={14} />
				</button>
				{#if moreError}
					<span class="ml-3 text-sm text-[var(--color-text-subtle)]">
						{t('common.load_error', 'Failed to load more. Please try again.')}
					</span>
				{/if}
			</div>
		{/if}
	</div>
</div>

<!-- "How it works" slide-over — reusable GuideSlideOver; staff with
     admin.settings edit the CTA title/link inline (sources.guide_*). -->
<GuideSlideOver
	bind:open={guideOpen}
	title={effectiveGuide.title}
	body={effectiveGuide.body}
	ctas={effectiveGuide.ctas}
	canManage={data.canManage}
	onSave={saveGuide}
/>

<ProposeSourceModal
	bind:open={proposeOpen}
	meta={data.sourceMeta ?? { categories: [], source_types: [] }}
/>
