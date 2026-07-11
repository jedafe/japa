<script lang="ts">
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

	let { data = {} as any } = $props();

	let results = $state<ScanResult[]>(data.initial?.results ?? []);
	let filterSeverity = $state<string>('all');
	let page = $state(0);
	let totalPages = $state(data.initial?.total_pages ?? 1);
	let loading = $state(false);

	const columns = [
		{ key: 'severity', label: 'Severity' },
		{ key: 'title', label: 'Title' },
		{ key: 'description', label: 'Description' },
		{ key: 'created_at', label: 'Found' },
		{ key: 'resolved', label: 'Status' }
	];

	let apiAvailable = $state(data.initial !== null);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		const sev = filterSeverity !== 'all' ? `&severity=${filterSeverity}` : '';
		try {
			const result = await api<{
				results: ScanResult[];
				page: number;
				total_pages: number;
			}>(`/control/security?page=${page}${sev}`);
			if (result.ok) {
				results = result.data.results;
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

	async function resolveFinding(id: string) {
		errorMessage = '';
		try {
			await api(`/control/security/${id}/resolve`, { method: 'POST' });
			await load();
		} catch {
			errorMessage = 'Failed to resolve finding. Please try again.';
		}
	}

	function handlePageChange(p: number) {
		page = p;
		load();
	}
</script>

<PageHeader title="Security" description="Scan findings and security posture" />

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

{#if !apiAvailable}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-warning); background-color: color-mix(in srgb, var(--color-warning) 8%, transparent); color: var(--color-warning)">
		Security analytics API is not available.
	</div>
{/if}

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
