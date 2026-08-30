<script lang="ts">
	import { t } from '$lib/i18n';
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
			<div class="flex justify-between">
				<span style="color: var(--color-text)">{fs.label}</span>
				<span
					class={(() => {
						const pct = fs.max_value ? fs.score_value / fs.max_value : fs.score_value / 100;
						if (fs.missing) return 'text-slate-400';
						if (pct >= 0.7) return 'text-green-600';
						if (pct >= 0.4) return 'text-amber-600';
						return 'text-red-600';
					})()}
				>
					{#if fs.missing}
						{t('jce.score.no_data')}
					{:else if fs.explanation}
						{fs.explanation}
					{:else}
						{Math.round(fs.max_value || 0) === 0
							? Math.round(fs.score_value)
							: `${Math.round(fs.score_value)}/${Math.round(fs.max_value)}`}
					{/if}
				</span>
			</div>
		{/each}
	</div>
{/if}
