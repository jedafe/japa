<script lang="ts">
	import { getFreshness } from '$lib/api/jce';
	import { t } from '$lib/i18n';
	import {
		deduplicateMatrix,
		uniqueDimensions,
		uniqueResolvedItems
	} from '$lib/utils/comparison';
	import type {
		FreshnessCell,
		FreshnessSummary,
		JCEComparisonDetail,
		JCEItem,
		JCEDimension,
		MatrixDimension,
		MatrixItem,
		ResolvedItem
	} from '$lib/types/jce';
	import type { PageData } from './$types';
	import FreshnessChip from '$lib/components/compare/FreshnessChip.svelte';
	import RefreshButton from '$lib/components/compare/RefreshButton.svelte';
	import AIAdvisorDrawer from '$lib/components/compare/AIAdvisorDrawer.svelte';
	import OwnerStatsCard from '$lib/components/content/OwnerStatsCard.svelte';
	import { ArrowLeft, ExternalLink } from 'lucide-svelte';

	let { data }: { data: PageData } = $props();

	let comparison = $derived(data.comparison);
	let matrix = $derived(deduplicateMatrix(data.matrix ?? []));
	let uniqueDims = $derived(uniqueDimensions(comparison.dimensions ?? []));
	let resolvedItems = $derived(uniqueResolvedItems(data.resolvedItems ?? []));
	let user = $derived(data.user ?? null);
	let features = $derived((data.features as Record<string, boolean> | undefined) ?? {});

	let freshness = $state<FreshnessSummary | null>(null);
	let advisorOpen = $state(false);

	$effect(() => {
		if (comparison?.content?.id && user && features['feature.jce.refresh']) {
			getFreshness(comparison.content.id).then((f) => {
				freshness = f;
			});
		}
	});

	function cellFor(item: MatrixItem, dim: JCEDimension): MatrixDimension | undefined {
		return item.dimensions.find((d: MatrixDimension) => d.dimension_id === dim.id);
	}

	function findCell(itemId: string, dimId: string): FreshnessCell | undefined {
		const item = freshness?.by_item.find((i) => i.item_id === itemId);
		return item?.cells.find((c) => c.dimension_id === dimId);
	}

	function formatValue(cell?: MatrixDimension) {
		if (!cell) return '—';
		if (cell.value !== undefined && cell.value !== null) return cell.value;
		if (cell.value_rating !== undefined && cell.value_rating !== null) return cell.value_rating;
		if (cell.value_boolean !== undefined && cell.value_boolean !== null)
			return cell.value_boolean ? t('jce.common.yes') : t('jce.common.no');
		return '—';
	}
</script>

