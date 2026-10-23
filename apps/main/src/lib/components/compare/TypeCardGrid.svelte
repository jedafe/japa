<script lang="ts">
	import { t } from '$lib/i18n';
	import type { ComparisonType } from '$lib/types/jce';
	import type { Component } from 'svelte';

	interface Props {
		types: ComparisonType[];
		iconFor: (type: ComparisonType) => Component;
		onSelect: (type: ComparisonType) => void;
	}

	let { types, iconFor, onSelect }: Props = $props();

	const FAMILY_ORDER = [
		'location',
		'education',
		'visa_routes',
		'banking',
		'embassies',
		'cost-of-living'
	];

	function familyLabel(family: string): string {
		const fallback = family
			.split(/[-_]/)
			.map((w) => w.charAt(0).toUpperCase() + w.slice(1))
			.join(' ');
		return t(`jce.compare.family.${family}`, fallback);
	}

	let families = $derived.by(() => {
		const groups = new Map<string, ComparisonType[]>();
		const ungrouped: ComparisonType[] = [];
		for (const cmpType of types) {
			const family = cmpType.config?.family;
			if (typeof family === 'string' && family) {
				const list = groups.get(family) ?? [];
				list.push(cmpType);
				groups.set(family, list);
			} else {
				ungrouped.push(cmpType);
			}
		}
		const ordered = [...groups.entries()].sort(([a], [b]) => {
			const ai = FAMILY_ORDER.indexOf(a);
			const bi = FAMILY_ORDER.indexOf(b);
			return (ai === -1 ? FAMILY_ORDER.length : ai) - (bi === -1 ? FAMILY_ORDER.length : bi);
		});
		return { ordered, ungrouped };
	});
</script>

{#snippet card(cmpType: ComparisonType)}
	{@const TypeIcon = iconFor(cmpType)}
	<button
		type="button"
		onclick={() => onSelect(cmpType)}
		class="flex items-start gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-start text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
	>
		<span
			class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-muted)]"
		>
			<TypeIcon size={20} />
		</span>
		<span class="min-w-0 flex-1">
			<span class="block font-medium text-[var(--color-text)]">{cmpType.name}</span>
			{#if cmpType.description}
				<span class="mt-0.5 line-clamp-2 block text-sm text-[var(--color-muted)]">
					{cmpType.description}
				</span>
			{/if}
			{#if cmpType.item_count}
				<span class="mt-1 block text-xs text-[var(--color-muted)]">
					{t('jce.compare.landing.type_items', '{n} items', { n: String(cmpType.item_count) })}
				</span>
			{/if}
		</span>
	</button>
{/snippet}

{#snippet grid(items: Array<ComparisonType>)}
	<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
		{#each items as cmpType (cmpType.id)}
			{@render card(cmpType)}
		{/each}
	</div>
{/snippet}

{#if families.ordered.length === 0}
	{@render grid(families.ungrouped)}
{:else}
	<div class="space-y-6">
		{#each families.ordered as [family, items] (family)}
			<section class="space-y-3">
				<h3 class="text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
					{familyLabel(family)}
				</h3>
				{@render grid(items)}
			</section>
		{/each}
		{#if families.ungrouped.length > 0}
			<section class="space-y-3">
				<h3 class="text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
					{t('jce.compare.family.other', 'Other')}
				</h3>
				{@render grid(families.ungrouped)}
			</section>
		{/if}
	</div>
{/if}
