<script lang="ts">
	import { User } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api, type FeedItem } from '$lib/api';
	import FeedCard from '$lib/components/feed/FeedCard.svelte';
	import { onMount } from 'svelte';
	import autoAnimate from '@formkit/auto-animate';

	let { data } = $props();

	const profile = $derived(data.profile);
	// svelte-ignore state_referenced_locally
	let feed = $state<FeedItem[]>(data.feed ?? []);
	let cardStyle = $state(data.cardStyle ?? 'reddit');
	let offset = $state(20);
	let hasMore = $state(feed.length >= 20);
	let loadingMore = $state(false);

	async function loadMore() {
		if (loadingMore || !hasMore) return;
		loadingMore = true;
		try {
			const items = await api.getProfileFeed(profile.id, { page_size: 20, page_offset: offset });
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
	<title>{profile.display_name || profile.username} | JapaConnect</title>
	<meta name="description" content="Posts by {profile.display_name || profile.username}." />
</svelte:head>

<main class="mx-auto max-w-3xl px-4 py-8">
	<header class="mb-8">
		<div class="flex items-center gap-4">
			<div
				class="flex h-12 w-12 items-center justify-center rounded-full text-lg font-bold text-white"
				style="background-color: var(--color-primary)"
			>
				{profile.display_name?.charAt(0)?.toUpperCase() ?? profile.username?.charAt(0)?.toUpperCase() ?? '?'}
			</div>
			<div>
				<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">
					{profile.display_name || profile.username}
				</h1>
				<p class="text-sm" style="color: var(--color-muted)">
					@{profile.username}
				</p>
			</div>
		</div>
		{#if profile.bio}
			<p class="mt-4 text-sm" style="color: var(--color-muted)">
				{profile.bio}
			</p>
		{/if}
	</header>

	<div class="mb-6 flex gap-3">
		<a
			href="/profile/{profile.username}"
			class="flex items-center gap-1.5 rounded-lg border px-3 py-2 text-sm font-medium transition-colors hover:opacity-80"
			style="border-color: var(--color-border); color: var(--color-foreground)"
		>
			<User class="h-4 w-4" />
			Profile
		</a>
	</div>

	{#if feed.length === 0}
		<EmptyState
			icon={User}
			title="No posts yet"
			message="This user hasn't posted anything yet."
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
