<script lang="ts">
	import { Rss, Clock, MessageCircle, Heart } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { onMount } from 'svelte';

	let { data } = $props();

	let feed = $state(data.feed ?? []);
	let hasMore = $state(data.hasMore ?? false);
	let loadingMore = $state(false);

	function formatDate(dateStr: string): string {
		if (!dateStr) return '';
		const d = new Date(dateStr);
		return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
	}

	async function loadMore() {
		if (loadingMore || !hasMore) return;
		loadingMore = true;
		try {
			const token = document.cookie.match(/jc_token=([^;]+)/)?.[1];
			const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};
			const lastItem = feed[feed.length - 1];
			const cursor = lastItem ? `&cursor=${lastItem.id}` : '';
			const res = await fetch(`/api/v1/feed/primary?limit=20${cursor}`, { headers });
			const body = await res.json();
			if (body.data?.items) {
				feed = [...feed, ...body.data.items];
				hasMore = body.data.hasMore;
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
	<title>Feed | JapaConnect</title>
	<meta name="description" content="Stay updated with the latest from your community." />
</svelte:head>

<main class="mx-auto max-w-3xl px-4 py-8">
	<header class="mb-8 flex items-start justify-between">
		<div>
			<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">Feed</h1>
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				Stay updated with the latest from your community.
			</p>
		</div>
		<a
			href="/feed"
			class="flex items-center gap-1.5 rounded-lg border px-3 py-2 text-sm font-medium transition-colors hover:opacity-80"
			style="border-color: var(--color-border); color: var(--color-muted)"
		>
			<Rss class="h-4 w-4" />
			Refresh
		</a>
	</header>

	{#if feed.length === 0}
		<EmptyState
			icon={Rss}
			title="Your feed is empty"
			message="Follow spaces and people to see their posts here."
		/>
	{:else}
		<div class="space-y-4">
			{#each feed as item (item.id)}
				<article
					class="rounded-xl border p-5 transition-all hover:shadow-md"
					style="border-color: var(--color-border); background: var(--color-surface)"
				>
					<div class="flex items-center gap-3">
						<div
							class="flex h-8 w-8 items-center justify-center rounded-full text-xs font-semibold text-white"
							style="background-color: var(--color-primary)"
						>
							{item.author_name?.charAt(0)?.toUpperCase() ?? '?'}
						</div>
						<div>
							<a
								href="/profile/{item.author_id}"
								class="text-sm font-medium hover:underline"
								style="color: var(--color-foreground)"
							>
								{item.author_name}
							</a>
							<p class="text-xs" style="color: var(--color-muted)">
								{formatDate(item.created_at)}
							</p>
						</div>
					</div>

					<a href="/blog/{item.slug ?? item.id}" class="mt-3 block">
						<h2
							class="text-base font-semibold hover:underline sm:text-lg"
							style="color: var(--color-foreground)"
						>
							{item.title}
						</h2>
						{#if item.excerpt}
							<p class="mt-2 text-sm line-clamp-3" style="color: var(--color-muted)">
								{item.excerpt}
							</p>
						{/if}
					</a>

					<div
						class="mt-4 flex items-center gap-4 text-xs"
						style="color: var(--color-muted)"
					>
						<span class="flex items-center gap-1">
							<Heart class="h-3.5 w-3.5" />
							{item.reaction_count}
						</span>
						<span class="flex items-center gap-1">
							<MessageCircle class="h-3.5 w-3.5" />
							{item.comment_count}
						</span>
						<span
							class="rounded-full px-2 py-0.5"
							style="background-color: var(--color-background)"
						>
							{item.content_type}
						</span>
					</div>
				</article>
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
