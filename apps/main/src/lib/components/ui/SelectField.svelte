<script lang="ts">
	// Reusable option-list select rendered through the shared Autocomplete
	// (searchable dropdown + selected chip), so every select-style field in
	// the app shares one interaction. Same value/options/onValueChange
	// contract as the previous Bits UI wrapper — call sites are unchanged.
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	interface Option {
		value: string;
		label: string;
		sublabel?: string;
	}

	interface Props {
		value: string;
		options: Option[];
		placeholder?: string;
		onValueChange: (v: string) => void;
		id?: string;
		disabled?: boolean;
		class?: string;
		name?: string;
		clearable?: boolean;
		ariaLabel?: string;
	}

	let {
		value = $bindable(''),
		options,
		placeholder = '',
		onValueChange,
		id,
		disabled = false,
		class: className = '',
		name,
		clearable = false,
		ariaLabel
	}: Props = $props();
</script>

<AutocompleteSelect
	bind:value
	{options}
	{placeholder}
	{onValueChange}
	{id}
	{disabled}
	{name}
	{clearable}
	{ariaLabel}
	class={className}
/>
