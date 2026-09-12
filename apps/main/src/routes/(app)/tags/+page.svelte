<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { Hash, ShieldX, ChevronLeft, ChevronRight } from 'lucide-svelte';
	import { EmptyState, FollowButton, SearchInput, DirectoryCard, FilterToggle, type SearchSuggestion } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type { PageData } from './$types';
import { safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('tags.title', 'Tags — JapaConnect'));
	const description = $derived(
		t('tags.description', 'Browse all tags on JapaConnect and find content that matters to you.')
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'CollectionPage',
			name: 'Tags',
			description,
			url: page.url.href
		})
	);

	let query = $state(untrack(() => data.q));
	let selectedSort = $state(untrack(() => data.sort));
	const searchPlaceholder = $derived(t('tags.search_placeholder', 'Search tags…'));

	// Tags the viewer already follows — drives each card's FollowButton
	// initial state so followed tags render "Following" (§08).
	const followedSlugs = $derived(new Set(data.followedTagSlugs ?? []));

	// Re-sync the filter controls when load() data changes — e.g. browser
	// back/forward navigation, where the component is reused and $state
	// initialisers do not re-run.
	$effect(() => {
		query = data.q;
		selectedSort = data.sort;
	});

	const sortOptions = $derived([
		{ value: '', label: t('tags.sort_newest', 'Newest') },
		{ value: 'popular', label: t('tags.sort_popular', 'Popular') },
		{ value: 'followers', label: t('tags.sort_followers', 'Most Followed') },
		{ value: 'content', label: t('tags.sort_content', 'Most Content') }
	]);

	// Filter toggle panel (same as /blog, /guides).
	const hasActiveFilters = $derived(!!selectedSort);
	let showFilters = $state(false);
	$effect(() => {
		if (hasActiveFilters) showFilters = true;
	});

	// pageNum is the 0-based target index; the API's ?page= is 1-based
	// (api-routes-catalog: page=1 is the first page), so URLs emit pageNum+1.
	function buildHref(pageNum: number): string {
		const params = new URLSearchParams();
		if (data.q) params.set('q', data.q);
		if (data.sort) params.set('sort', data.sort);
		if (pageNum > 0) params.set('page', String(pageNum + 1));
		const qs = params.toString();
		return qs ? `/tags?${qs}` : '/tags';
	}

	function onFilterChange() {
		const params = new URLSearchParams();
		if (query.trim()) params.set('q', query.trim());
		if (selectedSort) params.set('sort', selectedSort);
		const qs = params.toString();
		goto(qs ? `/tags?${qs}` : '/tags');
	}

	// Tag suggestions come from the tags directory endpoint (GET /tags?q=),
	// not /search?type=tag — the search index's "tag" type holds forum tags
	// (url: /forum/tags/{slug}), which are a different entity from the
	// directory tags listed on this page.
	async function tagSearchFetcher(q: string): Promise<SearchSuggestion[]> {
		const res = await api.get<{ id: string; name: string; slug: string; follower_count?: number }[]>(
			`/tags?q=${encodeURIComponent(q)}&page_size=8`
		);
		if (res.error || !res.data) return [];
		return res.data.map((t2) => ({
			id: t2.id,
			title: `#${t2.name}`,
			url: `/tags/${t2.slug}`,
			excerpt:
				t2.follower_count != null
					? `${t2.follower_count} ${t('tags.followers', 'followers')}`
					: undefined,
			type: 'tag'
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
	<!-- Header + search + filters -->
	<div class="border-b border-[var(--color-border)] px-4 py-4">
		<div class="flex items-start justify-between gap-4">
			<div>
				<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
					<Hash size={20} /> {t('tags.heading', 'Tags')}
				</h1>
				<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
					{t('tags.subtitle', 'Follow tags to shape your feed.')}
				</p>
			</div>
			<FilterToggle bind:show={showFilters} active={hasActiveFilters} controls="tags-filter-panel" />
		</div>

		<form class="mt-3" action="/tags" method="get" role="search">
			<SearchInput
				bind:value={query}
				placeholder={searchPlaceholder}
				ariaLabel={t('tags.heading', 'Tags')}
				fetcher={tagSearchFetcher}
			/>
			{#if selectedSort}
				<input type="hidden" name="sort" value={selectedSort} />
			{/if}
		</form>
	</div>

	{#if showFilters}
		<!-- Sort filter (moved from always-visible chips to the toggle panel) -->
		<div id="tags-filter-panel" class="flex flex-wrap gap-2 border-b border-[var(--color-border)] px-4 py-3">
			{#each sortOptions as so (so.value)}
				<button
					type="button"
					onclick={() => { selectedSort = so.value; onFilterChange(); }}
					class="rounded-full border px-3 py-1 text-xs transition-colors {selectedSort === so.value
						? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
						: 'border-[var(--color-border)] text-[var(--color-muted)] hover:border-[var(--color-primary)]/40'}"
				>
					{so.label}
				</button>
			{/each}
		</div>
	{/if}

	<div class="p-4">
		{#if data.forbidden}
			<div class="flex flex-col items-center gap-3 py-16 text-center">
				<ShieldX size={40} class="text-[var(--color-muted)]" />
				<p class="text-sm font-medium text-[var(--color-text)]">
					{t('tags.forbidden', 'You do not have permission to view this directory.')}
				</p>
			</div>
		{:else if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('tags.load_error', 'Failed to load tags. Please try again later.')}
			</p>
		{:else if data.tags.length === 0}
			<EmptyState icon={Hash} title={t('tags.empty', 'No tags yet.')} />
		{:else}
			<p class="mb-3 text-xs text-[var(--color-muted)]">
				{data.total} {t('tags.results', 'tags')}
			</p>
			<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{#each data.tags as tag (tag.id)}
					<DirectoryCard
						href="/tags/{tag.slug}"
						title="#{tag.name}"
						subtitle={t('tag.followers', '{n} followers').replace('{n}', String(tag.follower_count))}
						imageUrl={tag.logo_url}
						icon={Hash}
						hueSeed={tag.id}
						round
					>
						{#snippet actions()}
							{#if page.data.user}
								<FollowButton
									entityType="tag"
									entityId={tag.id}
									entitySlug={tag.slug}
									initiallyFollowing={followedSlugs.has(tag.slug)}
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
						<ChevronLeft size={16} /> {t('common.previous', 'Prev')}
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
