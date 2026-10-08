<script lang="ts">
	// "What's included" grid — package items (§111 service_package_items)
	// rendered as a two-column checklist. included+optional items carry an
	// "Optional" tag; items flagged !included render dashed "Not included"
	// rows so providers can list exclusions explicitly.
	// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§3.3)
	import { Check, Minus } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import type { PackageItem } from '$lib/types';

	interface Props {
		items: PackageItem[];
	}
	let { items }: Props = $props();

	const included = $derived(items.filter((i) => i.included));
	const excluded = $derived(items.filter((i) => !i.included));
</script>

{#if items.length > 0}
	<section aria-labelledby="included-heading">
		<h2
			id="included-heading"
			class="text-sm font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]"
		>
			{t('services.pkg_items_heading', "What's included")}
		</h2>
		<ul class="mt-4 grid gap-3 sm:grid-cols-2">
			{#each included as item (item.id)}
				<li
					class="flex items-start gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
					class:border-dashed={item.optional}
				>
					<span
						class="mt-0.5 grid h-6 w-6 shrink-0 place-items-center rounded-full bg-[var(--color-success)]/10"
					>
						<Check size={14} class="text-[var(--color-success)]" aria-hidden="true" />
					</span>
					<span class="min-w-0 text-sm text-[var(--color-text)]">
						<span class="font-medium">{item.title}</span>
						{#if item.optional}
							<span
								class="ml-1.5 rounded-full bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10px] text-[var(--color-muted)]"
								>{t('services.pkg_item_optional', 'Optional')}</span
							>
						{/if}
						{#if item.description}
							<span class="mt-0.5 block text-xs text-[var(--color-muted)]">{item.description}</span>
						{/if}
					</span>
				</li>
			{/each}
			{#each excluded as item (item.id)}
				<li
					class="flex items-start gap-3 rounded-[var(--radius)] border border-dashed border-[var(--color-border)] bg-[var(--color-surface-subtle)]/50 p-3"
				>
					<span
						class="mt-0.5 grid h-6 w-6 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)]"
					>
						<Minus size={14} class="text-[var(--color-muted)]" aria-hidden="true" />
					</span>
					<span class="min-w-0 text-sm text-[var(--color-text-subtle)]">
						<span>{item.title}</span>
						<span
							class="ml-1.5 rounded-full bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10px] text-[var(--color-muted)]"
						>
							{t('services.pkg_item_not_included', 'Not included')}
						</span>
						{#if item.description}
							<span class="mt-0.5 block text-xs text-[var(--color-muted)]">{item.description}</span>
						{/if}
					</span>
				</li>
			{/each}
		</ul>
	</section>
{/if}
