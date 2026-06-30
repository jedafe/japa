<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import MetricCard from '$lib/components/MetricCard.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { api, type DbHealth, type TableStat, type SlowQuery } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import {
		Database,
		RefreshCw,
		HardDrive,
		Table2,
		Clock,
		AlertTriangle,
		Server,
		Zap,
		Activity
	} from 'lucide-svelte';

	let dbHealth = $state<DbHealth | null>(null);
	let loading = $state(true);
	let apiAvailable = $state(true);
	let lastRefresh = $state(new Date());
	let secondsAgo = $state(0);
	let refreshing = $state(false);

	interface OverviewResponse {
		database?: DbHealth;
	}

	async function refresh() {
		refreshing = true;
		try {
			const result = await api<OverviewResponse>('/admin/analytics/overview');
			if (result.ok) {
				dbHealth = result.data.database ?? null;
				apiAvailable = true;
				lastRefresh = new Date();
				secondsAgo = 0;
			} else {
				apiAvailable = false;
			}
		} catch {
			apiAvailable = false;
		} finally {
			loading = false;
			refreshing = false;
		}
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

	function formatDuration(ms: number): string {
		if (ms >= 1000) return (ms / 1000).toFixed(2) + 's';
		return ms.toFixed(0) + 'ms';
	}

	function formatTimestamp(ts: string): string {
		return new Date(ts).toLocaleString();
	}

	function formatVacuum(v: string | null): string {
		if (!v) return 'Never';
		return new Date(v).toLocaleDateString();
	}

	function poolUsagePercent(): number {
		if (!dbHealth?.connection_pool) return 0;
		const pool = dbHealth.connection_pool;
		return pool.max > 0 ? Math.round(((pool.active + pool.waiting) / pool.max) * 100) : 0;
	}

	function hasPoolWarning(): boolean {
		return poolUsagePercent() > 80;
	}

	function hasSlowQueryWarning(): boolean {
		return (dbHealth?.slow_queries?.length ?? 0) > 0 &&
			(dbHealth?.slow_queries?.some(q => q.duration_ms > 1000) ?? false);
	}

	function hasReplicationWarning(): boolean {
		return (dbHealth?.replication?.lag_seconds ?? 0) > 5;
	}

	onMount(() => {
		refresh();

		const interval = setInterval(() => {
			if (document.visibilityState === 'visible') {
				refresh();
			}
		}, 60_000);

		const tickInterval = setInterval(() => {
			secondsAgo = Math.floor((Date.now() - lastRefresh.getTime()) / 1000);
		}, 1000);

		return () => {
			clearInterval(interval);
			clearInterval(tickInterval);
		};
	});
</script>

<svelte:window onvisibilitychange={() => {
	if (document.visibilityState === 'visible') {
		refresh();
	}
}} />

<PageHeader title={t('db.title')} description={t('db.description')} />

{#if !apiAvailable}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-warning); background-color: color-mix(in srgb, var(--color-warning) 8%, transparent); color: var(--color-warning)">
		{t('db.unavailable')}
	</div>
{/if}

<!-- Alert banners -->
{#if hasPoolWarning()}
	<div class="mb-3 flex items-center gap-2 rounded-md border px-4 py-2.5 text-sm" style="border-color: var(--color-warning); background-color: color-mix(in srgb, var(--color-warning) 8%, transparent); color: var(--color-warning)">
		<AlertTriangle class="h-4 w-4 flex-shrink-0" />
		{t('db.pool_warning')} ({poolUsagePercent()}%)
	</div>
{/if}
{#if hasSlowQueryWarning()}
	<div class="mb-3 flex items-center gap-2 rounded-md border px-4 py-2.5 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		<AlertTriangle class="h-4 w-4 flex-shrink-0" />
		{t('db.slow_query_warning')}
	</div>
{/if}
{#if hasReplicationWarning()}
	<div class="mb-3 flex items-center gap-2 rounded-md border px-4 py-2.5 text-sm" style="border-color: var(--color-warning); background-color: color-mix(in srgb, var(--color-warning) 8%, transparent); color: var(--color-warning)">
		<AlertTriangle class="h-4 w-4 flex-shrink-0" />
		{t('db.replication_lag_warning')} ({dbHealth?.replication?.lag_seconds.toFixed(1)}s)
	</div>
{/if}

<!-- Refresh indicator -->
<div class="mb-4 flex items-center justify-between">
	<div class="flex items-center gap-3">
		<p class="text-xs" style="color: var(--color-muted)">
			{t('db.last_updated').replace('{time}', lastRefresh.toLocaleTimeString())}
			<span class="ml-2">({secondsAgo}s ago)</span>
		</p>
		<span class="rounded-full px-2 py-0.5 text-[10px] font-medium" style="background-color: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)">
			{t('db.auto_refresh')}
		</span>
	</div>
	<button
		class="flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-50"
		style="border-color: var(--color-border)"
		disabled={refreshing}
		onclick={refresh}
	>
		<RefreshCw class="h-3 w-3 {refreshing ? 'animate-spin' : ''}" />
		{t('common.refresh')}
	</button>
</div>

{#if loading}
	<!-- Loading skeletons -->
	<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
		{#each Array(4) as _}
			<div class="h-24 animate-pulse rounded-lg border" style="border-color: var(--color-border); background-color: var(--color-surface)"></div>
		{/each}
	</div>
	<div class="h-64 animate-pulse rounded-xl border" style="border-color: var(--color-border); background-color: var(--color-surface)"></div>
{:else if dbHealth}
	<!-- Top metrics -->
	<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
		<MetricCard
			label={t('db.db_size')}
			value={formatBytes(dbHealth.size_bytes)}
		/>
		<MetricCard
			label={t('db.table_count')}
			value={dbHealth.table_count}
		/>
		<MetricCard
			label={t('db.index_count')}
			value={dbHealth.index_count}
		/>
		<MetricCard
			label={t('db.avg_query_time')}
			value={formatDuration(dbHealth.avg_query_time_ms)}
		/>
	</div>

	<!-- Connection & Version status -->
	<div class="mb-6 grid gap-4 sm:grid-cols-2">
		<!-- Connection status card -->
		<div class="rounded-xl border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<div class="mb-4 flex items-center gap-2">
				<Server class="h-4 w-4" style="color: var(--color-primary)" />
				<h3 class="text-sm font-semibold">{t('db.connection_status')}</h3>
			</div>
			<div class="space-y-3">
				<div class="flex items-center justify-between">
					<span class="text-sm" style="color: var(--color-muted)">Status</span>
					<StatusBadge status={dbHealth.connected ? 'healthy' : 'critical'}>
						{dbHealth.connected ? t('db.connected') : t('db.disconnected')}
					</StatusBadge>
				</div>
				<div class="flex items-center justify-between">
					<span class="text-sm" style="color: var(--color-muted)">{t('db.version')}</span>
					<span class="text-sm font-mono">{dbHealth.version || '—'}</span>
				</div>
				<div class="flex items-center justify-between">
					<span class="text-sm" style="color: var(--color-muted)">{t('db.db_latency')}</span>
					<span class="text-sm font-mono">{dbHealth.latency_ms?.toFixed(1) ?? '—'}ms</span>
				</div>
			</div>
		</div>

		<!-- Connection pool card -->
		<div class="rounded-xl border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<div class="mb-4 flex items-center gap-2">
				<Activity class="h-4 w-4" style="color: var(--color-primary)" />
				<h3 class="text-sm font-semibold">{t('db.connection_pool')}</h3>
			</div>
			<div class="space-y-3">
				<div class="flex items-center justify-between">
					<span class="text-sm" style="color: var(--color-muted)">{t('db.active_connections')}</span>
					<span class="text-sm font-mono">{dbHealth.connection_pool?.active ?? 0}</span>
				</div>
				<div class="flex items-center justify-between">
					<span class="text-sm" style="color: var(--color-muted)">{t('db.idle_connections')}</span>
					<span class="text-sm font-mono">{dbHealth.connection_pool?.idle ?? 0}</span>
				</div>
				<div class="flex items-center justify-between">
					<span class="text-sm" style="color: var(--color-muted)">{t('db.waiting_connections')}</span>
					<span class="text-sm font-mono" style:color={(dbHealth.connection_pool?.waiting ?? 0) > 0 ? 'var(--color-warning)' : undefined}>
						{dbHealth.connection_pool?.waiting ?? 0}
					</span>
				</div>
				<div class="flex items-center justify-between">
					<span class="text-sm" style="color: var(--color-muted)">{t('db.max_connections')}</span>
					<span class="text-sm font-mono">{dbHealth.connection_pool?.max ?? 0}</span>
				</div>
				<!-- Pool usage bar -->
				<div class="pt-1">
					<div class="flex items-center justify-between text-xs" style="color: var(--color-muted)">
						<span>Usage</span>
						<span>{poolUsagePercent()}%</span>
					</div>
					<div class="mt-1 h-1.5 w-full overflow-hidden rounded-full" style="background-color: var(--color-border)">
						<div
							class="h-full rounded-full transition-all"
							style="width: {poolUsagePercent()}%; background-color: {poolUsagePercent() > 80 ? 'var(--color-danger)' : poolUsagePercent() > 60 ? 'var(--color-warning)' : 'var(--color-success)'}"
						></div>
					</div>
				</div>
			</div>
		</div>
	</div>

	<!-- Table statistics -->
	<div class="mb-6 overflow-hidden rounded-xl border" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<div class="border-b px-6 py-4" style="border-color: var(--color-border)">
			<div class="flex items-center gap-2">
				<Table2 class="h-4 w-4" style="color: var(--color-primary)" />
				<h3 class="text-sm font-semibold">{t('db.table_stats')}</h3>
			</div>
		</div>
		<div class="overflow-x-auto">
			<table class="w-full text-sm">
				<thead>
					<tr style="border-bottom: 1px solid var(--color-border)">
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted)">{t('db.table_name')}</th>
						<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted)">{t('db.row_count')}</th>
						<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted)">{t('db.size')}</th>
						<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted)">{t('db.indexes')}</th>
						<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted)">{t('db.last_vacuum')}</th>
					</tr>
				</thead>
				<tbody>
					{#if dbHealth.tables && dbHealth.tables.length > 0}
						{#each dbHealth.tables as table (table.name)}
							<tr class="border-t" style="border-color: var(--color-border)">
								<td class="px-4 py-3 font-medium">{table.name}</td>
								<td class="px-4 py-3 text-right font-mono text-xs">{formatNumber(table.row_count)}</td>
								<td class="px-4 py-3 text-right font-mono text-xs">{formatBytes(table.size_bytes)}</td>
								<td class="px-4 py-3 text-right font-mono text-xs">{table.index_count}</td>
								<td class="px-4 py-3 text-right text-xs" style="color: var(--color-muted)">
									{formatVacuum(table.last_vacuum)}
								</td>
							</tr>
						{/each}
					{:else}
						<tr>
							<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
								No table statistics available
							</td>
						</tr>
					{/if}
				</tbody>
			</table>
		</div>
	</div>

	<!-- Slow queries -->
	<div class="mb-6 overflow-hidden rounded-xl border" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<div class="border-b px-6 py-4" style="border-color: var(--color-border)">
			<div class="flex items-center gap-2">
				<Zap class="h-4 w-4" style="color: var(--color-primary)" />
				<h3 class="text-sm font-semibold">{t('db.slow_queries')}</h3>
				{#if dbHealth.slow_queries?.length}
					<span class="rounded-full px-2 py-0.5 text-[10px] font-medium" style="background-color: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
						{dbHealth.slow_queries.length}
					</span>
				{/if}
			</div>
		</div>
		{#if dbHealth.slow_queries && dbHealth.slow_queries.length > 0}
			<div class="overflow-x-auto">
				<table class="w-full text-sm">
					<thead>
						<tr style="border-bottom: 1px solid var(--color-border)">
							<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted)">{t('db.query')}</th>
							<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted)">{t('db.duration')}</th>
							<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted)">{t('db.timestamp')}</th>
						</tr>
					</thead>
					<tbody>
						{#each dbHealth.slow_queries as query (query.timestamp + query.query)}
							<tr class="border-t" style="border-color: var(--color-border)">
								<td class="max-w-md truncate px-4 py-3 font-mono text-xs">{query.query}</td>
								<td class="px-4 py-3 text-right font-mono text-xs">
									<span style:color={query.duration_ms > 1000 ? 'var(--color-danger)' : 'var(--color-warning)'}>
										{formatDuration(query.duration_ms)}
									</span>
								</td>
								<td class="px-4 py-3 text-right text-xs" style="color: var(--color-muted)">
									{formatTimestamp(query.timestamp)}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{:else}
			<div class="px-6 py-8 text-center" style="color: var(--color-muted)">
				<Zap class="mx-auto h-6 w-6 opacity-40" />
				<p class="mt-2 text-sm">{t('db.no_slow_queries')}</p>
			</div>
		{/if}
	</div>

	<!-- Replication status (if applicable) -->
	{#if dbHealth.replication}
		<div class="overflow-hidden rounded-xl border" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<div class="border-b px-6 py-4" style="border-color: var(--color-border)">
				<div class="flex items-center gap-2">
					<Database class="h-4 w-4" style="color: var(--color-primary)" />
					<h3 class="text-sm font-semibold">{t('db.replication')}</h3>
				</div>
			</div>
			<div class="px-6 py-4">
				<div class="grid gap-4 sm:grid-cols-2">
					<div class="flex items-center justify-between">
						<span class="text-sm" style="color: var(--color-muted)">{t('db.replication_state')}</span>
						<StatusBadge status={dbHealth.replication.state === 'streaming' ? 'healthy' : 'warning'}>
							{dbHealth.replication.state}
						</StatusBadge>
					</div>
					<div class="flex items-center justify-between">
						<span class="text-sm" style="color: var(--color-muted)">{t('db.replication_lag')}</span>
						<span class="text-sm font-mono" style:color={dbHealth.replication.lag_seconds > 5 ? 'var(--color-danger)' : undefined}>
							{dbHealth.replication.lag_seconds.toFixed(1)}s
						</span>
					</div>
				</div>
			</div>
		</div>
	{/if}
{:else}
	<!-- No data state -->
	<div class="rounded-xl border p-12 text-center" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<Database class="mx-auto h-10 w-10 opacity-30" style="color: var(--color-muted)" />
		<p class="mt-4 text-sm font-medium" style="color: var(--color-muted)">No database health data available</p>
		<p class="mt-1 text-xs" style="color: var(--color-muted)">The backend may not yet expose database metrics.</p>
	</div>
{/if}
