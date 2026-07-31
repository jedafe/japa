<script lang="ts">
	import BlockRenderer from '$lib/components/pagebuilder/BlockRenderer.svelte';
	import type { LandingBlock } from '$lib/components/pagebuilder/types';

	interface Props {
		blocks: unknown[];
	}

	let { blocks }: Props = $props();

	// Normalize sidebar blocks (which may use block_type or type) into the
	// LandingBlock shape expected by BlockRenderer.
	const normalized = $derived(
		(blocks ?? []).map((b, i) => {
			const block = b as Record<string, unknown>;
			return {
				id: (block.id as string) ?? `sidebar-block-${i}`,
				type: (block.block_type as string) ?? (block.type as string) ?? '',
				props: (block.props as Record<string, unknown>) ?? {}
			} as LandingBlock;
		})
	);
</script>

{#each normalized as block (block.id)}
	<div class="sidebar-block-wrapper">
		<BlockRenderer {block} />
	</div>
{/each}
