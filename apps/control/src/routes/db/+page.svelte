<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import MetricCard from '$lib/components/MetricCard.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import { api } from '$lib/api';
	import type { DbHealth, DataTableColumn } from '$lib/api/types';
	import { t } from '$lib/stores/i18n.svelte';
	import { escapeHtml } from '$lib/utils';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import Tabs from '$lib/components/ui/Tabs.svelte';
	import TabsContent from '$lib/components/ui/TabsContent.svelte';
	import Tooltip from '$lib/components/ui/Tooltip.svelte';
	import {
		Database,
		RefreshCw,
		Table2,
		AlertTriangle,
		Server,
		Zap,
		Activity,
		Scissors
	} from 'lucide-svelte';

	let {} = $props();

	let dbHealth = $state<DbHealth | null>(null);
	let loading = $state(true);
	let apiAvailable = $state(true);
	let lastRefresh = $state(new Date());
	let secondsAgo = $state(0);
	let refreshing = $state(false);
	let vacuuming = $state(false);
	let showVacuumConfirm = $state(false);
	let vacuumMessage = $state<string | null>(null);
	let activeTab = $state('overview');

	async function refresh() {
		refreshing = true;
		try {
			const result = await api<DbHealth>('/control/health/db');
			if (result.ok) {
				dbHealth = result.data;
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

	async function vacuum() {
		vacuuming = true;
		vacuumMessage = null;
		try {
			const result = await api<{ status: string }>('/control/health/db/vacuum', { method: 'POST' });
			if (result.ok && (result.data.status === 'enqueued' || result.data.status === 'completed')) {
				vacuumMessage = result.data.status === 'enqueued'
					? t('db.vacuum_enqueued', 'Vacuum task enqueued successfully')
					: t('db.vacuum_completed', 'Vacuum completed successfully');
				await refresh();
			} else {
				vacuumMessage = t('db.vacuum_failed', 'Vacuum failed');
			}
		} catch {
			vacuumMessage = t('db.vacuum_request_failed', 'Vacuum request failed');
		} finally {
			vacuuming = false;
		}
	}

	function formatBytes(bytes: number | undefined | null): string {
		if (bytes == null) return '—';
		if (bytes >= 1_073_741_824) return (bytes / 1_073_741_824).toFixed(1) + ' GB';
		if (bytes >= 1_048_576) return (bytes / 1_048_576).toFixed(1) + ' MB';
		if (bytes >= 1024) return (bytes / 1024).toFixed(1) + ' KB';
		return bytes + ' B';
	}

	function formatNumber(n: number | undefined | null): string {
		if (n == null) return '—';
		if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
		if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K';
		return n.toLocaleString();
	}

	function formatDuration(ms: number | undefined | null): string {
		if (ms == null) return '—';
		if (ms >= 1000) return (ms / 1000).toFixed(2) + 's';
		return ms.toFixed(0) + 'ms';
	}

	function formatTimestamp(ts: string | undefined | null): string {
		if (!ts) return '—';
		const d = new Date(ts);
		if (Number.isNaN(d.getTime())) return '—';
		return d.toLocaleString();
	}

	function formatVacuum(v: string | null): string {
		if (!v) return t('db.never', 'Never');
		return new Date(v).toLocaleDateString();
	}

	function poolUsagePercent(): number {
		if (!dbHealth?.connection_pool) return 0;
		const pool = dbHealth.connection_pool;
		return pool.max > 0 ? Math.round(((pool.active + pool.idle) / pool.max) * 100) : 0;
	}

	function hasPoolWarning(): boolean {
		return poolUsagePercent() > 80;
	}

	function hasSlowQueryWarning(): boolean {
		return (dbHealth?.slow_queries?.length ?? 0) > 0 &&
			(dbHealth?.slow_queries?.some(q => q.duration_ms > 1000) ?? false);
	}

	function hasReplicationWarning(): boolean {
		// Backend bases replication health on WAL-lag bytes (lag_bytes),
		// not the replay-timestamp lag — lag_seconds grows artificially
		// during idle periods (see the "How to read these numbers" note).
		return dbHealth?.replication?.healthy === false;
	}

	function poolBarColor(): string {
		const pct = poolUsagePercent();
		if (pct > 80) return 'bg-destructive';
		if (pct > 60) return 'bg-accent';
		return 'bg-foreground';
	}

	// M7: Convert raw table to DataTable — table-stats & slow-queries
	type TableStat = DbHealth['tables'][number];
	type SlowQuery = DbHealth['slow_queries'][number];

	let tablesPage = $state(0);
	let tablesPageSize = $state(10);
	let tablesSortKey = $state('');
	let tablesSortDirection = $state<'asc' | 'desc'>('asc');

	let slowQueriesPage = $state(0);
	let slowQueriesPageSize = $state(10);
	let slowQueriesSortKey = $state('');
	let slowQueriesSortDirection = $state<'asc' | 'desc'>('asc');

	const tableColumns: DataTableColumn[] = [
		{
			key: 'name',
			label: t('db.table_name'),
			sortable: true,
			render: (val) => `<span class="font-medium text-foreground">${escapeHtml(val) || '—'}</span>`
		},
		{
			key: 'row_count',
			label: t('db.row_count'),
			sortable: true,
			class: 'text-right',
			render: (val) => `<span class="font-mono text-xs text-foreground">${formatNumber(val as number)}</span>`
		},
		{
			key: 'size_bytes',
			label: t('db.size'),
			sortable: true,
			class: 'text-right',
			render: (val) => `<span class="font-mono text-xs text-foreground">${formatBytes(val as number)}</span>`
		},
		{
			key: 'index_count',
			label: t('db.indexes'),
			sortable: true,
			class: 'text-right',
			render: (val) => `<span class="font-mono text-xs text-foreground">${val ?? '—'}</span>`
		},
		{
			key: 'last_vacuum',
			label: t('db.last_vacuum'),
			sortable: true,
			class: 'text-right',
			render: (val) => `<span class="text-xs text-muted-foreground">${escapeHtml(formatVacuum(val as string | null))}</span>`
		}
	];

	const slowQueryColumns: DataTableColumn[] = [
		{
			key: 'query',
			label: t('db.query'),
			sortable: true,
			render: (val) => `<span class="font-mono text-xs text-foreground max-w-md truncate inline-block align-bottom">${escapeHtml(val) || '—'}</span>`
		},
		{
			key: 'duration_ms',
			label: t('db.duration'),
			sortable: true,
			class: 'text-right',
			render: (val) => {
				const ms = val as number;
				const danger = ms > 1000;
				return `<span class="font-mono text-xs ${danger ? 'text-destructive' : 'text-accent-foreground'}">${formatDuration(ms)}</span>`;
			}
		},
		{
			key: 'timestamp',
			label: t('db.timestamp'),
			sortable: true,
			class: 'text-right',
			render: (val) => `<span class="text-xs text-muted-foreground">${escapeHtml(formatTimestamp(val as string))}</span>`
		}
	];

	function sortRows<T>(list: T[], key: string, direction: 'asc' | 'desc'): T[] {
		if (!key) return list;
		const sorted = [...list];
		sorted.sort((a, b) => {
			const av = (a as Record<string, unknown>)[key];
			const bv = (b as Record<string, unknown>)[key];
			if (av == null && bv == null) return 0;
			if (av == null) return 1;
			if (bv == null) return -1;
			let cmp: number;
			if (typeof av === 'number' && typeof bv === 'number') {
				cmp = av - bv;
			} else {
				cmp = String(av).localeCompare(String(bv));
			}
			return direction === 'asc' ? cmp : -cmp;
		});
		return sorted;
	}

	let sortedTables = $derived.by(() => {
		const list = dbHealth?.tables ?? [];
		return sortRows(list, tablesSortKey, tablesSortDirection);
	});

	let tablesTotalRows = $derived(sortedTables.length);

	let paginatedTables = $derived.by(() => {
		const start = tablesPage * tablesPageSize;
		return sortedTables.slice(start, start + tablesPageSize);
	});

	let sortedSlowQueries = $derived.by(() => {
		const list = dbHealth?.slow_queries ?? [];
		return sortRows(list, slowQueriesSortKey, slowQueriesSortDirection);
	});

	let slowQueriesTotalRows = $derived(sortedSlowQueries.length);

	let paginatedSlowQueries = $derived.by(() => {
		const start = slowQueriesPage * slowQueriesPageSize;
		return sortedSlowQueries.slice(start, start + slowQueriesPageSize);
	});

	function getTableRowId(row: unknown): string {
		return (row as TableStat).name;
	}

	function getSlowQueryRowId(row: unknown): string {
		const q = row as SlowQuery;
		return q.timestamp + q.query;
	}

	$effect(() => {
		refresh();

		const interval = setInterval(() => {
			if (document.visibilityState === 'visible') {
				refresh();
			}
		}, 60_000);

		const tickInterval = setInterval(() => {
			if (document.visibilityState === 'visible') {
				secondsAgo = Math.floor((Date.now() - lastRefresh.getTime()) / 1000);
			}
		}, 1000);

		const onVisible = () => {
			if (document.visibilityState === 'visible') refresh();
		};
		document.addEventListener('visibilitychange', onVisible);

		return () => {
			clearInterval(interval);
			clearInterval(tickInterval);
			document.removeEventListener('visibilitychange', onVisible);
		};
	});
</script>

<PageHeader title={t('db.title')} description={t('db.description')} />

{#if !apiAvailable}
	<div class="mb-4 rounded-card border border-border-card bg-destructive/10 px-4 py-3 text-sm text-destructive">
		{t('db.unavailable')}
	</div>
{/if}

<!-- Alert banners -->
{#if hasPoolWarning()}
	<div class="mb-3 flex items-center gap-2 rounded-card border border-border-card bg-accent/10 px-4 py-2.5 text-sm text-accent-foreground">
		<AlertTriangle class="h-4 w-4 flex-shrink-0" />
		{t('db.pool_warning')} ({poolUsagePercent()}%)
	</div>
{/if}
{#if hasSlowQueryWarning()}
	<div class="mb-3 flex items-center gap-2 rounded-card border border-border-card bg-destructive/10 px-4 py-2.5 text-sm text-destructive">
		<AlertTriangle class="h-4 w-4 flex-shrink-0" />
		{t('db.slow_query_warning')}
	</div>
{/if}
{#if hasReplicationWarning()}
	<div class="mb-3 flex items-center gap-2 rounded-card border border-border-card bg-accent/10 px-4 py-2.5 text-sm text-accent-foreground">
		<AlertTriangle class="h-4 w-4 flex-shrink-0" />
		{t('db.replication_lag_warning')} ({dbHealth?.replication?.lag_seconds?.toFixed(1)}s)
	</div>
{/if}

<!-- Refresh indicator -->
<div class="mb-4 flex items-center justify-between">
	<div class="flex items-center gap-3">
		<p class="text-xs text-muted-foreground">
			{t('db.last_updated').replace('{time}', lastRefresh.toLocaleTimeString())}
			<span class="ml-2">({t('db.seconds_ago', '{seconds}s ago').replace('{seconds}', String(secondsAgo))})</span>
		</p>
		<span class="rounded-full bg-accent/10 px-2 py-0.5 text-[10px] font-medium text-accent-foreground">
			{t('db.auto_refresh')}
		</span>
	</div>
	<div class="flex items-center gap-2">
		<button
			class="border-border-input bg-background-alt hover:bg-muted text-foreground flex items-center gap-1.5 rounded-9px border px-3 py-1.5 text-xs font-medium transition-colors disabled:opacity-50"
			disabled={refreshing}
			onclick={refresh}
		>
			<RefreshCw class="size-3 {refreshing ? 'animate-spin' : ''}" />
			{t('common.refresh')}
		</button>
		<Tooltip content={t('db.vacuum_tooltip')} arrow arrowClass="fill-background-alt">
			<button
				class="border-border-input bg-background-alt hover:bg-muted text-foreground flex items-center gap-1.5 rounded-9px border px-3 py-1.5 text-xs font-medium transition-colors disabled:opacity-50"
				disabled={vacuuming}
				onclick={() => { showVacuumConfirm = true; }}
			>
				<Scissors class="size-3 {vacuuming ? 'animate-pulse' : ''}" />
				{t('db.vacuum_button')}
			</button>
		</Tooltip>
	</div>
</div>

{#if loading}
	<!-- Loading skeletons -->
	<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
		{#each Array(4) as _}
			<div class="h-24 animate-pulse rounded-card border border-border-card bg-background-alt"></div>
		{/each}
	</div>
	<div class="h-64 animate-pulse rounded-card border border-border-card bg-background-alt"></div>
{:else if dbHealth}
	<Tabs
		bind:value={activeTab}
		listClass="mb-6 flex gap-1 rounded-card bg-muted p-1"
		triggerClass="flex items-center gap-1.5 rounded-9px px-4 py-2 text-xs font-medium text-muted-foreground transition-colors data-[state=active]:bg-background data-[state=active]:text-foreground data-[state=active]:shadow-card"
		tabs={[
			{ value: 'overview', label: t('db.tab_overview'), icon: Activity },
			{ value: 'tables', label: t('db.tab_tables'), icon: Table2 },
			{ value: 'slow-queries', label: t('db.tab_slow_queries'), icon: Zap },
			{ value: 'replication', label: t('db.tab_replication'), icon: Database }
		]}
	>
		<!-- Overview Tab -->
		<TabsContent value="overview" class="mt-0">
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
			<div class="grid gap-4 sm:grid-cols-2">
				<!-- Connection status card -->
				<div class="rounded-card border border-border-card bg-background p-6 shadow-card">
					<div class="mb-4 flex items-center gap-2">
						<Server class="h-4 w-4 text-accent-foreground" />
						<h3 class="text-sm font-semibold text-foreground">{t('db.connection_status')}</h3>
					</div>
					<div class="space-y-3">
						<div class="flex items-center justify-between">
							<span class="text-sm text-muted-foreground">{t('db.status_label')}</span>
							<StatusBadge status={dbHealth.connected ? 'healthy' : 'critical'}>
								{dbHealth.connected ? t('db.connected') : t('db.disconnected')}
							</StatusBadge>
						</div>
						<div class="flex items-center justify-between">
							<span class="text-sm text-muted-foreground">{t('db.version')}</span>
							<span class="text-sm font-mono text-foreground">{dbHealth.version || '—'}</span>
						</div>
						<div class="flex items-center justify-between">
							<span class="text-sm text-muted-foreground">{t('db.db_latency')}</span>
							<span class="text-sm font-mono text-foreground">{dbHealth.latency_ms != null ? `${dbHealth.latency_ms.toFixed(1)}ms` : '—'}</span>
						</div>
					</div>
				</div>

				<!-- Connection pool card -->
				<div class="rounded-card border border-border-card bg-background p-6 shadow-card">
					<div class="mb-4 flex items-center gap-2">
						<Activity class="h-4 w-4 text-accent-foreground" />
						<h3 class="text-sm font-semibold text-foreground">{t('db.connection_pool')}</h3>
					</div>
					<div class="space-y-3">
						<div class="flex items-center justify-between">
							<span class="text-sm text-muted-foreground">{t('db.active_connections')}</span>
							<span class="text-sm font-mono text-foreground">{dbHealth.connection_pool?.active ?? 0}</span>
						</div>
						<div class="flex items-center justify-between">
							<span class="text-sm text-muted-foreground">{t('db.idle_connections')}</span>
							<span class="text-sm font-mono text-foreground">{dbHealth.connection_pool?.idle ?? 0}</span>
						</div>
						<div class="flex items-center justify-between">
							<span class="text-sm text-muted-foreground">{t('db.max_connections')}</span>
							<span class="text-sm font-mono text-foreground">{dbHealth.connection_pool?.max ?? 0}</span>
						</div>
						<!-- Pool usage bar -->
						<div class="pt-1">
							<div class="flex items-center justify-between text-xs text-muted-foreground">
								<span>{t('db.usage_label')}</span>
								<span>{poolUsagePercent()}%</span>
							</div>
							<div class="bg-muted h-2 rounded-full mt-1 overflow-hidden">
								<div class="{poolBarColor()} h-full rounded-full transition-all" style="width: {poolUsagePercent()}%"></div>
							</div>
						</div>
					</div>
				</div>
			</div>
		</TabsContent>

		<!-- Tables Tab -->
		<TabsContent value="tables" class="mt-0">
			<div class="overflow-hidden rounded-card border border-border-card bg-background shadow-card">
				<div class="border-b border-border-card px-6 py-4">
					<div class="flex items-center gap-2">
						<Table2 class="h-4 w-4 text-accent-foreground" />
						<h3 class="text-sm font-semibold text-foreground">{t('db.table_stats')}</h3>
					</div>
				</div>
				<DataTable
					columns={tableColumns}
					rows={paginatedTables}
					totalRows={tablesTotalRows}
					bind:page={tablesPage}
					bind:pageSize={tablesPageSize}
					bind:sortKey={tablesSortKey}
					bind:sortDirection={tablesSortDirection}
					loading={loading}
					emptyMessage={t('db.no_table_stats')}
					emptyIcon={Table2}
					selectable={false}
					getRowId={getTableRowId}
				/>
			</div>
		</TabsContent>

		<!-- Slow Queries Tab -->
		<TabsContent value="slow-queries" class="mt-0">
			<div class="overflow-hidden rounded-card border border-border-card bg-background shadow-card">
				<div class="border-b border-border-card px-6 py-4">
					<div class="flex items-center gap-2">
						<Zap class="h-4 w-4 text-accent-foreground" />
						<h3 class="text-sm font-semibold text-foreground">{t('db.slow_queries')}</h3>
						{#if dbHealth.slow_queries?.length}
							<span class="rounded-full bg-destructive/10 px-2 py-0.5 text-[10px] font-medium text-destructive">
								{dbHealth.slow_queries.length}
							</span>
						{/if}
					</div>
				</div>
				<DataTable
					columns={slowQueryColumns}
					rows={paginatedSlowQueries}
					totalRows={slowQueriesTotalRows}
					bind:page={slowQueriesPage}
					bind:pageSize={slowQueriesPageSize}
					bind:sortKey={slowQueriesSortKey}
					bind:sortDirection={slowQueriesSortDirection}
					loading={loading}
					emptyMessage={t('db.no_slow_queries')}
					emptyIcon={Zap}
					selectable={false}
					getRowId={getSlowQueryRowId}
				/>
			</div>
		</TabsContent>

		<!-- Replication Tab -->
		<TabsContent value="replication" class="mt-0">
			{#if dbHealth.replication}
				<!-- Overall health banner -->
				<div class="overflow-hidden rounded-card border shadow-card {dbHealth.replication.healthy ? 'border-emerald-500/30 bg-emerald-500/5' : 'border-destructive/30 bg-destructive/5'}">
					<div class="px-6 py-5">
						<div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
							<div class="flex items-start gap-4">
								<div class="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center rounded-full {dbHealth.replication.healthy ? 'bg-emerald-500/15 text-emerald-600' : 'bg-destructive/15 text-destructive'}">
									<Activity class="h-5 w-5" />
								</div>
								<div>
									<h3 class="text-base font-semibold text-foreground">
										{dbHealth.replication.healthy ? t('db.replication_healthy', 'Replication healthy') : t('db.replication_unhealthy', 'Replication unhealthy')}
									</h3>
									<p class="mt-1 max-w-2xl text-sm text-muted-foreground">
										{dbHealth.replication.healthy_text}
									</p>
								</div>
							</div>
							<div class="shrink-0">
								<StatusBadge status={dbHealth.replication.healthy ? 'healthy' : 'critical'}>
									{dbHealth.replication.healthy ? t('db.healthy', 'Healthy') : t('db.unhealthy', 'Unhealthy')}
								</StatusBadge>
							</div>
						</div>
					</div>
				</div>

				<!-- Key metrics -->
				<div class="mt-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
					<MetricCard
						label={t('db.replication_wal_lag', 'WAL lag')}
						value={dbHealth.replication.lag_pretty}
						subtitle={dbHealth.replication.lag_bytes === 0
							? t('db.replication_fully_caught_up', 'Replica is fully caught up')
							: t('db.replication_wal_lag_behind', 'WAL bytes waiting to be replayed')}
						trend={dbHealth.replication.healthy ? 'down' : 'up'}
					/>
					<MetricCard
						label={t('db.replication_state', 'Node role')}
						value={dbHealth.replication.state}
						subtitle={dbHealth.replication.state === 'standby'
							? t('db.replication_reader_node', 'Read replica receiving WAL stream')
							: t('db.replication_primary_node', 'Primary read-write database')}
					/>
					<MetricCard
						label={t('db.replication_lag', 'Replay timestamp lag')}
						value={`${dbHealth.replication.lag_seconds?.toFixed(1)}s`}
						subtitle={t('db.replication_timestamp_note', 'Can be high during idle periods')}
					/>
				</div>

				<!-- Explanation -->
				<div class="mt-4 rounded-card border border-border-card bg-background px-5 py-4 shadow-card">
					<div class="flex items-start gap-3">
						<AlertTriangle class="mt-0.5 h-4 w-4 shrink-0 text-amber-500" />
						<div>
							<p class="text-sm font-medium text-foreground">{t('db.replication_how_to_read', 'How to read these numbers')}</p>
							<p class="mt-1 text-sm text-muted-foreground">
								{t('db.replication_explanation', 'WAL lag is the real measure of replication health: it shows how much data the replica has not yet replayed. Replay timestamp lag can grow artificially when the database is idle, because no new transactions have arrived to advance the timestamp. The health status above is based on WAL lag, not timestamp lag.')}
							</p>
						</div>
					</div>
				</div>

				{#if dbHealth.pools}
					<div class="mt-6 overflow-hidden rounded-card border border-border-card bg-background shadow-card">
						<div class="border-b border-border-card px-6 py-4">
							<div class="flex items-center gap-2">
								<Server class="h-4 w-4 text-accent-foreground" />
								<h3 class="text-sm font-semibold text-foreground">{t('db.pool_status', 'Pool Status')}</h3>
							</div>
						</div>
						<div class="px-6 py-4">
							<div class="grid gap-4 sm:grid-cols-2">
								<div class="flex items-center justify-between">
									<span class="text-sm text-muted-foreground">{t('db.primary_pool', 'Primary (writer)')}</span>
									<div class="flex items-center gap-2">
										<StatusBadge status={dbHealth.pools.primary.reachable ? 'healthy' : 'critical'}>
											{dbHealth.pools.primary.reachable ? t('db.reachable', 'Reachable') : t('db.unreachable', 'Unreachable')}
										</StatusBadge>
										<span class="text-xs font-mono text-muted-foreground">{dbHealth.pools.primary.latency_ms}ms</span>
									</div>
								</div>
								<div class="flex items-center justify-between">
									<span class="text-sm text-muted-foreground">{t('db.reader_pool', 'Reader (replica)')}</span>
									<div class="flex items-center gap-2">
										<StatusBadge status={dbHealth.pools.reader.reachable ? 'healthy' : 'critical'}>
											{dbHealth.pools.reader.reachable ? t('db.reachable', 'Reachable') : t('db.unreachable', 'Unreachable')}
										</StatusBadge>
										<span class="text-xs font-mono text-muted-foreground">{dbHealth.pools.reader.latency_ms}ms</span>
									</div>
								</div>
							</div>
						</div>
					</div>
				{/if}
			{:else}
				<div class="rounded-card border border-border-card bg-background p-12 text-center shadow-card">
					<Database class="mx-auto h-10 w-10 text-muted-foreground opacity-30" />
					<p class="mt-4 text-sm font-medium text-muted-foreground">{t('db.no_replication_data')}</p>
					<p class="mt-1 text-xs text-muted-foreground">{t('db.replication_not_configured')}</p>
				</div>
			{/if}
		</TabsContent>
	</Tabs>
{:else}
	<!-- No data state -->
	<div class="rounded-card border border-border-card bg-background p-12 text-center shadow-card">
		<Database class="mx-auto h-10 w-10 text-muted-foreground opacity-30" />
		<p class="mt-4 text-sm font-medium text-muted-foreground">{t('db.no_health_data')}</p>
		<p class="mt-1 text-xs text-muted-foreground">{t('db.metrics_not_exposed')}</p>
	</div>
{/if}

{#if vacuumMessage}
	<div class="mb-4 rounded-card border border-border-card bg-foreground/10 px-4 py-3 text-sm text-foreground">
		{vacuumMessage}
	</div>
{/if}

<ConfirmModal
	bind:open={showVacuumConfirm}
	title={t('db.vacuum_title')}
	consequence={t('db.vacuum_consequence')}
	confirmText={t('db.vacuum_confirm')}
	onConfirm={vacuum}
	onCancel={() => {}}
/>
