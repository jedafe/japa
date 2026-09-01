<script lang="ts">
	/**
	 * Reusable single-select built on Bits UI Select, styled for the JCE surface.
	 * Ref: japaconnect-engineering/conventions/svelte-component-patterns.md
	 */
	import { Select } from 'bits-ui';
	import { Check, ChevronDown } from 'lucide-svelte';

	export interface JceSelectOption {
		value: string;
		label: string;
		disabled?: boolean;
	}

	interface Props {
		options: JceSelectOption[];
		value?: string;
		placeholder?: string;
		disabled?: boolean;
		class?: string;
		id?: string;
		onchange?: (value: string) => void;
	}

	let {
		options,
		value = $bindable(''),
		placeholder = '',
		disabled = false,
		class: className = '',
		id,
		onchange
	}: Props = $props();

	let selectedLabel = $derived(
		options.find((o) => o.value === value)?.label ?? placeholder
	);

	function handleValueChange(newValue: string | undefined) {
		value = newValue ?? '';
		onchange?.(value);
	}
</script>

<Select.Root type="single" {value} {disabled} onValueChange={handleValueChange}>
	<Select.Trigger
		{id}
		class="flex h-10 w-full min-w-[8rem] items-center justify-between rounded-[var(--radius-button)] border bg-[var(--color-surface)] px-3 py-2 text-sm outline-none focus:border-[var(--color-text)] disabled:cursor-not-allowed disabled:opacity-50 {className}"
		style="border-color: var(--color-border); color: var(--color-text)"
	>
		<Select.Value>
			<span class:placeholder={!value}>{selectedLabel}</span>
		</Select.Value>
		<ChevronDown size={16} class="shrink-0 opacity-50" />
	</Select.Trigger>

	<Select.Portal>
		<Select.Content
			class="relative z-50 max-h-96 min-w-[8rem] overflow-hidden rounded-[var(--radius-button)] border shadow-md"
			style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-text)"
			sideOffset={4}
		>
			<Select.Viewport class="p-1">
				{#each options as option (option.value)}
					<Select.Item
						value={option.value}
						label={option.label}
						disabled={option.disabled}
						class="relative flex w-full cursor-default select-none items-center rounded-[var(--radius-button)] py-1.5 pl-8 pr-2 text-sm outline-none hover:bg-[var(--color-bg)] focus:bg-[var(--color-bg)] data-[disabled]:pointer-events-none data-[disabled]:opacity-50"
					>
						{#snippet children({ selected })}
							<span class="absolute left-2 flex h-3.5 w-3.5 items-center justify-center">
								{#if selected}
									<Check size={14} style="color: var(--color-text)" />
								{/if}
							</span>
							<span class="truncate">{option.label}</span>
						{/snippet}
					</Select.Item>
				{/each}
			</Select.Viewport>
		</Select.Content>
	</Select.Portal>
</Select.Root>

<style>
	.placeholder {
		color: var(--color-muted);
	}
</style>
