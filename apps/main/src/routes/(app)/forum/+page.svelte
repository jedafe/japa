<script lang="ts">
	import { PenSquare, Tag, X } from 'lucide-svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { ListPageBar, UrlTabs, type UrlTab, FilterToggle, PageAction } from '$lib/components/ui';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { staticAutocompleteFetcher } from '$lib/autocomplete';
	import ForumList from '$lib/components/forum/ForumList.svelte';
	import FeedCard from '$lib/components/feed/FeedCard.svelte';
	import { compose } from '$lib/components/shell/compose.svelte';
	import { t } from '$lib/i18n';
	import { cardStyleFor, type CardStyleConfig } from '$lib/feed/cardStyle';
	import type { FeedItem } from '$lib/types';
	import type { GuideListItem } from '$lib/types/content';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	const cardStyleConfig = $derived((page.data.cardStyleConfig as CardStyleConfig) ?? undefined);

	const title = $derived(t('forum.list_title', 'Forum — JapaConnect'));
	const description = $derived(
		t('forum.list_description', 'Discussions, questions, and answers from the Nigerian diaspora community.')
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
		return `/forum${params.toString() ? `?${params.toString()}` : ''}`;
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
			content_type: 'discussion',
			author_id: g.author_id ?? '',
			author_name: g.author_display_name ?? g.author_username ?? '',
			space_id: g.space_id ?? undefined,
			space_name: g.space_name ?? undefined,
			excerpt: g.purpose ?? undefined,
			created_at: g.published_at ?? g.created_at,
			view_count: g.view_count,
			comment_count: g.comment_count,
			vote_score: g.reshare_count ?? g.share_count ?? 0,
			reading_time_minutes: g.read_time_minutes
		};
	}

	const featuredItems = $derived(data.featured.map(featuredToFeedItem));

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

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<h1 class="sr-only">{t('forum.heading', 'Forum')}</h1>
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
					href="/login?redirect=/forum"
					icon={PenSquare}
					label={t('forum.new_discussion', 'New discussion')}
				/>
			{/if}
		{/snippet}
	</ListPageBar>

	<div class="p-3">
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
				<div class="divide-y divide-[var(--color-border)] overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					{#each featuredItems as item (item.id)}
						<FeedCard {item} style={cardStyleFor(item.content_type, cardStyleConfig ?? { default: 'reddit', byType: {} })} layout="stream" />
					{/each}
				</div>
			{/if}
		{:else}
			<ForumList
				threads={data.threads}
				pageSize={data.pageSize}
				loadError={data.loadError}
				config={cardStyleConfig}
			/>
		{/if}
	</div>
</div>
