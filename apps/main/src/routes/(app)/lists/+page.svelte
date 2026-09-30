<script lang="ts">
	// Lists hub at /lists — modeled on /guides minus categories, series, and
	// roadmap (lists carry no taxonomy). GET /content?type=list drives all
	// three tabs; popular paginates by offset (view_count order has no
	// cursor) via the `offset:N` pseudo-cursor convention.
	// Ref: japaconnect-engineering/sections/14-content-type-system.md (List amendment)
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { ChevronDown, ListOrdered, PenSquare } from 'lucide-svelte';
	import { EmptyState, ListPageBar, PageAction, UrlTabs, type UrlTab } from '$lib/components/ui';
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
			content_type: 'list',
			author_id: g.author_id ?? '',
			author_name: g.author_display_name ?? g.author_username ?? '',
			author_username: g.author_username,
			author_avatar_url: g.author_avatar_url,
			space_id: g.space_id,
			space_name: g.space_name,
			excerpt: g.meta_description ?? undefined,
			created_at: g.created_at,
			published_at: g.published_at ?? undefined,
			view_count: g.view_count,
			comment_count: g.comment_count,
			reaction_count: g.reaction_count,
			bookmark_count: g.bookmark_count,
			vote_score: g.reshare_count ?? g.share_count ?? 0,
			reading_time_minutes: g.read_time_minutes,
			cover_image_url: g.featured_image_url ?? g.header_image_url ?? null,
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

	// Re-seed client state whenever the server re-runs load (e.g. ?tab= nav).
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
			const params = new URLSearchParams({ type: 'list', page_size: String(data.pageSize) });
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

	const title = $derived(t('lists.title', 'Lists'));
	const description = $derived(
		t('lists.description', 'Ranked and curated lists from the community')
	);

	const tabs = $derived<UrlTab[]>([
		{ value: 'latest', label: t('lists.sort.latest', 'Latest') },
		{ value: 'popular', label: t('lists.sort.popular', 'Popular') },
		{ value: 'featured', label: t('lists.tab.featured', 'Featured') }
	]);

	function switchTab(tab: string) {
		const params = new URLSearchParams();
		if (tab !== 'latest') params.set('tab', tab);
		const qs = params.toString();
		goto(qs ? `/lists?${qs}` : '/lists');
	}
</script>

<svelte:head>
	<title>{title} — {t('app.name')}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-full" style="--page-gutter: 1rem">
	<h1 class="sr-only">{t('lists.title', 'Lists')}</h1>
	<ListPageBar>
		<UrlTabs
			{tabs}
			value={data.tab}
			onValueChange={(v) => switchTab(v)}
			class="!border-b-0 !bg-transparent"
		/>
		{#snippet actions()}
			{#if data.authed}
				<PageAction
					href="/create?type=list"
					icon={PenSquare}
					label={t('lists.new_list', 'New list')}
				/>
			{:else}
				<PageAction
					href="/login?redirect=/create?type=list"
					icon={PenSquare}
					label={t('lists.new_list', 'New list')}
				/>
			{/if}
		{/snippet}
	</ListPageBar>

	<div class="p-4">
		{#if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('lists.load_error', 'Failed to load lists. Please try again later.')}
			</p>
		{:else if feedItems.length === 0}
			<EmptyState icon={ListOrdered} title={t('lists.empty', 'No lists yet')} />
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
						{loadingMore ? t('common.loading', 'Loading…') : t('common.load_more', 'Load more')}
						<ChevronDown size={14} />
					</button>
				</div>
			{:else if moreError}
				<p class="mt-4 text-center text-sm text-[var(--color-text-subtle)]">
					{t('lists.load_error', 'Failed to load lists. Please try again later.')}
				</p>
			{/if}
		{/if}
	</div>
</div>
