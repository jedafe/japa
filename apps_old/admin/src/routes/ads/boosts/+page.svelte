<script lang="ts">
	import { onMount } from 'svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { adBoostsApi } from '$lib/api';

	type Boost = {
		id: string;
		organization_name: string;
		space_name: string;
		boost_type: string;
		start_date: string;
		end_date: string;
		status: string;
	};

	let boosts = $state<Boost[]>([]);
	let page = $state(0);
	let totalPages = $state(1);
	let loading = $state(true);
	let errorMessage = $state('');

	let showModal = $state(false);
	let saving = $state(false);
	let showDeleteConfirm = $state(false);
	let deleteTargetId = $state<string | null>(null);

	let form = $state({
		organization_id: '',
		space_id: '',
		boost_type: 'featured',
		start_date: '',
		end_date: ''
	});

	const boostTypes = ['featured', 'pinned', 'highlighted', 'sponsored'];

	async function load() {
		loading = true;
		errorMessage = '';
		const params = new URLSearchParams({ page: String(page) });
		const result = await adBoostsApi.list(params.toString());
		if (result.ok) {
			const data = result.data as { items: Boost[]; total_pages: number };
			boosts = data.items ?? [];
			totalPages = data.total_pages ?? 1;
		} else {
			errorMessage = result.error;
		}
		loading = false;
	}

	function openCreate() {
		form = { organization_id: '', space_id: '', boost_type: 'featured', start_date: '', end_date: '' };
		showModal = true;
	}

	async function submit() {
		if (!form.organization_id.trim() || !form.space_id.trim() || !form.start_date || !form.end_date) return;
		saving = true;
		errorMessage = '';

		const result = await adBoostsApi.create({
			organization_id: form.organization_id.trim(),
			space_id: form.space_id.trim(),
			boost_type: form.boost_type,
			start_date: form.start_date,
			end_date: form.end_date
		});

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
		const result = await adBoostsApi.delete(deleteTargetId);
		if (result.ok) {
			showDeleteConfirm = false;
			deleteTargetId = null;
			load();
		} else {
			errorMessage = result.error;
		}
	}

	function statusColor(status: string) {
		switch (status) {
			case 'active': return 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400';
			case 'scheduled': return 'bg-blue-500/15 text-blue-700 dark:text-blue-400';
			case 'expired': return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
			default: return 'bg-amber-500/15 text-amber-700 dark:text-amber-400';
		}
	}

	function handlePageChange(p: number) {
		page = p;
		load();
	}

	onMount(load);
</script>

<svelte:head>
	<title>Ad Boosts — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">Ad Boosts</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">Manage content boosts for organizations and spaces</p>

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
		Create Boost
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Organization</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Space</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Type</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Start Date</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">End Date</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Status</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
				</tr>
			</thead>
			<tbody>
				{#each boosts as boost}
					<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium">{boost.organization_name}</td>
						<td class="px-4 py-3" style="color: var(--color-muted)">{boost.space_name}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full bg-purple-500/15 px-2 py-0.5 text-xs text-purple-700 dark:text-purple-400">{boost.boost_type}</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{boost.start_date}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{boost.end_date}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {statusColor(boost.status)}">
								{boost.status}
							</span>
						</td>
						<td class="px-4 py-3">
							<button
								class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
								style="color: var(--color-danger)"
								onclick={() => confirmDelete(boost.id)}
							>
								Delete
							</button>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="7" class="px-4 py-8 text-center" style="color: var(--color-muted)">No boosts configured</td>
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

<!-- Create Boost Modal -->
<Modal bind:open={showModal} title="Create Boost" onConfirm={submit} confirmLabel={saving ? 'Creating...' : 'Create'}>
	<div class="space-y-3">
		<div>
			<label for="boost-org" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Organization ID</label>
			<input id="boost-org" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="org-uuid" bind:value={form.organization_id} />
		</div>
		<div>
			<label for="boost-space" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Space ID</label>
			<input id="boost-space" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="space-uuid" bind:value={form.space_id} />
		</div>
		<div>
			<label for="boost-type" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Boost Type</label>
			<select id="boost-type" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.boost_type}>
				{#each boostTypes as bt}
					<option value={bt}>{bt}</option>
				{/each}
			</select>
		</div>
		<div class="grid grid-cols-2 gap-3">
			<div>
				<label for="boost-start" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Start Date</label>
				<input id="boost-start" type="date" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.start_date} />
			</div>
			<div>
				<label for="boost-end" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">End Date</label>
				<input id="boost-end" type="date" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.end_date} />
			</div>
		</div>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title="Confirm Delete" onConfirm={executeDelete} confirmLabel="Delete">
	<p class="text-sm" style="color: var(--color-muted)">Are you sure you want to delete this boost? This action cannot be undone.</p>
</Modal>
