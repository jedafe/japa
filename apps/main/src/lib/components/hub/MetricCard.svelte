<script lang="ts">
	import { t } from '$lib/i18n';

	interface Props {
		labelKey: string;
		value: number | string;
		delta?: number;
	}
	let { labelKey, value, delta }: Props = $props();

	const displayValue = $derived(typeof value === 'number' ? Math.round(value).toLocaleString() : value);
	const deltaSign = $derived(delta !== undefined && delta > 0 ? '+' : '');
</script>

<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
	<p class="text-xs text-[var(--color-muted)]">{t(labelKey)}</p>
	<p class="text-2xl font-medium text-[var(--color-text)]">{displayValue}</p>
	{#if delta !== undefined}
		<p class="mt-1 text-xs {delta >= 0 ? 'text-[var(--color-success)]' : 'text-[var(--color-danger)]'}">
			{deltaSign}{Math.round(delta)}
		</p>
	{/if}
</div>
