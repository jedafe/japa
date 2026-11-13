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
	let rtl = false;

	// scrollLeft is direction-dependent: negative in RTL on Chrome/Firefox,
	// positive-reversed on legacy Safari. Normalize to distance-from-start so
	// the edge gates work in either convention. (raw === 0 alone can't tell
	// negative-start from reverse-end; treated as start — the common case.)
	function update() {
		if (!scroller) return;
		const max = scroller.scrollWidth - scroller.clientWidth;
		const raw = scroller.scrollLeft;
		const fromStart = !rtl ? raw : raw <= 0 ? -raw : max - raw;
		canPrev = fromStart > 4;
		canNext = fromStart < max - 4;
	}

	// Both RTL conventions move *toward* the visual end on a negative delta,
	// so a single sign flip covers prev/next in either one.
	function scrollBy(dir: 1 | -1) {
		scroller?.scrollBy({
			left: dir * (rtl ? -1 : 1) * scroller.clientWidth * 0.8,
			behavior: 'smooth'
		});
	}

	// Re-measure when the item set or the scroller's size changes.
	$effect(() => {
		items;
		rtl = getComputedStyle(scroller!).direction === 'rtl';
		update();
		const ro = new ResizeObserver(update);
		if (scroller) ro.observe(scroller);
		return () => ro.disconnect();
	});
</script>

<section class="space-y-3">
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
