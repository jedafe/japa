<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { GitCompare, Table, Star, Check, X } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import type { ComparisonMatrixDimensionValue } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const comparison = $derived(data.comparison);
	const matrix = $derived(data.matrix ?? []);

	// Sort items + dimensions by position for stable table rendering.
	const items = $derived([...comparison.items].sort((a, b) => a.position - b.position));
	const dimensions = $derived(
		[...comparison.dimensions].sort((a, b) => a.position - b.position)
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

	const title = $derived(t('compare.detail.title', 'Comparison — JapaConnect'));
	const description = $derived(
		t('compare.detail.description', 'Compare items across multiple dimensions.')
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<!-- Header -->
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<GitCompare size={20} /> {t('compare.detail.heading', 'Comparison')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
			{t('compare.detail.subtitle', 'Items compared across dimensions.')}
		</p>
	</div>

	<div class="p-3">
		{#if items.length === 0 && dimensions.length === 0}
			<EmptyState
				icon={Table}
				title={t('compare.detail.empty', 'This comparison has no items or dimensions yet')}
			/>
		{:else if dimensions.length === 0}
			<EmptyState
				icon={Table}
				title={t('compare.detail.no_dimensions', 'No dimensions defined yet')}
			/>
		{:else if items.length === 0}
			<EmptyState
				icon={Table}
				title={t('compare.detail.no_items', 'No items added to this comparison yet')}
			/>
		{:else}
			<!-- Discourse-style .prose-table-wrapper: horizontal scroll, never clips -->
			<section
				class="prose-table-wrapper overflow-x-auto rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]"
			>
				<table class="w-full text-sm">
					<thead>
						<tr class="border-b border-[var(--color-border)]">
							<th class="p-3 text-left text-xs font-medium uppercase text-[var(--color-muted)]">
								{t('compare.detail.dimension', 'Dimension')}
							</th>
							{#each items as item (item.id)}
								<th class="p-3 text-left text-sm font-medium text-[var(--color-text)]">
									{item.title}
								</th>
							{/each}
						</tr>
					</thead>
					<tbody>
						{#each dimensions as dimension (dimension.id)}
							<tr class="border-b border-[var(--color-border)] last:border-b-0">
								<td class="p-3 text-[var(--color-muted)]">{dimension.label}</td>
								{#each items as item (item.id)}
									{@const dv = cell(item.id, dimension.id)}
									<td class="p-3 text-[var(--color-text-subtle)]">
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
												<Check size={16} class="text-green-600" aria-label={t('compare.detail.yes', 'Yes')} />
											{:else if dv.value_boolean === false}
												<X size={16} class="text-[var(--color-muted)]" aria-label={t('compare.detail.no', 'No')} />
											{:else}
												<span class="text-[var(--color-muted)]">—</span>
											{/if}
										{:else}
											<!-- text, number, currency — render the string value -->
											{dv.value ?? '—'}
										{/if}
									</td>
								{/each}
							</tr>
						{/each}
					</tbody>
				</table>
			</section>
		{/if}
	</div>
</div>
