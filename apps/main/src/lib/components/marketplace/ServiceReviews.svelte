<script lang="ts">
	// Reviews section — aggregate header (avg stars + count) plus the review
	// list from the §29 sidecar on GET /services/:slug. Same data contract
	// as the previous inline block (BUG-014); featured/first review renders
	// as a highlighted card above the rest.
	// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§3.3)
	import { Star } from 'lucide-svelte';
	import UserHoverCard from '$lib/components/ui/UserHoverCard.svelte';
	import { t } from '$lib/i18n';
	import type { ServiceReview, ServiceReviewAggregate } from '$lib/types';

	interface Props {
		reviews: ServiceReview[];
		aggregate: ServiceReviewAggregate;
	}
	let { reviews, aggregate }: Props = $props();

	const featured = $derived<ServiceReview | null>(reviews[0] ?? null);
	const rest = $derived(reviews.slice(1));

	function authorName(r: ServiceReview): string {
		return (
			r.author_display_name ?? r.author_username ?? t('services.anonymous_reviewer', 'Anonymous')
		);
	}

	function formatDate(iso: string): string {
		try {
			return new Date(iso).toLocaleDateString();
		} catch {
			return iso;
		}
	}
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
			{#snippet reviewCard(review: ServiceReview, highlight: boolean)}
				<li
					class="rounded-[var(--radius-card)] border bg-[var(--color-surface)] p-4 {highlight
						? 'border-[var(--color-primary)]/30'
						: 'border-[var(--color-border)]'}"
				>
					<div class="flex items-center justify-between gap-2">
						<div class="flex items-center gap-2">
							{#if review.author_username}
								<UserHoverCard
									id={review.author_id}
									slug={review.author_username}
									name={review.author_display_name ?? review.author_username}
									avatar={review.author_avatar_url}
									variant="row"
									class="inline-flex max-w-full items-center text-left text-sm font-medium text-[var(--color-text)] hover:underline"
								>
									{#snippet trigger()}
										<span>{authorName(review)}</span>
									{/snippet}
								</UserHoverCard>
							{:else}
								<span class="text-sm font-medium text-[var(--color-text)]"
									>{authorName(review)}</span
								>
							{/if}
							<span class="flex items-center gap-0.5 text-[var(--color-warning)]">
								{#each Array(5) as _, i (i)}
									<Star
										size={12}
										fill={i < review.rating ? 'currentColor' : 'none'}
										class={i < review.rating ? '' : 'text-[var(--color-muted)]'}
										aria-hidden="true"
									/>
								{/each}
							</span>
						</div>
						<span class="text-xs text-[var(--color-muted)]">{formatDate(review.created_at)}</span>
					</div>
					{#if review.title}
						<p class="mt-2 text-sm font-medium text-[var(--color-text)]">{review.title}</p>
					{/if}
					{#if review.body}
						<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{review.body}</p>
					{/if}
				</li>
			{/snippet}
			{#if featured}
				{@render reviewCard(featured, true)}
			{/if}
			{#each rest as review (review.id)}
				{@render reviewCard(review, false)}
			{/each}
		</ul>
	{/if}
</section>
