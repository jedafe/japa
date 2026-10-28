<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import type { SelectOption } from '$lib/types/prompt';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	interface Props {
		label: string;
		value: string;
		options: SelectOption[];
		placeholder?: string;
		disabled?: boolean;
		onchange?: (value: string) => void;
		hint?: string;
	}

	let { label, value, options, placeholder, disabled = false, onchange, hint }: Props = $props();
</script>

<label class="block text-sm font-medium">
	{label}
	<AutocompleteSelect
		class="mt-1 w-full"
		{value}
		{disabled}
		options={placeholder
			? [{ value: '', label: placeholder }, ...options]
			: options}
		onValueChange={(v) => onchange?.(v)}
	/>
	{#if hint}<HintIcon text={hint} />{/if}
</label>
