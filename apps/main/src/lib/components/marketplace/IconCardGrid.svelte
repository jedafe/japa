<script lang="ts">
	// "What's included" / "What you'll get" — the listing's included[]
	// rendered as the mockup's icon-card grid (§114 2026-10-10). Falls back
	// to the package items[] manifest when included[] is empty so existing
	// packages keep a visual grid. More than `collapseAt` entries collapse
	// behind a "See all N" expander (package mockup: "See all 12 services
	// included").
	import { Check, ChevronDown } from 'lucide-svelte';
	import { marketplaceIcon } from './marketplaceIcon';
	import { t } from '$lib/i18n';
	import type { PackageItem, ServiceIconEntry } from '$lib/types';

	interface Props {
		heading: string;
		included?: ServiceIconEntry[];
		items?: PackageItem[];
		/** Show the first N cards before the expander (0 = always show all). */
		collapseAt?: number;
	}
	let { heading, included = [], items = [], collapseAt = 6 }: Props = $props();

	// The package items manifest maps onto the same card shape — no icon
	// on the item rows, so the whitelist fallback applies.
	const cards = $derived<ServiceIconEntry[]>(
		included.length > 0
			? included
			: items
					.filter((i) => i.included)
					.map((i) => ({ title: i.title, description: i.description ?? '' }))
	);
	const hasExtra = $derived(collapseAt > 0 && cards.length > collapseAt);
	let expanded = $state(false);
	const visible = $derived(expanded || !hasExtra ? cards : cards.slice(0, collapseAt));
</script>

{#if cards.length > 0}
	<section aria-labelledby="included-cards-heading">
		<h2 id="included-cards-heading" class="text-xl font-bold text-[var(--color-text)]">
			{heading}
		</h2>
		<ul class="mt-5 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each visible as card, i (i)}
				{@const Icon = marketplaceIcon(card.icon)}
				<li
					class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5 text-center"
				>
					<span
						class="mx-auto grid h-11 w-11 place-items-center rounded-full bg-[var(--color-primary)]/10"
					>
						<Icon size={18} class="text-[var(--color-primary)]" aria-hidden="true" />
					</span>
					<p class="mt-3 text-sm font-semibold text-[var(--color-text)]">{card.title}</p>
					{#if card.description}
						<p class="mt-1 text-xs leading-relaxed text-[var(--color-muted)]">
							{card.description}
						</p>
					{/if}
				</li>
			{/each}
		</ul>
		{#if hasExtra}
			<button
				type="button"
				onclick={() => (expanded = !expanded)}
				class="mx-auto mt-5 flex items-center gap-1.5 text-sm font-medium text-[var(--color-primary)] transition-colors hover:underline"
			>
				{#if expanded}
					{t('services.included_show_less', 'Show less')}
					<ChevronDown size={15} class="rotate-180" aria-hidden="true" />
				{:else}
					{t('services.included_show_all', 'See all {count} services included', {
						count: cards.length
					})}
					<ChevronDown size={15} aria-hidden="true" />
				{/if}
			</button>
		{/if}
	</section>
{/if}
