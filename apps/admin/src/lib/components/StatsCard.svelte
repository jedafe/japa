<script lang="ts">
	let {
		label,
		value,
		subtitle,
		icon
	}: {
		label: string;
		value: string | number;
		subtitle?: string;
		icon?: string;
	} = $props();
</script>

<div class="card-interactive group rounded-2xl border p-5" style="border-color: var(--color-border); background: var(--color-surface)">
	<div class="flex items-center justify-between">
		<p class="text-sm font-medium" style="color: var(--color-muted)">{label}</p>
		{#if icon}
			<span class="text-xl transition-transform group-hover:scale-110">{icon}</span>
		{/if}
	</div>
	<p class="mt-2 text-3xl font-bold tracking-tight" style="color: var(--color-foreground)">{value}</p>
	{#if subtitle}
		<p class="mt-1.5 text-xs font-medium" style="color: var(--color-muted)">{subtitle}</p>
	{/if}
</div>
