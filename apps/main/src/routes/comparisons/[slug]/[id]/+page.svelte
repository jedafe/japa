<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { ArrowLeft, Edit, Plus, Trash2, X } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { formatTimestamp } from '$lib/utils/timezone';

	let { data } = $props();
	let comparison = $derived(data.comparison);
	let matrix = $derived(data.matrix);

	let editing = $state(false);
	let editTitle = $state('');

	$effect(() => {
		editTitle = comparison?.title ?? '';
	});
	let saving = $state(false);
	let deleting = $state(false);
	let error = $state<string | null>(null);

	let showAddItem = $state(false);
	let newItemName = $state('');
	let addingItem = $state(false);

	let showAddDimension = $state(false);
	let newDimName = $state('');
	let addingDimension = $state(false);

	const slug = $derived(page.params.slug);
	const dimensions = $derived(matrix?.dimensions ?? []);
	const items = $derived(matrix?.items ?? []);

	function formatDate(dateStr: string) {
		return formatTimestamp(dateStr);
	}

	async function handleSaveTitle() {
		if (!comparison || !editTitle.trim() || saving) return;
		saving = true;
		error = null;
		try {
			await api.updateComparison(comparison.id, { title: editTitle.trim() });
			editing = false;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to save';
		} finally {
			saving = false;
		}
	}

	async function handleDelete() {
		if (!comparison || deleting || !confirm('Delete this comparison?')) return;
		deleting = true;
		try {
			await api.deleteComparison(comparison.id);
			goto('/comparisons/' + slug);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to delete';
			deleting = false;
		}
	}

	async function handleAddItem() {
		if (!comparison || !newItemName.trim() || addingItem) return;
		addingItem = true;
		error = null;
		try {
			await api.addComparisonItem(comparison.id, { name: newItemName.trim() });
			newItemName = '';
			showAddItem = false;
			goto('/comparisons/' + slug + '/' + comparison.id, { invalidateAll: true });
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to add item';
		} finally {
			addingItem = false;
		}
	}

	async function handleDeleteItem(itemId: string) {
		if (!confirm('Remove this item?')) return;
		try {
			await api.deleteComparisonItem(itemId);
			goto('/comparisons/' + slug + '/' + comparison!.id, { invalidateAll: true });
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to remove item';
		}
	}

	async function handleAddDimension() {
		if (!comparison || !newDimName.trim() || addingDimension) return;
		addingDimension = true;
		error = null;
		try {
			await api.addComparisonDimension(comparison.id, { name: newDimName.trim() });
			newDimName = '';
			showAddDimension = false;
			goto('/comparisons/' + slug + '/' + comparison.id, { invalidateAll: true });
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to add dimension';
		} finally {
			addingDimension = false;
		}
	}

	async function handleDeleteDimension(dimId: string) {
		if (!confirm('Remove this dimension?')) return;
		try {
			await api.deleteComparisonDimension(dimId);
			goto('/comparisons/' + slug + '/' + comparison!.id, { invalidateAll: true });
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to remove dimension';
		}
	}
</script>

<svelte:head>
	<title>{comparison?.title ?? 'Comparison'} | JapaConnect</title>
</svelte:head>

