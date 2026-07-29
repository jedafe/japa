<script lang="ts">
	import { untrack } from 'svelte';
	import { navigating, page } from '$app/state';
	import { TriangleAlert } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { feedPath, type FeedTab, type SortValue } from '$lib/feed/endpoint';
	import { cardStyleFor, gridMode, type CardStyleConfig } from '$lib/feed/cardStyle';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
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
	}
	let { items: initial, tab, sort, pageSize, config, loadError }: Props = $props();

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

	// Re-seed on a new SSR payload (tab/sort change) — replace, never append.
	$effect(() => {
		const seed = initial;
		untrack(() => {
			items = seed;
			atEnd = seed.length < pageSize;
			loading = false;
		});
	});

	// Layout mode is fixed from the initial SSR page (never flips mid-scroll).
	const gridLayout = $derived(gridMode(initial.map((i) => i.content_type), config));

	$effect(() => {
		const el = sentinel;
		if (!el || atEnd) return;
		const obs = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) loadMore();
			},
			{ rootMargin: '600px' }
		);
		obs.observe(el);
		return () => obs.disconnect();
	});

	async function loadMore() {
		if (loading || atEnd) return;
		loading = true;
		const res = await api.get<FeedItem[]>(feedPath(tab, sort, pageSize, items.length, geo));
		if (res.error) {
			loading = false;
			return;
		}
		const batch = res.data ?? [];
		items = [...items, ...batch];
		atEnd = batch.length < pageSize;
		loading = false;
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
				<FeedCard {item} style={cardStyleFor(item.content_type, config)} layout="grid-cell" />
			{/each}
		</div>
	{:else}
		<div
			class="divide-y divide-[var(--color-border)] overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]"
		>
			{#each items as item (item.id)}
				<FeedCard {item} style={cardStyleFor(item.content_type, config)} layout="stream" />
			{/each}
		</div>
	{/if}

	{#if !atEnd}
		<div bind:this={sentinel} class="h-8" aria-hidden="true"></div>
		{#if loading}<LoadingSkeleton variant="lines" count={2} class="p-4" />{/if}
	{:else}
		<p class="py-6 text-center text-xs text-[var(--color-muted)]">{t('feed.end', "You're all caught up")}</p>
	{/if}
{/if}
