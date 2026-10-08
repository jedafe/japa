<script lang="ts">
	// Marketplace listing card — stretched-link pattern: the whole <article>
	// is covered by an absolutely-positioned <a> rendered last in DOM order
	// (z-0), so every pixel navigates to the detail page; genuinely
	// interactive siblings (the wishlist heart) sit on relative z-10 above it.
	// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§3.2)
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import {
		ArrowRight,
		BadgeCheck,
		BadgePercent,
		Heart,
		MapPin,
		Star,
		Wifi
	} from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { getViewerState, updateViewerState } from '$lib/api/viewerState';
	import { accentClasses, priceLabel, serviceCategoryInfo } from '$lib/marketplace';
	import { excerpt } from '$lib/utils/seo-score';
	import { serviceCategoryIcon } from './serviceCategoryIcon';
	import type { ServiceCategoryInfo, ServiceListing } from '$lib/types';
	import type { CurrencyInfo } from '$lib/utils/currency';

	interface Props {
		listing: ServiceListing;
		currencies: CurrencyInfo[];
		categories: ServiceCategoryInfo[];
		authed: boolean;
	}
	let { listing, currencies, categories, authed }: Props = $props();

	const category = $derived(serviceCategoryInfo(categories, listing.service_category));
	const accent = $derived(accentClasses(category?.accent_color));
	const CategoryIcon = $derived(serviceCategoryIcon(category?.icon));
	const price = $derived(priceLabel(listing, currencies));
	const blurb = $derived(excerpt(listing.body_html || listing.body, 140));
	const ratingAvg = $derived(listing.rating_avg ?? 0);
	const ratingCount = $derived(listing.review_count ?? 0);

	// The wishlist heart disappears entirely when the bookmarks feature flag
	// is off — never render a dead control (same gate as ReactionRail).
	const bookmarksEnabled = $derived(
		((page.data.features as Record<string, boolean> | undefined)?.['feature.bookmarks']) ?? true
	);

	let bookmarked = $state(false);
	let bookmarkPending = $state(false);
	let bookmarkLoaded = $state(false);

	$effect(() => {
		if (bookmarkLoaded || !authed || !bookmarksEnabled) return;
		bookmarkLoaded = true;
		// Batched: one GET /me/content-states serves every card on the page.
		getViewerState(listing.id).then((s) => {
			bookmarked = s.bookmarked;
		});
	});

	async function toggleBookmark() {
		if (bookmarkPending) return;
		// Guests can't bookmark — redirect to login instead of hitting a 401
		// (same guard as BookmarkButton / requireAuthOrRedirect).
		if (!authed) {
			await goto(`/login?redirect=${encodeURIComponent(page.url.pathname + page.url.search)}`);
			return;
		}
		bookmarkPending = true;
		const next = !bookmarked;
		bookmarked = next;
		updateViewerState(listing.id, { bookmarked: next });
		try {
			// listing.id is the contents row id — service listings are contents
			// with service_listing_meta sidecars, so the shared
			// POST/DELETE /content/{id}/bookmark routes apply (§14).
			const res = next
				? await api.post(`/content/${listing.id}/bookmark`, {})
				: await api.delete(`/content/${listing.id}/bookmark`);
			if (res.error) throw new Error(res.error.message);
		} catch {
			// Revert on failure.
			bookmarked = !next;
			updateViewerState(listing.id, { bookmarked: !next });
		} finally {
			bookmarkPending = false;
		}
	}
</script>

<article
	class="group relative flex flex-col overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] transition-all hover:shadow-lg motion-safe:hover:-translate-y-0.5"
