<script lang="ts">
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { PenSquare, Tag, X } from 'lucide-svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { ListPageBar, UrlTabs, type UrlTab, FilterToggle, PageAction } from '#lib/components/ui/index.js';
	import Autocomplete from '#lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '#lib/components/create/Autocomplete.svelte';
	import { staticAutocompleteFetcher } from '#lib/autocomplete.js';
	import ForumList from '#lib/components/forum/ForumList.svelte';
	import FeedCard from '#lib/components/feed/FeedCard.svelte';
	import { api } from '#lib/api/client.js';
	import { Loader2 } from 'lucide-svelte';
	import { compose } from '#lib/components/shell/compose.svelte.js';
	import { t } from '#lib/i18n/index.js';
	import { tiptapJsonFirstImageSrc } from '#lib/tiptap/serialize.js';
	import type { FeedItem } from '#lib/types/index.js';
	import type { GuideListItem } from '#lib/types/content.js';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('forum.list_title', 'Threads — {site}'));
	const description = $derived(
		t('forum.list_description', 'Discussions, questions, and answers from the Nigerian diaspora community.')
	);
	// SEO-004: the unfiltered default tab serves the same /explore feed —
	// two indexable near-identical lists would compete. noindex it and
	// canonical to /explore; filtered views (status/tag/author/featured)
	// are thread-specific and keep their own indexable URL.
	const isFiltered = $derived(
		data.status !== 'all' || !!data.tagSlug || !!data.authorId || data.sort === 'featured'
	);
	const robots = $derived(isFiltered ? '' : 'noindex,follow');
	const canonical = $derived(
		isFiltered ? `${page.url.origin}/thread` : `${page.url.origin}/explore`
	);

	function buildUrl(sort: string, status: string) {
		const params = new URLSearchParams();
		if (sort !== 'latest') params.set('sort', sort);
		// status/tag filters apply to the thread list only — the featured view
		// is a curated list with no filter semantics (same pattern as /blog).
		if (sort !== 'featured') {
			if (status !== 'all') params.set('status', status);
			if (data.tagSlug) params.set('tag', data.tagSlug);
		}
		return `/thread${params.toString() ? `?${params.toString()}` : ''}`;
	}

	function applyFilter(kind: 'sort' | 'status', value: string) {
		const sort = kind === 'sort' ? value : data.sort;
		const status = kind === 'status' ? value : data.status;
		goto(buildUrl(sort, status));
	}

	const tabs = $derived<UrlTab[]>([
		{ value: 'latest', label: t('forum.latest', 'Latest') },
		{ value: 'top', label: t('forum.top', 'Top') },
		{ value: 'featured', label: t('forum.tab_featured', 'Featured') }
	]);

	function featuredToFeedItem(g: GuideListItem): FeedItem {
		return {
			id: g.id,
			title: g.title,
			slug: g.slug,
			content_type: g.content_type ?? 'forum_thread',
			author_id: g.author_id ?? '',
			author_name: g.author_display_name ?? g.author_username ?? '',
			author_username: g.author_username,
			author_avatar_url: g.author_avatar_url,
			space_id: g.space_id ?? undefined,
			space_name: g.space_name ?? undefined,
			excerpt: g.purpose ?? undefined,
			// contents.body holds the raw TipTap JSON for forum threads — derive
			// the card cover from the first embedded image (no explicit cover).
			cover_image_url: g.featured_image_url ?? g.header_image_url ?? tiptapJsonFirstImageSrc(g.body),
			created_at: g.published_at ?? g.created_at,
			view_count: g.view_count,
			comment_count: g.comment_count,
			vote_score: g.vote_score ?? 0,
			bookmark_count: g.bookmark_count,
			reading_time_minutes: g.read_time_minutes
		};
	}

	const featuredItems = $derived(data.featured.map(featuredToFeedItem));

	// The default list is /explore — ALL threads (every contents row is a
	// thread). Cards open /thread/{slug} (threadDefault). ForumList only
	// renders when forum-specific filters (status/tag/author) narrow the
	// list to forum_thread rows.
	let moreItems = $state<FeedItem[]>([]);
	let exhausted = $state(false);
	let loadingMore = $state(false);
	// Ranking can drift between offset page fetches — an item may shift into
	// the next page's window. Dedupe by id so the keyed each never sees a
	// repeat (§6.3 each_key_duplicate).
	const allItems = $derived.by(() => {
		const seen = new Set<string>();
		return [...data.items, ...moreItems].filter((i) => {
			if (seen.has(i.id)) return false;
			seen.add(i.id);
			return true;
		});
	});
	const forumFiltered = $derived(data.status !== 'all' || !!data.tagSlug || !!data.authorId);

	$effect(() => {
		// Reset pagination whenever the server list re-loads (sort change).
		data.items;
		moreItems = [];
		exhausted = false;
	});

	async function loadMore() {
		if (loadingMore) return;
		loadingMore = true;
		const res = await api.get<FeedItem[]>(
			`/explore?sort=${data.sort === 'top' ? 'top' : 'latest'}&page_size=${data.pageSize}&page_offset=${allItems.length}`
		);
		loadingMore = false;
		const next = res.data ?? [];
		moreItems = [...moreItems, ...next];
		if (next.length < data.pageSize) exhausted = true;
	}

	const hasActiveFilters = $derived(data.status !== 'all' || !!data.tagSlug);

	const statusOptions = $derived<AutocompleteItem[]>([
		{ id: 'solved', label: t('forum.solved', 'Solved') },
		{ id: 'open', label: t('forum.open', 'Open') }
	]);
	const statusFetcher = $derived(staticAutocompleteFetcher(statusOptions));
	const selectedStatusLabel = $derived(
		statusOptions.find((o) => o.id === data.status)?.label ?? null
	);

	let showFilters = $state(false);
	$effect(() => {
		if (hasActiveFilters) showFilters = true;
	});
