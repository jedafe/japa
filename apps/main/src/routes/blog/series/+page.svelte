<script lang="ts">
	import { BookOpen, Layers } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';

	let { data } = $props();

	const series = $derived(data.series ?? []);

	function formatDate(dateStr: string): string {
		if (!dateStr) return '';
		const d = new Date(dateStr);
		return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
	}
</script>

<svelte:head>
	<title>Blog Series | JapaConnect</title>
	<meta name="description" content="Explore multi-part blog series on JapaConnect." />
</svelte:head>

<main class="mx-auto max-w-4xl px-4 py-8">
	<header class="mb-8">
		<a
			href="/blog"
			class="mb-4 inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
			style="color: var(--color-muted)"
		>
			&larr; Back to blog
		</a>
		<h1 class="text-3xl font-bold tracking-tight sm:text-4xl" style="color: var(--color-foreground)">
			Blog Series
		</h1>
		<p class="mt-2 text-base" style="color: var(--color-muted)">
			Dive deeper with multi-part guides and stories.
		</p>
	</header>

	{#if series.length === 0}
		<EmptyState
			icon={Layers}
			title="No series yet"
			message="Series group related blog posts into a structured collection."
		/>
	{:else}
		<div class="grid gap-4 sm:grid-cols-2">
			{#each series as s (s.id)}
				<a
					href="/blog/series/{s.slug}"
					class="group rounded-xl border p-6 transition-all hover:shadow-md"
					style="border-color: var(--color-border); background: var(--color-surface)"
				>
					<div class="flex items-center gap-2 text-xs font-medium" style="color: var(--color-primary)">
						<Layers class="h-3.5 w-3.5" />
						Series
					</div>
					<h2
						class="mt-3 text-lg font-semibold group-hover:underline"
						style="color: var(--color-foreground)"
					>
						{s.title}
					</h2>
					{#if s.description}
						<p class="mt-2 text-sm line-clamp-2" style="color: var(--color-muted)">
							{s.description}
						</p>
					{/if}
					<div class="mt-4 text-xs" style="color: var(--color-muted)">
						{formatDate(s.created_at)}
					</div>
				</a>
			{/each}
		</div>
	{/if}
</main>
