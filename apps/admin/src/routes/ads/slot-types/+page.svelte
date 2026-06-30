<script lang="ts">
	import { onMount } from 'svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { adSlotTypesApi } from '$lib/api';

	type SlotType = {
		id: string;
		name: string;
		slug: string;
		width: number;
		height: number;
		description: string;
	};

	let slotTypes = $state<SlotType[]>([]);
	let page = $state(0);
	let totalPages = $state(1);
	let loading = $state(true);
	let errorMessage = $state('');

	let showModal = $state(false);
	let editingId = $state<string | null>(null);
	let saving = $state(false);
	let showDeleteConfirm = $state(false);
	let deleteTargetId = $state<string | null>(null);

	let form = $state({
		name: '',
		slug: '',
		width: 300,
		height: 250,
		description: ''
	});

	function slugify(text: string): string {
		return text
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-|-$/g, '');
	}

	async function load() {
		loading = true;
		errorMessage = '';
		const params = new URLSearchParams({ page: String(page) });
		const result = await adSlotTypesApi.list(params.toString());
		if (result.ok) {
			const data = result.data as { items: SlotType[]; total_pages: number };
			slotTypes = data.items ?? [];
			totalPages = data.total_pages ?? 1;
		} else {
			errorMessage = result.error;
		}
		loading = false;
	}

	function openCreate() {
		editingId = null;
		form = { name: '', slug: '', width: 300, height: 250, description: '' };
		showModal = true;
	}

	function openEdit(item: SlotType) {
		editingId = item.id;
		form = {
			name: item.name,
			slug: item.slug,
			width: item.width,
			height: item.height,
			description: item.description ?? ''
		};
		showModal = true;
	}

	async function submit() {
		if (!form.name.trim()) return;
		saving = true;
		errorMessage = '';

		const slug = form.slug.trim() || slugify(form.name);
		const body = {
			name: form.name.trim(),
			slug,
			width: form.width,
			height: form.height,
			description: form.description.trim()
		};

		const result = editingId
			? await adSlotTypesApi.update(editingId, body)
			: await adSlotTypesApi.create(body);

		if (result.ok) {
			showModal = false;
			load();
		} else {
			errorMessage = result.error;
		}
		saving = false;
	}

	function confirmDelete(id: string) {
		deleteTargetId = id;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTargetId) return;
		errorMessage = '';
		const result = await adSlotTypesApi.delete(deleteTargetId);
		if (result.ok) {
			showDeleteConfirm = false;
			deleteTargetId = null;
			load();
		} else {
			errorMessage = result.error;
		}
	}

	function handlePageChange(p: number) {
		page = p;
		load();
	}

	onMount(load);
</script>

<svelte:head>
	<title>Ad Slot Types — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">Ad Slot Types</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">Define ad slot dimensions and configurations</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button
		class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
		style="background-color: var(--color-primary)"
		onclick={openCreate}
	>
		Add Slot Type
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Name</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Slug</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Dimensions</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Description</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
				</tr>
			</thead>
			<tbody>
				{#each slotTypes as item}
					<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium">{item.name}</td>
						<td class="px-4 py-3 font-mono text-xs" style="color: var(--color-muted)">{item.slug}</td>
						<td class="px-4 py-3 font-mono text-xs">{item.width}×{item.height}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{item.description || '—'}</td>
						<td class="px-4 py-3">
							<div class="flex items-center gap-2">
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-info)"
									onclick={() => openEdit(item)}
								>
									Edit
								</button>
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-danger)"
									onclick={() => confirmDelete(item.id)}
								>
									Delete
								</button>
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">No slot types configured</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>

	<div class="mt-3 flex items-center justify-between">
		<p class="text-xs" style="color: var(--color-muted)">Page {page + 1} of {totalPages}</p>
		<div class="flex gap-1">
			<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40" disabled={page === 0} onclick={() => handlePageChange(page - 1)}>&larr;</button>
			<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40" disabled={page >= totalPages - 1} onclick={() => handlePageChange(page + 1)}>&rarr;</button>
		</div>
	</div>
{/if}

<!-- Create/Edit Modal -->
<Modal bind:open={showModal} title={editingId ? 'Edit Slot Type' : 'Add Slot Type'} onConfirm={submit} confirmLabel={saving ? 'Saving...' : (editingId ? 'Update' : 'Create')}>
	<div class="space-y-3">
		<div>
			<label for="slot-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Name</label>
			<input id="slot-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="Leaderboard" bind:value={form.name} oninput={() => { if (!editingId) form.slug = slugify(form.name); }} />
		</div>
		<div>
			<label for="slot-slug" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Slug</label>
			<input id="slot-slug" type="text" class="w-full rounded-md border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="leaderboard" bind:value={form.slug} />
		</div>
		<div class="grid grid-cols-2 gap-3">
			<div>
				<label for="slot-width" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Width (px)</label>
				<input id="slot-width" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.width} min="1" />
			</div>
			<div>
				<label for="slot-height" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Height (px)</label>
				<input id="slot-height" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.height} min="1" />
			</div>
		</div>
		<div>
			<label for="slot-desc" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Description</label>
			<input id="slot-desc" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="728×90 banner at top of page" bind:value={form.description} />
		</div>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title="Confirm Delete" onConfirm={executeDelete} confirmLabel="Delete">
	<p class="text-sm" style="color: var(--color-muted)">Are you sure you want to delete this slot type? This action cannot be undone.</p>
</Modal>
