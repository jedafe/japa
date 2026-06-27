<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import MetricCard from '$lib/components/MetricCard.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import { api } from '$lib/api';

	type QueueInfo = {
		tier: string;
		depth: number;
		processing: number;
	};

	type Task = {
		id: string;
		type: string;
		tier: string;
		status: string;
		created_at: string;
	};

	let queues = $state<QueueInfo[]>([
		{ tier: 'critical', depth: 0, processing: 0 },
		{ tier: 'default', depth: 0, processing: 0 },
		{ tier: 'low', depth: 0, processing: 0 }
	]);
	let deadLetterDepth = $state(0);
	let tasks = $state<Task[]>([]);
	let taskPage = $state(0);
	let taskTotalPages = $state(1);
	let loading = $state(true);

	const columns = [
		{ key: 'id', label: 'ID' },
		{ key: 'type', label: 'Type' },
		{ key: 'tier', label: 'Tier' },
		{ key: 'status', label: 'Status' },
		{ key: 'created_at', label: 'Created' }
	];

	async function load() {
		try {
			const data = await api<{
				queues: QueueInfo[];
				dead_letter_depth: number;
				tasks: Task[];
				page: number;
				total_pages: number;
			}>(`/control/queue?page=${taskPage}`);
			queues = data.queues;
			deadLetterDepth = data.dead_letter_depth;
			tasks = data.tasks;
			taskTotalPages = data.total_pages;
		} catch {
			// API not available
		} finally {
			loading = false;
		}
	}

	function handlePageChange(p: number) {
		taskPage = p;
		load();
	}

	let interval: ReturnType<typeof setInterval>;
	onMount(() => {
		load();
		interval = setInterval(load, 15_000);
		return () => clearInterval(interval);
	});
</script>

<PageHeader title="Queue Monitor" description="Asynq task queue depth and recent history" />

<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
	{#each queues as q}
		<MetricCard label="{q.tier} queue" value={q.depth} subtitle="{q.processing} processing" />
	{/each}
	<MetricCard label="Dead Letter" value={deadLetterDepth} subtitle="unrecoverable tasks" />
</div>

<h2 class="mb-3 text-lg font-semibold">Recent Tasks</h2>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else}
	<DataTable {columns} rows={tasks} page={taskPage} totalPages={taskTotalPages} onPageChange={handlePageChange} />
{/if}
