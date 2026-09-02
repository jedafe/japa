<script lang="ts">
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { uniqueBy } from '$lib/utils/comparison';
	import type { ComparisonRecommendation } from '$lib/types/jce';

	interface Props {
		recommendations: ComparisonRecommendation[];
	}

	let { recommendations }: Props = $props();

	// Backend text may be a pre-translated sentence or a dotted i18n key.
	// Dotted keys in the `jce.*` namespace are translated; everything else is shown as-is.
	function displayText(text: string): string {
		if (text.startsWith('jce.')) {
			return t(text);
		}
		return text;
	}

	let uniqueRecommendations = $derived(
		uniqueBy(recommendations, (r) => `${r.text}|${r.action_type ?? ''}|${r.action_url ?? ''}`)
	);

	function linkAttributes(url: string) {
		try {
			const parsed = new URL(url, page.url);
			if (parsed.origin !== page.url.origin) {
				return { target: '_blank' as const, rel: 'noopener noreferrer' as const };
			}
		} catch {
			// Malformed URL: opening in a new tab is the safest default.
			return { target: '_blank' as const, rel: 'noopener noreferrer' as const };
		}
		return {};
	}

	function actionLabel(type: string | null | undefined): string {
		const key = type ? `jce.recommendations.action_${type}` : 'jce.recommendations.action_generic';
		return t(key, type ?? t('jce.recommendations.action_generic'));
	}
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
			{#each uniqueRecommendations as rec (rec.id)}
				<li class="flex items-start gap-2">
					<span class="text-sm" style="color: var(--color-text)">{displayText(rec.text)}</span>
					{#if rec.action_url}
						<a
							href={rec.action_url}
							class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium shrink-0"
							style="background-color: var(--color-surface); color: var(--color-text)"
							{...linkAttributes(rec.action_url)}
						>
							{actionLabel(rec.action_type)}
						</a>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</div>
