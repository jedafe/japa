<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { t, i18n } from '$lib/i18n';
	import { page } from '$app/state';
	import {
		Timer,
		Gauge,
		BarChart3,
		Zap,
		Hourglass,
		Building2,
		ChartLine,
		Lock
	} from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton, SelectField } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import { LineChart } from 'layerchart';
	import type { GeoCountry, ProcessingStat, ProcessingTrendPoint } from '$lib/types';
	import type { PageData } from './$types';

	/**
	 * Visa Processing Time Tracker tool — §93.
	 * Destination selector → visa category input → stats cards + trend chart.
	 * Reuses the §83 visa timeline backend.
	 */
	let { data }: { data: PageData } = $props();

	const title = $derived(t('processing_tracker.title', 'Visa Processing Time Tracker'));
	const description = $derived(
		t(
			'processing_tracker.subtitle',
			'See how long visa processing takes — average, median, and percentile wait times for your corridor.'
		)
	);

	const destinations = $derived<GeoCountry[]>(data.destinations);
	const disabled = $derived(data.disabled);
	const authed = $derived(data.authed);

	let stats = $state<ProcessingStat[]>([]);
	let trend = $state<ProcessingTrendPoint[]>([]);

	let selectedDest = $state('');
	let visaCategory = $state('');
	let originDest = $state('');
	let loadingStats = $state(false);
	let loadingTrend = $state(false);
	let hasSearched = $state(false);

	// Aggregate the best (latest period) stat row for headline display.
	const latestStat = $derived.by(() => {
		if (stats.length === 0) return null;
		return [...stats].sort((a, b) =>
			new Date(b.period_start).getTime() - new Date(a.period_start).getTime()
		)[0];
	});

	const totalSamples = $derived(
		stats.reduce((sum, s) => sum + s.sample_count, 0)
	);

	// Prepare trend data for LayerChart — x is a Date, y is avg_days.
	const chartData = $derived(
		trend
			.filter((p) => p.avg_days != null)
			.map((p) => ({
				period_start: new Date(p.period_start),
				avg_days: p.avg_days as number,
				median_days: p.median_days ?? null
			}))
	);

	async function loadStats() {
		if (!selectedDest || !visaCategory.trim()) return;
		hasSearched = true;
		loadingStats = true;
		const params = new URLSearchParams({
			dest: selectedDest,
			visa: visaCategory.trim()
		});
		if (originDest) params.set('origin', originDest);
		const res = await api.get<ProcessingStat[]>(`/visa-timelines/stats?${params.toString()}`);
		loadingStats = false;
		if (!res.error) stats = res.data ?? [];
		else stats = [];
		loadTrend();
	}

	async function loadTrend() {
		if (!selectedDest || !visaCategory.trim()) return;
		loadingTrend = true;
		const params = new URLSearchParams({
			dest: selectedDest,
			visa: visaCategory.trim(),
			months: '12'
		});
		const res = await api.get<ProcessingTrendPoint[]>(
			`/visa-timelines/stats/trend?${params.toString()}`
		);
		loadingTrend = false;
		if (!res.error) trend = res.data ?? [];
		else trend = [];
	}

	function onSearch(e: Event) {
		e.preventDefault();
		loadStats();
	}

	function onDestChange() {
		// Reset results when the destination changes — stale stats for a
		// different corridor are misleading.
		stats = [];
		trend = [];
		hasSearched = false;
	}

	function fmtDays(days?: number | null): string {
		if (days == null) return '—';
		return `${days.toFixed(0)} days`;
	}

	function fmtDate(iso: string): string {
		try {
			return new Date(iso).toLocaleDateString(i18n.locale, {
				year: 'numeric',
				month: 'short',
				day: 'numeric'
			});
		} catch {
			return iso;
		}
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Timer size={20} /> {title}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">{description}</p>
	</div>

	{#if disabled}
		<div class="p-4">
			<EmptyState
				icon={Lock}
				title={t('tools.disabled_title', 'This tool is currently unavailable')}
				body={t('tools.disabled_body', 'Check back soon or contact support.')}
			/>
		</div>
	{:else if destinations.length === 0}
		<div class="p-4">
			<EmptyState
				icon={Timer}
				title={t('processing_tracker.no_destinations', 'No destinations available')}
				body={t(
					'processing_tracker.no_destinations_desc',
					'Destination data has not been configured yet. Please check back later.'
				)}
			/>
		</div>
	{:else}
		{#if !authed}
			<div class="mx-4 mt-3 mb-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-xs text-[var(--color-muted)]">
				{t('processing_tracker.anon_hint', 'Sign in to save your progress.')}
			</div>
		{/if}

		<!-- Destination + visa category selector -->
		<form onsubmit={onSearch} class="border-b border-[var(--color-border)] px-4 py-3 space-y-4">
			<div>
				<label for="pt-dest" class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
					{t('processing_tracker.destination', 'Destination country')}
				</label>
				<HintIcon text={t('tools.pt.dest_hint', 'The country you applied to.')} />
				<div class="mt-2">
					<SelectField
						id="pt-dest"
						value={selectedDest}
						options={destinations.map((d) => ({ value: d.id, label: d.name }))}
						placeholder={t('geo.select_destination', 'Select a country…')}
						onValueChange={(v) => {
							selectedDest = v;
							onDestChange();
						}}
					/>
				</div>
			</div>

			<div class="flex flex-wrap items-end gap-3">
				<div class="flex-1 min-w-[200px]">
					<label
						for="visa-category"
						class="block text-sm font-medium text-[var(--color-text)] mb-1"
					>
						{t('processing_tracker.visa_category', 'Visa category')}
					</label>
						<HintIcon text={t('tools.pt.category_hint', 'The visa class you applied under.')} />
					<input
						id="visa-category"
						type="text"
						bind:value={visaCategory}
						placeholder={t(
							'processing_tracker.visa_placeholder',
							'student, work, tourist…'
						)}
						class="w-full px-4 py-2 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
					/>
				</div>

				<div class="flex-1 min-w-[200px]">
					<label
						for="origin-dest"
						class="block text-sm font-medium text-[var(--color-text)] mb-1"
					>
						{t('processing_tracker.origin', 'Origin country (optional)')}
					</label>
						<HintIcon text={t('tools.pt.origin_hint', 'The country you applied from.')} />
					<SelectField
						id="origin-dest"
						value={originDest}
						options={destinations.map((d) => ({ value: d.id, label: d.name }))}
						placeholder={t('geo.select_destination', 'Select a country…')}
						onValueChange={(v) => (originDest = v)}
					/>
				</div>

				<button
					type="submit"
					disabled={!selectedDest || !visaCategory.trim()}
					class="px-5 py-2 bg-[var(--color-primary)] text-white rounded-[var(--radius)] text-sm font-medium hover:opacity-90 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
				>
					{t('processing_tracker.search', 'Search')}
				</button>
			</div>
		</form>

		<!-- Stats cards -->
		{#if loadingStats}
			<div class="p-4 grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
				{#each Array(4) as _, i (i)}
					<LoadingSkeleton variant="card" count={1} />
				{/each}
			</div>
		{:else if hasSearched && stats.length === 0}
			<div class="p-4">
				<EmptyState
					icon={Timer}
					title={t('processing_tracker.no_stats', 'No processing data available')}
					body={t(
						'processing_tracker.no_stats_desc',
						'No processing time statistics are available for this corridor and visa category yet.'
					)}
				/>
			</div>
		{:else if latestStat}
			<div class="p-4">
				<div class="mb-8">
					<div class="flex items-center justify-between mb-3">
						<h2 class="text-sm font-semibold text-[var(--color-muted)] uppercase tracking-wide">
							{t('processing_tracker.headline_stats', 'Processing statistics')}
						</h2>
						<span class="text-xs text-[var(--color-muted)]">
							{t('processing_tracker.total_samples', '{n} samples').replace('{n}', String(totalSamples))}
						</span>
					</div>

					<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
						<!-- Average -->
						<div class="p-4 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius-card)]">
							<div class="flex items-center gap-2 text-[var(--color-muted)] text-xs mb-1">
								<Gauge size={14} />
								{t('processing_tracker.avg_days', 'Average')}
							</div>
							<p class="text-2xl font-bold text-[var(--color-text)]">
								{fmtDays(latestStat.avg_days)}
							</p>
						</div>

						<!-- Median -->
						<div class="p-4 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius-card)]">
							<div class="flex items-center gap-2 text-[var(--color-muted)] text-xs mb-1">
								<BarChart3 size={14} />
								{t('processing_tracker.median_days', 'Median')}
							</div>
							<p class="text-2xl font-bold text-[var(--color-text)]">
								{fmtDays(latestStat.median_days)}
							</p>
						</div>

						<!-- Fastest (p10) -->
						<div class="p-4 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius-card)]">
							<div class="flex items-center gap-2 text-[var(--color-muted)] text-xs mb-1">
								<Zap size={14} />
								{t('processing_tracker.fastest', 'Fastest (p10)')}
							</div>
							<p class="text-2xl font-bold text-green-600">
								{fmtDays(latestStat.p10_days)}
							</p>
						</div>

						<!-- Slowest (p90) -->
						<div class="p-4 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius-card)]">
							<div class="flex items-center gap-2 text-[var(--color-muted)] text-xs mb-1">
								<Hourglass size={14} />
								{t('processing_tracker.slowest', 'Slowest (p90)')}
							</div>
							<p class="text-2xl font-bold text-orange-600">
								{fmtDays(latestStat.p90_days)}
							</p>
						</div>
					</div>

					<!-- Meta row -->
					<div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[var(--color-muted)]">
						{#if latestStat.consulate}
							<span class="flex items-center gap-1">
								<Building2 size={12} />
								{latestStat.consulate}
							</span>
						{/if}
						<span>
							{t('processing_tracker.period', 'Period:')} {fmtDate(latestStat.period_start)}
						</span>
						<span>
							{t('processing_tracker.sample_count', 'Sample:')} {latestStat.sample_count}
						</span>
						{#if latestStat.refreshed_at}
							<span>
								{t('processing_tracker.refreshed', 'Updated:')} {fmtDate(latestStat.refreshed_at)}
							</span>
						{/if}
					</div>
				</div>

				<!-- All stat rows table -->
				{#if stats.length > 1}
					<div class="mb-8 overflow-x-auto">
						<h2 class="text-sm font-semibold text-[var(--color-muted)] uppercase tracking-wide mb-3">
							{t('processing_tracker.all_periods', 'All periods')}
						</h2>
						<table class="w-full text-sm">
							<thead>
								<tr class="text-start text-[var(--color-muted)] border-b border-[var(--color-border)]">
									<th class="py-2 pe-4 font-medium">{t('processing_tracker.period', 'Period')}</th>
									<th class="py-2 pe-4 font-medium">{t('processing_tracker.avg_days', 'Average')}</th>
									<th class="py-2 pe-4 font-medium">{t('processing_tracker.median_days', 'Median')}</th>
									<th class="py-2 pe-4 font-medium">{t('processing_tracker.fastest', 'p10')}</th>
									<th class="py-2 pe-4 font-medium">{t('processing_tracker.slowest', 'p90')}</th>
									<th class="py-2 font-medium">{t('processing_tracker.sample_count', 'Samples')}</th>
								</tr>
							</thead>
							<tbody>
								{#each [...stats].sort((a, b) => new Date(b.period_start).getTime() - new Date(a.period_start).getTime()) as stat (stat.period_start + stat.consulate)}
									<tr class="border-b border-[var(--color-border)]/50">
										<td class="py-2 pe-4 text-[var(--color-text)]">{fmtDate(stat.period_start)}</td>
										<td class="py-2 pe-4 text-[var(--color-text)]">{fmtDays(stat.avg_days)}</td>
										<td class="py-2 pe-4 text-[var(--color-text)]">{fmtDays(stat.median_days)}</td>
										<td class="py-2 pe-4 text-green-600">{fmtDays(stat.p10_days)}</td>
										<td class="py-2 pe-4 text-orange-600">{fmtDays(stat.p90_days)}</td>
										<td class="py-2 text-[var(--color-muted)]">{stat.sample_count}</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{/if}

				<!-- Trend chart -->
				<div class="mt-8">
					<h2 class="text-sm font-semibold text-[var(--color-muted)] uppercase tracking-wide mb-3">
						{t('processing_tracker.trend_title', '12-month trend (average days)')}
					</h2>
					{#if loadingTrend}
						<div class="h-60 w-full">
							<LoadingSkeleton variant="chart" class="h-full" />
						</div>
					{:else if chartData.length > 0}
						<div class="p-4 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius-card)]">
							<div class="h-60 w-full">
								<LineChart
									data={chartData}
									x="period_start"
									y="avg_days"
									props={{
										line: { class: 'stroke-[var(--chart-1)]' },
										area: { class: 'fill-[var(--chart-1)]/10' }
									}}
								/>
							</div>
						</div>
					{:else}
						<EmptyState
							icon={ChartLine}
							title={t('processing_tracker.no_trend', 'No trend data available')}
							body={t(
								'processing_tracker.no_trend_desc',
								'Trend data will appear here once enough historical processing data is collected.'
							)}
						/>
					{/if}
				</div>
			</div>
		{:else if !hasSearched}
			<div class="p-4">
				<EmptyState
					icon={Timer}
					title={t('processing_tracker.select_prompt', 'Select a destination and visa category')}
					body={t(
						'processing_tracker.select_prompt_desc',
						'Choose your destination country and enter a visa category to view processing time statistics.'
					)}
				/>
			</div>
		{/if}
	{/if}
</div>
