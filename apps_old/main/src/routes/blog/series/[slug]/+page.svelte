<script lang="ts">
	import { ArrowLeft, BookOpen, Clock, Layers } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { formatTimestamp } from '$lib/utils/timezone';

	let { data } = $props();

	let series = $derived(data.series);
	let posts = $derived(data.posts ?? []);

	function formatDate(dateStr: string): string {
		if (!dateStr) return '';
		const d = new Date(dateStr);
		return formatTimestamp(d);
	}
</script>

<svelte:head>
	<title>{series.title} | JapaConnect Blog</title>
	<meta name="description" content={series.description ?? series.title} />
</svelte:head>

<main class="mx-auto max-w-3xl px-4 py-8">
	<a
		href="/blog/series"
		class="mb-6 inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to series
	</a>

	<!-- Series Header -->
	<header class="mb-8 rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface)">
		<div class="flex items-center gap-2 text-xs font-medium" style="color: var(--color-primary)">
			<Layers class="h-3.5 w-3.5" />
			Series
		</div>
		<h1 class="mt-3 text-2xl font-bold tracking-tight sm:text-3xl" style="color: var(--color-foreground)">
			{series.title}
		</h1>
		{#if series.description}
			<p class="mt-3 text-sm leading-relaxed" style="color: var(--color-muted)">
				{series.description}
			</p>
		{/if}
		<div class="mt-4 text-xs" style="color: var(--color-muted)">
			{formatDate(series.created_at)}
		</div>
	</header>

	<!-- Posts in Series -->
	{#if posts.length === 0}
		<EmptyState
			icon={BookOpen}
			title="No posts in this series yet"
			message="Posts will appear here once they are added to the series."
		/>
	{:else}
		<ol class="space-y-3">
			{#each posts as post, i (post.content_id)}
				<li>
					<a
						href="/blog/{post.slug}"
						class="group flex items-start gap-4 rounded-xl border p-5 transition-all hover:shadow-md"
						style="border-color: var(--color-border); background: var(--color-surface)"
					>
						<span
							class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-sm font-bold"
							style="background-color: var(--color-primary); color: white"
						>
							{i + 1}
						</span>
						<div class="flex-1">
							<h3
								class="text-base font-semibold group-hover:underline"
								style="color: var(--color-foreground)"
							>
								{post.title}
							</h3>
							<div class="mt-1.5 flex items-center gap-3 text-xs" style="color: var(--color-muted)">
								{#if post.created_at}
									<span>{formatDate(post.created_at)}</span>
								{/if}
								<span
									class="rounded-full px-2 py-0.5"
									style="background-color: var(--color-background)"
								>
									{post.status}
								</span>
							</div>
						</div>
					</a>
				</li>
			{/each}
		</ol>
	{/if}
</main>
