<script lang="ts">
	import { onMount } from 'svelte';
	import FilterBar from '$lib/components/FilterBar.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { api } from '$lib/api';

	type ModItem = {
		id: string;
		type: 'user_report' | 'ai_flag';
		content_type: string;
		content_id: string;
		reporter_id: string;
		reason: string;
		status: string;
		created_at: string;
	};

	let items = $state<ModItem[]>([]);
	let page = $state(0);
	let totalPages = $state(1);
	let loading = $state(true);
	let filterType = $state('');
	let filterStatus = $state('');
	let selectedIds = $state<Set<string>>(new Set());

	// Appeal modal
	let showAppeal = $state(false);
	let appealItem = $state<ModItem | null>(null);
	let appealNote = $state('');

	const filterDefs = [
		{ key: 'type', label: 'Source', type: 'select' as const, options: ['user_report', 'ai_flag'] },
		{ key: 'status', label: 'Status', type: 'select' as const, options: ['pending', 'reviewed', 'resolved', 'dismissed'] }
	];

	async function load() {
		loading = true;
		const params = new URLSearchParams({ page: String(page) });
		if (filterType) params.set('type', filterType);
		if (filterStatus) params.set('status', filterStatus);
		try {
			const data = await api<{ items: ModItem[]; page: number; total_pages: number }>(`/admin/moderation?${params}`);
			items = data.items;
			totalPages = data.total_pages;
		} catch {
			// API not available
		} finally {
			loading = false;
		}
	}

	function handleFilter(values: Record<string, string>) {
		filterType = values.type ?? '';
		filterStatus = values.status ?? '';
		page = 0;
		load();
	}

	function toggleSelect(id: string) {
		if (selectedIds.has(id)) {
			selectedIds.delete(id);
		} else {
			selectedIds.add(id);
		}
		selectedIds = selectedIds; // trigger reactivity
	}

	function toggleAll() {
		if (selectedIds.size === items.length) {
			selectedIds = new Set();
		} else {
			selectedIds = new Set(items.map((i) => i.id));
		}
	}

	async function bulkAction(action: 'dismiss' | 'resolve') {
		const ids = Array.from(selectedIds);
		await api('/admin/moderation/bulk', { method: 'POST', body: { ids, action } });
		selectedIds = new Set();
		load();
	}

	function openAppeal(item: ModItem) {
		appealItem = item;
		appealNote = '';
		showAppeal = true;
	}

	async function submitAppeal() {
		if (!appealItem) return;
		await api(`/admin/moderation/${appealItem.id}/appeal`, { method: 'POST', body: { note: appealNote } });
		showAppeal = false;
		load();
	}

	function handlePageChange(p: number) {
		page = p;
		load();
	}

	onMount(load);
</script>

<svelte:head>
	<title>Moderation — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">Content Moderation</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">Unified queue: user reports and AI flags</p>

<FilterBar filters={filterDefs} onApply={handleFilter} />

<!-- Bulk actions -->
{#if selectedIds.size > 0}
	<div class="mb-3 flex items-center gap-2 rounded-md border p-2" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<span class="text-xs font-medium" style="color: var(--color-muted)">{selectedIds.size} selected</span>
		<button
			class="rounded px-2 py-1 text-xs font-medium hover:bg-black/5 dark:hover:bg-white/10"
			style="color: var(--color-success)"
			onclick={() => bulkAction('resolve')}
		>
			Resolve
		</button>
		<button
			class="rounded px-2 py-1 text-xs font-medium hover:bg-black/5 dark:hover:bg-white/10"
			style="color: var(--color-muted)"
			onclick={() => bulkAction('dismiss')}
		>
			Dismiss
		</button>
	</div>
{/if}

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left" style="background-color: var(--color-surface)">
						<input type="checkbox" checked={selectedIds.size === items.length && items.length > 0} onchange={toggleAll} />
					</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Source</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Content</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Reason</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Status</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Date</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
				</tr>
			</thead>
			<tbody>
				{#each items as item}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3">
							<input type="checkbox" checked={selectedIds.has(item.id)} onchange={() => toggleSelect(item.id)} />
						</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {item.type === 'ai_flag' ? 'bg-purple-500/15 text-purple-700 dark:text-purple-400' : 'bg-blue-500/15 text-blue-700 dark:text-blue-400'}">
								{item.type === 'ai_flag' ? 'AI Flag' : 'User Report'}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{item.content_type}/{item.content_id.slice(0, 8)}</td>
						<td class="px-4 py-3 text-xs">{item.reason}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {item.status === 'pending' ? 'bg-amber-500/15 text-amber-700 dark:text-amber-400' : 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400'}">
								{item.status}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{item.created_at}</td>
						<td class="px-4 py-3">
							<button
								class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
								style="color: var(--color-info)"
								onclick={() => openAppeal(item)}
							>
								Appeal
							</button>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="7" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							No moderation items
						</td>
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

<!-- Appeal Modal -->
<Modal bind:open={showAppeal} title="Review Appeal" onConfirm={submitAppeal} confirmLabel="Submit">
	{#if appealItem}
		<p class="text-sm mb-3">Appeal for {appealItem.content_type} item</p>
		<textarea
			class="w-full rounded-md border px-3 py-2 text-sm"
			style="border-color: var(--color-border); background-color: var(--color-surface)"
			rows="3"
			placeholder="Appeal note..."
			bind:value={appealNote}
		></textarea>
	{/if}
</Modal>