</script>

<SeoHead title={title} description={description} ogTitle={title} {canonical} {robots} />

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	<h1 class="sr-only">{t('forum.heading', 'Threads')}</h1>
	<ListPageBar>
		<UrlTabs
			{tabs}
			value={data.sort}
			onValueChange={(v) => applyFilter('sort', v)}
			class="!border-b-0 !bg-transparent"
		/>
		{#snippet actions()}
			{#if data.sort !== 'featured'}
				<FilterToggle bind:show={showFilters} active={hasActiveFilters} controls="forum-filter-panel" />
			{/if}
			{#if data.authed}
				<PageAction
					onclick={() => compose.openDiscussion()}
					icon={PenSquare}
					label={t('forum.new_discussion', 'New discussion')}
				/>
			{:else}
				<PageAction
					href="/login?redirect=/thread"
					icon={PenSquare}
					label={t('forum.new_discussion', 'New discussion')}
				/>
			{/if}
		{/snippet}
	</ListPageBar>

	<div class="mx-auto w-full max-w-3xl p-4">
		{#if showFilters && data.sort !== 'featured'}
			<div
				id="forum-filter-panel"
				class="mb-4 flex flex-wrap items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
			>
				<div class="flex items-center gap-2 text-xs text-[var(--color-text-subtle)]">
					<div class="w-44">
						<Autocomplete
							id="forum-status"
							placeholder={t('forum.status', 'Status')}
							ariaLabel={t('forum.status', 'Status')}
							emptyMessage={t('common.no_results', 'No results found')}
							fetcher={statusFetcher}
							initialFetcher={statusFetcher}
							selectedId={data.status !== 'all' ? data.status : null}
							selectedLabel={selectedStatusLabel}
							onSelect={(item) => applyFilter('status', item.id)}
							onClear={() => applyFilter('status', 'all')}
						/>
					</div>
				</div>
				{#if data.tagSlug}
					<div class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-1.5 text-xs text-[var(--color-text-subtle)]">
						<Tag size={12} />
						<span>{data.tagSlug}</span>
						<a href={buildUrl(data.sort, data.status)} class="inline-flex items-center gap-1 text-[var(--color-muted)] hover:text-[var(--color-text)]">
							<X size={11} /> {t('common.clear', 'Clear')}
						</a>
					</div>
				{/if}
			</div>
		{/if}
		{#if data.sort === 'featured'}
			{#if data.loadError}
				<p class="py-12 text-center text-[var(--color-text-subtle)]">
					{t('forum.load_error', 'Failed to load discussions. Please try again later.')}
				</p>
			{:else if featuredItems.length === 0}
				<p class="py-12 text-center text-[var(--color-text-subtle)]">
					{t('forum.featured_empty', 'No featured discussions yet.')}
				</p>
			{:else}
				<div class="flex flex-col gap-3">
					{#each featuredItems as item (item.id)}
						<!-- Featured threads use the same forum card as the thread list. -->
						<FeedCard {item} style="daily" layout="stream" threadDefault voteRail />
					{/each}
				</div>
			{/if}
		{:else if forumFiltered}
			<ForumList
				threads={data.threads}
				pageSize={data.pageSize}
				loadError={data.loadError}
				adSlot="native_forum"
			/>
		{:else}
			{#if data.loadError}
				<p class="py-12 text-center text-[var(--color-text-subtle)]">
					{t('forum.load_error', 'Failed to load discussions. Please try again later.')}
				</p>
			{:else if allItems.length === 0}
				<p class="py-12 text-center text-[var(--color-text-subtle)]">
					{t('thread.no_discussions', 'No discussions yet.')}
				</p>
			{:else}
				<div class="flex flex-col gap-3">
					{#each allItems as item (item.id)}
						<FeedCard {item} style="daily" layout="stream" threadDefault voteRail />
					{/each}
				</div>
				{#if !exhausted}
					<div class="mt-4 flex justify-center">
						<button
							type="button"
							onclick={loadMore}
							disabled={loadingMore}
							class="inline-flex items-center gap-2 rounded-full border border-[var(--color-border)] px-4 py-1.5 text-sm font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
						>
							{#if loadingMore}<Loader2 size={14} class="animate-spin" />{/if}
							{t('common.load_more', 'Load more')}
						</button>
					</div>
				{/if}
			{/if}
		{/if}
	</div>
</div>