<div class="max-w-5xl mx-auto p-4 sm:p-6 space-y-6">
	<div class="space-y-1">
		<a
			href="/comparisons"
			class="inline-flex items-center gap-1 text-sm transition-colors hover:opacity-80"
			style="color: var(--color-muted)"
		>
			<ArrowLeft size={14} /> {t('jce.comparisons.back')}
		</a>
		<h1 class="text-2xl sm:text-3xl font-bold" style="color: var(--color-text)">{comparison.content.title}</h1>
		<p class="text-sm capitalize" style="color: var(--color-muted)">
			{comparison.metadata.depth} · {comparison.content.status} · {comparison.content.visibility}
		</p>
	</div>

	{#if comparison.content.body_html}
		<!-- body_html is sanitized by the content service (SanitizeHTML) before storage. Ref: Rule 12, Section 30. -->
		<article
			class="prose prose-invert max-w-none rounded-[var(--radius-card)] border p-4"
			style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-text)"
		>
			{@html comparison.content.body_html}
		</article>
	{:else if comparison.content.body}
		<article
			class="prose prose-invert max-w-none rounded-[var(--radius-card)] border p-4"
			style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-text)"
		>
			{comparison.content.body}
		</article>
	{/if}

	<!-- Owner analytics (Section 40 — GET /content/{id}/analytics). -->
	{#if data.analytics}
		<OwnerStatsCard stats={data.analytics} />
	{/if}

	<section>
		<h2 class="text-lg font-semibold mb-3" style="color: var(--color-text)">{t('jce.compare.items_heading')}</h2>
		<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
			{#each resolvedItems as item (item.id)}
				<div
					class="rounded-[var(--radius-card)] border p-4 space-y-2"
					style="background-color: var(--color-surface); border-color: var(--color-border)"
				>
					<h3 class="font-medium" style="color: var(--color-text)">{item.title}</h3>
					{#if item.description}
						<p class="text-sm" style="color: var(--color-muted)">{item.description}</p>
					{/if}
					{#if item.url}
						<a
							href={item.url}
							target="_blank"
							class="inline-flex items-center gap-1 text-xs underline"
							style="color: var(--color-text)"
						>
							<ExternalLink size={12} /> {t('jce.common.view')}
						</a>
					{/if}
				</div>
			{/each}
		</div>
	</section>

	{#if matrix.length > 0}
		<section>
			<div class="mb-3 flex flex-wrap items-center justify-between gap-3">
				<h2 class="text-lg font-semibold" style="color: var(--color-text)">{t('jce.compare.matrix_heading')}</h2>
				<div class="flex flex-wrap items-center gap-2">
					{#if user && features['feature.jce.refresh']}
						<RefreshButton comparisonID={comparison.content.id} />
					{/if}
					{#if user && features['feature.jce.ai_advisor']}
						<button
							type="button"
							class="inline-flex items-center gap-1.5 rounded-[var(--radius-button)] px-3 py-1.5 text-xs font-medium transition-colors disabled:opacity-50"
							style="background-color: var(--color-primary, #3b82f6); color: white"
							onclick={() => (advisorOpen = true)}
						>
							{t('jce.advisor.open', 'Ask AI Advisor')}
						</button>
					{/if}
					{#if user && features['feature.jce.refresh'] && freshness}
						<FreshnessChip summary={freshness} />
					{/if}
				</div>
			</div>
			<div class="overflow-x-auto rounded-[var(--radius-card)] border" style="border-color: var(--color-border)">
				<table class="min-w-full text-sm">
					<thead>
						<tr style="background-color: var(--color-bg); border-bottom: 1px solid var(--color-border)">
							<th class="p-3 text-left font-medium" style="color: var(--color-text)">{t('jce.compare.matrix_dimension')}</th>
							{#each matrix as item (item.id)}
								<th class="p-3 text-left font-medium" style="color: var(--color-text)">{item.title}</th>
							{/each}
						</tr>
					</thead>
					<tbody class="divide-y" style="border-color: var(--color-border)">
						{#each uniqueDims as dim (dim.id)}
							<tr style="background-color: var(--color-surface)">
								<td class="p-3 font-medium" style="color: var(--color-text)">{dim.label}</td>
								{#each matrix as item (item.id)}
									<td class="p-3" style="color: var(--color-muted)">
										<div class="space-y-1">
											<div>{formatValue(cellFor(item, dim))}</div>
											{#if user && features['feature.jce.refresh'] && freshness}
												<FreshnessChip cell={findCell(item.id, dim.id)} />
											{/if}
										</div>
									</td>
								{/each}
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</section>
	{/if}

	<div class="flex gap-4">
		<a href="/comparisons" class="text-sm underline" style="color: var(--color-text)">← {t('jce.comparisons.back')}</a>
		<a href="/compare" class="text-sm underline" style="color: var(--color-muted)">{t('jce.comparisons.create_your_own')}</a>
	</div>

	{#if user && features['feature.jce.ai_advisor']}
		<AIAdvisorDrawer
			bind:open={advisorOpen}
			comparisonID={comparison.content.id}
			items={comparison.items ?? []}
		/>
	{/if}
</div>
