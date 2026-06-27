<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import { api } from '$lib/api';

	type TableBloat = {
		table_name: string;
		dead_tuples: number;
		live_tuples: number;
		bloat_pct: number;
	};

	type LongQuery = {
		pid: number;
		query: string;
		duration: string;
		state: string;
	};

	let tables = $state<TableBloat[]>([]);
	let longQueries = $state<LongQuery[]>([]);
	let slowThreshold = $state(1000);
	let vacuumin = $state(false);
	let vacuumMsg = $state('');

	const tableCols = [
		{ key: 'table_name', label: 'Table' },
		{ key: 'live_tuples', label: 'Live Tuples' },
		{ key: 'dead_tuples', label: 'Dead Tuples' },
		{ key: 'bloat_pct', label: 'Bloat %' }
	];

	const queryCols = [
		{ key: 'pid', label: 'PID' },
		{ key: 'query', label: 'Query' },
		{ key: 'duration', label: 'Duration' },
		{ key: 'state', label: 'State' }
	];

	async function load() {
		try {
			const data = await api<{
				tables: TableBloat[];
				long_queries: LongQuery[];
			}>(`/control/db?slow_threshold=${slowThreshold}`);
			tables = data.tables;
			longQueries = data.long_queries;
		} catch {
			// API not available
		}
	}

	async function triggerVacuum() {
		vacuumin = true;
		vacuumMsg = '';
		try {
			await api('/control/db/vacuum', { method: 'POST' });
			vacuumMsg = 'Vacuum queued successfully';
			setTimeout(load, 3000);
		} catch {
			vacuumMsg = 'Failed to queue vacuum';
		} finally {
			vacuumin = false;
		}
	}

	let interval: ReturnType<typeof setInterval>;
	onMount(() => {
		load();
		interval = setInterval(load, 30_000);
		return () => clearInterval(interval);
	});
</script>

<PageHeader title="DB Health" description="Table bloat, dead tuples, and long-running queries" />

<!-- Controls -->
<div class="mb-6 flex flex-wrap items-end gap-4">
	<div>
		<label for="slow-threshold" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Slow query threshold (ms)</label>
		<input
			id="slow-threshold"
			type="number"
			class="w-32 rounded-md border px-3 py-1.5 text-sm"
			style="border-color: var(--color-border); background-color: var(--color-surface)"
			bind:value={slowThreshold}
			onchange={load}
		/>
	</div>
	<button
		class="rounded-md border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-50"
		style="border-color: var(--color-border)"
		disabled={vacuumin}
		onclick={triggerVacuum}
	>
		{vacuumin ? 'Queuing...' : 'Trigger VACUUM'}
	</button>
	{#if vacuumMsg}
		<span class="text-xs" style="color: var(--color-muted)">{vacuumMsg}</span>
	{/if}
</div>

<!-- Table bloat -->
<h2 class="mb-3 text-lg font-semibold">Table Bloat</h2>
<div class="mb-8">
	<DataTable columns={tableCols} rows={tables} />
</div>

<!-- Long queries -->
<h2 class="mb-3 text-lg font-semibold">Long-Running Queries</h2>
{#if longQueries.length === 0}
	<p class="py-4 text-sm" style="color: var(--color-muted)">No long-running queries detected</p>
{:else}
	<DataTable columns={queryCols} rows={longQueries} />
{/if}