>
	<!-- Image area -->
	<div class="relative aspect-[16/10] w-full overflow-hidden">
		{#if listing.featured_image_url}
			<img
				src={listing.featured_image_url}
				alt={listing.title}
				loading="lazy"
				class="h-full w-full object-cover transition-transform duration-500 motion-safe:group-hover:scale-105"
			/>
		{:else}
			<!-- Category-tinted placeholder tile (accent palette from
			     service_categories.accent_color). -->
			<div class="grid h-full w-full place-items-center {accent.chip} border-0">
				<CategoryIcon size={44} strokeWidth={1.5} class={accent.icon} />
			</div>
		{/if}

		<!-- Ribbons -->
		<div class="absolute left-3 top-3 flex flex-wrap gap-1.5">
			{#if listing.has_coupon}
				<span
					class="inline-flex items-center gap-1 rounded-full bg-[var(--color-surface)]/95 px-2 py-0.5 text-[11px] font-semibold shadow-sm {accent.icon}"
				>
					<BadgePercent size={12} />
					{t('services.coupon_badge', 'Deal')}
				</span>
			{/if}
			{#if listing.is_verified}
				<span
					class="inline-flex items-center gap-1 rounded-full bg-[var(--color-primary)]/95 px-2 py-0.5 text-[11px] font-semibold text-[var(--color-primary-contrast)] shadow-sm"
				>
					<BadgeCheck size={12} />
					{t('services.verified', 'Verified')}
				</span>
			{/if}
		</div>

		{#if bookmarksEnabled}
			<button
				type="button"
				class="absolute right-3 top-3 z-10 grid h-9 w-9 place-items-center rounded-full bg-black/40 backdrop-blur-sm transition-colors hover:bg-black/60 disabled:opacity-50 {bookmarked
					? 'text-[var(--color-primary)]'
					: 'text-white'}"
				aria-label={bookmarked
					? t('bookmarks.remove', 'Remove bookmark')
					: t('bookmarks.toggle', 'Bookmark')}
				aria-pressed={bookmarked}
				disabled={bookmarkPending}
				onclick={toggleBookmark}
			>
				<Heart size={17} fill={bookmarked ? 'currentColor' : 'none'} />
			</button>
		{/if}
	</div>

	<!-- Body -->
	<div class="flex flex-1 flex-col gap-2 p-4">
		<div class="flex items-center gap-1.5 text-xs text-[var(--color-muted)]">
			{#if listing.org_name}
				<span class="truncate font-medium text-[var(--color-text-subtle)]">{listing.org_name}</span>
				{#if listing.is_verified}
					<BadgeCheck size={13} class="shrink-0 text-[var(--color-primary)]" />
				{/if}
			{:else if listing.author_name}
				<span class="truncate font-medium text-[var(--color-text-subtle)]">{listing.author_name}</span>
			{/if}
			{#if listing.destination_space_name || listing.city_space_name}
				<span class="ml-auto inline-flex shrink-0 items-center gap-0.5">
					<MapPin size={11} />
					{listing.city_space_name ?? listing.destination_space_name}
				</span>
			{/if}
		</div>

		<h2
			class="line-clamp-2 text-[15px] font-semibold leading-snug text-[var(--color-text)] transition-colors group-hover:text-[var(--color-primary)]"
		>
			{listing.title}
		</h2>

		{#if blurb}
			<p class="line-clamp-2 text-xs leading-relaxed text-[var(--color-text-subtle)]">{blurb}</p>
		{/if}

		<div class="flex flex-wrap items-center gap-1.5">
			{#if category}
				<span
					class="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] font-medium {accent.chip}"
				>
					<CategoryIcon size={11} />
					{category.label}
				</span>
			{/if}
			{#if listing.is_remote}
				<span class="inline-flex items-center gap-1 text-[11px] text-[var(--color-muted)]">
					<Wifi size={11} />
					{t('services.remote', 'Remote')}
				</span>
			{/if}
		</div>

		{#if ratingCount > 0}
			<div class="flex items-center gap-1.5">
				<span
					class="flex items-center gap-0.5"
					aria-label={t('services.rating_aria', '{avg} out of 5 stars', {
						avg: ratingAvg.toFixed(1)
					})}
				>
					{#each Array(5) as _, i (i)}
						<Star
							size={12}
							class={i < Math.round(ratingAvg)
								? 'fill-[var(--color-warning)] text-[var(--color-warning)]'
								: 'text-[var(--color-muted)]'}
							aria-hidden="true"
						/>
					{/each}
				</span>
				<span class="text-xs text-[var(--color-muted)]">({ratingCount})</span>
			</div>
		{/if}

		<!-- Footer: price + view affordance (non-interactive span — the
		     stretched link carries the click). -->
		<div class="mt-auto flex items-center justify-between gap-2 border-t border-[var(--color-border)] pt-3">
			{#if price}
				<p class="text-sm">
					<span class="text-xs text-[var(--color-muted)]">{t('services.price_from', 'From')}</span>
					<span class="font-semibold text-[var(--color-text)]">{price}</span>
				</p>
			{:else if listing.price_range}
				<!-- §98 legacy free-text range (e.g. "£50–£150/hr") — shown until
				     numeric pricing exists on the listing. -->
				<p class="text-xs font-medium text-[var(--color-text-subtle)]">{listing.price_range}</p>
			{:else}
				<p class="text-xs text-[var(--color-muted)]">
					{t('services.pkg_price_on_request', 'Price on request')}
				</p>
			{/if}
			<span
				class="inline-flex shrink-0 items-center gap-1 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-2.5 py-1 text-xs font-medium text-[var(--color-text)] transition-colors group-hover:bg-[var(--color-primary)] group-hover:text-[var(--color-primary-contrast)]"
			>
				{t('services.card_view', 'View')}
				<ArrowRight size={12} />
			</span>
		</div>
	</div>

	<!-- Stretched link — last child so it paints above the decorative body
	     but below z-10 interactive siblings. Focus ring mirrors
	     SourceDailyCard's covering anchor. -->
	<a
		href="/services/{listing.slug}"
		class="absolute inset-0 z-0 rounded-[var(--radius-card)] focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary)]"
		aria-label={listing.title}
	>
		<span class="sr-only">{listing.title}</span>
	</a>
</article>
