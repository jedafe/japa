<script lang="ts">
	/**
	 * JCE-styled single select rendered through the shared Autocomplete
	 * (searchable dropdown + selected chip). Same options/value/onchange
	 * contract as the previous Bits UI implementation — call sites are
	 * unchanged. Per-option `disabled` flags are not supported by the shared
	 * component; no call site uses them.
	 */
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

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

	const enabledOptions = $derived(options.filter((o) => !o.disabled));
</script>

<AutocompleteSelect
	bind:value
	options={enabledOptions}
	{placeholder}
	{disabled}
	{id}
	class={className}
	onValueChange={(v) => onchange?.(v)}
/>
