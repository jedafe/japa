<script lang="ts">
	import { User, Clock, MessageCircle, Heart } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { onMount } from 'svelte';
	import { formatTimestamp, formatRelativeTime } from '$lib/utils/timezone';
	import autoAnimate from '@formkit/auto-animate';

	let { data } = $props();

	const profile = $derived(data.profile);
	let feed = $state(data.feed ?? []);
	let hasMore = $state(data.hasMore ?? false);
	let loadingMore = $state(false);

	async function loadMore() {
		if (loadingMore || !hasMore) return;
		loadingMore = true;
		try {
			const token = document.cookie.match(/jc_token=([^;]+)/)?.[1];
			const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};
			const lastItem = feed[feed.length - 1];
			const cursor = lastItem ? `&cursor=${lastItem.id}` : '';
			const res = await fetch(`/api/v1/feed/profile/${profile.id}?limit=20${cursor}`, { headers });
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
								{formatRelativeTime(item.created_at)}
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
