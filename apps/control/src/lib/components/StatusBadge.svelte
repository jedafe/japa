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
		healthy: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400 border-emerald-500/20',
		warning: 'bg-amber-500/15 text-amber-700 dark:text-amber-400 border-amber-500/20',
		critical: 'bg-red-500/15 text-red-700 dark:text-red-400 border-red-500/20',
		unknown: 'bg-slate-500/15 text-slate-700 dark:text-slate-400 border-slate-500/20'
	};

	const dots = {
		healthy: 'bg-emerald-500',
		warning: 'bg-amber-500',
		critical: 'bg-red-500',
		unknown: 'bg-slate-500'
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
