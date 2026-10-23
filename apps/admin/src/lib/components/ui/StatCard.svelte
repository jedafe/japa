<script lang="ts">
	import clsx from 'clsx';

	let { label, value, delta, class: className = '' }: { label: string; value: string | number; delta?: number; class?: string } = $props();
</script>

<div
	class={clsx('rounded-lg border p-4', className)}
	style="border-color: var(--color-border); background-color: var(--color-surface)"
>
	<div class="text-sm font-medium" style="color: var(--color-muted)">{label}</div>
	<div class="mt-2">
		<span class="text-2xl font-bold" style="color: var(--color-foreground)">{value}</span>
		{#if delta !== undefined}
			<span
				class="ms-2 text-sm font-medium"
				style="color: {delta >= 0 ? 'var(--color-success)' : 'var(--color-danger)'}"
			>
				{delta >= 0 ? '+' : ''}{delta}%
			</span>
		{/if}
	</div>
</div>
