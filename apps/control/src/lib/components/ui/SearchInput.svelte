<script lang="ts">
	import { Search } from 'lucide-svelte';
	import type { Snippet } from 'svelte';

	let {
		value = $bindable(''),
		placeholder = 'Search...',
		class: className = '',
		children
	}: {
		value?: string;
		placeholder?: string;
		class?: string;
		children?: Snippet;
	} = $props();
</script>

<div class="relative {className}">
	<Search class="text-muted-foreground absolute left-3 top-1/2 size-4 -translate-y-1/2" />
	<input
		type="text"
		bind:value
		{placeholder}
		class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 h-input rounded-9px w-full border px-10 py-2.5 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
	/>
	{#if children}
		<div class="absolute right-2 top-1/2 -translate-y-1/2">
			{@render children()}
		</div>
	{/if}
</div>
