<script lang="ts">
	let {
		type = 'text',
		placeholder = '',
		value = $bindable(''),
		disabled = false,
		required = false,
		class: className = '',
		id = '',
		...restProps
	}: {
		type?: string;
		placeholder?: string;
		value?: string;
		disabled?: boolean;
		required?: boolean;
		class?: string;
		id?: string;
		[key: string]: any;
	} = $props();
</script>

<input
	{type}
	{placeholder}
	bind:value
	{disabled}
	{required}
	{id}
	class="w-full rounded-lg border px-3 py-2.5 text-sm outline-none transition-colors focus:border-[var(--color-primary)] {className}"
	style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
	{...restProps}
/>
