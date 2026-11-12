<script lang="ts">
	/**
	 * ComparisonVsCard — the hub's signature card: diagonal split header from
	 * the first two preview_items images (tinted fallback), a flag-pair "vs"
	 * medallion when the items carry geo codes, badge chip, title, excerpt,
	 * "Compare now →" and a viewer-state bookmark + compact view count.
	 */
	import { Bookmark, Eye } from 'lucide-svelte';
	import { api } from '#lib/api/client.js';
	import { getViewerState, updateViewerState } from '#lib/api/viewerState.js';
	import { t } from '#lib/i18n/index.js';
	import { flagUrl } from '#lib/utils/flags.js';
	import type { JCEComparisonSummary, JCEPreviewItem } from '#lib/types/jce.js';
	import type { CompareBadgeKind } from './compareBadge.js';

	let {
		summary,
		badge = '',
		authed = false
	}: { summary: JCEComparisonSummary; badge?: CompareBadgeKind; authed?: boolean } = $props();

	const previews = $derived((summary.preview_items ?? []).slice(0, 3));
	const href = $derived(
		summary.depth === 'quick' ? `/compare/${summary.slug}` : `/comparisons/${summary.slug}`
	);

	const codeOf = (item: JCEPreviewItem): string =>
		(item.country_code ?? item.geo_code ?? '').toLowerCase();
	// The medallion shows when ≥2 previews resolve an actual flag asset.
	const flagPair = $derived.by(() => {
		const urls = previews.map((p) => flagUrl(codeOf(p))).filter((u): u is string => Boolean(u));
		return urls.length >= 2 ? urls.slice(0, 2) : null;
	});

	const views = $derived(
		new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 }).format(
			summary.view_count ?? 0
		)
	);

	let bookmarked = $state(false);
	let bookmarkInit = $state(false);
	$effect(() => {
		if (bookmarkInit || !authed) return;
		bookmarkInit = true;
		// Batched: one GET /me/content-states serves every card on the page.
		getViewerState(summary.id).then((s) => {
			bookmarked = s.bookmarked;
		});
	});

	async function toggleBookmark(e: MouseEvent) {
		e.preventDefault();
		e.stopPropagation();
		if (!authed) return;
		const next = !bookmarked;
		bookmarked = next;
		updateViewerState(summary.id, { bookmarked: next });
		try {
			const res = next
				? await api.post(`/content/${summary.id}/bookmark`, {})
				: await api.delete(`/content/${summary.id}/bookmark`);
			if (res.error) throw new Error(res.error.message);
		} catch {
			bookmarked = !next;
			updateViewerState(summary.id, { bookmarked: !next });
		}
	}
</script>

<a
	{href}
	class="group flex flex-col overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] transition-colors hover:bg-[var(--color-surface-subtle)]"
>
	<!-- Split header: first two preview images, diagonal seam; falls back to
	     a tinted placeholder when previews/images are absent. -->
	<div class="relative h-36 overflow-hidden bg-[var(--color-surface-subtle)]">
		{#if previews[0]?.image_url && previews[1]?.image_url}
			<img src={previews[0].image_url} alt="" class="absolute inset-0 h-full w-full object-cover" />
			<img
				src={previews[1].image_url}
				alt=""
				class="absolute inset-0 h-full w-full object-cover"
				style="clip-path: polygon(58% 0, 100% 0, 100% 100%, 42% 100%)"
			/>
			<div
				class="absolute inset-y-0 w-[3px] bg-[var(--color-surface)]"
				style="left: 50%; transform: translateX(-50%) skewX(-14deg)"
				aria-hidden="true"
			></div>
		{:else if previews[0]?.image_url}
			<img src={previews[0].image_url} alt="" class="absolute inset-0 h-full w-full object-cover" />
		{/if}

		{#if flagPair}
			<div class="absolute inset-x-0 bottom-2 flex items-center justify-center gap-1" aria-hidden="true">
				<img src={flagPair[0]} alt="" class="h-7 w-7 rounded-full border-2 border-[var(--color-surface)] object-cover shadow-sm" />
				<span class="rounded-full bg-[var(--color-surface)] px-1.5 py-0.5 text-[10px] font-bold text-[var(--color-muted)] shadow-sm">{t('jce.compare.vs', 'vs')}</span>
				<img src={flagPair[1]} alt="" class="h-7 w-7 rounded-full border-2 border-[var(--color-surface)] object-cover shadow-sm" />
			</div>
		{/if}

		{#if badge}
			<span
				class="absolute start-2 top-2 rounded-full px-2 py-0.5 text-xs font-medium
					{badge === 'popular' ? 'bg-emerald-500/90 text-white' : ''}
					{badge === 'hot' ? 'bg-red-500/90 text-white' : ''}
					{badge === 'trending' ? 'bg-[var(--color-primary)]/90 text-[var(--color-primary-contrast)]' : ''}"
			>{t(`jce.compare.badge.${badge}`, badge)}</span>
		{/if}
	</div>

	<div class="flex flex-1 flex-col gap-2 p-4">
		<h3 class="font-semibold leading-snug text-[var(--color-text)]">{summary.title}</h3>
		{#if summary.excerpt}
			<p class="line-clamp-2 text-sm text-[var(--color-muted)]">{summary.excerpt}</p>
		{/if}
		<div class="mt-auto flex items-center justify-between gap-2 pt-1">
			<span class="inline-flex items-center gap-1 text-sm font-medium text-[var(--color-primary-text)]">
				{t('jce.compare.compare_now', 'Compare now')}
				<span aria-hidden="true" class="rtl:-scale-x-100">→</span>
			</span>
			<span class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
				{#if authed}
					<button
						type="button"
						onclick={toggleBookmark}
						aria-pressed={bookmarked}
						aria-label={t('jce.compare.bookmark', 'Bookmark')}
						class="rounded p-0.5 transition-colors hover:text-[var(--color-text)] {bookmarked ? 'text-[var(--color-primary-text)]' : ''}"
					>
						<Bookmark size={14} fill={bookmarked ? 'currentColor' : 'none'} />
					</button>
				{/if}
				<span class="inline-flex items-center gap-1"><Eye size={12} /> {views}</span>
			</span>
		</div>
	</div>
</a>
