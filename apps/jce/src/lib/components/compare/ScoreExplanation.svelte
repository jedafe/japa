<script lang="ts">
	import { t } from '$lib/i18n';
	import { AlertTriangle } from 'lucide-svelte';
	import type { JapaScore } from '$lib/types/jce';

	interface Props {
		score: JapaScore;
	}

	let { score }: Props = $props();
	let expanded = $state(false);

	let ariaLabel = $derived(
		expanded
			? `${t('jce.score.title')} - ${t('jce.common.collapse')}`
			: `${t('jce.score.title')} - ${t('jce.common.expand')}`
	);

	function isMissingScore(fs: JapaScore['factor_scores'][number]): boolean {
		return (
			(fs.missing ?? false) ||
			(fs.missing_score !== undefined &&
				fs.score_value === fs.missing_score &&
				(!fs.explanation || /no data|missing/i.test(fs.explanation)))
		);
	}

	function deltaForScore(fs: JapaScore['factor_scores'][number]): number {
		const max = fs.max_value || 100;
		const baseline = max / 2;
		return Math.round(fs.score_value - baseline);
	}
</script>

<button
	type="button"
	class="text-sm"
	style="color: var(--color-muted)"
	aria-expanded={expanded}
	aria-label={ariaLabel}
	onclick={() => (expanded = !expanded)}
>
	<span aria-hidden="true">{expanded ? '▾' : '▸'}</span>
	{t('jce.score.title')}
</button>

{#if expanded}
	<div class="space-y-1 text-sm">
		{#each score.factor_scores as fs (fs.factor_key)}
			{@const isMissing = isMissingScore(fs)}
			{@const delta = isMissing ? 0 : deltaForScore(fs)}
			{@const absDelta = Math.abs(delta)}
			{@const sign = delta > 0 ? '+' : delta < 0 ? '−' : ''}
			{@const deltaClass = isMissing
				? 'text-slate-400'
				: delta > 0
					? 'text-green-600'
					: delta < 0
						? 'text-red-600'
						: 'text-amber-600'}
			<div class="flex justify-between">
				<span style="color: var(--color-text)">{fs.label}</span>
				<span class={deltaClass}>
					{#if isMissing}
						<span class="inline-flex items-center gap-1">
							<AlertTriangle size={14} />
							{t('jce.score.no_data')}
						</span>
					{:else}
						<span class="font-medium">{sign}{absDelta}</span>
						{#if fs.explanation}
							<span class="ml-1" style="color: var(--color-muted)">— {fs.explanation}</span>
						{:else}
							<span class="ml-1" style="color: var(--color-muted)">
								({Math.round(fs.score_value)}/{Math.round(fs.max_value || 100)})
							</span>
						{/if}
					{/if}
				</span>
			</div>
		{/each}
	</div>
{/if}
