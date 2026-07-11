<script lang="ts">
	let {
		value = $bindable(''),
		placeholder = 'Select...',
		disabled = false,
		class: className = '',
		id = '',
		options = [],
		...restProps
	}: {
		value?: string;
		placeholder?: string;
		disabled?: boolean;
		class?: string;
		id?: string;
		options?: { value: string; label: string }[];
		[key: string]: any;
	} = $props();
</script>

<select
	bind:value
	{disabled}
	{id}
	class="flex h-10 w-full items-center rounded-lg border px-3 py-2.5 text-sm outline-none transition-colors focus:border-[var(--color-primary)] {className}"
	style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
	{...restProps}
>
	{#if placeholder}
		<option value="" disabled>{placeholder}</option>
	{/if}
	{#each options as option}
		<option value={option.value}>{option.label}</option>
	{/each}
</select>
