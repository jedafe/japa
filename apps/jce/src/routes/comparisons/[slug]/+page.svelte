<script lang="ts">
	import type {
		JCEComparisonDetail,
		JCEDimension,
		MatrixDimension,
		MatrixItem,
		ResolvedItem
	} from '$lib/types/jce';

	let { comparison, matrix, resolvedItems } = $props<{
		comparison: JCEComparisonDetail;
		matrix: MatrixItem[];
		resolvedItems: ResolvedItem[];
	}>();

	function cellFor(item: MatrixItem, dim: JCEDimension): MatrixDimension | undefined {
		return item.dimensions.find((d: MatrixDimension) => d.dimension_id === dim.id);
	}
</script>

<div class="max-w-5xl mx-auto p-6 space-y-8">
	<div>
		<h1 class="text-3xl font-bold">{comparison.content.title}</h1>
		<p class="text-sm text-slate-400 capitalize">
			{comparison.metadata.depth} · {comparison.content.status} · {comparison.content.visibility}
		</p>
	</div>

	<section>
		<h2 class="text-lg font-semibold mb-3">Items</h2>
		<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
			{#each resolvedItems as item (item.id)}
				<div class="rounded-lg border border-slate-700 p-4 space-y-2">
					<h3 class="font-medium">{item.title}</h3>
					{#if item.description}
						<p class="text-sm text-slate-400">{item.description}</p>
					{/if}
				</div>
			{/each}
		</div>
	</section>

	{#if matrix.length > 0}
		<section>
			<h2 class="text-lg font-semibold mb-3">Comparison matrix</h2>
			<div class="overflow-x-auto">
				<table class="min-w-full text-sm border border-slate-700 rounded-lg">
					<thead>
						<tr class="border-b border-slate-700 bg-slate-800">
							<th class="p-3 text-left">Dimension</th>
							{#each matrix as item (item.id)}
								<th class="p-3 text-left">{item.title}</th>
							{/each}
						</tr>
					</thead>
					<tbody>
						{#each comparison.dimensions as dim (dim.id)}
							<tr class="border-b border-slate-800">
								<td class="p-3 font-medium">{dim.label}</td>
								{#each matrix as item (item.id)}
									{@const cell = cellFor(item, dim)}
									<td class="p-3 text-slate-300">
										{#if cell}
											{#if cell.value !== undefined && cell.value !== null}
												{cell.value}
											{:else if cell.value_rating !== undefined && cell.value_rating !== null}
												{cell.value_rating}
											{:else if cell.value_boolean !== undefined && cell.value_boolean !== null}
												{cell.value_boolean ? 'Yes' : 'No'}
											{:else}
												—
											{/if}
										{:else}
											—
										{/if}
									</td>
								{/each}
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</section>
	{/if}

	<div class="flex gap-3">
		<a href="/comparisons" class="text-sm text-slate-400 hover:text-white">← Back to comparisons</a>
		<a href="/compare" class="text-sm text-blue-400 hover:text-blue-300">Create your own</a>
	</div>
</div>
