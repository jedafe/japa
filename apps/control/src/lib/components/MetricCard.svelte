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

<div class="border-border-card bg-background-alt shadow-card rounded-card p-4">
	<p class="text-muted-foreground text-xs font-medium uppercase tracking-wider">{label}</p>
	<p class="text-foreground mt-2 text-2xl font-bold tracking-tight">{value}</p>
	{#if subtitle}
		<p class="text-muted-foreground mt-1 flex items-center gap-1 text-xs font-medium">
			{#if trend === 'up'}
				<span class="text-green-500">&#8593;</span>
			{:else if trend === 'down'}
				<span class="text-destructive">&#8595;</span>
			{/if}
			{subtitle}
		</p>
	{/if}
</div>
