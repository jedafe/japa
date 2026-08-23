<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { deleteComparison, promoteComparison } from '$lib/api/jce';
	import type {
	JCEComparisonDetail,
	JCEDimension,
	MatrixDimension,
	MatrixItem,
	ResolvedItem
} from '$lib/types/jce';

	let { comparison, matrix, resolvedItems, isOwner, mainOrigin, typeSlug } = $props<{
		comparison: JCEComparisonDetail;
		matrix: MatrixItem[];
		resolvedItems: ResolvedItem[];
		isOwner: boolean;
		mainOrigin: string;
		typeSlug: string;
	}>();

	let deleting = $state(false);
	let promoting = $state(false);
	let error = $state('');

	async function remove() {
		if (!isOwner) return;
		deleting = true;
		error = '';
		try {
			await deleteComparison(comparison.content.id);
			goto('/compare');
		} catch (e) {
			error = e instanceof Error ? e.message : 'Delete failed';
		} finally {
			deleting = false;
		}
	}

	async function promote() {
		if (!isOwner) return;
		promoting = true;
		error = '';
		try {
			const promoted = await promoteComparison(comparison.content.id);
			if (promoted?.slug) {
				goto(`/comparisons/${promoted.slug}`);
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Promotion failed';
		} finally {
			promoting = false;
		}
	}

	function edit() {
		const ids = resolvedItems.map((i: ResolvedItem) => i.id).join(',');
		const url = new URL('/compare', $page.url);
		if (typeSlug) url.searchParams.set('type', typeSlug);
		url.searchParams.set('preselect', ids);
		goto(url);
	}

	function promoteToMain() {
		window.location.href = `${mainOrigin}/create?type=comparison&draft=${comparison.content.id}`;
	}

	function cellFor(item: MatrixItem, dim: JCEDimension): MatrixDimension | undefined {
		return item.dimensions.find((d: MatrixDimension) => d.dimension_id === dim.id);
	}
</script>

<div class="max-w-5xl mx-auto p-6 space-y-8">
	<div class="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
		<div>
			<h1 class="text-3xl font-bold">{comparison.content.title}</h1>
			<p class="text-sm text-slate-400 capitalize">
				{comparison.metadata.depth} · {comparison.content.status} · {comparison.content.visibility}
			</p>
		</div>
		{#if isOwner}
			<div class="flex flex-wrap gap-2">
				<button
					type="button"
					class="px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-sm"
					onclick={edit}
				>
					Edit selection
				</button>
				<button
					type="button"
					class="px-3 py-1.5 rounded-lg bg-blue-600 hover:bg-blue-500 text-sm disabled:opacity-50"
					disabled={promoting}
					onclick={promote}
				>
					{promoting ? 'Promoting...' : 'Promote to detailed'}
				</button>
				<button
					type="button"
					class="px-3 py-1.5 rounded-lg bg-green-600 hover:bg-green-500 text-sm"
					onclick={promoteToMain}
				>
					Open in main editor
				</button>
				<button
					type="button"
					class="px-3 py-1.5 rounded-lg bg-red-600 hover:bg-red-500 text-sm disabled:opacity-50"
					disabled={deleting}
					onclick={remove}
				>
					{deleting ? 'Deleting...' : 'Delete'}
				</button>
			</div>
		{/if}
	</div>

	{#if error}
		<div class="rounded-lg bg-red-500/10 text-red-500 p-4" role="alert">{error}</div>
	{/if}

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

	{#if matrix.length > 0 && comparison.metadata.depth === 'detailed'}
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

	{#if comparison.metadata.depth === 'quick'}
		<p class="text-sm text-slate-400">
			This is a quick comparison. Use <strong>Promote to detailed</strong> to publish it, or
			<strong>Open in main editor</strong> to expand it in the main app.
		</p>
	{/if}
</div>
