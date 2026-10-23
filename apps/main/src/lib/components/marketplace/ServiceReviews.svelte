<script lang="ts">
	// Reviews section — the §114 2026-10-10 mockup layout: a big heading,
	// "Based on N verified reviews" summary, and a horizontally scrollable
	// row of quote-style review cards with edge chevron controls. Data
	// contract unchanged — the §29 sidecar embedded on GET /services/:slug.
	// "View all reviews" links to /services/[slug]/reviews when the
	// aggregate count exceeds the embedded rows.
	import { ArrowRight, ChevronLeft, ChevronRight, Quote, Star } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import type { ServiceReview, ServiceReviewAggregate } from '$lib/types';

	interface Props {
		reviews: ServiceReview[];
		aggregate: ServiceReviewAggregate;
		heading?: string;
		/** §114 — link to the standalone reviews page. */
		viewAllHref?: string;
	}
	let { reviews, aggregate, heading, viewAllHref }: Props = $props();

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

	let rail = $state<HTMLElement | null>(null);
	let canPrev = $state(false);
	let canNext = $state(false);

	function updateArrows() {
		if (!rail) return;
		canPrev = rail.scrollLeft > 4;
		canNext = rail.scrollLeft + rail.clientWidth < rail.scrollWidth - 4;
	}
	function scrollBy(dir: 1 | -1) {
		rail?.scrollBy({ left: dir * (rail.clientWidth * 0.8), behavior: 'smooth' });
	}
	$effect(() => {
		reviews.length;
		updateArrows();
	});
</script>

<section aria-labelledby="reviews-heading">
	<h2 id="reviews-heading" class="text-xl font-bold text-[var(--color-text)]">
		{heading || t('services.reviews_heading', 'Reviews')}
	</h2>

	{#if aggregate.count > 0}
		<div class="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1">
			<span class="flex items-center gap-1 text-[var(--color-warning)]">
				{#each Array(5) as _, i (i)}
					<Star
						size={14}
						fill={i < Math.round(aggregate.avg) ? 'currentColor' : 'none'}
						class={i < Math.round(aggregate.avg) ? '' : 'text-[var(--color-muted)]'}
						aria-hidden="true"
					/>
				{/each}
			</span>
			<span class="text-sm font-semibold text-[var(--color-text)]">{aggregate.avg.toFixed(1)}</span>
			<span class="text-sm text-[var(--color-muted)]">
				{t('services.reviews_based_on', 'Based on {count} verified reviews', {
					count: aggregate.count
				})}
			</span>
		</div>
	{/if}

	{#if reviews.length === 0}
		<p class="mt-4 text-sm text-[var(--color-muted)]">
			{t('services.no_reviews_yet', 'No reviews yet. Be the first to share your experience.')}
		</p>
	{:else}
		<div class="relative mt-5">
			{#if canPrev}
				<button
					type="button"
					onclick={() => scrollBy(-1)}
					class="absolute -start-4 top-1/2 z-10 hidden -translate-y-1/2 place-items-center rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] p-2 shadow-sm transition hover:bg-[var(--color-surface-subtle)] md:grid"
					aria-label={t('services.reviews_prev', 'Previous reviews')}
				>
					<ChevronLeft class="rtl:-scale-x-100" size={16} aria-hidden="true" />
				</button>
			{/if}
			<ul
				bind:this={rail}
				onscroll={updateArrows}
				class="flex snap-x snap-mandatory gap-4 overflow-x-auto pb-2 [scrollbar-width:thin]"
			>
				{#each reviews as review (review.id)}
					<li
						class="relative flex w-72 shrink-0 snap-start flex-col rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5 sm:w-80"
					>
						<Quote
							size={26}
							class="absolute end-4 top-4 text-[var(--color-primary)]/15"
							aria-hidden="true"
						/>
						<!-- Mockup card header: avatar + name, review title as the
							sub-descriptor line, then the star row. -->
						<div class="flex items-center gap-2.5">
							{#if review.author_avatar_url}
								<img
									src={review.author_avatar_url}
									alt=""
									class="h-9 w-9 rounded-full object-cover"
								/>
							{:else}
								<span
									class="grid h-9 w-9 place-items-center rounded-full bg-[var(--color-primary)]/10 text-xs font-semibold text-[var(--color-primary)]"
									aria-hidden="true"
								>
									{authorName(review).slice(0, 1).toUpperCase()}
								</span>
							{/if}
							<div class="min-w-0 flex-1">
								<p class="truncate text-sm font-semibold text-[var(--color-text)]">
									{authorName(review)}
								</p>
								<p class="truncate text-[11px] text-[var(--color-muted)]">
									{review.title || formatDate(review.created_at)}
								</p>
							</div>
						</div>
						<span class="mt-2.5 flex items-center gap-0.5 text-[var(--color-warning)]">
							{#each Array(5) as _, i (i)}
								<Star
									size={12}
									fill={i < review.rating ? 'currentColor' : 'none'}
									class={i < review.rating ? '' : 'text-[var(--color-muted)]'}
									aria-hidden="true"
								/>
							{/each}
						</span>
						{#if review.body}
							<p
								class="mt-3 line-clamp-5 flex-1 text-sm leading-relaxed text-[var(--color-text-subtle)]"
							>
								{review.body}
							</p>
						{/if}
						<p class="mt-3 text-[11px] text-[var(--color-muted)]">
							{formatDate(review.created_at)}
						</p>
					</li>
				{/each}
			</ul>
			{#if canNext}
				<button
					type="button"
					onclick={() => scrollBy(1)}
					class="absolute -end-4 top-1/2 z-10 hidden -translate-y-1/2 place-items-center rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] p-2 shadow-sm transition hover:bg-[var(--color-surface-subtle)] md:grid"
					aria-label={t('services.reviews_next', 'More reviews')}
				>
					<ChevronRight class="rtl:-scale-x-100" size={16} aria-hidden="true" />
				</button>
			{/if}
		</div>
	{/if}

	{#if viewAllHref && aggregate.count > reviews.length}
		<a
			href={viewAllHref}
			class="mt-4 inline-flex items-center gap-1 text-sm font-medium text-[var(--color-primary)] transition-colors hover:underline"
		>
			{t('services.reviews_view_all', 'View all {count} reviews', { count: aggregate.count })}
			<ArrowRight class="rtl:-scale-x-100" size={14} aria-hidden="true" />
		</a>
	{/if}
</section>
