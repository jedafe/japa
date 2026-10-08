<script lang="ts">
	// Reviews section — aggregate header (avg stars + count) plus the review
	// list from the §29 sidecar on GET /services/:slug. Same data contract
	// as the previous inline block (BUG-014); featured/first review renders
	// as a highlighted card above the rest. When the aggregate reports more
	// reviews than the detail payload embeds, a "View all reviews" footer
	// links to /services/[slug]/reviews (§114 paginated route).
	// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§3.3)
	import { ArrowRight, Star } from 'lucide-svelte';
	import ReviewListItem from './ReviewListItem.svelte';
	import { t } from '$lib/i18n';
	import type { ServiceReview, ServiceReviewAggregate } from '$lib/types';

	interface Props {
		reviews: ServiceReview[];
		aggregate: ServiceReviewAggregate;
		/** §114 — link to the standalone reviews page; the footer shows only
		 *  when the aggregate count exceeds the embedded rows. */
		viewAllHref?: string;
	}
	let { reviews, aggregate, viewAllHref }: Props = $props();

	const featured = $derived<ServiceReview | null>(reviews[0] ?? null);
	const rest = $derived(reviews.slice(1));
</script>

<section aria-labelledby="reviews-heading">
	<h2
		id="reviews-heading"
		class="flex flex-wrap items-center gap-2 text-sm font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]"
	>
		{t('services.reviews_heading', 'Reviews')}
	</h2>

	{#if aggregate.count > 0}
		<div class="mt-4 flex items-center gap-3">
			<span class="text-3xl font-bold text-[var(--color-text)]">{aggregate.avg.toFixed(1)}</span>
			<span>
				<span
					class="flex items-center gap-0.5"
					aria-label={t('services.rating_aria', '{avg} out of 5 stars', {
						avg: aggregate.avg.toFixed(1)
					})}
				>
					{#each Array(5) as _, i (i)}
						<Star
							size={15}
							class={i < Math.round(aggregate.avg)
								? 'fill-[var(--color-warning)] text-[var(--color-warning)]'
								: 'text-[var(--color-muted)]'}
							aria-hidden="true"
						/>
					{/each}
				</span>
				<span class="mt-0.5 block text-xs text-[var(--color-muted)]">
					{t('services.reviews_summary', '{count} reviews', { count: aggregate.count })}
				</span>
			</span>
		</div>
	{/if}

	{#if reviews.length === 0}
		<p class="mt-4 text-sm text-[var(--color-muted)]">
			{t('services.no_reviews_yet', 'No reviews yet. Be the first to share your experience.')}
		</p>
	{:else}
		<ul class="mt-4 space-y-4">
			{#if featured}
				<ReviewListItem review={featured} highlight />
			{/if}
			{#each rest as review (review.id)}
				<ReviewListItem {review} />
			{/each}
		</ul>
	{/if}

	{#if viewAllHref && aggregate.count > reviews.length}
		<a
			href={viewAllHref}
			class="mt-4 inline-flex items-center gap-1 text-sm font-medium text-[var(--color-primary)] transition-colors hover:underline"
		>
			{t('services.reviews_view_all', 'View all {count} reviews', { count: aggregate.count })}
			<ArrowRight size={14} aria-hidden="true" />
		</a>
	{/if}
</section>
