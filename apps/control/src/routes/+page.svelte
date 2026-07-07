<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import MetricCard from '$lib/components/MetricCard.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { ListTodo, Database, ArrowRight } from 'lucide-svelte';

	type HealthGroup = {
		name: string;
		items: { name: string; status: 'healthy' | 'warning' | 'critical' | 'unknown' }[];
	};

	type QueueSummary = {
		total_pending: number;
		total_active: number;
		total_failed_24h: number;
	};

	type DbSummary = {
		connected: boolean;
		size_bytes: number;
		slow_queries_count: number;
		active_connections: number;
		max_connections: number;
	};

	let { data = {} as any } = $props();

	function applyOverview(overview: {
		health: HealthGroup[];
		metrics: { throughput: string; latency: string; errorRate: string; queueDepth: string; wsConnections: number };
		queues?: QueueSummary;
		database?: {
			connected: boolean;
			size_bytes: number;
			slow_queries?: unknown[];
			connection_pool?: { active: number; max: number };
		};
	}) {
		groups = overview.health;
		throughput = overview.metrics.throughput;
		latency = overview.metrics.latency;
		errorRate = overview.metrics.errorRate;
		queueDepth = overview.metrics.queueDepth;
		wsConnections = overview.metrics.wsConnections;
		lastRefresh = new Date().toLocaleTimeString();
		apiAvailable = true;

		if (overview.queues) {
			queueSummary = overview.queues;
		}

		if (overview.database) {
			const db = overview.database;
			dbSummary = {
				connected: db.connected,
				size_bytes: db.size_bytes,
				slow_queries_count: db.slow_queries?.length ?? 0,
				active_connections: db.connection_pool?.active ?? 0,
				max_connections: db.connection_pool?.max ?? 0
			};
		}
	}

	let groups = $state<HealthGroup[]>([
		{
			name: t('dashboard.groups.infra'),
			items: [
				{ name: 'PostgreSQL', status: 'unknown' },
				{ name: 'Valkey', status: 'unknown' },
				{ name: 'Meilisearch', status: 'unknown' },
				{ name: 'SeaweedFS', status: 'unknown' }
			]
		},
		{
			name: t('dashboard.groups.app'),
			items: [
				{ name: 'API Server', status: 'unknown' },
				{ name: 'imgproxy', status: 'unknown' },
				{ name: 'Nginx', status: 'unknown' }
			]
		},
		{
			name: t('dashboard.groups.background'),
			items: [
				{ name: 'Asynq Workers', status: 'unknown' },
				{ name: 'Queue Depth', status: 'unknown' }
			]
		},
		{
			name: t('dashboard.groups.realtime'),
			items: [
				{ name: 'WebSocket Hub', status: 'unknown' },
				{ name: 'SSE Connections', status: 'unknown' }
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

	// Queue & DB summary data
	let queueSummary = $state<QueueSummary | null>(null);
	let dbSummary = $state<DbSummary | null>(null);

	// Apply server-provided initial data
	if (data.initial) {
		applyOverview(data.initial);
	}

	function formatBytes(bytes: number): string {
		if (bytes >= 1_073_741_824) return (bytes / 1_073_741_824).toFixed(1) + ' GB';
		if (bytes >= 1_048_576) return (bytes / 1_048_576).toFixed(1) + ' MB';
		if (bytes >= 1024) return (bytes / 1024).toFixed(1) + ' KB';
		return bytes + ' B';
	}

	function formatNumber(n: number): string {
		if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
		if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K';
		return n.toLocaleString();
	}

	async function refresh() {
		try {
			const result = await api<{
				health: HealthGroup[];
				metrics: { throughput: string; latency: string; errorRate: string; queueDepth: string; wsConnections: number };
				queues?: QueueSummary;
				database?: {
					connected: boolean;
					size_bytes: number;
					slow_queries?: unknown[];
					connection_pool?: { active: number; max: number };
				};
			}>('/admin/analytics/overview');
			if (result.ok) {
				applyOverview(result.data);
			} else {
				apiAvailable = false;
			}
		} catch {
			apiAvailable = false;
		}
	}

	let interval: ReturnType<typeof setInterval>;
	onMount(() => {
		interval = setInterval(refresh, 30_000);
		return () => clearInterval(interval);
	});
</script>

<PageHeader title={t('dashboard.title')} description={t('dashboard.description')} />

{#if !apiAvailable}
	<div class="animate-fade-in mb-4 rounded-xl border px-4 py-3 text-sm" style="border-color: var(--color-warning); background-color: color-mix(in srgb, var(--color-warning) 8%, transparent); color: var(--color-warning)">
		{t('dashboard.analytics_warning')}
	</div>
{/if}

<div class="mb-4 flex items-center justify-between">
	<p class="text-xs font-medium" style="color: var(--color-muted)">{t('dashboard.last_refresh').replace('{time}', lastRefresh)}</p>
	<button
		class="btn btn-secondary text-xs"
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
	{#each groups as group, i}
		<div
			class="rounded-2xl border p-5"
			style="border-color: var(--color-border); background: var(--gradient-surface); animation: slide-up 0.3s ease-out {i * 50}ms both"
		>
			<h3 class="mb-4 text-xs font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{group.name}</h3>
			<div class="space-y-2.5">
				{#each group.items as item}
					<div class="flex items-center justify-between">
						<span class="text-sm font-medium" style="color: var(--color-foreground)">{item.name}</span>
						<StatusBadge status={item.status} />
					</div>
				{/each}
			</div>
		</div>
	{/each}
</div>

<!-- Queue & DB Summary -->
<div class="mt-6 grid gap-4 sm:grid-cols-2">
	<!-- Queue summary card -->
	<div class="rounded-2xl border p-5" style="border-color: var(--color-border); background: var(--gradient-surface)">
		<div class="mb-4 flex items-center justify-between">
			<div class="flex items-center gap-2">
				<div
					class="flex h-8 w-8 items-center justify-center rounded-lg"
					style="background: color-mix(in srgb, var(--color-primary) 10%, transparent)"
				>
					<ListTodo class="h-4 w-4" style="color: var(--color-primary)" />
				</div>
				<h3 class="text-sm font-semibold">{t('dashboard.queue_summary')}</h3>
			</div>
			<a href="/queue" class="flex items-center gap-1 text-xs font-medium transition-colors hover:opacity-80" style="color: var(--color-primary)">
				{t('dashboard.view_queue')}
			</a>
		</div>
		<div class="grid grid-cols-3 gap-3">
			<div>
				<p class="text-[10px] font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('dashboard.queue_pending')}</p>
				<p class="mt-1 text-xl font-bold">{queueSummary ? formatNumber(queueSummary.total_pending) : '—'}</p>
			</div>
			<div>
				<p class="text-[10px] font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('dashboard.queue_active')}</p>
				<p class="mt-1 text-xl font-bold">{queueSummary ? formatNumber(queueSummary.total_active) : '—'}</p>
			</div>
			<div>
				<p class="text-[10px] font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('dashboard.queue_failed')}</p>
				<p class="mt-1 text-xl font-bold" style:color={(queueSummary?.total_failed_24h ?? 0) > 0 ? 'var(--color-danger)' : undefined}>
					{queueSummary ? formatNumber(queueSummary.total_failed_24h) : '—'}
				</p>
			</div>
		</div>
	</div>

	<!-- DB summary card -->
	<div class="rounded-2xl border p-5" style="border-color: var(--color-border); background: var(--gradient-surface)">
		<div class="mb-4 flex items-center justify-between">
			<div class="flex items-center gap-2">
				<div
					class="flex h-8 w-8 items-center justify-center rounded-lg"
					style="background: color-mix(in srgb, var(--color-primary) 10%, transparent)"
				>
					<Database class="h-4 w-4" style="color: var(--color-primary)" />
				</div>
				<h3 class="text-sm font-semibold">{t('dashboard.db_summary')}</h3>
			</div>
			<a href="/db" class="flex items-center gap-1 text-xs font-medium transition-colors hover:opacity-80" style="color: var(--color-primary)">
				{t('dashboard.view_db')}
			</a>
		</div>
		<div class="grid grid-cols-3 gap-3">
			<div>
				<p class="text-[10px] font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('dashboard.db_status')}</p>
				<div class="mt-1.5">
					{#if dbSummary}
						<StatusBadge status={dbSummary.connected ? 'healthy' : 'critical'}>
							{dbSummary.connected ? 'Connected' : 'Down'}
						</StatusBadge>
					{:else}
						<StatusBadge status="unknown">—</StatusBadge>
					{/if}
				</div>
			</div>
			<div>
				<p class="text-[10px] font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('dashboard.db_size')}</p>
				<p class="mt-1 text-xl font-bold">{dbSummary ? formatBytes(dbSummary.size_bytes) : '—'}</p>
			</div>
			<div>
				<p class="text-[10px] font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('dashboard.db_connections')}</p>
				<p class="mt-1 text-xl font-bold">
					{#if dbSummary}
						<span style:color={dbSummary.max_connections > 0 && (dbSummary.active_connections / dbSummary.max_connections) > 0.8 ? 'var(--color-danger)' : undefined}>
							{dbSummary.active_connections}/{dbSummary.max_connections}
						</span>
					{:else}
						—
					{/if}
				</p>
			</div>
		</div>
	</div>
</div>
