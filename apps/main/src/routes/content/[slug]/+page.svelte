<script lang="ts">
	import { onMount } from 'svelte';
	import { ArrowLeft, MessageSquare, Eye, Clock, Tag } from 'lucide-svelte';
	import { api, type FeedItem } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	let { data } = $props();

	let content = $derived(data.content);
	let related = $state<FeedItem[]>([]);
	let relatedLoading = $state(false);

	onMount(async () => {
		if (content?.id) {
			relatedLoading = true;
			try {
				related = await api.getRelatedContent(content.id);
			} catch {
				// Related content is non-critical
			} finally {
				relatedLoading = false;
			}
		}
	});

	function formatDate(dateStr: string): string {
		return new Date(dateStr).toLocaleDateString(undefined, {
			year: 'numeric',
			month: 'short',
			day: 'numeric'
		});
	}
</script>

<svelte:head>
	<title>{content?.title ?? 'Content'} — JapaConnect</title>
	{#if content?.excerpt}
		<meta name="description" content={content.excerpt} />
	{/if}
	{#if content?.cover_image_url}
		<meta property="og:image" content={content.cover_image_url} />
	{/if}
</svelte:head>

<main class="mx-auto max-w-3xl px-4 py-8">
	<a
		href="/"
		class="mb-6 inline-flex items-center gap-1 text-sm transition-colors hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		{t('common.back')}
	</a>

	{#if content}
		<article>
			{#if content.cover_image_url}
				<img
					src={content.cover_image_url}
					alt={content.title}
					class="mb-6 w-full rounded-xl object-cover"
					style="max-height: 400px"
				/>
			{/if}

			<h1 class="text-2xl font-bold leading-snug" style="color: var(--color-foreground)">
				{content.title}
			</h1>

			<div class="mt-3 flex flex-wrap items-center gap-3 text-xs" style="color: var(--color-muted)">
				<span class="font-medium" style="color: var(--color-foreground)">{content.author_name}</span>
				<span>&middot;</span>
				<span>{formatDate(content.created_at)}</span>
				{#if content.reading_time_minutes}
					<span>&middot;</span>
					<span class="inline-flex items-center gap-1">
						<Clock class="h-3 w-3" />
						{content.reading_time_minutes} min
					</span>
				{/if}
				{#if content.space_name}
					<span>&middot;</span>
					<span>{content.space_name}</span>
				{/if}
			</div>

			{#if content.tags && content.tags.length > 0}
				<div class="mt-3 flex flex-wrap gap-1.5">
					{#each content.tags as tag}
						<span
							class="inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs"
							style="background-color: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)"
						>
							<Tag class="h-3 w-3" />
							{tag}
						</span>
					{/each}
				</div>
			{/if}

			<div class="mt-6 flex items-center gap-4 text-xs" style="color: var(--color-muted)">
				{#if content.view_count !== undefined}
					<span class="inline-flex items-center gap-1">
						<Eye class="h-3.5 w-3.5" />
						{content.view_count}
					</span>
				{/if}
				{#if content.comment_count !== undefined}
					<span class="inline-flex items-center gap-1">
						<MessageSquare class="h-3.5 w-3.5" />
						{content.comment_count}
					</span>
				{/if}
			</div>

			{#if content.excerpt}
				<p class="mt-6 text-base leading-relaxed" style="color: var(--color-foreground)">
					{content.excerpt}
				</p>
			{/if}
		</article>

		{#if related.length > 0}
			<section class="mt-12">
				<h2 class="mb-4 text-lg font-semibold" style="color: var(--color-foreground)">
					{t('content.related')}
				</h2>
				<div class="space-y-3">
					{#each related as item (item.id)}
						<a
							href="/content/{item.slug ?? item.id}"
							class="block rounded-lg border p-4 transition-colors hover:bg-black/5"
							style="border-color: var(--color-border)"
						>
							<p class="font-medium" style="color: var(--color-foreground)">{item.title}</p>
							{#if item.excerpt}
								<p class="mt-1 text-xs line-clamp-2" style="color: var(--color-muted)">{item.excerpt}</p>
							{/if}
						</a>
					{/each}
				</div>
			</section>
		{/if}
	{/if}
</main>
