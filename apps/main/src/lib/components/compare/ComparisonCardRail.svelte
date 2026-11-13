<script lang="ts">
	/**
	 * ComparisonCardRail — one-row card slider for /comparisons sections
	 * (Trending, Popular). Snap-x scroller with edge arrows gated by scroll
	 * position — the same carousel pattern as ServiceReviews/GuestHomePage.
	 */
	import { ChevronLeft, ChevronRight } from 'lucide-svelte';
	import { t } from '#lib/i18n/index.js';
	import ComparisonVsCard from './ComparisonVsCard.svelte';
	import { badgeFor, type CompareBadgeKind } from './compareBadge.js';
	import type { JCEComparisonSummary } from '#lib/types/jce.js';

	let {
		items,
		authed = false,
		badgeOf = (item: JCEComparisonSummary) => badgeFor(item)
	}: {
		items: JCEComparisonSummary[];
		authed?: boolean;
		badgeOf?: (item: JCEComparisonSummary, index: number) => CompareBadgeKind;
	} = $props();

	let scroller = $state<HTMLDivElement>();
	let canPrev = $state(false);
	let canNext = $state(false);

	function update() {
		if (!scroller) return;
		canPrev = scroller.scrollLeft > 4;
		canNext = scroller.scrollLeft + scroller.clientWidth < scroller.scrollWidth - 4;
	}

	function scrollBy(dir: 1 | -1) {
		scroller?.scrollBy({ left: dir * (scroller.clientWidth * 0.8), behavior: 'smooth' });
	}

	// Re-measure when the item set or container size changes.
	$effect(() => {
		items;
		update();
	});
</script>

<div class="relative">
	{#if canPrev}
		<button
			type="button"
			onclick={() => scrollBy(-1)}
			aria-label={t('landing.home.carousel_prev', 'Previous')}
			class="absolute -start-3 top-1/2 z-10 hidden h-9 w-9 -translate-y-1/2 place-items-center rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text)] shadow-md transition-colors hover:bg-[var(--color-surface-subtle)] md:grid"
		>
			<ChevronLeft class="rtl:-scale-x-100" size={18} />
		</button>
	{/if}

	<div
		bind:this={scroller}
		onscroll={update}
		class="flex snap-x snap-mandatory gap-3.5 overflow-x-auto pb-2 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
	>
		{#each items as item, i (item.id)}
			<div class="w-72 shrink-0 snap-start sm:w-[19rem]">
				<ComparisonVsCard summary={item} badge={badgeOf(item, i)} {authed} />
			</div>
		{/each}
	</div>

	{#if canNext}
		<button
			type="button"
			onclick={() => scrollBy(1)}
			aria-label={t('landing.home.carousel_next', 'Next')}
			class="absolute -end-3 top-1/2 z-10 hidden h-9 w-9 -translate-y-1/2 place-items-center rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text)] shadow-md transition-colors hover:bg-[var(--color-surface-subtle)] md:grid"
		>
			<ChevronRight class="rtl:-scale-x-100" size={18} />
		</button>
	{/if}
</div>
