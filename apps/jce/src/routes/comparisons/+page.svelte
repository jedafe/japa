<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import type { JCEComparisonSummary } from '$lib/types/jce';

	let { items, filters } = $props<{
		items: JCEComparisonSummary[];
		filters: {
			type: string;
			depth: string;
			status: string;
			visibility: string;
			q: string;
		};
	}>();

	let query = $state('');

	$effect.pre(() => {
		if (query === '' && filters.q) {
			query = filters.q;
		}
	});

	function apply() {
		const url = new URL($page.url);
		if (query) url.searchParams.set('q', query);
		else url.searchParams.delete('q');
		goto(url, { replaceState: true, keepFocus: true });
	}

	$effect(() => {
		if (query.length > 2) {
			const t = setTimeout(apply, 300);
			return () => clearTimeout(t);
		}
	});
</script>

<div class="max-w-5xl mx-auto p-6 space-y-8">
	<h1 class="text-3xl font-bold">Comparisons</h1>

	<div class="flex gap-2">
		<input
			type="text"
			bind:value={query}
			placeholder="Search comparisons..."
			class="flex-1 rounded-lg bg-slate-900 border border-slate-700 px-4 py-2"
		/>
		<button
			type="button"
			class="px-4 py-2 rounded-lg bg-blue-600 hover:bg-blue-500"
			onclick={apply}
		>
			Search
		</button>
	</div>

	{#if items.length === 0}
		<p class="text-slate-400">No public comparisons found.</p>
	{:else}
		<ul class="divide-y divide-slate-800 border border-slate-700 rounded-lg">
			{#each items as item (item.id)}
				<li class="p-4 hover:bg-slate-800/50">
					<a href="/comparisons/{item.slug}" class="block space-y-1">
						<h2 class="text-lg font-semibold">{item.title}</h2>
						<p class="text-sm text-slate-400 capitalize">
							{item.depth} · {item.status} · {item.visibility}
						</p>
					</a>
				</li>
			{/each}
		</ul>
	{/if}

	<div class="flex justify-end">
		<a
			href="/compare"
			class="inline-block px-4 py-2 rounded-lg bg-green-600 hover:bg-green-500 text-sm"
		>
			Create a comparison
		</a>
	</div>
</div>
