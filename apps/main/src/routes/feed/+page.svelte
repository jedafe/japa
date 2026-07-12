<script lang="ts">
	import { Rss, RefreshCw } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api, type FeedItem } from '$lib/api';
	import TrendingPanel from '$lib/components/TrendingPanel.svelte';
	import SuggestionsPanel from '$lib/components/SuggestionsPanel.svelte';
	import AdSlot from '$lib/components/AdSlot.svelte';
	import FeedCard from '$lib/components/feed/FeedCard.svelte';
	import { onMount } from 'svelte';
	import autoAnimate from '@formkit/auto-animate';
	import { t } from '$lib/stores/i18n.svelte';

	let { data } = $props();

	// svelte-ignore state_referenced_locally
	let feed = $state<FeedItem[]>(data.feed ?? []);
	let cardStyle = $state(data.cardStyle ?? 'reddit');
	let offset = $state(20);
	let hasMore = $state(feed.length >= 20);
	let loadingMore = $state(false);
	let refreshing = $state(false);

	async function loadMore() {
		if (loadingMore || !hasMore) return;
		loadingMore = true;
		try {
			const items = await api.getFeedPrimary({ page_size: 20, cursor: String(offset) });
			if (items && items.length > 0) {
				feed = [...feed, ...items];
				offset += items.length;
				hasMore = items.length >= 20;
			} else {
				hasMore = false;
			}
		} catch {
			// silent
		} finally {
			loadingMore = false;
		}
	}

	async function handleRefresh() {
		refreshing = true;
		await new Promise(r => setTimeout(r, 600));
		refreshing = false;
		window.location.reload();
	}

	onMount(() => {
		const observer = new IntersectionObserver(
			(entries) => {
				if (entries[0].isIntersecting) loadMore();
			},
			{ threshold: 0.1 }
		);
		const sentinel = document.getElementById('feed-sentinel');
		if (sentinel) observer.observe(sentinel);
		return () => observer.disconnect();
	});
</script>

<svelte:head>
	<title>Feed | JapaConnect</title>
	<meta name="description" content="Stay updated with the latest posts, discussions, and stories from the JapaConnect community." />

</svelte:head>

<main class="mx-auto max-w-6xl px-4 py-8">
	<header class="mb-8 flex items-start justify-between">
		<div>
			<h1 class="text-2xl font-bold tracking-tight sm:text-3xl" style="color: var(--color-foreground)">
				{t('feed.title')}
			</h1>
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				{t('feed.subtitle')}
			</p>
		</div>
		<button
			class="btn btn-secondary group gap-2"
			onclick={handleRefresh}
			disabled={refreshing}
		>
			<RefreshCw class="h-4 w-4 transition-transform group-hover:rotate-45 {refreshing ? 'animate-spin' : ''}" />
			{t('feed.refresh')}
		</button>
	</header>

	<div class="grid grid-cols-1 gap-6 lg:grid-cols-[1fr_300px]">
		<!-- Main feed column -->
		<div>
			{#if feed.length === 0}
				<EmptyState
					icon={Rss}
					title={t('feed.empty')}
					message={t('feed.empty_hint')}
				/>
			{:else}
				<div class="space-y-4" use:autoAnimate>
			{#each feed as item, i (item.id)}
				<FeedCard {item} style={cardStyle} index={i} />

				{#if (i + 1) % 5 === 0}
					<AdSlot slotName="feed_inline" />
				{/if}
			{/each}
		</div>

		{#if hasMore}
			<div id="feed-sentinel" class="flex justify-center py-8">
				{#if loadingMore}
					<div class="flex items-center gap-3">
						<div class="h-5 w-5 animate-spin rounded-full border-2 border-t-transparent" style="border-color: var(--color-primary); border-top-color: transparent"></div>
						<p class="text-sm font-medium" style="color: var(--color-muted)">{t('feed.loading')}</p>
					</div>
				{:else}
					<button
						onclick={loadMore}
						class="btn btn-secondary"
					>
						{t('feed.load_more')}
					</button>
				{/if}
			</div>
		{/if}
	{/if}
		</div>

		<!-- Right sidebar: trending + suggestions -->
		<aside class="hidden lg:flex lg:flex-col lg:gap-4">
			<TrendingPanel />
			<SuggestionsPanel />
		</aside>
	</div>
</main>
