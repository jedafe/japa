<script lang="ts">
	import BlockRenderer from './BlockRenderer.svelte';
	import type { LandingPage as LandingPageData } from './types';

	interface Props {
		page: LandingPageData;
	}
	let { page: page_data }: Props = $props();
</script>

<div class="landing-page min-h-screen bg-[var(--color-bg)]">
	{#each page_data.blocks as block (block.id ?? block.type + '-' + JSON.stringify(block.props))}
		<BlockRenderer {block} />
	{/each}
</div>
