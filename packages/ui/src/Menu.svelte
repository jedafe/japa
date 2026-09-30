<script lang="ts">
	// Menu — shared wrapper around bits-ui DropdownMenu for standard
	// trigger+panel menus. The trigger is a snippet receiving the bits-ui
	// trigger props to spread onto your button; items go in the default
	// children snippet (use MenuItem/MenuSeparator for standard styling).
	import { DropdownMenu } from 'bits-ui';
	import type { Snippet } from 'svelte';

	interface Props {
		open?: boolean;
		onOpenChange?: (open: boolean) => void;
		align?: 'start' | 'center' | 'end';
		side?: 'top' | 'right' | 'bottom' | 'left';
		sideOffset?: number;
		/** z-index for the portaled panel — raise when the menu opens inside a Modal (e.g. 70). */
		zIndex?: number;
		/** Apply the default item padding (p-1) to the panel. Set false for
		 * edge-to-edge panels (headers, lists with dividers). */
		padded?: boolean;
		/** Extra classes merged onto DropdownMenu.Content. */
		contentClass?: string;
		/** Trigger content — receives the trigger props to spread on your element. */
		trigger?: Snippet<[Record<string, unknown>]>;
		children?: Snippet;
	}

	let {
		open = $bindable(false),
		onOpenChange,
		align = 'end',
		side = 'bottom',
		sideOffset = 8,
		zIndex = 50,
		padded = true,
		contentClass = '',
		trigger,
		children
	}: Props = $props();
</script>

<DropdownMenu.Root bind:open {onOpenChange}>
	{#if trigger}
		<DropdownMenu.Trigger>
			{#snippet child({ props })}
				{@render trigger(props)}
			{/snippet}
		</DropdownMenu.Trigger>
	{/if}
	<DropdownMenu.Portal>
		<DropdownMenu.Content
			{align}
			{side}
			{sideOffset}
			class="min-w-48 rounded-[var(--radius,var(--radius-9px,8px))] border border-[var(--color-border)] bg-[var(--color-surface,var(--background))] {padded ? 'p-1' : ''} shadow-lg {contentClass}"
			style="z-index: {zIndex}"
		>
			{@render children?.()}
		</DropdownMenu.Content>
	</DropdownMenu.Portal>
</DropdownMenu.Root>
