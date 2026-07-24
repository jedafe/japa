<script lang="ts">
	import { X } from 'lucide-svelte';

	interface TagOption {
		value: string;
		label: string;
	}

	let {
		options,
		value = $bindable(),
		placeholder = 'Select...',
		disabled = false,
		id
	}: {
		options: TagOption[];
		value?: string[];
		placeholder?: string;
		disabled?: boolean;
		id?: string;
	} = $props();

	let inputValue = $state('');
	let highlightedIndex = $state(-1);
	let inputEl: HTMLInputElement | null = $state(null);

	const selected = $derived(value ?? []);

	const filteredOptions = $derived(() => {
		if (!inputValue) return [];
		const lower = inputValue.toLowerCase();
		return options.filter(
			(opt) =>
				!selected.includes(opt.value) && opt.label.toLowerCase().includes(lower)
		);
	});

	function addTag(option: TagOption) {
		if (!selected.includes(option.value)) {
			value = [...selected, option.value];
		}
		inputValue = '';
		highlightedIndex = -1;
	}

	function removeTag(val: string) {
		value = selected.filter((v) => v !== val);
	}

	function handleKeyDown(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			e.preventDefault();
			const optionsList = filteredOptions();
			if (highlightedIndex >= 0 && highlightedIndex < optionsList.length) {
				addTag(optionsList[highlightedIndex]);
			} else if (optionsList.length > 0) {
				addTag(optionsList[0]);
			}
		} else if (e.key === 'Backspace') {
			if (inputValue === '' && selected.length > 0) {
				e.preventDefault();
				const last = selected[selected.length - 1];
				removeTag(last);
			}
		} else if (e.key === 'ArrowDown') {
			e.preventDefault();
			const optionsList = filteredOptions();
			highlightedIndex = Math.min(highlightedIndex + 1, optionsList.length - 1);
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			highlightedIndex = Math.max(highlightedIndex - 1, -1);
		} else if (e.key === 'Escape') {
			highlightedIndex = -1;
		}
	}

	function getLabel(val: string): string {
		return options.find((o) => o.value === val)?.label ?? val;
	}
</script>

<div class="relative">
	<div class="flex flex-wrap items-center gap-1.5 rounded-lg border px-2 py-1.5 min-h-10"
		style="border-color: var(--color-border-input); background-color: var(--color-surface); color: var(--color-foreground)">
		{#each selected as val}
			{@const label = getLabel(val)}
			<span class="inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs font-medium"
				style="background: var(--color-primary); color: var(--color-primary-foreground)">
				{label}
				<button
					type="button"
					class="inline-flex items-center rounded-full hover:bg-black/10"
					onclick={() => removeTag(val)}
					aria-label="Remove {label}"
				>
					<X class="h-3 w-3" />
				</button>
			</span>
		{/each}
		<input
			{id}
			type="text"
			bind:this={inputEl}
			bind:value={inputValue}
			placeholder={selected.length === 0 && !inputValue ? placeholder : ''}
			class="flex-1 min-w-[120px] bg-transparent border-none outline-none text-sm placeholder:text-[var(--color-muted)] disabled:cursor-not-allowed"
			onkeydown={handleKeyDown}
			{disabled}
		/>
	</div>

	{#if filteredOptions().length > 0}
		<div class="absolute top-full left-0 right-0 z-50 mt-1 max-h-60 overflow-auto rounded-lg border shadow-md"
			style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)">
			{#each filteredOptions() as option, i}
				<button
					type="button"
					class="w-full cursor-pointer select-none rounded-md px-3 py-2 text-left text-sm transition-colors hover:bg-[var(--color-accent)] hover:text-[var(--color-accent-foreground)] outline-none first:rounded-t-md last:rounded-b-md"
					style={i === highlightedIndex ? 'background-color: var(--color-accent); color: var(--color-accent-foreground)' : ''}
					onmousedown={() => addTag(option)}
					onmouseenter={() => highlightedIndex = i}
				>
					{option.label}
				</button>
			{/each}
		</div>
	{/if}
</div>