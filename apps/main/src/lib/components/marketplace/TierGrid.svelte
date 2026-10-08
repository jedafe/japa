<script lang="ts">
	// "Packages & pricing" tier cards (§114 marketplace). One card per active
	// service_package_tiers row — the featured tier gets the primary ring +
	// "Most popular" ribbon. `Choose X` delegates to the page's onSelect,
	// which preselects the tier in the booking form and scrolls to #book.
	// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§3.3)
	import { Check } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { formatMoney, type CurrencyInfo } from '$lib/utils/currency';
	import type { PackageTier } from '$lib/types';

	interface Props {
		tiers: PackageTier[];
		currencies: CurrencyInfo[];
		perLabel?: string;
		onSelect: (tier: PackageTier) => void;
	}
	let { tiers, currencies, perLabel = '', onSelect }: Props = $props();

	// Position order is server-side (ORDER BY position) — copy before sort
	// so the prop array is never mutated.
	const ordered = $derived([...tiers].sort((a, b) => a.position - b.position));
</script>

{#if ordered.length > 0}
	<section aria-labelledby="tiers-heading">
		<h2
			id="tiers-heading"
			class="text-sm font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]"
		>
			{t('services.tiers_heading', 'Packages & pricing')}
		</h2>
		<div class="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each ordered as tier (tier.id)}
				<article
					class="relative flex flex-col rounded-[var(--radius-card)] border bg-[var(--color-surface)] p-5 {tier.is_featured
						? 'border-[var(--color-primary)] ring-2 ring-[var(--color-primary)]/30'
						: 'border-[var(--color-border)]'}"
				>
					{#if tier.is_featured}
						<span
							class="absolute -top-2.5 left-4 rounded-full bg-[var(--color-primary)] px-2.5 py-0.5 text-[11px] font-semibold text-[var(--color-primary-contrast)]"
						>
							{t('services.tier_popular', 'Most popular')}
						</span>
					{/if}
					{#if tier.badge}
						<span
							class="mb-2 inline-flex w-fit rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-[11px] font-medium text-[var(--color-text-subtle)]"
						>
							{tier.badge}
						</span>
					{/if}
					<h3 class="text-base font-semibold text-[var(--color-text)]">{tier.name}</h3>
					{#if tier.description}
						<p class="mt-1 text-xs text-[var(--color-muted)]">{tier.description}</p>
					{/if}
					<p class="mt-3 flex items-baseline gap-1.5">
						<span class="text-2xl font-bold text-[var(--color-text)]">
							{formatMoney(tier.price_amount, tier.price_currency, currencies, { minor: true })}
						</span>
						<span class="text-xs text-[var(--color-muted)]">
							{perLabel || t('services.tier_one_time', 'one-time payment')}
						</span>
					</p>
					{#if tier.features.length > 0}
						<ul class="mt-4 space-y-2">
							{#each tier.features as feature, i (i)}
								<li class="flex items-start gap-2 text-sm text-[var(--color-text-subtle)]">
									<Check
										size={15}
										class="mt-0.5 shrink-0 text-[var(--color-success)]"
										aria-hidden="true"
									/>
									<span>{feature}</span>
								</li>
							{/each}
						</ul>
					{/if}
					<button
						type="button"
						onclick={() => onSelect(tier)}
						class="mt-5 inline-flex items-center justify-center rounded-[var(--radius)] px-4 py-2 text-sm font-medium transition-colors {tier.is_featured
							? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]'
							: 'border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'}"
					>
						{t('services.tier_choose', 'Choose {name}', { name: tier.name })}
					</button>
				</article>
			{/each}
		</div>
	</section>
{/if}
