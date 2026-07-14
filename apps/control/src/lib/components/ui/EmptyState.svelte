<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { Component } from 'svelte';

	let {
		icon,
		message,
		description,
		children
	}: {
		icon: Component;
		message: string;
		description?: string;
		children?: Snippet;
	} = $props();
</script>

<div class="border-border-card bg-background-alt flex flex-col items-center justify-center rounded-xl border p-12 text-center">
	<icon class="text-muted-foreground size-10 opacity-30"></icon>
	<p class="text-foreground-alt mt-4 text-sm font-medium">{message}</p>
	{#if description}
		<p class="text-muted-foreground mt-1 text-xs">{description}</p>
	{/if}
	{#if children}
		<div class="mt-4">
			{@render children()}
		</div>
	{/if}
</div>