<main class="mx-auto max-w-5xl px-4 py-8">
	<a
		href="/comparisons/{slug}"
		class="mb-6 inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to comparisons
	</a>

	{#if !comparison}
		<p style="color: var(--color-muted)">Comparison not found.</p>
	{:else}
		<header class="mb-8 flex items-start justify-between gap-4">
			<div class="flex-1">
				{#if editing}
					<form onsubmit={(e) => { e.preventDefault(); handleSaveTitle(); }} class="flex items-center gap-2">
						<input
							type="text"
							bind:value={editTitle}
							class="flex-1 rounded-lg border px-3 py-2 text-2xl font-bold"
							style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						/>
						<button type="submit" disabled={saving} class="rounded-lg px-3 py-2 text-sm font-medium text-white" style="background: var(--color-primary)">
							{saving ? 'Saving...' : 'Save'}
						</button>
						<button type="button" onclick={() => { editing = false; editTitle = comparison.title; }} class="rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border)">Cancel</button>
					</form>
				{:else}
					<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">{comparison.title}</h1>
				{/if}
				<div class="mt-2 flex items-center gap-4 text-xs" style="color: var(--color-muted)">
					<span>{comparison.item_count} items</span>
					<span>{comparison.vote_count} votes</span>
					<span>{formatDate(comparison.created_at)}</span>
				</div>
			</div>
			<div class="flex items-center gap-2">
				<button onclick={() => { editing = true; }} class="inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-sm font-medium" style="border-color: var(--color-border); color: var(--color-foreground)">
					<Edit class="h-3.5 w-3.5" /> Edit
				</button>
				<button onclick={handleDelete} disabled={deleting} class="inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-sm font-medium" style="border-color: var(--color-border); color: #ef4444">
					<Trash2 class="h-3.5 w-3.5" />
				</button>
			</div>
		</header>

		{#if error}
			<div class="mb-4 rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-700">{error}</div>
		{/if}

		<!-- Items + Dimensions Controls -->
		<div class="mb-6 flex items-center gap-3">
			<button onclick={() => showAddItem = true} class="inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-sm font-medium" style="border-color: var(--color-border); color: var(--color-foreground)">
				<Plus class="h-3.5 w-3.5" /> Add Item
			</button>
			<button onclick={() => showAddDimension = true} class="inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-sm font-medium" style="border-color: var(--color-border); color: var(--color-foreground)">
				<Plus class="h-3.5 w-3.5" /> Add Dimension
			</button>
		</div>

		{#if showAddItem}
			<div class="mb-4 rounded-lg border p-4" style="border-color: var(--color-border); background: var(--color-surface)">
				<form onsubmit={(e) => { e.preventDefault(); handleAddItem(); }} class="flex items-center gap-2">
					<input type="text" bind:value={newItemName} placeholder="Item name" class="flex-1 rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-background)" />
					<button type="submit" disabled={addingItem} class="rounded-lg px-3 py-2 text-sm font-medium text-white" style="background: var(--color-primary)">Add</button>
					<button type="button" onclick={() => { showAddItem = false; newItemName = ''; }} class="rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border)"><X class="h-4 w-4" /></button>
				</form>
			</div>
		{/if}

		{#if showAddDimension}
			<div class="mb-4 rounded-lg border p-4" style="border-color: var(--color-border); background: var(--color-surface)">
				<form onsubmit={(e) => { e.preventDefault(); handleAddDimension(); }} class="flex items-center gap-2">
					<input type="text" bind:value={newDimName} placeholder="Dimension name" class="flex-1 rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-background)" />
					<button type="submit" disabled={addingDimension} class="rounded-lg px-3 py-2 text-sm font-medium text-white" style="background: var(--color-primary)">Add</button>
					<button type="button" onclick={() => { showAddDimension = false; newDimName = ''; }} class="rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border)"><X class="h-4 w-4" /></button>
				</form>
			</div>
		{/if}

		<!-- Matrix Table -->
		{#if items.length === 0}
			<div class="rounded-xl border p-8 text-center" style="border-color: var(--color-border)">
				<p style="color: var(--color-muted)">No items to compare yet. Add items and dimensions above.</p>
			</div>
		{:else}
			<div class="overflow-x-auto rounded-xl border" style="border-color: var(--color-border)">
				<table class="w-full text-sm">
					<thead>
						<tr style="background-color: var(--color-surface)">
							<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted)">Dimension</th>
							{#each items as item (item.id)}
								<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted)">
									<div class="flex items-center gap-1.5">
										{item.name}
										<button onclick={() => handleDeleteItem(item.id)} class="text-gray-400 hover:text-red-500"><X class="h-3 w-3" /></button>
									</div>
								</th>
							{/each}
						</tr>
					</thead>
					<tbody>
						{#each dimensions as dim (dim.id)}
							<tr style="border-top: 1px solid var(--color-border)">
								<td class="px-4 py-3 font-medium" style="color: var(--color-foreground)">
									<div class="flex items-center gap-1.5">
										{dim.name}
										<button onclick={() => handleDeleteDimension(dim.id)} class="text-gray-400 hover:text-red-500"><X class="h-3 w-3" /></button>
									</div>
								</td>
								{#each items as item (item.id)}
									<td class="px-4 py-3" style="color: var(--color-foreground)">
										{item.values?.[dim.name] ?? '—'}
									</td>
								{/each}
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	{/if}
</main>
