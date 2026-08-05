<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import MetricCard from '$lib/components/MetricCard.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { PieChart, BarChart, Spline } from 'layerchart';
	import { curveMonotoneX } from 'd3-shape';
	import { RotateCw } from 'lucide-svelte';

	type ServiceStatus = {
		name: string;
		status: 'healthy' | 'degraded' | 'down' | 'unknown';
		latency_ms: number;
		message?: string;
	};

	type HealthResponse = {
		infrastructure: ServiceStatus[];
		application: ServiceStatus[];
		background: ServiceStatus[];
		realtime: ServiceStatus[];
	};

	let health = $state<HealthResponse>({
		infrastructure: [],
		application: [],
		background: [],
		realtime: []
	});
	let lastRefresh = $state(new Date().toLocaleTimeString());
	let apiAvailable = $state(true);
	// M10: Time-series history of healthy-service-count for the dashboard
	// sparkline. Appends a point on each poll tick, capped at 60 points.
	let healthHistory = $state<{ time: number; healthy: number; total: number }[]>([]);

	const groupKeys: Array<{ key: keyof HealthResponse; fallback: string; i18n: string }> = [
		{ key: 'infrastructure', fallback: 'Infrastructure', i18n: 'dashboard.groups.infra' },
		{ key: 'application', fallback: 'Application', i18n: 'dashboard.groups.app' },
		{ key: 'background', fallback: 'Background', i18n: 'dashboard.groups.background' },
		{ key: 'realtime', fallback: 'Realtime', i18n: 'dashboard.groups.realtime' }
	];

	function statusToHealth(s: ServiceStatus['status']): 'healthy' | 'warning' | 'critical' | 'unknown' {
		if (s === 'degraded') return 'warning';
		if (s === 'down') return 'critical';
		return s === 'healthy' ? 'healthy' : 'unknown';
	}

	function applyOverview(resp: HealthResponse) {
		health = resp;
		lastRefresh = new Date().toLocaleTimeString();
		apiAvailable = true;
		// M10: Append a point to the health time-series history
		const all = groupKeys.flatMap((g) => resp[g.key]);
		const total = all.length;
		const healthy = all.filter((s) => s.status === 'healthy').length;
		healthHistory = [...healthHistory, { time: Date.now(), healthy, total }].slice(-60);
	}

	const allServices = $derived(
		groupKeys.flatMap((g) => health[g.key].map((s) => ({ ...s, group: g.key })))
	);

	const totalServices = $derived(allServices.length);
	const healthyCount = $derived(allServices.filter((s) => s.status === 'healthy').length);
	const uptimePercent = $derived(
		totalServices > 0 ? Math.round((healthyCount / totalServices) * 100) : 0
	);

	const pieData = $derived.by(() => {
		const counts = { healthy: 0, degraded: 0, down: 0, unknown: 0 };
		for (const s of allServices) {
			counts[s.status === 'healthy' ? 'healthy' : s.status === 'degraded' ? 'degraded' : s.status === 'down' ? 'down' : 'unknown']++;
		}
		return [
			{ key: 'healthy', label: t('dashboard.healthy', 'Healthy'), value: counts.healthy },
			{ key: 'degraded', label: t('dashboard.degraded', 'Degraded'), value: counts.degraded },
			{ key: 'down', label: t('dashboard.down', 'Down'), value: counts.down },
			{ key: 'unknown', label: t('dashboard.unknown_short', 'Unknown'), value: counts.unknown }
		].filter((d) => d.value > 0);
	});

	const barData = $derived.by(() => {
		return allServices
			.filter((s) => s.latency_ms > 0)
			.sort((a, b) => b.latency_ms - a.latency_ms)
			.map((s) => ({ key: s.name, label: s.name, value: s.latency_ms }));
	});

	async function refresh() {
		try {
			const result = await api<HealthResponse>('/control/health');
			if (result.ok) {
				applyOverview(result.data);
			} else {
				apiAvailable = false;
			}
		} catch {
			apiAvailable = false;
		}
	}

	$effect(() => {
		refresh();
		const interval = setInterval(() => {
			if (document.visibilityState === 'visible') {
				refresh();
			}
		}, 30_000);
		const onVisible = () => {
			if (document.visibilityState === 'visible') refresh();
		};
		document.addEventListener('visibilitychange', onVisible);
		return () => {
			clearInterval(interval);
			document.removeEventListener('visibilitychange', onVisible);
		};
	});
</script>

<PageHeader title={t('dashboard.title', 'Dashboard')} description={t('dashboard.description', 'System health overview')} />

