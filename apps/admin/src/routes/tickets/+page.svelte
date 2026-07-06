<script lang="ts">
	import { onMount } from 'svelte';
	import FilterBar from '$lib/components/FilterBar.svelte';
import { api } from '$lib/api';

	type Ticket = {
		id: string;
		type: string;
		subject: string;
		status: string;
		priority: string;
		assignee: string;
		created_at: string;
	};

	let tickets = $state<Ticket[]>([]);
	let page = $state(0);
	let totalPages = $state(1);
	let loading = $state(true);
	let sortKey = $state('created_at');
	let sortAsc = $state(false);

	const filterDefs = [
		{ key: 'type', label: 'Type', type: 'select' as const, options: ['bug', 'feature', 'question', 'account'] },
		{ key: 'status', label: 'Status', type: 'select' as const, options: ['open', 'in_progress', 'waiting', 'resolved', 'closed'] },
		{ key: 'priority', label: 'Priority', type: 'select' as const, options: ['low', 'medium', 'high', 'urgent'] },
		{ key: 'assignee', label: 'Assignee', type: 'text' as const },
		{ key: 'from', label: 'From', type: 'date' as const },
		{ key: 'to', label: 'To', type: 'date' as const }
	];

	let filterValues: Record<string, string> = {};

	async function load() {
		loading = true;
		const params = new URLSearchParams({ page: String(page), sort: sortKey, order: sortAsc ? 'asc' : 'desc' });
		for (const [k, v] of Object.entries(filterValues)) {
			if (v) params.set(k, v);
		}
		try {
			const result = await api<{ tickets: Ticket[]; page: number; total_pages: number }>(`/admin/tickets?${params}`);
			if (result.ok) {
				tickets = result.data.tickets;
				totalPages = result.data.total_pages;
			}
		} catch {
			// API not available
		} finally {
			loading = false;
		}
	}

	function handleFilter(values: Record<string, string>) {
		filterValues = values;
		page = 0;
		load();
	}

	function sort(key: string) {
		if (sortKey === key) {
			sortAsc = !sortAsc;
		} else {
			sortKey = key;
			sortAsc = false;
		}
		load();
	}

	function handlePageChange(p: number) {
		page = p;
		load();
	}

	onMount(load);
</script>

<svelte:head>
	<title>Tickets — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">Ticket Queue</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">Manage support tickets with bulk actions</p>

<FilterBar filters={filterDefs} onApply={handleFilter} />

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="cursor-pointer px-4 py-3 text-left font-medium hover:underline" style="color: var(--color-muted); background-color: var(--color-surface)" onclick={() => sort('type')}>
						Type {sortKey === 'type' ? (sortAsc ? '↑' : '↓') : ''}
					</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Subject</th>
					<th class="cursor-pointer px-4 py-3 text-left font-medium hover:underline" style="color: var(--color-muted); background-color: var(--color-surface)" onclick={() => sort('status')}>
						Status {sortKey === 'status' ? (sortAsc ? '↑' : '↓') : ''}
					</th>
					<th class="cursor-pointer px-4 py-3 text-left font-medium hover:underline" style="color: var(--color-muted); background-color: var(--color-surface)" onclick={() => sort('priority')}>
						Priority {sortKey === 'priority' ? (sortAsc ? '↑' : '↓') : ''}
					</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Assignee</th>
					<th class="cursor-pointer px-4 py-3 text-left font-medium hover:underline" style="color: var(--color-muted); background-color: var(--color-surface)" onclick={() => sort('created_at')}>
						Created {sortKey === 'created_at' ? (sortAsc ? '↑' : '↓') : ''}
					</th>
				</tr>
			</thead>
			<tbody>
				{#each tickets as ticket}
					<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs bg-black/5 dark:bg-white/10">{ticket.type}</span>
						</td>
						<td class="px-4 py-3 font-medium">{ticket.subject}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {ticket.status === 'open' ? 'bg-amber-500/15 text-amber-700 dark:text-amber-400' : ticket.status === 'resolved' ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : 'bg-slate-500/15 text-slate-700 dark:text-slate-400'}">
								{ticket.status}
							</span>
						</td>
						<td class="px-4 py-3">
							<span class="text-xs font-medium {ticket.priority === 'urgent' ? 'text-red-600 dark:text-red-400' : ticket.priority === 'high' ? 'text-amber-600 dark:text-amber-400' : ''}">
								{ticket.priority}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{ticket.assignee || '—'}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{ticket.created_at}</td>
					</tr>
				{:else}
					<tr>
						<td colspan="6" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							No tickets found
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
