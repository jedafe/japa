<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import { api } from '$lib/api';

	type ScanResult = {
		id: string;
		severity: 'info' | 'warning' | 'critical';
		title: string;
		description: string;
		created_at: string;
		resolved: boolean;
	};

	let results = $state<ScanResult[]>([]);
	let filterSeverity = $state<string>('all');
	let page = $state(0);
	let totalPages = $state(1);
	let loading = $state(true);

	const columns = [
		{ key: 'severity', label: 'Severity' },
		{ key: 'title', label: 'Title' },
		{ key: 'description', label: 'Description' },
		{ key: 'created_at', label: 'Found' },
		{ key: 'resolved', label: 'Status' }
	];

	async function load() {
		loading = true;
		const sev = filterSeverity !== 'all' ? `&severity=${filterSeverity}` : '';
		try {
			const data = await api<{
				results: ScanResult[];
				page: number;
				total_pages: number;
			}>(`/control/security?page=${page}${sev}`);
			results = data.results;
			totalPages = data.total_pages;
		} catch {
			// API not available
		} finally {
			loading = false;
		}
	}

	async function resolveFinding(id: string) {
		try {
			await api(`/control/security/${id}/resolve`, { method: 'POST' });
			await load();
		} catch {
			// ignore
		}
	}

	function handlePageChange(p: number) {
		page = p;
		load();
	}

	onMount(load);
</script>

<PageHeader title="Security" description="Scan findings and security posture" />

<div class="mb-4 flex items-center gap-3">
	<label for="severity-filter" class="text-xs font-medium" style="color: var(--color-muted)">Filter severity:</label>
	<select
		id="severity-filter"
		class="rounded-md border px-3 py-1.5 text-sm"
		style="border-color: var(--color-border); background-color: var(--color-surface)"
		bind:value={filterSeverity}
		onchange={() => { page = 0; load(); }}
	>
		<option value="all">All</option>
		<option value="info">Info</option>
		<option value="warning">Warning</option>
		<option value="critical">Critical</option>
	</select>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else}
	<DataTable {columns} rows={results} {page} {totalPages} onPageChange={handlePageChange} />
{/if}
