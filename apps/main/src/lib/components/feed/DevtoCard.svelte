<script lang="ts">
	import { Clock, MessageCircle, Bookmark } from 'lucide-svelte';
	import { imgproxy } from '$lib/utils/imgproxy';
	import { formatRelativeTime } from '$lib/utils/timezone';
	import type { FeedItem } from '$lib/api';

	let { item, index = 0 }: { item: FeedItem; index?: number } = $props();

	let coverUrl = $derived(
		item.cover_image_url ? imgproxy(item.cover_image_url, { width: 800, height: 400, format: 'webp', resize: 'fill' }) : ''
	);
</script>

<article
	class="group overflow-hidden rounded-2xl border transition-all hover:shadow-md"
	style="border-color: var(--color-border); background: var(--color-surface); animation: slide-up 0.3s ease-out {index * 50}ms both"
>
	{#if coverUrl}
		<a href="/blog/{item.slug ?? item.id}" class="block">
			<img
				src={coverUrl}
				alt={item.title}
				class="h-48 w-full object-cover transition-transform duration-300 group-hover:scale-105"
				loading="lazy"
			/>
		</a>
	{/if}

	<div class="p-5">
		<div class="flex items-center gap-3">
			<div
				class="flex h-8 w-8 items-center justify-center rounded-full text-xs font-semibold text-white"
				style="background: var(--gradient-primary)"
			>
				{item.author_name?.charAt(0)?.toUpperCase() ?? '?'}
			</div>
			<div class="min-w-0 flex-1">
				<a
					href="/profile/{item.author_id}"
					class="text-sm font-medium hover:underline"
					style="color: var(--color-foreground)"
				>
					{item.author_name}
				</a>
				<p class="text-xs" style="color: var(--color-muted)">
					{formatRelativeTime(item.created_at)}
					{#if item.reading_time_minutes}
						<span class="mx-1">&middot;</span>
						{item.reading_time_minutes} min read
					{/if}
				</p>
			</div>
		</div>

		<a href="/blog/{item.slug ?? item.id}" class="mt-3 block">
			<h2
				class="text-lg font-bold leading-snug transition-colors group-hover:opacity-80"
				style="color: var(--color-foreground)"
			>
				{item.title}
			</h2>
			{#if item.excerpt}
				<p class="mt-2 text-sm leading-relaxed line-clamp-2" style="color: var(--color-muted)">
					{item.excerpt}
				</p>
			{/if}
		</a>

		{#if item.tags && item.tags.length > 0}
			<div class="mt-3 flex flex-wrap gap-1.5">
				{#each item.tags.slice(0, 4) as tag}
					<span
						class="rounded-full px-2.5 py-0.5 text-xs font-medium"
						style="background: color-mix(in srgb, var(--color-primary) 8%, transparent); color: var(--color-primary)"
					>
						{tag}
					</span>
				{/each}
			</div>
		{/if}

		<div class="mt-4 flex items-center gap-4 text-xs" style="color: var(--color-muted)">
			<button class="flex items-center gap-1.5 transition-colors hover:text-blue-500">
				<MessageCircle class="h-3.5 w-3.5" />
				<span>{item.comment_count}</span>
			</button>
			{#if item.bookmark_count}
				<span class="flex items-center gap-1.5">
					<Bookmark class="h-3.5 w-3.5" />
					<span>{item.bookmark_count}</span>
				</span>
			{/if}
		</div>
	</div>
</article>
