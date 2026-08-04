<script lang="ts">
	// Reusable Bits UI Select wrapper for simple value/label option lists.
	// Ref: ADR-058 — all interactive primitives must be Bits UI dot-notation.
	import { Select } from 'bits-ui';
	import { Check, ChevronDown } from 'lucide-svelte';

	interface Option {
		value: string;
		label: string;
	}

	interface Props {
		value: string;
		options: Option[];
		placeholder: string;
		onValueChange: (v: string) => void;
		id?: string;
		disabled?: boolean;
		class?: string;
	}

	let {
		value,
		options,
		placeholder,
		onValueChange,
		id,
		disabled = false,
		class: className = ''
	}: Props = $props();

	const selected = $derived(options.find((o) => o.value === value) ?? null);
</script>

<Select.Root
	type="single"
	{value}
	{disabled}
	onValueChange={(v) => onValueChange((v as string) ?? '')}
>
	<Select.Trigger
		{id}
		class="flex w-full items-center justify-between gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-left text-sm text-[var(--color-text)] transition-colors hover:border-[var(--color-text-subtle)] focus:border-[var(--color-primary)] focus:outline-none disabled:opacity-50 {className}"
	>
		<span class="overflow-hidden text-ellipsis whitespace-nowrap">
			{selected?.label ?? placeholder}
		</span>
		<ChevronDown size={15} class="shrink-0 text-[var(--color-muted)]" />
	</Select.Trigger>
	<Select.Portal>
		<Select.Content
			class="z-40 max-h-60 overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg"
		>
			{#each options as opt (opt.value)}
				<Select.Item
					value={opt.value}
					class="flex cursor-pointer items-center justify-between gap-2 rounded-[calc(var(--radius)-2px)] px-2.5 py-2 text-left text-sm text-[var(--color-text)] outline-none data-[highlighted]:bg-[var(--color-surface-subtle)] data-[selected]:font-medium data-[selected]:text-[var(--color-primary)]"
				>
					{#snippet child({ props })}
						<div {...props}>
							<span class="overflow-hidden text-ellipsis whitespace-nowrap">{opt.label}</span>
							{#if opt.value === value}
								<Check size={14} class="shrink-0 text-[var(--color-primary)]" />
							{/if}
						</div>
					{/snippet}
				</Select.Item>
			{/each}
		</Select.Content>
	</Select.Portal>
</Select.Root>
