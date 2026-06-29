<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { goto } from '$app/navigation';

	type FieldDefinition = {
		id: string;
		name: string;
		slug: string;
		field_type: string;
		entity_type: string;
		required: boolean;
		searchable: boolean;
		sortable: boolean;
		created_at: string;
	};

	let fields = $state<FieldDefinition[]>([]);
	let loading = $state(true);
	let searchQuery = $state('');
	let filterEntityType = $state('');

	const entityTypes = ['content', 'user', 'space', 'tag'];

	async function load() {
		loading = true;
		const params = new URLSearchParams();
		if (searchQuery) params.set('search', searchQuery);
		if (filterEntityType) params.set('entity_type', filterEntityType);
		try {
			const result = await api<FieldDefinition[]>(`/custom-field-groups?${params}`);
			if (result.ok) {
				fields = result.data ?? [];
			}
		} catch {
			// API not available
		} finally {
			loading = false;
		}
	}

	function handleFilter() {
		load();
	}

	onMount(load);
</script>

<svelte:head>
	<title>Custom Fields — JapaConnect Admin</title>
</svelte:head>

<div class="mb-6 flex items-center justify-between">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">Custom Fields</h1>
		<p class="mt-1 text-sm" style="color: var(--color-muted)">Manage field definitions for entities</p>
	</div>
	<a
		href="/custom-fields/new"
		class="inline-flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
		style="background-color: var(--color-primary)"
	>
		Create Field
	</a>
</div>

<div class="mb-4 flex flex-wrap items-end gap-3">
	<div>
		<label for="search" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Search</label>
		<input
			id="search"
			type="text"
			class="rounded-md border px-3 py-1.5 text-sm"
			style="border-color: var(--color-border); background-color: var(--color-surface)"
			placeholder="Name or slug"
			bind:value={searchQuery}
			onkeydown={(e) => e.key === 'Enter' && handleFilter()}
		/>
	</div>
	<div>
		<label for="entity-type" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Entity Type</label>
		<select
			id="entity-type"
			class="rounded-md border px-3 py-1.5 text-sm"
			style="border-color: var(--color-border); background-color: var(--color-surface)"
			bind:value={filterEntityType}
			onchange={handleFilter}
		>
			<option value="">All</option>
			{#each entityTypes as et}
				<option value={et}>{et}</option>
			{/each}
		</select>
	</div>
	<button
		class="rounded-md border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
		style="border-color: var(--color-border)"
		onclick={handleFilter}
	>
		Apply
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Name</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Slug</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Type</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Entity</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Required</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Searchable</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Sortable</th>
				</tr>
			</thead>
			<tbody>
				{#each fields as field}
					<tr
						class="cursor-pointer border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]"
						style="border-color: var(--color-border)"
						onclick={() => goto(`/custom-fields/${field.id}`)}
					>
						<td class="px-4 py-3 font-medium">{field.name}</td>
						<td class="px-4 py-3 font-mono text-xs" style="color: var(--color-muted)">{field.slug}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full bg-black/5 px-2 py-0.5 text-xs dark:bg-white/10">{field.field_type}</span>
						</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full bg-black/5 px-2 py-0.5 text-xs dark:bg-white/10">{field.entity_type}</span>
						</td>
						<td class="px-4 py-3">
							{#if field.required}
								<span class="text-emerald-600 dark:text-emerald-400">&#10003;</span>
							{:else}
								<span style="color: var(--color-muted)">&mdash;</span>
							{/if}
						</td>
						<td class="px-4 py-3">
							{#if field.searchable}
								<span class="text-emerald-600 dark:text-emerald-400">&#10003;</span>
							{:else}
								<span style="color: var(--color-muted)">&mdash;</span>
							{/if}
						</td>
						<td class="px-4 py-3">
							{#if field.sortable}
								<span class="text-emerald-600 dark:text-emerald-400">&#10003;</span>
							{:else}
								<span style="color: var(--color-muted)">&mdash;</span>
							{/if}
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="7" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							No custom fields defined
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
