<script lang="ts">
	import { t } from '$lib/i18n';
	import type { FreshnessCell, FreshnessSummary } from '$lib/types/jce';
	import Self from './FreshnessChip.svelte';

	interface Props {
		cell?: FreshnessCell;
		summary?: FreshnessSummary;
	}

	let { cell, summary }: Props = $props();

	let expanded = $state(false);

	function chipColor(score: number, isStale: boolean): string {
		if (isStale || score < 40) {
			return 'text-red-400 border-red-400/30 bg-red-400/10';
		}
		if (score < 75) {
			return 'text-amber-400 border-amber-400/30 bg-amber-400/10';
		}
		return 'text-emerald-400 border-emerald-400/30 bg-emerald-400/10';
	}

	function chipLabel(score: number, isStale: boolean): string {
		if (isStale) return t('jce.freshness.stale', 'Stale');
		if (score >= 75) return t('jce.freshness.fresh', 'Fresh');
		return t('jce.freshness.aging', 'Aging');
	}
</script>

{#if cell}
	{@const color = chipColor(cell.freshness_score, cell.is_stale)}
	{@const label = chipLabel(cell.freshness_score, cell.is_stale)}
	<span
		class="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs {color}"
		title={cell.source_name
			? t('jce.freshness.source', 'Source: {source}', { source: cell.source_name })
			: ''}
	>
		{label} {cell.freshness_score}
	</span>
{:else if summary}
	<div class="inline-flex flex-col gap-2">
		<button
			type="button"
			class="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs {chipColor(
				summary.overall_freshness_score,
				summary.stale_count > 0
			)}"
			onclick={() => (expanded = !expanded)}
		>
			{t('jce.freshness.overall', 'Freshness')} {summary.overall_freshness_score}
			{#if summary.stale_count > 0}
				<span class="text-[10px] opacity-80"
					>({summary.stale_count} {t('jce.freshness.stale', 'stale')})</span
				>
			{/if}
		</button>
		{#if expanded}
			<div
				class="min-w-[16rem] max-w-sm rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 text-xs"
			>
				<p class="mb-2 font-medium text-[var(--color-text)]">
					{t('jce.freshness.cells', 'Per-cell freshness')}
				</p>
				<div class="max-h-48 space-y-2 overflow-y-auto">
					{#each summary.by_item as item (item.item_id)}
						<div class="space-y-1">
							<div class="font-medium text-[var(--color-text)]">{item.item_title}</div>
							<div class="flex flex-wrap gap-1">
								{#each item.cells as c (c.dimension_id)}
									<Self cell={c} />
								{/each}
							</div>
						</div>
					{/each}
				</div>
			</div>
		{/if}
	</div>
{:else}
	<span class="text-xs text-[var(--color-muted)]">—</span>
{/if}
