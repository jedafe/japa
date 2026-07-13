<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import type { PageData } from './$types';

	type AuditEntry = {
		id: string;
		actor_id: string;
		action_type: string;
		target_type: string;
		target_id: string;
		created_at: string;
	};

	let { data = {} as PageData } = $props();

	// svelte-ignore state_referenced_locally
	let entries = $state<AuditEntry[]>(data.initial?.entries ?? []);
	let page = $state(0);
	// svelte-ignore state_referenced_locally
	let totalPages = $state(data.initial?.total_pages ?? 1);
	let loading = $state(false);
	// svelte-ignore state_referenced_locally
	let apiAvailable = $state(data.initial !== null);

	// Filters
	let filterActor = $state('');
	let filterAction = $state('');
	let filterTarget = $state('');
	let filterFrom = $state('');
	let filterTo = $state('');

	const columns = [
		{ key: 'created_at', label: 'Timestamp' },
		{ key: 'actor_id', label: 'Actor' },
		{ key: 'action_type', label: 'Action' },
		{ key: 'target_type', label: 'Target Type' },
		{ key: 'target_id', label: 'Target ID' }
	];

	function buildQuery() {
		const params = new URLSearchParams({ page: String(page) });
		if (filterActor) params.set('actor', filterActor);
		if (filterAction) params.set('action_type', filterAction);
		if (filterTarget) params.set('target_type', filterTarget);
		if (filterFrom) params.set('from', filterFrom);
		if (filterTo) params.set('to', filterTo);
		return params.toString();
	}

	async function load() {
		loading = true;
		try {
			const result = await api<{
				entries: AuditEntry[];
				page: number;
				total_pages: number;
			}>(`/control/audit-logs?${buildQuery()}`);
			if (result.ok) {
				entries = result.data.entries;
				totalPages = result.data.total_pages;
				apiAvailable = true;
			} else {
				apiAvailable = false;
			}
		} catch {
			apiAvailable = false;
		} finally {
			loading = false;
		}
	}

	function applyFilters() {
		page = 0;
		load();
	}

	function handlePageChange(p: number) {
		page = p;
		load();
	}
</script>

<PageHeader title={t('audit.title')} description={t('audit.description')} />

{#if !apiAvailable}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-warning); background-color: color-mix(in srgb, var(--color-warning) 8%, transparent); color: var(--color-warning)">
		Audit log API is not available.
	</div>
{/if}

<!-- Filters -->
<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
	<input
		type="text"
		placeholder="Actor ID"
		class="rounded-md border px-3 py-1.5 text-sm"
		style="border-color: var(--color-border); background-color: var(--color-surface)"
		bind:value={filterActor}
	/>
	<input
		type="text"
		placeholder="Action type"
		class="rounded-md border px-3 py-1.5 text-sm"
		style="border-color: var(--color-border); background-color: var(--color-surface)"
		bind:value={filterAction}
	/>
	<input
		type="text"
		placeholder="Target type"
		class="rounded-md border px-3 py-1.5 text-sm"
		style="border-color: var(--color-border); background-color: var(--color-surface)"
		bind:value={filterTarget}
	/>
	<input
		type="date"
		class="rounded-md border px-3 py-1.5 text-sm"
		style="border-color: var(--color-border); background-color: var(--color-surface)"
		bind:value={filterFrom}
	/>
	<input
		type="date"
		class="rounded-md border px-3 py-1.5 text-sm"
		style="border-color: var(--color-border); background-color: var(--color-surface)"
		bind:value={filterTo}
	/>
</div>
<div class="mb-4">
	<button
		class="rounded-md border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
		style="border-color: var(--color-border)"
		onclick={applyFilters}
	>
		Apply Filters
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else}
	<DataTable {columns} rows={entries} {page} {totalPages} onPageChange={handlePageChange} />
{/if}
