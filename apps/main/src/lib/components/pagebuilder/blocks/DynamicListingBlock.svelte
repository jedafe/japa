<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import FeedCard from '$lib/components/feed/FeedCard.svelte';
	import { cardStyleFor, type CardStyleConfig } from '$lib/feed/cardStyle';
	import { page } from '$app/state';
	import type { DynamicListingProps } from '../types';
	import type { FeedItem } from '$lib/types';

	interface Props {
		props: DynamicListingProps;
	}
	let { props }: Props = $props();

	let items = $state<FeedItem[]>([]);
	let loading = $state(true);

	const config = $derived((page.data.cardStyleConfig as CardStyleConfig) ?? { default: 'reddit', byType: {} });

	onMount(async () => {
		const limit = props.limit ?? 6;
		const contentType = props.content_type ?? '';
		const path = contentType
			? `/explore?sort=relevant&page_size=${limit}&page_offset=0&content_type=${contentType}`
			: `/explore?sort=relevant&page_size=${limit}&page_offset=0`;
		const res = await api.get<FeedItem[]>(path);
		items = res.data ?? [];
		loading = false;
	});
</script>

<section class="mx-auto max-w-4xl px-6 py-10">
	{#if loading}
		<div class="grid gap-3 sm:grid-cols-2">
			{#each Array(4) as _, i (i)}
				<div class="h-40 animate-pulse rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)]"></div>
			{/each}
		</div>
	{:else if items.length > 0}
		<div class="grid gap-3.5 sm:grid-cols-2">
			{#each items as item (item.id)}
				<FeedCard {item} style={cardStyleFor(item.content_type, config)} layout="grid-cell" />
			{/each}
		</div>
	{/if}
</section>
