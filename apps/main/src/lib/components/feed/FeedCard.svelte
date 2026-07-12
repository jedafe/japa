<script lang="ts">
	import type { FeedItem } from '$lib/api';
	import RedditCard from './RedditCard.svelte';
	import DevtoCard from './DevtoCard.svelte';

	let { item, style = 'reddit', index = 0 }: { item: FeedItem; style?: string; index?: number } = $props();
</script>

{#if style === 'devto'}
	<DevtoCard {item} {index} />
{:else}
	<RedditCard {item} {index} />
{/if}
