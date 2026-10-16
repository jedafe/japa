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
</script>

<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
	{#each types as cmpType (cmpType.id)}
		{@const TypeIcon = iconFor(cmpType)}
		<button
			type="button"
			onclick={() => onSelect(cmpType)}
			class="flex items-start gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-left text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
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
	{/each}
</div>
