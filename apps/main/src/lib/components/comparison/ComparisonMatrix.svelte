<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Star, Check, X, Trash2 } from 'lucide-svelte';
	import { formatMoney, type CurrencyInfo } from '$lib/utils/currency';
	import type {
		ComparisonItem,
		ComparisonDimension,
		ComparisonMatrixItem,
		ComparisonMatrixDimensionValue
	} from '$lib/types';

	/**
	 * ComparisonMatrix — the shared items×dimensions table (Section 30,
	 * Phase 16.03). Columns are items (max 4), rows are dimensions. Values
	 * render per dimension type: stars for rating, ✓/✗ for boolean, plain
	 * text otherwise. Horizontally scrollable via the Discourse-style
	 * `.prose-table-wrapper`.
	 *
	 * Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
	 */
	interface Props {
		items: ComparisonItem[];
		dimensions: ComparisonDimension[];
		matrix: ComparisonMatrixItem[];
		canManage?: boolean;
		onRemove?: (itemId: string) => void;
	}

	let { items, dimensions, matrix, canManage = false, onRemove }: Props = $props();

	// §58 — currency dimension cells store their amount in value_rating and
	// carry currency_code on the dimension; resolve symbols via the shared
	// active-currency list from the root layout.
	const currencies = $derived(page.data.currencies as CurrencyInfo[] | undefined);

	// Sort items + dimensions by position for stable table rendering.
	const sortedItems = $derived([...items].sort((a, b) => a.position - b.position));
	const sortedDimensions = $derived(
		[...dimensions].sort((a, b) => a.position - b.position)
	);

	// Index the matrix by item ID for O(1) lookup of a row's dimension values.
	const matrixByItem = $derived.by(() => {
		const idx = new Map<string, Map<string, ComparisonMatrixDimensionValue>>();
		for (const row of matrix) {
			const dimMap = new Map<string, ComparisonMatrixDimensionValue>();
			for (const dv of row.dimensions ?? []) {
				dimMap.set(dv.dimension_id, dv);
			}
			idx.set(row.item.id, dimMap);
		}
		return idx;
	});

	/** Resolve the cell value for (item, dimension) from the matrix payload. */
	function cell(itemId: string, dimensionId: string): ComparisonMatrixDimensionValue | null {
		return matrixByItem.get(itemId)?.get(dimensionId) ?? null;
	}
</script>

<section class="prose-table-wrapper">
	<table class="text-sm">
		<thead>
			<tr>
				<th class="text-xs uppercase text-[var(--color-muted)]">
					{t('compare.detail.dimension', 'Dimension')}
				</th>
				{#each sortedItems as item (item.id)}
					<th class="text-sm text-[var(--color-text)]">
						<div class="flex items-center gap-1.5">
							<span class="min-w-0">{item.title}</span>
							{#if canManage && onRemove}
								<button
									type="button"
									onclick={() => onRemove(item.id)}
									class="text-[var(--color-muted)] hover:text-[var(--color-danger)]"
									title={t('compare.detail.remove_item', 'Remove item')}
								>
									<Trash2 size={14} />
								</button>
							{/if}
						</div>
					</th>
				{/each}
			</tr>
		</thead>
		<tbody>
			{#each sortedDimensions as dimension (dimension.id)}
				<tr>
					<td class="text-[var(--color-muted)]">{dimension.label}</td>
					{#each sortedItems as item (item.id)}
						{@const dv = cell(item.id, dimension.id)}
						<td class="text-[var(--color-text-subtle)]">
							{#if !dv}
								<span class="text-[var(--color-muted)]">—</span>
							{:else if dv.type === 'rating'}
								{#if dv.value_rating != null}
									<span class="inline-flex items-center gap-1">
										<Star size={14} class="fill-[var(--color-primary)] text-[var(--color-primary)]" />
										<span class="font-medium text-[var(--color-text)]">{dv.value_rating.toFixed(1)}</span>
									</span>
								{:else}
									<span class="text-[var(--color-muted)]">—</span>
								{/if}
							{:else if dv.type === 'boolean'}
								{#if dv.value_boolean === true}
									<Check size={16} class="text-[var(--color-success)]" aria-label={t('compare.detail.yes', 'Yes')} />
								{:else if dv.value_boolean === false}
									<X size={16} class="text-[var(--color-muted)]" aria-label={t('compare.detail.no', 'No')} />
								{:else}
									<span class="text-[var(--color-muted)]">—</span>
								{/if}
							{:else if dv.type === 'currency'}
								{#if dv.value_rating != null}
									{formatMoney(dv.value_rating, dv.currency_code, currencies)}
								{:else}
									{dv.value ?? '—'}
								{/if}
							{:else}
								<!-- text, number — render the string value -->
								{dv.value ?? '—'}
							{/if}
						</td>
					{/each}
				</tr>
			{/each}
		</tbody>
	</table>
</section>
