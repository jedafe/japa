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

<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
	<div class="flex items-center justify-between">
		<p class="text-sm font-medium" style="color: var(--color-muted)">{label}</p>
		{#if icon}
			<span class="text-lg">{icon}</span>
		{/if}
	</div>
	<p class="mt-1 text-2xl font-bold tracking-tight">{value}</p>
	{#if subtitle}
		<p class="mt-1 text-xs" style="color: var(--color-muted)">{subtitle}</p>
	{/if}
</div>