{#if !apiAvailable}
	<div class="border-destructive bg-destructive/10 text-destructive mb-4 flex items-center gap-2 rounded-xl border px-4 py-3 text-sm">
		{t('dashboard.analytics_warning', 'Unable to load health data.')}
	</div>
{/if}

<div class="mb-4 flex items-center justify-between">
	<p class="text-muted-foreground text-xs font-medium">{t('dashboard.last_refresh', 'Last refresh: {time}').replace('{time}', lastRefresh)}</p>
	<button
		class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-3 py-1.5 text-xs font-medium transition-colors inline-flex items-center gap-1.5"
		onclick={refresh}
	>
		<RotateCw class="h-3 w-3" />
		{t('common.refresh', 'Refresh')}
	</button>
</div>

<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4 mb-6">
	<MetricCard label={t('dashboard.total_services', 'Total Services')} value={totalServices} />
	<MetricCard label={t('dashboard.healthy_services', 'Healthy Services')} value={healthyCount} subtitle="{uptimePercent}%" trend={uptimePercent >= 80 ? 'up' : 'down'} />
	<MetricCard label={t('dashboard.api_uptime', 'API Uptime')} value="{uptimePercent}%" subtitle={t('dashboard.units.percent', '%')} trend={uptimePercent >= 90 ? 'up' : 'down'} />
	<MetricCard label={t('dashboard.last_check', 'Last Check')} value={lastRefresh} />
</div>

<!-- M10: Time-series sparkline of healthy-service-count over polling history -->
{#if healthHistory.length > 1}
	<div class="border-border-card bg-background-alt shadow-card rounded-card p-5 mb-6">
		<h3 class="text-muted-foreground mb-4 text-xs font-semibold uppercase tracking-wider">{t('dashboard.health_trend', 'Health Trend (Last {n} polls)').replace('{n}', String(healthHistory.length))}</h3>
		<div class="h-[120px]">
			<Spline
				data={healthHistory}
				x="time"
				y="healthy"
				curve={curveMonotoneX}
				stroke="var(--color-accent-foreground)"
			/>
		</div>
	</div>
{/if}

<div class="grid gap-4 lg:grid-cols-2 mb-6">
	<div class="border-border-card bg-background-alt shadow-card rounded-card p-5">
		<h3 class="text-muted-foreground mb-4 text-xs font-semibold uppercase tracking-wider">{t('dashboard.health_distribution', 'Health Distribution')}</h3>
		{#if pieData.length > 0}
			<div class="h-[240px]">
				<PieChart
					data={pieData}
					key="key"
					label="label"
					value="value"
					innerRadius={50}
					outerRadius={90}
					cornerRadius={4}
					cRange={[
						'var(--color-accent-foreground)',
						'var(--color-tertiary)',
						'var(--color-destructive)',
						'var(--color-muted-foreground)'
					]}
					legend
				/>
			</div>
		{:else}
			<p class="text-muted-foreground text-sm py-8 text-center">{t('dashboard.no_services', 'No services reported.')}</p>
		{/if}
	</div>

	<div class="border-border-card bg-background-alt shadow-card rounded-card p-5">
		<h3 class="text-muted-foreground mb-4 text-xs font-semibold uppercase tracking-wider">{t('dashboard.latency_chart', 'Latency Chart')}</h3>
		{#if barData.length > 0}
			<div class="h-[240px]">
				<BarChart
					data={barData}
					x="value"
					y="label"
					orientation="horizontal"
					grid={false}
					xPadding={[0.2, 0.2]}
					cRange={['var(--color-accent-foreground)']}
				/>
			</div>
		{:else}
			<p class="text-muted-foreground text-sm py-8 text-center">{t('dashboard.no_services', 'No services reported.')}</p>
		{/if}
	</div>
</div>

<div class="grid gap-4 sm:grid-cols-2">
	{#each groupKeys as group, i}
		{@const items = health[group.key]}
		<div
			class="border-border-card bg-background-alt shadow-card rounded-card-lg p-5"
			style="animation: fade-in 0.3s ease-out {i * 50}ms both"
		>
			<h3 class="text-muted-foreground mb-4 text-xs font-semibold uppercase tracking-wider">{t(group.i18n, group.fallback)}</h3>
			<div class="space-y-3">
				{#each items as item}
					<div class="flex items-center justify-between">
						<div class="flex items-center gap-2">
							<span class="text-foreground text-sm font-medium" title={item.message ? `${t('dashboard.latency_label', '{ms} ms latency').replace('{ms}', String(item.latency_ms))} - ${item.message}` : ''}>{item.name}</span>
						</div>
						<div class="flex items-center gap-3">
							{#if item.latency_ms > 0}
								<span class="text-muted-foreground text-xs font-medium">{t('dashboard.latency_value', '{ms} ms').replace('{ms}', String(item.latency_ms))}</span>
							{/if}
							<StatusBadge status={statusToHealth(item.status)} />
						</div>
					</div>
				{/each}
				{#if items.length === 0}
					<p class="text-muted-foreground text-xs text-center py-2">{t('dashboard.no_services', 'No services reported.')}</p>
				{/if}
			</div>
		</div>
	{/each}
</div>
