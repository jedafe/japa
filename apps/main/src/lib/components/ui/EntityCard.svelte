<script lang="ts">
	import RelativeTime from './RelativeTime.svelte';

	interface Props {
		title: string;
		excerpt?: string;
		href: string;
		type?: string;
		date?: string;
		thumbnail?: string;
	}

	let { title, excerpt, href, type, date, thumbnail }: Props = $props();
</script>

<a
	{href}
	class="block rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]"
>
	{#if thumbnail}
		<img
			src={thumbnail}
			alt=""
			class="mb-3 h-32 w-full rounded-[var(--radius)] object-cover"
			loading="lazy"
		/>
	{/if}

	<div class="flex items-center gap-2">
		{#if type}
			<span
				class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs font-medium text-[var(--color-text-subtle)]"
			>
				{type}
			</span>
		{/if}
		{#if date}
			<RelativeTime iso={date} />
		{/if}
	</div>

	<h3 class="mt-2 line-clamp-2 text-base font-medium text-[var(--color-text)]">{title}</h3>

	{#if excerpt}
		<p class="mt-1 line-clamp-2 text-sm text-[var(--color-text-subtle)]">{excerpt}</p>
	{/if}
</a>
