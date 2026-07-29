<script lang="ts">
	// Ref: japaconnect-engineering/sections/29-ratings-reviews.md (aggregate display)
	// Renders the rolled-up rating for an entity: average star fill, review
	// count, and a small label. The aggregate is fetched by the parent and
	// passed in (or null when there are no reviews yet).
	import { t } from '$lib/i18n';
	import { Star } from 'lucide-svelte';
	import type { ReviewAggregate } from '$lib/types';

	interface Props {
		aggregate: ReviewAggregate | null;
		/** Optional label override; defaults to "reviews". */
		labelKey?: string;
	}
	let { aggregate, labelKey = 'reviews.label' }: Props = $props();

	const avg = $derived(aggregate?.average_rating ?? 0);
	const count = $derived(aggregate?.review_count ?? 0);
	const rounded = $derived(Math.round(avg * 10) / 10);
</script>

<div class="flex items-center gap-3">
	<div class="flex items-center gap-0.5" aria-label={`${t('reviews.average_aria')}: ${rounded}`}>
		{#each Array(5) as _, i (i)}
			<Star
				class="h-4 w-4 {i < Math.round(avg) ? 'fill-amber-400 text-amber-400' : 'text-[var(--color-muted)]'}"
				aria-hidden="true"
			/>
		{/each}
	</div>
	<span class="text-sm font-medium text-[var(--color-text)]">{rounded.toFixed(1)}</span>
	<span class="text-xs text-[var(--color-muted)]">
		{#if count > 0}
			({count} {t(labelKey)})
		{:else}
			{t('reviews.no_reviews')}
		{/if}
	</span>
</div>
