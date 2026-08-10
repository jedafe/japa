<script lang="ts">
	import { Select } from "bits-ui";
	import { Check, ChevronDown, X } from "lucide-svelte";
	import { clsx, type ClassValue } from "clsx";

	let {
		options,
		value = $bindable(),
		placeholder = "Select...",
		disabled = false,
		class: className,
		id,
		onchange
	}: {
		options: Array<{ value: string; label: string }>;
		value?: string[];
		placeholder?: string;
		disabled?: boolean;
		class?: ClassValue;
		id?: string;
		onchange?: (val: string[]) => void;
	} = $props();

	let searchQuery = $state("");

	const filteredOptions = $derived(
		searchQuery
			? options.filter(o => o.label.toLowerCase().includes(searchQuery.toLowerCase()))
			: options
	);

	function handleRemove(e: MouseEvent, val: string) {
		e.stopPropagation();
		value = (value ?? []).filter(v => v !== val);
		onchange?.(value ?? []);
	}
</script>

<div class={clsx("relative", className)}>
	<Select.Root type="multiple" bind:value {disabled} items={options}>
		<Select.Trigger
			{id}
			class={clsx(
				"flex min-h-10 w-full items-center gap-2 rounded-lg border px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-[var(--color-primary)] focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50",
			)}
			style="border-color: var(--color-border-input); background-color: var(--color-surface); color: var(--color-foreground)"
		>
			<div class="flex flex-wrap gap-1 flex-1 min-w-0 items-center">
				{#if value && value.length > 0}
					{#each value as val (val)}
						{@const label = options.find(o => o.value === val)?.label ?? val}
						<span class="inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs font-medium" style="background: var(--color-primary); color: var(--color-primary-foreground)">
							{label}
							<button
								type="button"
								class="inline-flex items-center rounded-full hover:bg-black/10"
								onclick={(e) => handleRemove(e, val)}
							>
								<X class="h-3 w-3" />
							</button>
						</span>
					{/each}
				{/if}
				<input
					type="text"
					bind:value={searchQuery}
					placeholder={(value && value.length > 0) || searchQuery ? "Search..." : ""}
					class="flex-1 min-w-0 bg-transparent border-none outline-none text-sm placeholder:text-[var(--color-muted)]"
					onmousedown={(e) => e.stopPropagation()}
					onclick={(e) => e.stopPropagation()}
				/>
				{#if (!value || value.length === 0) && !searchQuery}
					<span class="text-[var(--color-muted)] pointer-events-none">{placeholder}</span>
				{/if}
			</div>
			<ChevronDown class="h-4 w-4 shrink-0 opacity-50" />
		</Select.Trigger>

		<Select.Portal>
			<Select.Content
				class="relative z-50 max-h-96 min-w-[8rem] overflow-hidden rounded-lg border shadow-md data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
				sideOffset={4}
			>
				<Select.Viewport class="p-1">
					{#each filteredOptions as option (option)}
						<Select.Item
							value={option.value}
							label={option.label}
							class="relative flex w-full cursor-default select-none items-center rounded-md py-1.5 pl-8 pr-2 text-sm outline-none focus:bg-[var(--color-accent)] focus:text-[var(--color-accent-foreground)]"
						>
							{#snippet children({ selected })}
								<span class="absolute left-2 flex h-3.5 w-3.5 items-center justify-center">
									{#if selected}
										<Check class="h-4 w-4" />
									{/if}
								</span>
								{option.label}
							{/snippet}
						</Select.Item>
					{/each}
				</Select.Viewport>
			</Select.Content>
		</Select.Portal>
	</Select.Root>
</div>
