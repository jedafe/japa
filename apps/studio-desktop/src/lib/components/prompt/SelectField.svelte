<script lang="ts">
	import type { SelectOption } from '$lib/types/prompt';

	interface Props {
		label: string;
		value: string;
		options: SelectOption[];
		placeholder?: string;
		disabled?: boolean;
		onchange?: (value: string) => void;
	}

	let { label, value, options, placeholder, disabled = false, onchange }: Props = $props();

	function handleChange(event: Event) {
		const target = event.currentTarget as HTMLSelectElement;
		onchange?.(target.value);
	}
</script>

<label class="block text-sm font-medium">
	{label}
	<select
		class="mt-1 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary disabled:opacity-60"
		{value}
		{disabled}
		onchange={handleChange}
	>
		{#if placeholder}
			<option value="">{placeholder}</option>
		{/if}
		{#each options as option (option.value)}
			<option value={option.value}>{option.label}</option>
		{/each}
	</select>
</label>
