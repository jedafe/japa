<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import MetricCard from '$lib/components/MetricCard.svelte';
	import { api } from '$lib/api';

	type HealthGroup = {
		name: string;
		items: { name: string; status: 'healthy' | 'warning' | 'critical' | 'unknown' }[];
	};

	let groups = $state<HealthGroup[]>([
		{
			name: 'Infrastructure',
			items: [
				{ name: 'PostgreSQL', status: 'healthy' },
				{ name: 'Valkey', status: 'healthy' },
				{ name: 'Meilisearch', status: 'healthy' },
				{ name: 'MinIO', status: 'healthy' }
			]
		},
		{
			name: 'Application',
			items: [
				{ name: 'API Server', status: 'healthy' },
				{ name: 'imgproxy', status: 'healthy' },
				{ name: 'Nginx', status: 'healthy' }
			]
		},
		{
			name: 'Background Processing',
			items: [
				{ name: 'Asynq Workers', status: 'healthy' },
				{ name: 'Queue Depth', status: 'healthy' }
			]
		},
		{
			name: 'Real-time',
			items: [
				{ name: 'WebSocket Hub', status: 'healthy' },
				{ name: 'SSE Connections', status: 'healthy' }
			]
		}
	]);

	let throughput = $state('—');
	let latency = $state('—');
	let errorRate = $state('—');
	let queueDepth = $state('—');
	let wsConnections = $state(0);
	let lastRefresh = $state(new Date().toLocaleTimeString());

	let apiAvailable = $state(true);

	async function refresh() {
		try {
			const result = await api<{
				health: HealthGroup[];
				metrics: { throughput: string; latency: string; errorRate: string; queueDepth: string; wsConnections: number };
			}>('/admin/analytics/overview');
			if (result.ok) {
				groups = result.data.health;
				throughput = result.data.metrics.throughput;
				latency = result.data.metrics.latency;
				errorRate = result.data.metrics.errorRate;
				queueDepth = result.data.metrics.queueDepth;
				wsConnections = result.data.metrics.wsConnections;
				lastRefresh = new Date().toLocaleTimeString();
				apiAvailable = true;
			} else {
				apiAvailable = false;
			}
		} catch {
			apiAvailable = false;
		}
	}

	let interval: ReturnType<typeof setInterval>;
	onMount(() => {
		refresh();
		interval = setInterval(refresh, 30_000);
		return () => clearInterval(interval);
	});
</script>

<PageHeader title="Dashboard" description="System health overview with live polling" />

{#if !apiAvailable}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-warning); background-color: color-mix(in srgb, var(--color-warning) 8%, transparent); color: var(--color-warning)">
		Analytics API is not available. Showing default health state.
	</div>
{/if}

<div class="mb-4 flex items-center justify-between">
	<p class="text-xs" style="color: var(--color-muted)">Last refresh: {lastRefresh}</p>
	<button
		class="rounded-md border px-3 py-1.5 text-xs font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
		style="border-color: var(--color-border)"
		onclick={refresh}
	>
		Refresh
	</button>
</div>

<!-- Metrics row -->
<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
	<MetricCard label="Throughput" value={throughput} subtitle="req/s" />
	<MetricCard label="Latency p99" value={latency} subtitle="ms" />
	<MetricCard label="Error Rate" value={errorRate} subtitle="%" />
	<MetricCard label="Queue Depth" value={queueDepth} subtitle="pending tasks" />
	<MetricCard label="WS Connections" value={wsConnections} subtitle="active" />
</div>

<!-- Health groups -->
<div class="grid gap-4 sm:grid-cols-2">
	{#each groups as group}
		<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<h3 class="mb-3 text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{group.name}</h3>
			<div class="space-y-2">
				{#each group.items as item}
					<div class="flex items-center justify-between">
						<span class="text-sm">{item.name}</span>
						<StatusBadge status={item.status} />
					</div>
				{/each}
			</div>
		</div>
	{/each}
</div>
