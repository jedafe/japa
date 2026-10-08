<script lang="ts">
	// Mobile sticky bottom CTA band (§114 detail spec §3.3). Shows the
	// "From $X" price + rating and a purchase-mode-aware CTA — anchor href
	// scrolls to #book for on-platform booking; external listings link out
	// to booking_url. Desktop uses the inline booking section instead, so
	// the bar hides at lg+.
	// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§3.3)
	import { ExternalLink, Star } from 'lucide-svelte';
	import { t } from '$lib/i18n';

	interface Props {
		price: string | null;
		ratingAvg: number;
		ratingCount: number;
		ctaLabel: string;
		ctaHref: string;
		external?: boolean;
	}
	let { price, ratingAvg, ratingCount, ctaLabel, ctaHref, external = false }: Props = $props();
</script>

<div
	class="fixed inset-x-0 bottom-0 z-40 border-t border-[var(--color-border)] bg-[var(--color-surface)]/95 px-4 py-3 backdrop-blur-sm lg:hidden"
>
	<div class="mx-auto flex max-w-[1200px] items-center gap-3">
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
		<a
			href={ctaHref}
			target={external ? '_blank' : undefined}
			rel={external ? 'noopener noreferrer' : undefined}
			class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
		>
			{ctaLabel}
			{#if external}
				<ExternalLink size={14} aria-hidden="true" />
			{/if}
		</a>
	</div>
</div>
