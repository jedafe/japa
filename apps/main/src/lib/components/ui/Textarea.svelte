<script lang="ts">
	let {
		placeholder = '',
		value = $bindable(''),
		disabled = false,
		required = false,
		class: className = '',
		id = '',
		rows = 4,
		...restProps
	}: {
		placeholder?: string;
		value?: string;
		disabled?: boolean;
		required?: boolean;
		class?: string;
		id?: string;
		rows?: number;
		[key: string]: any;
	} = $props();
</script>

<textarea
	{placeholder}
	bind:value
	{disabled}
	{required}
	{id}
	{rows}
	class="w-full rounded-lg border px-3 py-2.5 text-sm outline-none transition-colors focus:border-[var(--color-primary)] {className}"
	style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
	{...restProps}
></textarea>
