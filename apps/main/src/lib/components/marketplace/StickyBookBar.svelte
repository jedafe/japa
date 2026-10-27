<script lang="ts">
	// Mobile sticky bottom bar (§114 detail spec §3.3). Shows the
	// "From $X" price + rating, engagement actions (message / share /
	// report), and a purchase-mode-aware CTA — anchor href scrolls to
	// #book for on-platform booking; external listings link out to
	// booking_url. Rendered for every listing on mobile (engagement
	// actions apply even without a booking surface); desktop uses the
	// inline booking + share/report sections instead, so the bar hides
	// at lg+.
	// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§3.3)
	import { Check, ExternalLink, Flag, MessageCircle, Share2, Star } from 'lucide-svelte';
	import { t } from '$lib/i18n';

	interface Props {
		price: string | null;
		ratingAvg: number;
		ratingCount: number;
		/** CTA omitted when no booking surface exists — the bar still
		 *  carries the engagement actions. */
		ctaLabel?: string;
		ctaHref?: string;
		external?: boolean;
		/** When set (and not external), the CTA renders as a button that
		 *  opens the booking dialog instead of an anchor to #book. */
		onCta?: () => void;
		onMessage?: () => void;
		onShare?: () => void;
		onReport?: () => void;
		/** Transient "link copied" state for the share button. */
		linkCopied?: boolean;
	}
	let {
		price,
		ratingAvg,
		ratingCount,
		ctaLabel,
		ctaHref,
		external = false,
		onCta,
		onMessage,
		onShare,
		onReport,
		linkCopied = false
	}: Props = $props();

	const iconBtn =
		'inline-flex shrink-0 items-center justify-center rounded-[var(--radius)] border border-[var(--color-border)] p-2 text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]';
</script>

<div
	class="fixed inset-x-0 bottom-0 z-40 border-t border-[var(--color-border)] bg-[var(--color-surface)]/95 px-4 py-3 backdrop-blur-sm lg:hidden"
>
	<div class="mx-auto flex max-w-[1200px] items-center gap-2">
		<div class="min-w-0 flex-1">
			<p class="truncate text-sm">
				{#if price}
					<span class="text-xs text-[var(--color-muted)]"
						>{t('services.price_from', 'From')}</span
					>
					<span class="font-semibold text-[var(--color-text)]">{price}</span>
				{:else}
					<span class="text-xs text-[var(--color-muted)]">
						{t('services.pkg_price_on_request', 'Price on request')}
					</span>
				{/if}
			</p>
			{#if ratingCount > 0}
				<p class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
					<Star
						size={11}
						class="fill-[var(--color-warning)] text-[var(--color-warning)]"
						aria-hidden="true"
					/>
					{ratingAvg.toFixed(1)} ({ratingCount})
				</p>
			{/if}
		</div>
		{#if onMessage}
			<button
				type="button"
				onclick={onMessage}
				class={iconBtn}
				aria-label={t('services.message_provider', 'Message')}
			>
				<MessageCircle size={18} aria-hidden="true" />
			</button>
		{/if}
		{#if onShare}
			<button
				type="button"
				onclick={onShare}
				class={iconBtn}
				aria-label={t('feed.card.share', 'Share')}
			>
				{#if linkCopied}
					<Check size={18} aria-hidden="true" />
				{:else}
					<Share2 size={18} aria-hidden="true" />
				{/if}
			</button>
		{/if}
		{#if onReport}
			<button
				type="button"
				onclick={onReport}
				class={iconBtn}
				aria-label={t('services.report_listing', 'Report this listing')}
			>
				<Flag size={18} aria-hidden="true" />
			</button>
		{/if}
		{#if ctaLabel}
			{#if onCta && !external}
				<button
					type="button"
					onclick={onCta}
					class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
				>
					{ctaLabel}
				</button>
			{:else}
				<a
					href={ctaHref}
					target={external ? '_blank' : undefined}
					rel={external ? 'noopener noreferrer' : undefined}
					class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
				>
					{ctaLabel}
					{#if external}
						<ExternalLink class="rtl:-scale-x-100" size={14} aria-hidden="true" />
					{/if}
				</a>
			{/if}
		{/if}
	</div>
</div>
