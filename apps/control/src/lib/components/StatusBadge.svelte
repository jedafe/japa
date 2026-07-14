<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		status,
		children
	}: {
		status: 'healthy' | 'warning' | 'critical' | 'unknown';
		children?: Snippet;
	} = $props();

	const colors = {
		healthy: 'bg-green-500/15 text-green-600 dark:text-green-400 border-green-500/20',
		warning: 'bg-amber-500/15 text-amber-600 dark:text-amber-400 border-amber-500/20',
		critical: 'bg-destructive/15 text-destructive border-destructive/20',
		unknown: 'bg-muted text-muted-foreground border-border'
	};

	const dots = {
		healthy: 'bg-green-500',
		warning: 'bg-amber-500',
		critical: 'bg-destructive',
		unknown: 'bg-muted-foreground'
	};
</script>

<span class="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs font-medium {colors[status]}">
	<span class="h-1.5 w-1.5 rounded-full {dots[status]}" class:animate-pulse={status === 'healthy'}></span>
	{#if children}
		{@render children()}
	{:else}
		{status}
	{/if}
</span>
