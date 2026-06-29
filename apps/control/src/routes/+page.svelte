<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import MetricCard from '$lib/components/MetricCard.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type HealthGroup = {
		name: string;
		items: { name: string; status: 'healthy' | 'warning' | 'critical' | 'unknown' }[];
	};

	let groups = $state<HealthGroup[]>([
		{
			name: t('dashboard.groups.infra'),
			items: [
				{ name: 'PostgreSQL', status: 'healthy' },
				{ name: 'Valkey', status: 'healthy' },
				{ name: 'Meilisearch', status: 'healthy' },
				{ name: 'MinIO', status: 'healthy' }
			]
		},
		{
			name: t('dashboard.groups.app'),
			items: [
				{ name: 'API Server', status: 'healthy' },
				{ name: 'imgproxy', status: 'healthy' },
				{ name: 'Nginx', status: 'healthy' }
			]
		},
		{
			name: t('dashboard.groups.background'),
			items: [
				{ name: 'Asynq Workers', status: 'healthy' },
				{ name: 'Queue Depth', status: 'healthy' }
			]
		},
		{
			name: t('dashboard.groups.realtime'),
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

<PageHeader title={t('dashboard.title')} description={t('dashboard.description')} />

{#if !apiAvailable}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-warning); background-color: color-mix(in srgb, var(--color-warning) 8%, transparent); color: var(--color-warning)">
		{t('dashboard.analytics_warning')}
	</div>
{/if}

<div class="mb-4 flex items-center justify-between">
	<p class="text-xs" style="color: var(--color-muted)">{t('dashboard.last_refresh').replace('{time}', lastRefresh)}</p>
	<button
		class="rounded-md border px-3 py-1.5 text-xs font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
		style="border-color: var(--color-border)"
		onclick={refresh}
	>
		{t('common.refresh')}
	</button>
</div>

<!-- Metrics row -->
<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
	<MetricCard label={t('dashboard.metrics.throughput')} value={throughput} subtitle={t('dashboard.units.req_s')} />
	<MetricCard label={t('dashboard.metrics.latency')} value={latency} subtitle={t('dashboard.units.ms')} />
	<MetricCard label={t('dashboard.metrics.error_rate')} value={errorRate} subtitle={t('dashboard.units.percent')} />
	<MetricCard label={t('dashboard.metrics.queue_depth')} value={queueDepth} subtitle={t('dashboard.units.pending')} />
	<MetricCard label={t('dashboard.metrics.ws_connections')} value={wsConnections} subtitle={t('dashboard.units.active')} />
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
