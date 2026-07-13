<script lang="ts">
	import { TrendingUp, Hash, Users, Layers } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { onMount } from 'svelte';

	type TrendingItem = { id: string; title?: string; name?: string; slug?: string; username?: string };

	let trendingContent = $state<TrendingItem[]>([]);
	let trendingSpaces = $state<TrendingItem[]>([]);
	let trendingTags = $state<TrendingItem[]>([]);
	let trendingUsers = $state<TrendingItem[]>([]);
	let loading = $state(true);
	let tab = $state<'content' | 'spaces' | 'tags' | 'users'>('content');

	onMount(async () => {
		try {
			const [c, s, t, u] = await Promise.allSettled([
				api.getTrendingContent(5),
				api.getTrendingSpaces(5),
				api.getTrendingTags(5),
				api.getTrendingUsers(5)
			]);
			if (c.status === 'fulfilled' && Array.isArray(c.value)) trendingContent = c.value;
			if (s.status === 'fulfilled' && Array.isArray(s.value))
				trendingSpaces = s.value as TrendingItem[];
			if (t.status === 'fulfilled' && Array.isArray(t.value))
				trendingTags = t.value as TrendingItem[];
			if (u.status === 'fulfilled' && Array.isArray(u.value))
				trendingUsers = u.value as TrendingItem[];
		} catch {
			// degrade gracefully
		} finally {
			loading = false;
		}
	});

	const tabs = [
		{ key: 'content' as const, label: 'Posts', icon: TrendingUp },
		{ key: 'spaces' as const, label: 'Spaces', icon: Layers },
		{ key: 'tags' as const, label: 'Tags', icon: Hash },
		{ key: 'users' as const, label: 'People', icon: Users }
	] as const;

	const activeItems = $derived(
		tab === 'content'
			? trendingContent
			: tab === 'spaces'
				? trendingSpaces
				: tab === 'tags'
					? trendingTags
					: trendingUsers
	);

	function itemHref(item: TrendingItem) {
		if (tab === 'content') return `/blog/${item.slug ?? item.id}`;
		if (tab === 'spaces') return `/spaces/${item.slug ?? item.id}`;
		if (tab === 'tags') return `/tags/${item.slug ?? item.name}`;
		return `/profile/${item.username ?? item.id}`;
	}

	function itemLabel(item: TrendingItem) {
		return item.title ?? item.name ?? item.username ?? item.id;
	}
</script>

<aside
	class="rounded-2xl border p-4"
	style="border-color: var(--color-border); background: var(--color-surface)"
>
	<div class="mb-3 flex items-center gap-2">
		<TrendingUp class="h-4 w-4" style="color: var(--color-primary)" />
		<h2 class="text-sm font-semibold" style="color: var(--color-foreground)">Trending</h2>
	</div>

	<!-- Tab strip -->
	<div class="mb-3 flex gap-1 rounded-lg p-1" style="background: var(--color-background)">
		{#each tabs as { key, label }}
			<button
				onclick={() => (tab = key)}
				class="flex-1 rounded-md py-1 text-xs font-medium transition-colors"
				style={tab === key
					? 'background: var(--color-primary); color: white'
					: 'color: var(--color-muted)'}
			>
				{label}
			</button>
		{/each}
	</div>

	{#if loading}
		<div class="space-y-2">
			{#each Array(4) as _, i (i)}
				<div
					class="h-8 animate-pulse rounded-lg"
					style="background: var(--color-background)"
				></div>
			{/each}
		</div>
	{:else if activeItems.length === 0}
		<p class="text-center text-xs py-4" style="color: var(--color-muted)">Nothing trending yet</p>
	{:else}
		<ol class="space-y-1">
			{#each activeItems as item, i (item.id)}
				<li>
					<a
						href={itemHref(item)}
						class="flex items-center gap-2.5 rounded-lg px-2 py-1.5 text-sm transition-colors hover:opacity-80"
						style="color: var(--color-foreground)"
					>
						<span
							class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-xs font-bold"
							style="background: color-mix(in srgb, var(--color-primary) 12%, transparent); color: var(--color-primary)"
						>
							{i + 1}
						</span>
						<span class="min-w-0 truncate">{itemLabel(item)}</span>
					</a>
				</li>
			{/each}
		</ol>
	{/if}

	<a
		href="/trending"
		class="mt-3 block text-center text-xs font-medium transition-colors hover:underline"
		style="color: var(--color-primary)"
	>
		See all trending →
	</a>
</aside>
