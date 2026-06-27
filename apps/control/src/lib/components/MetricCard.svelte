<script lang="ts">
	let {
		label,
		value,
		subtitle,
		trend
	}: {
		label: string;
		value: string | number;
		subtitle?: string;
		trend?: 'up' | 'down' | 'neutral';
	} = $props();
</script>

<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
	<p class="text-sm font-medium" style="color: var(--color-muted)">{label}</p>
	<p class="mt-1 text-2xl font-bold tracking-tight">{value}</p>
	{#if subtitle}
		<p class="mt-1 text-xs" style="color: var(--color-muted)">
			{#if trend === 'up'}
				<span class="text-emerald-600 dark:text-emerald-400">&#8593;</span>
			{:else if trend === 'down'}
				<span class="text-red-600 dark:text-red-400">&#8595;</span>
			{/if}
			{subtitle}
		</p>
	{/if}
</div>
