<script lang="ts">
	// Listing coupon card (§114 marketplace). Shows the discount label,
	// dashed code chip with a copy-to-clipboard button (transient "Copied"
	// state — same pattern as the deals pages), expiry, terms link, and an
	// outbound "Go to provider" CTA for external referral coupons.
	// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§3.3)
	import { BadgePercent, Check, Copy, ExternalLink } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { discountLabel } from '$lib/marketplace';
	import type { CurrencyInfo } from '$lib/utils/currency';
	import type { ListingCoupon } from '$lib/types';

	interface Props {
		coupon: ListingCoupon;
		currencies: CurrencyInfo[];
	}
	let { coupon, currencies }: Props = $props();

	const discount = $derived(discountLabel(coupon, currencies));
	const expiryLabel = $derived(
		coupon.expires_at
			? t('services.coupon_valid_until', 'Valid until {date}', {
					date: new Date(coupon.expires_at).toLocaleDateString()
				})
			: ''
	);

	let copied = $state(false);

	async function copyCode() {
		if (!coupon.code) return;
		try {
			await navigator.clipboard.writeText(coupon.code);
			copied = true;
			setTimeout(() => (copied = false), 2000);
		} catch {
			// Clipboard may be unavailable (permissions, non-secure ctx).
		}
	}
</script>

<article
	class="rounded-[var(--radius-card)] border border-dashed border-[var(--color-primary)]/40 bg-[var(--color-primary)]/5 p-4"
>
	<div class="flex flex-wrap items-center gap-2">
		<span
			class="inline-flex items-center gap-1 rounded-full bg-[var(--color-primary)] px-2.5 py-0.5 text-xs font-semibold text-[var(--color-primary-contrast)]"
		>
			<BadgePercent size={12} aria-hidden="true" />
			{discount}
		</span>
		<h3 class="text-sm font-semibold text-[var(--color-text)]">{coupon.title}</h3>
	</div>

	<div class="mt-3 flex flex-wrap items-center gap-2">
		{#if coupon.code}
			<code
				class="rounded-[var(--radius)] border border-dashed border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 font-mono text-sm font-semibold tracking-wider text-[var(--color-text)]"
			>
				{coupon.code}
			</code>
			<button
				type="button"
				onclick={copyCode}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				{#if copied}
					<Check size={13} class="text-[var(--color-success)]" aria-hidden="true" />
					{t('common.copied', 'Copied!')}
				{:else}
					<Copy size={13} aria-hidden="true" />
					{t('common.copy', 'Copy')}
				{/if}
			</button>
		{/if}
		{#if expiryLabel}
			<span class="text-xs text-[var(--color-muted)]">{expiryLabel}</span>
		{/if}
	</div>

	<div class="mt-2 flex flex-wrap items-center gap-3 text-xs">
		{#if coupon.terms_url}
			<a
				href={coupon.terms_url}
				target="_blank"
				rel="noopener noreferrer"
				class="text-[var(--color-muted)] underline hover:text-[var(--color-text)]"
			>
				{t('services.coupon_terms', 'Terms & conditions')}
			</a>
		{/if}
		{#if coupon.referral_url}
			<a
				href={coupon.referral_url}
				target="_blank"
				rel="noopener sponsored"
				class="inline-flex items-center gap-1 font-medium text-[var(--color-primary)] hover:underline"
			>
				{t('services.coupon_go_provider', 'Go to provider')}
				<ExternalLink class="rtl:-scale-x-100" size={12} aria-hidden="true" />
			</a>
		{/if}
	</div>
</article>
