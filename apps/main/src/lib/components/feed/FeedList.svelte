<script lang="ts">
	import { untrack } from 'svelte';
	import { navigating, page } from '$app/state';
	import { TriangleAlert } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { feedQueryString, type FeedTab, type SortValue } from '$lib/feed/endpoint';
	import type { CardStyleConfig } from '$lib/feed/cardStyle';
	import { EmptyState, LoadingSkeleton, PaginationControls } from '$lib/components/ui';
	import { t } from '$lib/i18n';
	import type { FeedItem } from '$lib/types';
	import FeedCard from './FeedCard.svelte';

	interface Props {
		items: FeedItem[];
		tab: FeedTab;
		sort: SortValue;
		pageSize: number;
		config: CardStyleConfig;
		loadError: boolean;
		hasMore?: boolean;
	}
	let { items: initial, tab, sort, pageSize, config, loadError, hasMore: initialHasMore }: Props = $props();

	// Geo re-weight from the user's content_country_preference or ?geo= param
	// (ADR-064). Forwarded on load-more so paginated results stay geo-consistent.
	const geo = $derived(
		page.url.searchParams.get('geo') ??
			page.data.user?.content_country_preference ??
			undefined
	);

	let items = $state<FeedItem[]>(untrack(() => initial));
	let atEnd = $state(false);
	let loading = $state(false);
	let sentinel = $state<HTMLElement | null>(null);
	let currentPage = $state(1);
	let hasMore = $state(false);
	let clientError = $state<string | null>(null);

	// Feed pagination style from system_settings (Rule 3). Defaults to
	// infinite_scroll so existing behaviour is preserved when the setting
	// is absent or invalid.
	// Ref: conventions/system-settings-catalog.md (content.pagination.style)
	const paginationStyle = $derived(
		(page.data.siteSettings?.['content.pagination.style'] as string | undefined) ?? 'infinite_scroll'
	);

	// Re-seed on a new SSR payload (tab/sort change) — replace, never append.
	$effect(() => {
		const seed = initial;
		untrack(() => {
			items = seed;
			currentPage = 1;
			clientError = null;
			// When the loader over-fetched by 1, it passes hasMore. Otherwise
			// fall back to the old length heuristic for backward compatibility.
			hasMore = initialHasMore ?? false;
			atEnd = initialHasMore !== undefined ? !initialHasMore : seed.length < pageSize;
			loading = false;
		});
	});

	// Feeds always render the daily card in a stream; dedicated list pages may
	// use other cards, but FeedList is only used for unified feed surfaces.
	const gridLayout = false;

	$effect(() => {
		const el = sentinel;
		if (!el || atEnd || paginationStyle !== 'infinite_scroll') return;
		const obs = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) loadMore();
			},
			{ rootMargin: '600px' }
		);
		obs.observe(el);
		return () => obs.disconnect();
	});

	function feedPath(offset: number, limit: number) {
		const qs = feedQueryString(tab, sort, limit, offset, geo);
		return tab === 'following'
			? `/feed?${qs}`
			: tab === 'for_you'
				? `/feed/for-you?${qs}`
				: `/explore?${qs}`;
	}

	async function loadMore() {
		if (loading || atEnd) return;
		loading = true;
		clientError = null;
		const res = await api.get<FeedItem[]>(feedPath(items.length, pageSize));
		if (res.error) {
			clientError = res.error.message ?? t('error.500_body');
			loading = false;
			return;
		}
		const batch = res.data ?? [];
		items = [...items, ...batch];
		atEnd = batch.length < pageSize;
		hasMore = !atEnd;
		loading = false;
	}

	async function loadPage(nextPage: number) {
		if (loading) return;
		loading = true;
		clientError = null;
		const offset = (nextPage - 1) * pageSize;
		// Over-fetch by 1 so we know whether another page exists without a
		// dedicated count endpoint.
		const res = await api.get<FeedItem[]>(feedPath(offset, pageSize + 1));
		if (res.error) {
			clientError = res.error.message ?? t('error.500_body');
			loading = false;
			return;
		}
		const batch = res.data ?? [];
		items = batch.slice(0, pageSize);
		hasMore = batch.length > pageSize;
		atEnd = !hasMore;
		currentPage = nextPage;
		loading = false;
	}

	function retry() {
		if (paginationStyle === 'pagination') {
			loadPage(currentPage);
		} else {
			loadMore();
		}
	}
</script>

{#if loadError}
	<EmptyState icon={TriangleAlert} title={t('error.500_title')} body={t('error.500_body')} />
{:else if items.length === 0}
	<EmptyState title={t('empty.default_title')} body={t('empty.default_body')} />
{:else}
	{#if navigating.to}
		<LoadingSkeleton variant="card" count={3} class="p-2" />
	{/if}

	{#if gridLayout}
		<div class="grid grid-cols-1 gap-3.5 p-3 sm:grid-cols-2">
			{#each items as item (item.id)}
				<FeedCard {item} style="daily" layout="grid-cell" />
			{/each}
		</div>
	{:else}
		<div class="flex flex-col gap-3">
			{#each items as item (item.id)}
				<FeedCard {item} style="daily" layout="stream" />
			{/each}
		</div>
	{/if}

	{#if !atEnd || (paginationStyle === 'pagination' && hasMore)}
		{#if paginationStyle === 'infinite_scroll'}
			<div bind:this={sentinel} class="h-8" aria-hidden="true"></div>
			{#if loading}<LoadingSkeleton variant="lines" count={2} class="p-4" />{/if}
		{:else if paginationStyle === 'load_more'}
			<button
				type="button"
				disabled={loading}
				onclick={loadMore}
				class="mt-4 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
			>
				{loading ? t('common.loading', 'Loading…') : t('feed.load_more', 'Load more')}
			</button>
		{:else if paginationStyle === 'pagination'}
			<div class="mt-4 flex justify-center">
				<PaginationControls
					page={currentPage}
					hasMore={hasMore || false}
					onPageChange={loadPage}
				/>
			</div>
			{#if loading}<LoadingSkeleton variant="lines" count={2} class="p-4" />{/if}
		{/if}

		{#if clientError}
			<div class="mt-4 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-center">
				<p class="text-sm text-[var(--color-text)]">{clientError}</p>
				<button
					type="button"
					disabled={loading}
					onclick={retry}
					class="mt-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:opacity-90 disabled:opacity-50"
				>
					{t('common.retry')}
				</button>
			</div>
		{/if}
	{:else}
		<p class="py-6 text-center text-xs text-[var(--color-muted)]">{t('feed.end', "You're all caught up")}</p>
	{/if}
{/if}
