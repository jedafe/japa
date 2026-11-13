<script lang="ts">
	/**
	 * ComparisonCardRail — one-row card slider for /comparisons sections
	 * (Trending, Popular). Header row carries the section title + ‹ › arrows;
	 * the snap-x scroller stretches slides so every card shares the tallest
	 * card's height (consistent height across the row).
	 */
	import { ChevronLeft, ChevronRight } from 'lucide-svelte';
	import { t } from '#lib/i18n/index.js';
	import ComparisonVsCard from './ComparisonVsCard.svelte';
	import { badgeFor, type CompareBadgeKind } from './compareBadge.js';
	import type { JCEComparisonSummary } from '#lib/types/jce.js';

	let {
		heading,
		items,
		authed = false,
		badgeOf = (item: JCEComparisonSummary) => badgeFor(item)
	}: {
		heading: string;
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

	// Re-measure when the item set or the scroller's size changes.
	$effect(() => {
		items;
		update();
		const ro = new ResizeObserver(update);
		if (scroller) ro.observe(scroller);
		return () => ro.disconnect();
	});
</script>

<section class="space-y-3" aria-label={heading}>
	<div class="flex items-center justify-between gap-3">
		<h2 class="text-sm font-semibold text-[var(--color-text)]">{heading}</h2>
		<div class="flex items-center gap-1.5">
			<button
				type="button"
				onclick={() => scrollBy(-1)}
				disabled={!canPrev}
				aria-label={t('landing.home.carousel_prev', 'Previous')}
				class="grid h-8 w-8 place-items-center rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-40 disabled:hover:bg-[var(--color-surface)]"
			>
				<ChevronLeft class="rtl:-scale-x-100" size={16} />
			</button>
			<button
				type="button"
				onclick={() => scrollBy(1)}
				disabled={!canNext}
				aria-label={t('landing.home.carousel_next', 'Next')}
				class="grid h-8 w-8 place-items-center rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-40 disabled:hover:bg-[var(--color-surface)]"
			>
				<ChevronRight class="rtl:-scale-x-100" size={16} />
			</button>
		</div>
	</div>

	<div
		bind:this={scroller}
		onscroll={update}
		class="flex snap-x snap-mandatory items-stretch gap-3.5 overflow-x-auto pb-2 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
	>
		{#each items as item, i (item.id)}
			<div class="flex w-72 shrink-0 snap-start sm:w-[19rem]">
				<ComparisonVsCard summary={item} badge={badgeOf(item, i)} {authed} class="w-full" />
			</div>
		{/each}
	</div>
</section>
