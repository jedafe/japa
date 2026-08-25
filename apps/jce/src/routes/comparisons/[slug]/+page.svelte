<script lang="ts">
	import { t } from '$lib/i18n';
	import type { JCEComparisonDetail, JCEDimension, MatrixDimension, MatrixItem, ResolvedItem } from '$lib/types/jce';
	import type { PageData } from './$types';
	import { ArrowLeft, ExternalLink } from 'lucide-svelte';

	let { data }: { data: PageData } = $props();

	let comparison = $derived(data.comparison);
	let matrix = $derived(data.matrix ?? []);
	let resolvedItems = $derived(data.resolvedItems ?? []);

	function cellFor(item: MatrixItem, dim: JCEDimension): MatrixDimension | undefined {
		return item.dimensions.find((d: MatrixDimension) => d.dimension_id === dim.id);
	}

	function formatValue(cell?: MatrixDimension) {
		if (!cell) return '—';
		if (cell.value !== undefined && cell.value !== null) return cell.value;
		if (cell.value_rating !== undefined && cell.value_rating !== null) return cell.value_rating;
		if (cell.value_boolean !== undefined && cell.value_boolean !== null) return cell.value_boolean ? t('jce.common.yes') : t('jce.common.no');
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
			<h2 class="text-lg font-semibold mb-3" style="color: var(--color-text)">{t('jce.compare.matrix_heading')}</h2>
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
						{#each comparison.dimensions as dim (dim.id)}
							<tr style="background-color: var(--color-surface)">
								<td class="p-3 font-medium" style="color: var(--color-text)">{dim.label}</td>
								{#each matrix as item (item.id)}
									<td class="p-3" style="color: var(--color-muted)">{formatValue(cellFor(item, dim))}</td>
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
</div>
