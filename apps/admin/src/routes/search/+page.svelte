<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { DataTable } from '$lib/components/ui/data-table';
	import Pagination from '$lib/components/ui/Pagination.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw } from 'lucide-svelte';

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
	let period = $state('7d');
	let activeTab = $state<'top' | 'zero' | 'volume'>('top');

	// Pagination state per tab
	let topPage = $state(0);
	let topPageSize = $state(10);
	let zeroPage = $state(0);
	let zeroPageSize = $state(10);
	let volumePage = $state(0);
	let volumePageSize = $state(10);

	const topQueryColumns = [
		{ key: 'query', label: t('search.query'), sortable: true },
		{ key: 'search_count', label: t('search.searches'), sortable: true },
		{ key: 'avg_results', label: t('search.avg_results'), sortable: true },
		{ key: 'zero_result_count', label: t('search.zero_results', 'Zero Results'), sortable: true }
	];

	const zeroResultColumns = [
		{ key: 'query', label: t('search.query'), sortable: true },
		{ key: 'search_count', label: t('search.searches'), sortable: true }
	];

	const volumeColumns = [
		{ key: 'day', label: t('search.date', 'Date'), sortable: true },
		{ key: 'total_searches', label: t('search.total_searches', 'Total Searches'), sortable: true },
		{ key: 'zero_result_searches', label: t('search.zero_result_searches', 'Zero Result Searches'), sortable: true }
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

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<SearchAnalyticsResponse>(`/admin/search/analytics?period=${period}`);
			if (result.ok && result.data) {
				topQueries = result.data.top_queries ?? [];
				zeroResultQueries = result.data.zero_result_queries ?? [];
				dailyVolume = result.data.daily_volume ?? [];
			} else if (result.error) {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	function setPeriod(p: string) {
		period = p;
		topPage = 0;
		zeroPage = 0;
		volumePage = 0;
		load();
	}

	onMount(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('search.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('search.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('search.description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex items-center justify-between">
	<div class="flex gap-1">
		{#each [{ value: '24h', label: '24 Hours' }, { value: '7d', label: '7 Days' }, { value: '30d', label: '30 Days' }, { value: '90d', label: '90 Days' }, { value: '1y', label: '1 Year' }] as opt}
			<button
				class="rounded px-3 py-1.5 text-xs font-medium transition-colors"
				style={period === opt.value ? 'background-color: var(--color-primary); color: var(--color-background)' : 'color: var(--color-muted)'}
				onclick={() => setPeriod(opt.value)}
			>
				{opt.label}
			</button>
		{/each}
	</div>
	<button class="btn btn-secondary flex items-center gap-1.5 text-xs" onclick={load}>
		<RefreshCw class="h-3 w-3" />
		{t('common.refresh')}
	</button>
</div>

{#if loading}
	<div class="space-y-4">
		<div class="h-10 w-full animate-pulse rounded-lg" style="background-color: var(--color-surface)"></div>
		<div class="h-64 animate-pulse rounded-lg" style="background-color: var(--color-surface)"></div>
	</div>
{:else}
	<!-- Tabs -->
	<div class="mb-4 flex gap-1 border-b" style="border-color: var(--color-border)">
		{#each tabs as tab}
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
