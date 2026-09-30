<script lang="ts" module>
	export interface SelectOption {
		value: string;
		label: string;
		disabled?: boolean;
	}
</script>

<script lang="ts">
	import { Select } from 'bits-ui';
	import { ChevronDown } from 'lucide-svelte';

	interface Props {
		value?: string;
		items: SelectOption[];
		onValueChange?: (value: string) => void;
		placeholder?: string;
		ariaLabel?: string;
		disabled?: boolean;
		id?: string;
		/** Small variant — compact trigger and items (e.g. table footers). */
		compact?: boolean;
		/** z-index for the popover content — raise it when used inside a Modal. */
		zIndex?: number;
		triggerClass?: string;
		contentClass?: string;
		viewportClass?: string;
		itemClass?: string;
	}

	let {
		value = $bindable(''),
		items,
		onValueChange,
		placeholder = '',
		ariaLabel,
		disabled = false,
		id,
		compact = false,
		zIndex,
		triggerClass = '',
		contentClass = '',
		viewportClass = '',
		itemClass = ''
	}: Props = $props();
</script>

<Select.Root type="single" bind:value {items} {disabled} onValueChange={(v) => onValueChange?.(v)}>
	<Select.Trigger
		{id}
		aria-label={ariaLabel}
		class="border-border-input bg-background data-placeholder:text-muted-foreground/50 rounded-9px hover:border-border-input-hover flex items-center border transition-colors {compact
			? 'h-7 px-2 text-xs'
			: 'h-9 px-3 text-sm'} {triggerClass}"
	>
		<Select.Value {placeholder} />
		<ChevronDown class="text-muted-foreground ml-auto {compact ? 'size-3' : 'size-3.5'} shrink-0" />
	</Select.Trigger>
	<Select.Portal>
		<Select.Content
			class="border-border-input bg-background shadow-popover z-50 rounded-xl border px-1 py-1 {contentClass}"
			style={zIndex !== undefined ? `z-index: ${zIndex}` : undefined}
			sideOffset={4}
		>
			<Select.Viewport class="p-1 {viewportClass}">
				{#each items as item (item.value)}
					<Select.Item
						class="data-highlighted:bg-accent flex cursor-pointer items-center px-3 text-sm outline-none {compact
							? 'rounded-lg py-1.5'
							: 'rounded-9px h-9'} {itemClass}"
						value={item.value}
						label={item.label}
						disabled={item.disabled}
					>
						{#snippet children({ selected })}
							{item.label}
							{#if selected}<span class="ml-auto">✓</span>{/if}
						{/snippet}
					</Select.Item>
				{/each}
			</Select.Viewport>
		</Select.Content>
	</Select.Portal>
</Select.Root>
