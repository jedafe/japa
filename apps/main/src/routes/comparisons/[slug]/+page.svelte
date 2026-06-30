<script lang="ts">
	import { ArrowLeft, Plus, Vote, Users, BarChart3, Loader2 } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api } from '$lib/api';
	import { isFeatureEnabled } from '$lib/stores/features.svelte';
	import { formatTimestamp } from '$lib/utils/timezone';

	let { data } = $props();
	let compType = $derived(data.type);
	let comparisons = $derived(data.comparisons);

	let selectedItems = $state<string[]>([]);
	let matrixData = $state<Record<string, Record<string, string>>>({});
	let loadingMatrix = $state(false);

	async function loadMatrix(comparisonId: string) {
		loadingMatrix = true;
		try {
			const result = await api.getComparisonMatrix(comparisonId);
			if (result.items) {
				matrixData = result.items.reduce((acc, item) => {
					acc[item.name] = item.values;
					return acc;
				}, {} as Record<string, Record<string, string>>);
			}
		} catch {
			matrixData = {};
		} finally {
			loadingMatrix = false;
		}
	}

	function formatDate(dateStr: string) {
		return formatTimestamp(dateStr);
	}
</script>

<svelte:head>
	<title>{compType.name} Comparisons | JapaConnect</title>
	<meta name="description" content={compType.description} />
</svelte:head>

{#if !isFeatureEnabled('feature.comparison')}
	<main class="mx-auto max-w-2xl px-6 py-16">
		<div class="text-center">
			<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">Comparisons is not available</h1>
			<p class="mt-2" style="color: var(--color-muted)">This feature is currently disabled.</p>
			<a href="/" class="mt-4 inline-block" style="color: var(--color-primary)">Go home</a>
		</div>
	</main>
{:else}
	<main class="mx-auto max-w-6xl px-4 py-8">
	<a
		href="/comparisons"
		class="mb-6 inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to comparisons
	</a>

	<header class="mb-8 flex items-start justify-between">
		<div>
			<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">
				{compType.name}
			</h1>
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				{compType.description}
			</p>
		</div>
		<a
			href="/comparisons/{compType.slug}/new"
			class="flex items-center gap-1.5 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
			style="background-color: var(--color-primary)"
		>
			<Plus class="h-4 w-4" />
			Create comparison
		</a>
	</header>

	{#if comparisons.length === 0}
		<EmptyState
			icon={BarChart3}
			title="No comparisons yet"
			message="Be the first to create a comparison for this category."
		/>
	{:else}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each comparisons as comp (comp.id)}
				<div
					class="rounded-xl border p-5 transition-all hover:shadow-md"
					style="border-color: var(--color-border); background: var(--color-surface)"
				>
					<h3 class="text-base font-semibold" style="color: var(--color-foreground)">
						{comp.title}
					</h3>
					<div class="mt-4 flex items-center gap-4 text-xs" style="color: var(--color-muted)">
						<span class="flex items-center gap-1">
							<Users class="h-3.5 w-3.5" />
							{comp.item_count} item{comp.item_count !== 1 ? 's' : ''}
						</span>
						<span class="flex items-center gap-1">
							<Vote class="h-3.5 w-3.5" />
							{comp.vote_count} vote{comp.vote_count !== 1 ? 's' : ''}
						</span>
						<span>{formatDate(comp.created_at)}</span>
					</div>
					<button
						onclick={() => loadMatrix(comp.id)}
						class="mt-4 w-full rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors hover:opacity-90"
						style="border-color: var(--color-border); color: var(--color-foreground)"
					>
						{loadingMatrix ? 'Loading...' : 'View matrix'}
					</button>
				</div>
			{/each}
		</div>
	{/if}

	{#if Object.keys(matrixData).length > 0}
		<section class="mt-10">
			<h2 class="mb-4 text-lg font-semibold" style="color: var(--color-foreground)">
				Comparison Matrix
			</h2>
			<div class="overflow-x-auto rounded-xl border" style="border-color: var(--color-border)">
				<table class="w-full text-sm">
					<thead>
						<tr style="background-color: var(--color-surface)">
							<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted)">
								Dimension
							</th>
							{#each Object.keys(matrixData) as item}
								<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted)">
									{item}
								</th>
							{/each}
						</tr>
					</thead>
					<tbody>
						{#each getDimensions(matrixData) as dimension}
							<tr style="border-top: 1px solid var(--color-border)">
								<td class="px-4 py-3 font-medium" style="color: var(--color-foreground)">
									{dimension}
								</td>
								{#each Object.keys(matrixData) as item}
									<td class="px-4 py-3" style="color: var(--color-foreground)">
										{matrixData[item]?.[dimension] ?? '—'}
									</td>
								{/each}
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</section>
	{/if}
	</main>
{/if}

<script module>
	function getDimensions(matrix: Record<string, Record<string, string>>): string[] {
		const dims = new Set<string>();
		for (const item of Object.values(matrix)) {
			for (const key of Object.keys(item)) {
				dims.add(key);
			}
		}
		return Array.from(dims);
	}
</script>
