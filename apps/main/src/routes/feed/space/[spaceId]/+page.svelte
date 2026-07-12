<script lang="ts">
	import { Users } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api, type FeedItem } from '$lib/api';
	import FeedCard from '$lib/components/feed/FeedCard.svelte';
	import { onMount } from 'svelte';
	import autoAnimate from '@formkit/auto-animate';

	let { data } = $props();

	const space = $derived(data.space);
	let feed = $state<FeedItem[]>([]);
	let cardStyle = $state('reddit');
	let offset = $state(20);
	let hasMore = $state(true);

	$effect(() => {
		feed = data.feed ?? [];
		cardStyle = data.cardStyle ?? 'reddit';
		hasMore = feed.length >= 20;
	});
	let loadingMore = $state(false);

	async function loadMore() {
		if (loadingMore || !hasMore) return;
		loadingMore = true;
		try {
			const items = await api.getSpaceFeed(space.id, { page_size: 20, page_offset: offset });
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
	<title>{space.name} Feed | JapaConnect</title>
	<meta name="description" content="Latest posts from {space.name}." />
</svelte:head>

<main class="mx-auto max-w-3xl px-4 py-8">
	<header class="mb-8">
		<div class="flex items-center gap-2 text-xs" style="color: var(--color-muted)">
			<a href="/spaces" class="hover:underline">Spaces</a>
			<span>/</span>
			<span>{space.name}</span>
		</div>
		<h1
			class="mt-2 text-2xl font-bold sm:text-3xl"
			style="color: var(--color-foreground)"
		>
			{space.name}
		</h1>
		{#if space.description}
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				{space.description}
			</p>
		{/if}
	</header>

	<div class="mb-6 flex gap-3">
		<a
			href="/spaces/{space.slug}"
			class="flex items-center gap-1.5 rounded-lg border px-3 py-2 text-sm font-medium transition-colors hover:opacity-80"
			style="border-color: var(--color-border); color: var(--color-foreground)"
		>
			<Users class="h-4 w-4" />
			About
		</a>
		<button
			class="flex items-center gap-1.5 rounded-lg px-3 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
			style="background-color: var(--color-primary)"
		>
			New post
		</button>
	</div>

	{#if feed.length === 0}
		<EmptyState
			icon={Users}
			title="No posts yet"
			message="Be the first to post in this space."
		/>
	{:else}
		<div class="space-y-4" use:autoAnimate>
			{#each feed as item, i (item.id)}
				<FeedCard {item} style={cardStyle} index={i} />
			{/each}
		</div>

		{#if hasMore}
			<div id="feed-sentinel" class="flex justify-center py-8">
				{#if loadingMore}
					<p class="text-sm" style="color: var(--color-muted)">Loading more...</p>
				{:else}
					<button
						onclick={loadMore}
						class="rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:opacity-80"
						style="border-color: var(--color-border); color: var(--color-foreground)"
					>
						Load more
					</button>
				{/if}
			</div>
		{/if}
	{/if}
</main>
