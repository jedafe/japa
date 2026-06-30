<script lang="ts">
	import { Rss, Clock, MessageCircle, Heart, RefreshCw } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api } from '$lib/api';
	import AdSlot from '$lib/components/AdSlot.svelte';
	import { onMount } from 'svelte';
	import { formatTimestamp, formatRelativeTime } from '$lib/utils/timezone';
	import autoAnimate from '@formkit/auto-animate';
	import { t } from '$lib/stores/i18n.svelte';

	let { data } = $props();

	// svelte-ignore state_referenced_locally
	let feed = $state(data.feed ?? []);
	// svelte-ignore state_referenced_locally
	let hasMore = $state(data.hasMore ?? false);
	let loadingMore = $state(false);
	let refreshing = $state(false);

	async function loadMore() {
		if (loadingMore || !hasMore) return;
		loadingMore = true;
		try {
			const lastItem = feed[feed.length - 1];
			const cursor = lastItem?.id;
			const result = await api.getFeedPrimary({ limit: 20, cursor });
			if (result.items) {
				feed = [...feed, ...result.items];
				hasMore = result.has_more;
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
	<title>{t('feed.title')} | JapaConnect</title>
	<meta name="description" content="{t('feed.subtitle')}" />
</svelte:head>

<main class="mx-auto max-w-3xl px-4 py-8">
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

	{#if feed.length === 0}
		<EmptyState
			icon={Rss}
			title={t('feed.empty')}
			message={t('feed.empty_hint')}
		/>
	{:else}
		<div class="space-y-4" use:autoAnimate>
			{#each feed as item, i (item.id)}
				<article
					class="card-interactive group rounded-2xl border p-5"
					style="border-color: var(--color-border); background: var(--color-surface); animation: slide-up 0.3s ease-out {i * 50}ms both"
				>
					<div class="flex items-center gap-3">
						<div
							class="flex h-10 w-10 items-center justify-center rounded-full text-sm font-semibold text-white shadow-sm"
							style="background: var(--gradient-primary)"
						>
							{item.author_name?.charAt(0)?.toUpperCase() ?? '?'}
						</div>
						<div class="min-w-0 flex-1">
							<a
								href="/profile/{item.author_id}"
								class="text-sm font-semibold hover:underline"
								style="color: var(--color-foreground)"
							>
								{item.author_name}
							</a>
							<p class="flex items-center gap-1.5 text-xs" style="color: var(--color-muted)">
								<Clock class="h-3 w-3" />
								{formatRelativeTime(item.created_at)}
							</p>
						</div>
					</div>

					<a href="/blog/{item.slug ?? item.id}" class="mt-4 block">
						<h2
							class="text-base font-semibold leading-snug transition-colors group-hover:opacity-80 sm:text-lg"
							style="color: var(--color-foreground)"
						>
							{item.title}
						</h2>
						{#if item.excerpt}
							<p class="mt-2 text-sm leading-relaxed line-clamp-3" style="color: var(--color-muted)">
								{item.excerpt}
							</p>
						{/if}
					</a>

					<div
						class="mt-4 flex items-center gap-5 border-t pt-4 text-sm"
						style="border-color: var(--color-border); color: var(--color-muted)"
					>
						<button class="flex items-center gap-1.5 transition-colors hover:text-red-500">
							<Heart class="h-4 w-4" />
							<span class="text-xs font-medium">{item.reaction_count}</span>
						</button>
						<button class="flex items-center gap-1.5 transition-colors hover:text-blue-500">
							<MessageCircle class="h-4 w-4" />
							<span class="text-xs font-medium">{item.comment_count}</span>
						</button>
						<span
							class="ml-auto rounded-full px-2.5 py-0.5 text-xs font-medium"
							style="background: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)"
						>
							{item.content_type}
						</span>
					</div>
				</article>

				<!-- Ad slot every 5 items -->
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
</main>
