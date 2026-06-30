<script lang="ts">
	import { onMount } from 'svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { ticketTypesApi } from '$lib/api';

	type TicketType = {
		id: string;
		name: string;
		slug: string;
		description: string;
		is_active: boolean;
		default_priority: string;
	};

	let ticketTypes = $state<TicketType[]>([]);
	let page = $state(0);
	let totalPages = $state(1);
	let loading = $state(true);
	let errorMessage = $state('');

	// Modal state
	let showModal = $state(false);
	let editingId = $state<string | null>(null);
	let saving = $state(false);
	let showDeleteConfirm = $state(false);
	let deleteTargetId = $state<string | null>(null);

	// Form
	let form = $state({
		name: '',
		slug: '',
		description: '',
		default_priority: 'medium',
		is_active: true
	});

	const priorities = ['low', 'medium', 'high', 'urgent'];

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
		const result = await ticketTypesApi.list(params.toString());
		if (result.ok) {
			const data = result.data as { items: TicketType[]; total_pages: number };
			ticketTypes = data.items ?? [];
			totalPages = data.total_pages ?? 1;
		} else {
			errorMessage = result.error;
		}
		loading = false;
	}

	function openCreate() {
		editingId = null;
		form = { name: '', slug: '', description: '', default_priority: 'medium', is_active: true };
		showModal = true;
	}

	function openEdit(item: TicketType) {
		editingId = item.id;
		form = {
			name: item.name,
			slug: item.slug,
			description: item.description ?? '',
			default_priority: item.default_priority,
			is_active: item.is_active
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
			description: form.description.trim(),
			default_priority: form.default_priority,
			is_active: form.is_active
		};

		const result = editingId
			? await ticketTypesApi.update(editingId, body)
			: await ticketTypesApi.create(body);

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
		const result = await ticketTypesApi.delete(deleteTargetId);
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
	<title>Ticket Types — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">Ticket Types</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">Manage ticket type categories and their default priorities</p>

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
		Add Ticket Type
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
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Description</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Active</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Priority</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
				</tr>
			</thead>
			<tbody>
				{#each ticketTypes as item}
					<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium">{item.name}</td>
						<td class="px-4 py-3 font-mono text-xs" style="color: var(--color-muted)">{item.slug}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{item.description || '—'}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {item.is_active ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : 'bg-slate-500/15 text-slate-700 dark:text-slate-400'}">
								{item.is_active ? 'Active' : 'Inactive'}
							</span>
						</td>
						<td class="px-4 py-3">
							<span class="text-xs font-medium {item.default_priority === 'urgent' ? 'text-red-600 dark:text-red-400' : item.default_priority === 'high' ? 'text-amber-600 dark:text-amber-400' : ''}">
								{item.default_priority}
							</span>
						</td>
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
						<td colspan="6" class="px-4 py-8 text-center" style="color: var(--color-muted)">No ticket types configured</td>
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
<Modal bind:open={showModal} title={editingId ? 'Edit Ticket Type' : 'Add Ticket Type'} onConfirm={submit} confirmLabel={saving ? 'Saving...' : (editingId ? 'Update' : 'Create')}>
	<div class="space-y-3">
		<div>
			<label for="tt-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Name</label>
			<input id="tt-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="Bug Report" bind:value={form.name} oninput={() => { if (!editingId) form.slug = slugify(form.name); }} />
		</div>
		<div>
			<label for="tt-slug" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Slug</label>
			<input id="tt-slug" type="text" class="w-full rounded-md border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="bug-report" bind:value={form.slug} />
		</div>
		<div>
			<label for="tt-desc" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Description</label>
			<textarea id="tt-desc" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" rows="2" bind:value={form.description}></textarea>
		</div>
		<div>
			<label for="tt-priority" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Default Priority</label>
			<select id="tt-priority" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.default_priority}>
				{#each priorities as p}
					<option value={p}>{p}</option>
				{/each}
			</select>
		</div>
		<div class="flex items-center gap-2">
			<input id="tt-active" type="checkbox" bind:checked={form.is_active} />
			<label for="tt-active" class="text-sm">Active</label>
		</div>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title="Confirm Delete" onConfirm={executeDelete} confirmLabel="Delete">
	<p class="text-sm" style="color: var(--color-muted)">Are you sure you want to delete this ticket type? This action cannot be undone.</p>
</Modal>
