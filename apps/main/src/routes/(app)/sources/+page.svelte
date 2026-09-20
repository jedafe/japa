<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Rss, Plus } from 'lucide-svelte';
	import {
		FilterToggle,
		ListPageBar,
		SearchInput,
		SelectField,
		UrlTabs,
		type SearchSuggestion,
		type UrlTab
	} from '$lib/components/ui';
	import SourceCard from '$lib/components/source/SourceCard.svelte';
	import ProposeSourceModal from '$lib/components/source/ProposeSourceModal.svelte';
	import { api } from '$lib/api/client';
	import type { PageData } from './$types';
	import type { Source } from '$lib/types/sources';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('sources.title', 'Sources — JapaConnect'));
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

	const sortTabs = $derived<UrlTab[]>(
		(data.sourceMeta?.sorts ?? []).map((s) => ({ value: s.value, label: s.label }))
	);

	const categoryOptions = $derived(data.sourceMeta?.categories ?? []);
	// ?source_kind= filters the publisher kind (publisher/institution/
	// individual/aggregator) — NOT the crawl mechanism values in
	// source_types (rss/video/api/static), which belong to the proposal
	// form. Using source_types here always produced an empty directory.
	const sourceKindOptions = $derived(data.sourceMeta?.source_kinds ?? []);

	const categorySelectOptions = $derived([
		{ value: '', label: t('sources.category.all', 'All categories') },
		...categoryOptions.map((c) => ({ value: c, label: c }))
	]);

	const sourceKindSelectOptions = $derived([
		{ value: '', label: t('sources.source_kind.all', 'All source types') },
		...sourceKindOptions
	]);

	// Only category / source type are inside the filter panel now.
	const hasActiveFilters = $derived(!!selectedCategory || !!selectedSourceKind);
	let showFilters = $state(false);
	$effect(() => {
		if (hasActiveFilters) showFilters = true;
	});

	const searchPlaceholder = $derived(t('sources.search_placeholder', 'Search sources…'));

	function onFilterChange() {
		const params = new URLSearchParams();
		if (query.trim()) params.set('q', query.trim());
		if (selectedSort) params.set('sort', selectedSort);
		if (selectedCategory) params.set('category', selectedCategory);
		if (selectedSourceKind) params.set('source_kind', selectedSourceKind);
		const qs = params.toString();
		goto(qs ? `/sources?${qs}` : '/sources', { keepFocus: true });
	}

	function pageUrl(pageNumber: number) {
		const url = new URL(page.url);
		url.searchParams.set('sort', data.sort);
		if (data.q) url.searchParams.set('q', data.q);
		else url.searchParams.delete('q');
		if (data.category) url.searchParams.set('category', data.category);
		else url.searchParams.delete('category');
		if (data.sourceKind) url.searchParams.set('source_kind', data.sourceKind);
		else url.searchParams.delete('source_kind');
		if (pageNumber > 1) url.searchParams.set('page', String(pageNumber));
		else url.searchParams.delete('page');
		return url;
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

</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<!-- Header -->
	<div class="border-b border-[var(--color-border)] px-4 py-4">
		<div class="flex items-start justify-between gap-4">
			<div>
				<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
					<Rss size={20} /> {t('sources.heading', 'Sources')}
				</h1>
				<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
					{description}
				</p>
			</div>
			<div class="flex shrink-0 items-center gap-2">
				<button
					type="button"
					onclick={() => (proposeOpen = true)}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
				>
					<Plus size={15} />
					{t('sources.propose', 'Propose a source')}
				</button>
				<FilterToggle
					bind:show={showFilters}
					active={hasActiveFilters}
					controls="sources-filter-panel"
				/>
			</div>
		</div>
	</div>

	{#if showFilters}
		<div id="sources-filter-panel" class="border-b border-[var(--color-border)] px-4 py-3">
			<div class="grid gap-3 sm:grid-cols-3">
				<form
					class="w-full"
					role="search"
					onsubmit={(e) => {
						e.preventDefault();
						onFilterChange();
					}}
				>
					<SearchInput
						bind:value={query}
						placeholder={searchPlaceholder}
						ariaLabel={t('sources.heading', 'Sources')}
						fetcher={sourceSearchFetcher}
					/>
				</form>

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
		</div>
	{/if}

	<!-- Sort tabs -->
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
	</ListPageBar>

	<div class="p-4">
		<!-- Curated collections (default directory view only) -->
		{#if (data.collections ?? []).length > 0}
			<div class="mb-6 space-y-5">
				{#each data.collections as collection (collection.slug)}
					{#if collection.sources?.length > 0}
						<section>
							<div class="mb-2 flex items-baseline justify-between gap-2">
								<h2 class="text-sm font-semibold text-[var(--color-text)]">{collection.name}</h2>
								{#if collection.description}
									<p class="text-xs text-[var(--color-text-subtle)]">{collection.description}</p>
								{/if}
							</div>
							<div class="flex gap-3 overflow-x-auto pb-2">
								{#each collection.sources as source (source.slug)}
									<div class="w-72 shrink-0">
										<SourceCard {source} />
									</div>
								{/each}
							</div>
						</section>
					{/if}
				{/each}
			</div>
		{/if}

		{#if data.sources.length === 0}
			<p class="py-8 text-center text-sm text-[var(--color-text-subtle)]">
				{t('sources.empty', 'No sources found.')}
			</p>
		{:else}
			<p class="mb-3 text-xs text-[var(--color-muted)]">
				{data.meta?.total ?? 0} {t('sources.results', 'sources')}
			</p>
			<div class="grid gap-3 grid-cols-1 sm:grid-cols-2">
				{#each data.sources as source (source.slug)}
					<SourceCard {source} />
				{/each}
			</div>
		{/if}

		{#if data.meta && data.meta.total}
			{@const total = data.meta.total}
			{@const pageSize = data.pageSize}
			{@const lastPage = Math.max(1, Math.ceil(total / pageSize))}
			<div class="flex items-center justify-between pt-4 text-sm">
				{#if data.page > 1}
					<a
						href={pageUrl(data.page - 1).toString()}
						class="rounded-md border border-[var(--color-border)] px-3 py-2"
					>
						{t('pagination.previous', 'Previous')}
					</a>
				{:else}
					<span></span>
				{/if}
				<span class="text-[var(--color-text-subtle)]">
					{t('pagination.page', 'Page {page} of {total}')
						.replace('{page}', String(data.page))
						.replace('{total}', String(lastPage))}
				</span>
				{#if data.page < lastPage}
					<a
						href={pageUrl(data.page + 1).toString()}
						class="rounded-md border border-[var(--color-border)] px-3 py-2"
					>
						{t('pagination.next', 'Next')}
					</a>
				{:else}
					<span></span>
				{/if}
			</div>
		{/if}
	</div>
</div>

<ProposeSourceModal
	bind:open={proposeOpen}
	meta={data.sourceMeta ?? { categories: [], source_types: [] }}
/>
