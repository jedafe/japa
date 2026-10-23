<script lang="ts">
	// "Packages & pricing" tier cards — the §114 2026-10-10 mockup layout:
	// name + badge chip, description, big price + payment suffix, feature
	// checklist, full-width Choose button. The featured tier gets the
	// primary border + centered "Most popular" ribbon. `Choose X`
	// delegates to the page's onSelect, which preselects the tier in the
	// booking dialog.
	// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§3.3)
	import { Check, Scale } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { formatMoney, type CurrencyInfo } from '$lib/utils/currency';
	import { marketplaceIcon } from './marketplaceIcon';
	import type { PackageTier } from '$lib/types';

	interface Props {
		tiers: PackageTier[];
		currencies: CurrencyInfo[];
		heading?: string;
		perLabel?: string;
		chooseLabel?: (tier: PackageTier) => string;
		/** "Compare packages" header link target (mockup header row). */
		compareHref?: string;
		/** False when this viewer has no booking surface (owner, or authed
		 *  without service.booking.create) — the Choose button hides rather
		 *  than scrolling to an absent #book section. Guests stay enabled:
		 *  the dialog renders their login prompt. */
		ctaEnabled?: boolean;
		onSelect: (tier: PackageTier) => void;
	}
	let {
		tiers,
		currencies,
		heading = '',
		perLabel = '',
		chooseLabel,
		compareHref = '',
		ctaEnabled = true,
		onSelect
	}: Props = $props();

	// Position order is server-side (ORDER BY position) — copy before sort
	// so the prop array is never mutated.
	const ordered = $derived([...tiers].sort((a, b) => a.position - b.position));
	const choose = $derived(
		chooseLabel ?? ((tier: PackageTier) => t('services.tier_choose', 'Choose {name}', { name: tier.name }))
	);
</script>

{#if ordered.length > 0}
	<section aria-labelledby="tiers-heading">
		<div class="flex flex-wrap items-baseline justify-between gap-2">
			<h2 id="tiers-heading" class="text-xl font-bold text-[var(--color-text)]">
				{heading || t('services.tiers_heading', 'Packages & pricing')}
			</h2>
			{#if compareHref}
				<a
					href={compareHref}
					class="inline-flex items-center gap-1.5 text-sm font-medium text-[var(--color-primary)] hover:underline"
				>
					<Scale size={14} aria-hidden="true" />
					{t('services.tiers_compare', 'Compare Packages')}
				</a>
			{/if}
		</div>
		<div class="mt-6 grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
			{#each ordered as tier (tier.id)}
				<article
					class="relative flex flex-col rounded-2xl border bg-[var(--color-surface)] p-6 {tier.is_featured
						? 'border-[var(--color-primary)] shadow-[0_8px_30px_-6px_var(--color-primary)]/25 ring-1 ring-[var(--color-primary)]/40 lg:-my-3 lg:py-9'
						: 'border-[var(--color-border)]'}"
				>
					{#if tier.is_featured}
						<span
							class="absolute -top-3 start-1/2 -translate-x-1/2 whitespace-nowrap rounded-full bg-[var(--color-primary)] px-3 py-1 text-[11px] font-semibold text-[var(--color-primary-contrast)]"
						>
							{t('services.tier_popular', 'Most popular')}
						</span>
					{/if}
					{#if tier.icon}
						{@const TierIcon = marketplaceIcon(tier.icon)}
						<span
							class="mb-3 grid h-10 w-10 place-items-center rounded-xl bg-[var(--color-primary)]/10 text-[var(--color-primary)]"
							aria-hidden="true"
						>
							<TierIcon size={18} />
						</span>
					{/if}
					<div class="flex flex-wrap items-center gap-2">
						<h3 class="text-base font-bold text-[var(--color-text)]">{tier.name}</h3>
						{#if tier.badge && !tier.is_featured}
							<span
								class="rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-primary)]"
							>
								{tier.badge}
							</span>
						{/if}
					</div>
					{#if tier.description}
						<p class="mt-1.5 text-xs leading-relaxed text-[var(--color-muted)]">
							{tier.description}
						</p>
					{/if}
					<p class="mt-4 flex items-baseline gap-2">
						<span class="text-3xl font-bold tracking-tight text-[var(--color-text)]">
							{formatMoney(tier.price_amount, tier.price_currency, currencies, { minor: true })}
						</span>
					</p>
					<p class="text-xs text-[var(--color-muted)]">
						{perLabel || t('services.tier_one_time', 'One-time payment')}
					</p>
					{#if tier.features.length > 0}
						<ul class="mt-5 space-y-2.5 border-t border-[var(--color-border)] pt-5">
							{#each tier.features as feature, i (i)}
								<li class="flex items-start gap-2.5 text-sm text-[var(--color-text-subtle)]">
									<Check
										size={15}
										class="mt-0.5 shrink-0 text-[var(--color-primary)]"
										aria-hidden="true"
									/>
									<span>{feature}</span>
								</li>
							{/each}
						</ul>
					{/if}
					{#if ctaEnabled}
						<button
							type="button"
							onclick={() => onSelect(tier)}
							class="mt-6 inline-flex w-full items-center justify-center rounded-lg px-4 py-2.5 text-sm font-semibold transition-colors {tier.is_featured
								? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]'
								: 'border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'}"
						>
							{choose(tier)}
						</button>
					{/if}
				</article>
			{/each}
		</div>
	</section>
{/if}
