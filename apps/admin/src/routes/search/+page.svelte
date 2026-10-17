<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	import { api } from '$lib/api';
	import { DataTable } from '$lib/components/ui/data-table';
	import Pagination from '$lib/components/ui/Pagination.svelte';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import { CardSkeleton } from '$lib/components/ui/skeleton';
	import { t } from '$lib/stores/i18n.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { initTabParam, bindTabParam } from '$lib/utils/url-sync.svelte';
	import { RefreshCw } from 'lucide-svelte';
	import { LineChart, BarChart } from 'layerchart';
	import { curveMonotoneX } from 'd3-shape';

	type TopQuery = {
		query: string;
		search_count: number;
		avg_results: number;
		zero_result_count: number;
	};

	type ZeroResultQuery = {
		query: string;
		search_count: number;
	};

	type DailyVolume = {
		day: string;
		total_searches: number;
		zero_result_searches: number;
	};

	type SearchAnalyticsResponse = {
		period: string;
		top_queries: TopQuery[];
		zero_result_queries: ZeroResultQuery[];
		daily_volume: DailyVolume[];
	};

	let topQueries = $state<TopQuery[]>([]);
	let zeroResultQueries = $state<ZeroResultQuery[]>([]);
	let dailyVolume = $state<DailyVolume[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	// Period + explicit date range sourced from the URL so they survive navigation
	let period = $state(page.url.searchParams.get('period') ?? '7d');
	let from = $state(page.url.searchParams.get('from') ?? '');
	let to = $state(page.url.searchParams.get('to') ?? '');
	const SEARCH_TABS = ['top', 'zero', 'volume'] as const;
	let activeTab = $state<'top' | 'zero' | 'volume'>(initTabParam('top', { valid: SEARCH_TABS }));
	bindTabParam(() => activeTab, (v) => (activeTab = v), { valid: SEARCH_TABS, preserve: ['period', 'from', 'to'] });
	let loadAbortController: AbortController | null = null;

	const timePeriods = $derived([
		{ value: '24h', label: t('search.period_24h', '24 Hours') },
		{ value: '7d', label: t('search.period_7d', '7 Days') },
		{ value: '30d', label: t('search.period_30d', '30 Days') },
		{ value: '90d', label: t('search.period_90d', '90 Days') },
		{ value: '1y', label: t('search.period_1y', '1 Year') }
	]);

	// Pagination state per tab
	let topPage = $state(0);
	let topPageSize = $state(10);
	let zeroPage = $state(0);
	let zeroPageSize = $state(10);
	let volumePage = $state(0);
	let volumePageSize = $state(10);

	const topQueryColumns = [
		{ key: 'query', label: t('search.query') },
		{ key: 'search_count', label: t('search.searches') },
		{ key: 'avg_results', label: t('search.avg_results') },
		{ key: 'zero_result_count', label: t('search.zero_results', 'Zero Results') }
	];

	const zeroResultColumns = [
		{ key: 'query', label: t('search.query') },
		{ key: 'search_count', label: t('search.searches') }
	];

	const volumeColumns = [
		{ key: 'day', label: t('search.date', 'Date') },
		{ key: 'total_searches', label: t('search.total_searches', 'Total Searches') },
		{ key: 'zero_result_searches', label: t('search.zero_result_searches', 'Zero Result Searches') }
	];

	const tabs = [
		{ key: 'top' as const, label: t('search.top_queries', 'Top Queries'), count: () => topQueries.length },
		{ key: 'zero' as const, label: t('search.zero_result_queries', 'Zero Result Queries'), count: () => zeroResultQueries.length },
		{ key: 'volume' as const, label: t('search.daily_volume', 'Daily Volume'), count: () => dailyVolume.length }
	];

	// Derived pagination values
	const topTotalPages = $derived(Math.max(1, Math.ceil(topQueries.length / topPageSize)));
	const zeroTotalPages = $derived(Math.max(1, Math.ceil(zeroResultQueries.length / zeroPageSize)));
	const volumeTotalPages = $derived(Math.max(1, Math.ceil(dailyVolume.length / volumePageSize)));

	const topPaginated = $derived.by(() => {
		const start = topPage * topPageSize;
		return topQueries.slice(start, start + topPageSize);
	});

	const zeroPaginated = $derived.by(() => {
		const start = zeroPage * zeroPageSize;
		return zeroResultQueries.slice(start, start + zeroPageSize);
	});

	const volumePaginated = $derived.by(() => {
		const start = volumePage * volumePageSize;
		return dailyVolume.slice(start, start + volumePageSize);
	});

	// Chart-ready derived data (Ref: ADR-059 LayerChart theming)
	const dailyVolumeChartData = $derived(
		dailyVolume.map((d) => ({
			date: new Date(d.day),
			total: d.total_searches,
			zero: d.zero_result_searches
		}))
	);

	const topQueriesChartData = $derived(
		topQueries.slice(0, 10).map((q) => ({
			query: q.query,
			searches: q.search_count
		}))
	);

	// KPI cards
	const totalSearches = $derived(dailyVolume.reduce((sum, d) => sum + d.total_searches, 0));
	const totalZeroResult = $derived(dailyVolume.reduce((sum, d) => sum + d.zero_result_searches, 0));
	const zeroResultRate = $derived(totalSearches > 0 ? ((totalZeroResult / totalSearches) * 100).toFixed(1) : '0.0');

	async function load(f = from, toVal = to, p = period) {
		if (loadAbortController) loadAbortController.abort();
		loadAbortController = new AbortController();
		const signal = loadAbortController.signal;

		loading = true;
		errorMessage = '';
		try {
			const params = [f && `from=${f}`, toVal && `to=${toVal}`].filter(Boolean).join('&') || `period=${p}`;
			const result = await api<SearchAnalyticsResponse>(`/admin/search/analytics?${params}`);
			if (signal.aborted) return;
			if (result.ok) {
				topQueries = result.data.top_queries ?? [];
				zeroResultQueries = result.data.zero_result_queries ?? [];
				dailyVolume = result.data.daily_volume ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			if (signal.aborted) return;
			errorMessage = t('common.error');
		} finally {
			if (!signal.aborted) loading = false;
		}
	}

	function setPeriod(p: string) {
		const url = new URL(page.url);
		url.searchParams.set('period', p);
		url.searchParams.delete('from');
		url.searchParams.delete('to');
		goto(url, { keepFocus: true, noScroll: true });
	}

	function applyRange() {
		const url = new URL(page.url);
		if (from) url.searchParams.set('from', from);
		else url.searchParams.delete('from');
		if (to) url.searchParams.set('to', to);
		else url.searchParams.delete('to');
		goto(url, { keepFocus: true, noScroll: true });
	}

	$effect(() => {
		// Re-sync controls from the URL and reload whenever the range changes
		const f = page.url.searchParams.get('from') ?? '';
		const tVal = page.url.searchParams.get('to') ?? '';
		const p = page.url.searchParams.get('period') ?? '7d';
		from = f;
		to = tVal;
		period = p;
		topPage = 0;
		zeroPage = 0;
		volumePage = 0;
		load(f, tVal, p);

		return () => {
			if (loadAbortController) loadAbortController.abort();
		};
	});
</script>

<svelte:head>
	<title>{t('search.title')} — {siteName()} Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('search.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('search.description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<!-- Period + date-range controls (Ref: jc-admin-develop analytics rules) -->
<div class="mb-6 flex flex-wrap items-end gap-4">
	<div class="flex items-center gap-2">
		<span class="text-sm" style="color: var(--color-muted)">{t('common.period', 'Period')}:</span>
		{#each timePeriods as opt (opt)}
			<Button
				variant={period === opt.value ? 'default' : 'ghost'}
				size="sm"
				onclick={() => setPeriod(opt.value)}
			>
				{opt.label}
			</Button>
		{/each}
	</div>
	<div class="flex flex-wrap items-end gap-2">
		<div>
			<label for="range-from" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('common.from', 'From')}</label>
			<input
				id="range-from"
				type="date"
				bind:value={from}
				class="rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
			/>
		</div>
		<div>
			<label for="range-to" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('common.to', 'To')}</label>
			<input
				id="range-to"
				type="date"
				bind:value={to}
				class="rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
			/>
		</div>
		<Button variant="outline" size="sm" onclick={applyRange}>{t('common.apply', 'Apply')}</Button>
	</div>
	<Button variant="ghost" size="sm" onclick={() => load()}>
		<RefreshCw class="h-3.5 w-3.5" />
		{t('common.refresh')}
	</Button>
</div>

{#if loading}
	<div class="space-y-4">
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
			{#each Array(3) as _, i (i)}
				<CardSkeleton />
			{/each}
		</div>
		<CardSkeleton />
	</div>
{:else}
	<!-- KPI Stat Cards -->
	<div class="mb-6 grid grid-cols-1 gap-4 sm:grid-cols-3">
		<Card>
			<CardContent class="p-4">
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('search.total_searches', 'Total Searches')}</p>
				<p class="mt-1 text-2xl font-bold">{totalSearches.toLocaleString()}</p>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-4">
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('search.zero_result_searches', 'Zero Result Searches')}</p>
				<p class="mt-1 text-2xl font-bold" style="color: var(--color-warning)">{totalZeroResult.toLocaleString()}</p>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-4">
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('search.zero_result_rate', 'Zero Result Rate')}</p>
				<p class="mt-1 text-2xl font-bold">{zeroResultRate}%</p>
			</CardContent>
		</Card>
	</div>

	<!-- Charts (Ref: ADR-059 LayerChart theming) -->
	{#if dailyVolumeChartData.length > 0}
		<div class="mb-6 grid grid-cols-1 gap-6 lg:grid-cols-2">
			<Card>
				<CardHeader>
					<h3 class="text-sm font-medium" style="color: var(--color-muted)">
						{t('search.daily_volume_chart', 'Daily Search Volume')}
					</h3>
				</CardHeader>
				<CardContent>
					<div class="h-64">
						<LineChart
							data={dailyVolumeChartData}
							x="date"
							y="total"
							cRange={['var(--chart-1)']}
							props={{
								spline: { curve: curveMonotoneX, strokeWidth: 2, stroke: 'var(--chart-1)' }
							}}
						/>
					</div>
				</CardContent>
			</Card>
			<Card>
				<CardHeader>
					<h3 class="text-sm font-medium" style="color: var(--color-muted)">
						{t('search.zero_result_trend', 'Zero-Result Search Trend')}
					</h3>
				</CardHeader>
				<CardContent>
					<div class="h-64">
						<LineChart
							data={dailyVolumeChartData}
							x="date"
							y="zero"
							cRange={['var(--chart-2)']}
							props={{
								spline: { curve: curveMonotoneX, strokeWidth: 2, stroke: 'var(--chart-2)' }
							}}
						/>
					</div>
				</CardContent>
			</Card>
		</div>
	{/if}

	{#if topQueriesChartData.length > 0}
		<Card class="mb-6">
			<CardHeader>
				<h3 class="text-sm font-medium" style="color: var(--color-muted)">
					{t('search.top_queries_chart', 'Top 10 Queries by Search Count')}
				</h3>
			</CardHeader>
			<CardContent>
				<div class="h-64">
					<BarChart
						data={topQueriesChartData}
						x="query"
						y="searches"
						xPadding={[0.1, 0.1]}
						cRange={['var(--chart-3)']}
						props={{
							bars: { fill: 'var(--chart-3)' }
						}}
					/>
				</div>
			</CardContent>
		</Card>
	{/if}

	<!-- Tabs -->
	<div class="mb-4 flex gap-1 border-b" style="border-color: var(--color-border)">
		{#each tabs as tab (tab)}
			<button
				class="relative px-4 py-2 text-sm font-medium transition-colors"
				style={activeTab === tab.key
					? 'color: var(--color-primary); border-bottom: 2px solid var(--color-primary)'
					: 'color: var(--color-muted)'}
				onclick={() => activeTab = tab.key}
			>
				{tab.label}
				{#if tab.count() > 0}
					<span class="ml-1.5 inline-flex items-center justify-center rounded-full px-1.5 text-xs" style="background-color: color-mix(in srgb, var(--color-primary) 15%, transparent); color: var(--color-primary)">
						{tab.count()}
					</span>
				{/if}
			</button>
		{/each}
	</div>

	<!-- Tab content -->
	{#if activeTab === 'top'}
		{#if topQueries.length > 0}
			<DataTable columns={topQueryColumns} data={topPaginated} showPagination={false} searchable={false} emptyMessage={t('search.empty')} />
			<Pagination bind:page={topPage} totalPages={topTotalPages} totalItems={topQueries.length} bind:pageSize={topPageSize} />
		{:else}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('search.empty')}</p>
		{/if}
	{:else if activeTab === 'zero'}
		{#if zeroResultQueries.length > 0}
			<DataTable columns={zeroResultColumns} data={zeroPaginated} showPagination={false} searchable={false} emptyMessage={t('search.empty')} />
			<Pagination bind:page={zeroPage} totalPages={zeroTotalPages} totalItems={zeroResultQueries.length} bind:pageSize={zeroPageSize} />
		{:else}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('search.empty')}</p>
		{/if}
	{:else if activeTab === 'volume'}
		{#if dailyVolume.length > 0}
			<DataTable columns={volumeColumns} data={volumePaginated} showPagination={false} searchable={false} emptyMessage={t('search.empty')} />
			<Pagination bind:page={volumePage} totalPages={volumeTotalPages} totalItems={dailyVolume.length} bind:pageSize={volumePageSize} />
		{:else}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('search.empty')}</p>
		{/if}
	{/if}
{/if}
