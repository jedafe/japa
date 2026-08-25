<script lang="ts">
	import { t } from '$lib/i18n';
	import type { ComparisonRecommendation } from '$lib/types/jce';

	interface Props {
		recommendations: ComparisonRecommendation[];
	}

	let { recommendations }: Props = $props();
</script>

<div
	class="rounded-[var(--radius-card)] border p-4"
	style="background-color: var(--color-surface); border-color: var(--color-border)"
>
	<h3 class="font-semibold" style="color: var(--color-text)">{t('jce.recommendations.title')}</h3>
	{#if recommendations.length === 0}
		<p class="text-sm" style="color: var(--color-muted)">{t('jce.recommendations.empty')}</p>
	{:else}
		<ul class="space-y-2 mt-2">
			{#each recommendations.slice(0, 5) as rec (rec.id)}
				<li class="flex items-start gap-2">
					<span class="text-sm" style="color: var(--color-text)">{rec.text}</span>
					{#if rec.action_url}
						<a
							href={rec.action_url}
							class="text-sm underline shrink-0"
							style="color: var(--color-text)"
							target="_blank"
							rel="noopener noreferrer"
						>
							{rec.action_type ?? t('jce.common.view')}
						</a>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</div>
