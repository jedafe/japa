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
	let focused = $state(false);
	let inputEl: HTMLInputElement | null = $state(null);
	let containerEl: HTMLDivElement | null = $state(null);
	let dropdownStyle = $state('');

	const selected = $derived(value ?? []);

	const filteredOptions = $derived.by(() => {
		const lower = inputValue.toLowerCase();
		return options.filter(
			(opt) =>
				!selected.includes(opt.value) &&
				(!inputValue || opt.label.toLowerCase().includes(lower))
		);
	});

	$effect(() => {
		if (focused && containerEl) {
			const rect = containerEl.getBoundingClientRect();
			const spaceBelow = window.innerHeight - rect.bottom;
			const dropdownHeight = Math.min(filteredOptions.length * 40 + 8, 240);
			const openBelow = spaceBelow > dropdownHeight || spaceBelow > rect.top;
			const top = openBelow ? rect.bottom + 4 : rect.top - dropdownHeight - 4;
			dropdownStyle = `position:fixed; top:${top}px; left:${rect.left}px; width:${rect.width}px; z-index:99999;`;
		}
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
			const optionsList = filteredOptions;
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
			const optionsList = filteredOptions;
			highlightedIndex = Math.min(highlightedIndex + 1, optionsList.length - 1);
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			highlightedIndex = Math.max(highlightedIndex - 1, -1);
		} else if (e.key === 'Escape') {
			highlightedIndex = -1;
			focused = false;
		}
	}

	function getLabel(val: string): string {
		return options.find((o) => o.value === val)?.label ?? val;
	}
</script>

<div class="relative" bind:this={containerEl} onfocusout={(e) => {
	if (!e.currentTarget.contains(e.relatedTarget as Node)) {
		focused = false;
	}
}}>
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
			onfocus={() => (focused = true)}
			{disabled}
		/>
	</div>

	{#if focused && filteredOptions.length > 0}
		<div
			class="max-h-60 overflow-auto rounded-lg border shadow-md"
			style="{dropdownStyle} border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground);"
			onmousedown={(e) => e.preventDefault()}
		>
			{#each filteredOptions as option, i}
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
