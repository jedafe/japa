<script lang="ts">
	import { t } from '$lib/i18n';
	import type { JapaScore } from '$lib/types/jce';

	interface Props {
		score: JapaScore;
		best: boolean;
	}

	let { score, best }: Props = $props();

	let badgeClass = $derived.by(() => {
		if (best && score.eligible_status === 'eligible') {
			return 'bg-green-100 dark:bg-green-900';
		}
		return 'bg-gray-100 dark:bg-gray-800';
	});
</script>

<div
	class="relative inline-flex items-center gap-1 rounded px-2 py-1 text-sm font-semibold {badgeClass}"
	class:opacity-50={score.eligible_status === 'ineligible'}
	style="color: var(--color-text)"
>
	<span>{Math.round(score.japa_score)}</span>
	{#if best}<span class="text-xs">{t('jce.score.best_match')}</span>{/if}
	{#if score.eligible_status === 'ineligible'}<span>{t('jce.score.ineligible')}</span>{/if}
	{#if score.eligible_status === 'limited'}<span>{t('jce.score.limited')}</span>{/if}
</div>
