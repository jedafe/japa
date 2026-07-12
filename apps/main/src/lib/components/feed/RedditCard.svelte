<script lang="ts">
	import { Clock, MessageCircle, Heart } from 'lucide-svelte';
	import { formatRelativeTime } from '$lib/utils/timezone';
	import type { FeedItem } from '$lib/api';

	let { item, index = 0 }: { item: FeedItem; index?: number } = $props();
</script>

<article
	class="card-interactive group rounded-2xl border p-5"
	style="border-color: var(--color-border); background: var(--color-surface); animation: slide-up 0.3s ease-out {index * 50}ms both"
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
			<span class="text-xs font-medium">{item.vote_score}</span>
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
