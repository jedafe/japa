<script lang="ts">
	// Hero fact rows / overlay stats — the listing's facts[] (§114
	// 2026-10-10). 'rows' is the service mockup's card under the hero
	// image (icon | label — value). 'grid' is the package mockup's stat
	// row overlaid at the bottom of the hero image.
	import { marketplaceIcon } from './marketplaceIcon';
	import type { ServiceFact } from '$lib/types';

	interface Props {
		facts: ServiceFact[];
		variant?: 'rows' | 'grid';
	}
	let { facts, variant = 'rows' }: Props = $props();
</script>

{#if facts.length > 0}
	{#if variant === 'rows'}
		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] px-5 py-2">
			<dl class="divide-y divide-[var(--color-border)]">
				{#each facts as fact, i (i)}
					{@const Icon = marketplaceIcon(fact.icon)}
					<div class="flex items-center justify-between gap-4 py-3.5">
						<dt class="flex min-w-0 items-center gap-2.5 text-sm text-[var(--color-text-subtle)]">
							<Icon size={16} class="shrink-0 text-[var(--color-muted)]" aria-hidden="true" />
							<span class="truncate">{fact.label}</span>
						</dt>
						<dd class="shrink-0 text-right text-sm font-semibold text-[var(--color-primary)]">
							{fact.value}
						</dd>
					</div>
				{/each}
			</dl>
		</div>
	{:else}
		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-4 shadow-[var(--shadow-md,0_8px_24px_rgba(0,0,0,0.08))]">
			<dl class="grid grid-cols-2 gap-4 sm:grid-cols-4">
				{#each facts as fact, i (i)}
					{@const Icon = marketplaceIcon(fact.icon)}
					<div class="flex min-w-0 flex-col items-center gap-1 text-center">
						<Icon size={16} class="text-[var(--color-primary)]" aria-hidden="true" />
						<dt class="text-[11px] text-[var(--color-muted)]">{fact.label}</dt>
						<dd class="truncate text-sm font-semibold text-[var(--color-text)]">{fact.value}</dd>
					</div>
				{/each}
			</dl>
		</div>
	{/if}
{/if}
