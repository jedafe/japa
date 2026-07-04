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

<div class="card-interactive rounded-2xl border p-4" style="border-color: var(--color-border); background: var(--color-surface)">
	<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{label}</p>
	<p class="mt-2 text-2xl font-bold tracking-tight" style="color: var(--color-foreground)">{value}</p>
	{#if subtitle}
		<p class="mt-1 flex items-center gap-1 text-xs font-medium" style="color: var(--color-muted)">
			{#if trend === 'up'}
				<span class="text-emerald-500">&#8593;</span>
			{:else if trend === 'down'}
				<span class="text-red-500">&#8595;</span>
			{/if}
			{subtitle}
		</p>
	{/if}
</div>
