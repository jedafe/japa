<script lang="ts">
	import { Tooltip } from 'bits-ui';
	import type { Snippet } from 'svelte';

	interface Props {
		content?: string | Snippet;
		side?: 'top' | 'right' | 'bottom' | 'left';
		sideOffset?: number;
		delayDuration?: number;
		class?: string;
		arrow?: boolean;
		arrowClass?: string;
		children?: Snippet;
	}

	let {
		content,
		side = 'top',
		sideOffset = 8,
		delayDuration = 200,
		class: className = '',
		arrow = false,
		arrowClass = '',
		children
	}: Props = $props();
</script>

<Tooltip.Provider {delayDuration}>
	<Tooltip.Root>
		<Tooltip.Trigger>
			{@render children?.()}
		</Tooltip.Trigger>
		<Tooltip.Portal>
			<Tooltip.Content
				{side}
				{sideOffset}
				class="border-dark-10 bg-background shadow-popover rounded-input z-50 border px-3 py-1.5 text-xs font-medium {className}"
			>
				{#if typeof content === 'string'}
					{content}
				{:else}
					{@render content?.()}
				{/if}
				{#if arrow}
					<Tooltip.Arrow class="fill-[var(--background-alt,var(--background))] {arrowClass}" />
				{/if}
			</Tooltip.Content>
		</Tooltip.Portal>
	</Tooltip.Root>
</Tooltip.Provider>
