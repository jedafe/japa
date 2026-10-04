<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { ArrowRight, BookOpen, ChevronDown, PenSquare, X } from 'lucide-svelte';
	import { EmptyState, FilterToggle, ListPageBar, PageAction, UrlTabs, type UrlTab } from '$lib/components/ui';
	import Autocomplete, { type AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { staticAutocompleteFetcher } from '$lib/autocomplete';
	import type { FeedItem } from '$lib/types';
	import type { GuideListItem } from '$lib/types/content';
	import ContentGridCard from '$lib/components/feed/ContentGridCard.svelte';
	import { api } from '$lib/api/client';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	function toFeedItem(g: GuideListItem): FeedItem {
		return {
			id: g.id,
			title: g.title,
			slug: g.slug,
			content_type: 'guide',
			author_id: g.author_id ?? '',
			author_name: g.author_display_name ?? g.author_username ?? '',
			author_username: g.author_username,
			author_avatar_url: g.author_avatar_url,
			space_id: g.space_id,
			space_name: g.space_name,
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
			guide_category: g.guide_category,
			journey_stage: g.journey_stage,
			visa_route: g.visa_route,
			source_country: g.source_country,
			target_country: g.target_country,
			tags: g.tags
		};
	}

	// Initialize from SSR data — $effect never runs during SSR, so seeding
	// inside it would render the empty state on the initial page load.
	// svelte-ignore state_referenced_locally
	let items = $state<GuideListItem[]>(data.items);
	// svelte-ignore state_referenced_locally
	let nextCursor = $state<string | null>(data.nextCursor);
	let loadingMore = $state(false);
	let moreError = $state(false);

	// Re-seed client state whenever the server re-runs load (e.g. ?category= nav).
	$effect(() => {
		items = data.items;
		nextCursor = data.nextCursor;
		moreError = false;
	});

	const feedItems = $derived(items.map(toFeedItem));

	async function loadMore() {
		if (!nextCursor || loadingMore) return;
		loadingMore = true;
		moreError = false;
		try {
			const params = new URLSearchParams({ type: 'guide', page_size: String(data.pageSize) });
			if (data.category) params.set('guide_category', data.category);
			// Popular paginates by offset (view_count order has no cursor);
			// latest uses the composite created_at|id cursor.
			if (nextCursor.startsWith('offset:')) {
				params.set('sort', 'popular');
				params.set('page_offset', nextCursor.slice('offset:'.length));
			} else {
				params.set('cursor', nextCursor);
			}
			const res = await api.get<GuideListItem[]>(`/content?${params.toString()}`);
			const rows = res.data ?? [];
			const last = rows[rows.length - 1];
			items = [...items, ...rows];
			if (nextCursor.startsWith('offset:')) {
				nextCursor = rows.length >= data.pageSize
					? `offset:${parseInt(nextCursor.slice('offset:'.length), 10) + rows.length}`
					: null;
			} else {
				nextCursor = rows.length >= data.pageSize && last ? `${last.created_at}|${last.id}` : null;
			}
		} catch {
			moreError = true;
		} finally {
			loadingMore = false;
		}
	}

	const title = $derived(t('guides.title', 'Guides — JapaConnect'));
	const description = $derived(
		t('guides.description', 'Step-by-step relocation, visa, and settlement guides from the diaspora community.')
	);

	const tabs = $derived<UrlTab[]>([
		{ value: 'latest', label: t('guides.tab_latest', 'Latest') },
		{ value: 'popular', label: t('guides.tab_popular', 'Popular') },
		{ value: 'featured', label: t('guides.tab_featured', 'Featured') },
		{ value: 'series', label: t('guides.tab_series', 'Series') },
		{ value: 'roadmap', label: t('guides.tab_roadmap', 'Roadmap') }
	]);

	// The category filter applies to every guide list view — only featured and
	// series (curated views) clear it.
	const categoryFilterApplies = $derived(data.tab !== 'featured' && data.tab !== 'series');

	function switchTab(tab: string) {
		const params = new URLSearchParams();
		if (tab !== 'latest') params.set('tab', tab);
		if (data.category && tab !== 'featured' && tab !== 'series') params.set('category', data.category);
		const qs = params.toString();
		goto(qs ? `/guides?${qs}` : '/guides');
	}

	function applyCategory(slug: string) {
		const params = new URLSearchParams();
		if (data.tab !== 'latest') params.set('tab', data.tab);
		if (slug) params.set('category', slug);
		const qs = params.toString();
		goto(qs ? `/guides?${qs}` : '/guides');
	}

	// Category filter lives in the toggle panel like the other list pages —
	// options come from GET /content-types (admin-configured, §14).
	const categoryOptions = $derived<AutocompleteItem[]>(
		data.categories.map((c) => ({ id: c.Slug, label: c.Label }))
	);
	const categoryFetcher = $derived(staticAutocompleteFetcher(categoryOptions));
	const selectedCategoryLabel = $derived(
		data.categories.find((c) => c.Slug === data.category)?.Label ?? null
	);
	const hasActiveFilters = $derived(!!data.category);

	let showFilters = $state(false);
	$effect(() => {
		if (hasActiveFilters) showFilters = true;
	});

	// Roadmap tab: group guides by journey_stage in the canonical §87 japa
	// stage order; guides without a stage land in a trailing "General" group.
	const JOURNEY_STAGE_ORDER = [
		'considering',
		'planning',
		'preparing',
		'applied',
		'approved',
		'relocated',
		'settled'
	] as const;

	const roadmapGroups = $derived.by(() => {
		const groups: { stage: string; label: string; items: FeedItem[] }[] = [];
		const byStage = new Map<string, GuideListItem[]>();
		for (const g of items) {
			const stage = (g.journey_stage ?? '').trim() || 'general';
			const list = byStage.get(stage) ?? [];
			list.push(g);
			byStage.set(stage, list);
		}
		for (const stage of JOURNEY_STAGE_ORDER) {
			const list = byStage.get(stage);
			if (list?.length) {
				groups.push({
					stage,
					label: t(`guides.journey_stage.${stage}`, humanizeKey(stage)),
					items: list.map(toFeedItem)
				});
				byStage.delete(stage);
			}
		}
		for (const [stage, list] of byStage) {
			groups.push({
				stage,
				label: stage === 'general' ? t('guides.journey_stage.general', 'General') : humanizeKey(stage),
				items: list.map(toFeedItem)
			});
		}
		return groups;
	});

	function humanizeKey(key: string): string {
		return key.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-full" style="--page-gutter: 1rem">
	<h1 class="sr-only">{t('guides.heading', 'Guides')}</h1>
	<ListPageBar>
		<UrlTabs
			{tabs}
			value={data.tab}
			onValueChange={(v) => switchTab(v)}
			class="!border-b-0 !bg-transparent"
		/>
		{#snippet actions()}
			{#if categoryFilterApplies}
				<FilterToggle bind:show={showFilters} active={hasActiveFilters} controls="guides-filter-panel" />
			{/if}
			{#if data.authed}
				<PageAction
					href="/create?type=guide"
					icon={PenSquare}
					label={t('guides.new_guide', 'New guide')}
				/>
			{:else}
				<PageAction
					href="/login?redirect=/create?type=guide"
					icon={PenSquare}
					label={t('guides.new_guide', 'New guide')}
				/>
			{/if}
		{/snippet}
	</ListPageBar>

	<div class="p-4">
		{#if showFilters && categoryFilterApplies}
			<!-- Category filter — admin-configured options from GET /content-types
			     (§14), same FilterToggle/Autocomplete pattern as /blog and /forum. -->
			<div
				id="guides-filter-panel"
				class="mb-4 flex flex-wrap items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
			>
				<div class="w-56">
					<Autocomplete
						id="guides-category"
						placeholder={t('guides.filter_category_all', 'All categories')}
						ariaLabel={t('guides.filter_category', 'Category')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={categoryFetcher}
						initialFetcher={categoryFetcher}
						selectedId={data.category || null}
						selectedLabel={selectedCategoryLabel}
						onSelect={(item) => applyCategory(item.id)}
						onClear={() => applyCategory('')}
					/>
				</div>
				{#if hasActiveFilters}
					<button
						type="button"
						onclick={() => applyCategory('')}
						class="inline-flex items-center gap-1 text-xs text-[var(--color-muted)] underline hover:text-[var(--color-text)]"
					>
						<X size={12} /> {t('common.clear_filters', 'Clear filters')}
					</button>
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
									{t('blog_series.view', 'View series')} <ArrowRight size={14} />
								</span>
							</div>
						</a>
					{/each}
				</div>
			{/if}
		{:else if data.tab === 'roadmap'}
			{#if data.loadError}
				<p class="py-12 text-center text-[var(--color-text-subtle)]">
					{t('guides.load_error', 'Failed to load guides. Please try again later.')}
				</p>
			{:else if roadmapGroups.length === 0}
				<EmptyState icon={BookOpen} title={t('guides.empty', 'No guides yet. Check back soon.')} />
			{:else}
				<!-- Journey roadmap: guides grouped by stage in japa order (§87) -->
				<div class="space-y-6">
					{#each roadmapGroups as group, i (group.stage)}
						<section>
							<div class="mb-2 flex items-center gap-2">
								<span class="grid h-6 w-6 place-items-center rounded-full bg-[var(--color-primary)]/10 text-xs font-semibold text-[var(--color-primary)]">
									{i + 1}
								</span>
								<h2 class="text-sm font-semibold text-[var(--color-text)]">{group.label}</h2>
								<span class="text-xs text-[var(--color-muted)]">{group.items.length}</span>
							</div>
							<div class="grid gap-3.5 sm:grid-cols-2">
								{#each group.items as item (item.id)}
									<ContentGridCard {item} />
								{/each}
							</div>
						</section>
					{/each}
				</div>
			{/if}
		{:else if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('guides.load_error', 'Failed to load guides. Please try again later.')}
			</p>
		{:else if feedItems.length === 0}
			<EmptyState icon={BookOpen} title={t('guides.empty', 'No guides yet. Check back soon.')} />
		{:else}
			<div class="grid gap-3.5 sm:grid-cols-2">
				{#each feedItems as item (item.id)}
					<ContentGridCard {item} />
				{/each}
			</div>

			{#if nextCursor}
				<div class="mt-4 flex justify-center">
					<button
						type="button"
						onclick={loadMore}
						disabled={loadingMore}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>
						{loadingMore ? t('common.loading', 'Loading…') : t('guides.load_more', 'Load more')}
						<ChevronDown size={14} />
					</button>
				</div>
			{:else if moreError}
				<p class="mt-4 text-center text-sm text-[var(--color-text-subtle)]">
					{t('guides.load_error', 'Failed to load guides. Please try again later.')}
				</p>
			{/if}
		{/if}
	</div>
</div>
