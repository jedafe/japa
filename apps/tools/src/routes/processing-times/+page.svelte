<script lang="ts">
	import { untrack, onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { buildSeoMeta } from '$lib/seo';
	import SeoHead from '$lib/components/seo/SeoHead.svelte';
	import { t, siteName, i18n } from '$lib/i18n';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import LoadingSkeleton from '$lib/components/ui/LoadingSkeleton.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import DestinationSelector from '$lib/components/tools/DestinationSelector.svelte';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import { LineChart } from 'layerchart';
	import type { GeoSpaceOption, ProcessingStat, ProcessingTrendPoint } from '$lib/types';

	/**
	 * Visa Processing Time Tracker tool — §93.
	 * Destination selector → visa category input → stats cards + trend chart.
	 * Reuses the §83 visa timeline backend.
	 */
	let { data } = $props();

	let destinations = $state<GeoSpaceOption[]>(untrack(() => data.destinations));
	let stats = $state<ProcessingStat[]>(untrack(() => data.stats));
	let trend = $state<ProcessingTrendPoint[]>(untrack(() => data.trend));

	let selectedDest = $state('');
	let visaCategory = $state('');
	let originDest = $state('');
	let loadingStats = $state(false);
	let loadingTrend = $state(false);
	let hasSearched = $state(false);

	const seo = $derived(
		buildSeoMeta({
			title: t(
				'processing_tracker.seo_title',
				'Visa Processing Time Tracker — {site} Tools'
			),
			description: t(
				'processing_tracker.seo_description',
				'View visa processing time statistics and trends — average, median, and percentile wait times for your corridor.'
			),
			path: '/processing-times',
			origin: page.url.origin,
			ogImage: `${page.url.origin}/og/og-processing-times.png`,
			jsonLd: {
				'@context': 'https://schema.org',
				'@type': 'Dataset',
				name: 'Visa Processing Time Statistics',
				description: 'Crowd-sourced visa processing times — average, median, and percentile wait times.',
				creator: { '@type': 'Organization', name: siteName() }
			}
		})
	);

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

	onMount(() => {
		// No auto-load — requires dest + visa selection.
	});

	function fetchDestinations(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? destinations
			: destinations.filter((d) => d.name.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((d) => ({ id: d.id, label: d.name })));
	}

	function fetchDestinationsInitial(): Promise<AutocompleteItem[]> {
		return fetchDestinations('');
	}

	function getDestinationLabel(id: string): string {
		return destinations.find((d) => d.id === id)?.name ?? id;
	}

	// §83 — every corridor has a shareable SEO aggregate page at
	// /processing-times/{country-slug}/{visa-slug}.
	const aggregateHref = $derived.by(() => {
		if (!selectedDest || !visaCategory.trim()) return null;
		const slug = destinations.find((d) => d.id === selectedDest)?.slug;
		if (!slug) return null;
		return `/processing-times/${slug}/${visaCategory.trim().toLowerCase().replace(/[\s_]+/g, '-')}`;
	});
</script>

<SeoHead seo={seo} ogType="website" twitterCard="summary" />

<div class="mx-auto max-w-5xl px-4 py-8">
	{#if !data.enabled}
		<ToolDisabled tool={data.tool} />
	{:else}
		<div class="mb-8">
			<div class="flex items-start justify-between gap-4">
				<div>
					<h1 class="text-2xl sm:text-3xl font-bold text-text">
						{t('processing_tracker.title', 'Visa Processing Time Tracker')}
					</h1>
					<p class="mt-2 text-text-subtle">
						{t(
							'processing_tracker.subtitle',
							'See how long visa processing takes — average, median, and percentile wait times for your corridor.'
						)}
					</p>
				</div>
				{#if data.user}
					<a
						href="/processing-times/my-applications"
						class="flex-shrink-0 inline-flex items-center gap-2 px-4 py-2 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors"
					>
						<Icon name="timer" size={16} />
						{t('processing_tracker.my_apps_link', 'My applications')}
					</a>
				{/if}
			</div>
		</div>

		{#if destinations.length === 0}
			<EmptyState
				icon="timer"
				title={t('processing_tracker.no_destinations', 'No destinations available')}
				description={t(
					'processing_tracker.no_destinations_desc',
					'Destination data has not been configured yet. Please check back later.'
				)}
			/>
		{:else}
			<!-- Destination + visa category selector -->
			<form onsubmit={onSearch} class="mb-6 space-y-4">
				<DestinationSelector
					{destinations}
					bind:selectedId={selectedDest}
					onchange={onDestChange}
				/>

				<div class="flex flex-wrap items-end gap-3">
					<div class="flex-1 min-w-[200px]">
						<label
							for="visa-category"
							class="block text-sm font-medium text-text mb-1"
						>
							{t('processing_tracker.visa_category', 'Visa category')}
						</label>
						<input
							id="visa-category"
							type="text"
							bind:value={visaCategory}
							placeholder={t(
								'processing_tracker.visa_placeholder',
								'student, work, tourist…'
							)}
							class="w-full px-4 py-2 bg-surface border border-border rounded-lg text-text focus:border-primary focus:outline-none"
						/>
					</div>

					<div class="flex-1 min-w-[200px]">
						<label
							for="origin-dest"
							class="block text-sm font-medium text-text mb-1"
						>
							{t('processing_tracker.origin', 'Origin country (optional)')}
						</label>
						<Autocomplete
							id="origin-dest"
							placeholder={t('geo.select_destination', 'Select a country…')}
							emptyMessage={t('geo.no_destinations', 'No destinations found')}
							fetcher={fetchDestinations}
							initialFetcher={fetchDestinationsInitial}
							selectedId={originDest || null}
							selectedLabel={originDest ? getDestinationLabel(originDest) : null}
							onSelect={(item) => (originDest = item.id)}
							onClear={() => (originDest = '')}
						/>
					</div>

					<button
						type="submit"
						disabled={!selectedDest || !visaCategory.trim()}
						class="px-5 py-2 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
					>
						{t('processing_tracker.search', 'Search')}
					</button>
				</div>
			</form>

			<!-- Stats cards -->
			{#if loadingStats}
				<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
					{#each Array(4) as _}
						<LoadingSkeleton height="100px" />
					{/each}
				</div>
			{:else if hasSearched && stats.length === 0}
				<EmptyState
					icon="timer"
					title={t('processing_tracker.no_stats', 'No processing data available')}
					description={t(
						'processing_tracker.no_stats_desc',
						'No processing time statistics are available for this corridor and visa category yet.'
					)}
				/>
			{:else if latestStat}
				<div class="mb-8">
					<div class="flex items-center justify-between mb-3">
						<h2 class="text-sm font-semibold text-muted uppercase tracking-wide">
							{t('processing_tracker.headline_stats', 'Processing statistics')}
						</h2>
						<div class="flex items-center gap-3">
							<span class="text-xs text-muted">
								{t('processing_tracker.total_samples', '{n} samples').replace('{n}', String(totalSamples))}
							</span>
							{#if aggregateHref}
								<a
									href={aggregateHref}
									class="inline-flex items-center gap-1 text-xs text-[var(--tool-accent)] hover:underline"
								>
									<Icon name="link" size={12} />
									{t('processing_tracker.shareable_page', 'Shareable page')}
								</a>
							{/if}
						</div>
					</div>

					<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
						<!-- Average -->
						<div class="p-4 bg-surface border border-border rounded-lg">
							<div class="flex items-center gap-2 text-muted text-xs mb-1">
								<Icon name="gauge" size={14} />
								{t('processing_tracker.avg_days', 'Average')}
							</div>
							<p class="text-2xl font-bold text-text">
								{fmtDays(latestStat.avg_days)}
							</p>
						</div>

						<!-- Median -->
						<div class="p-4 bg-surface border border-border rounded-lg">
							<div class="flex items-center gap-2 text-muted text-xs mb-1">
								<Icon name="bar-chart-3" size={14} />
								{t('processing_tracker.median_days', 'Median')}
							</div>
							<p class="text-2xl font-bold text-text">
								{fmtDays(latestStat.median_days)}
							</p>
						</div>

						<!-- Fastest (p10) -->
						<div class="p-4 bg-surface border border-border rounded-lg">
							<div class="flex items-center gap-2 text-muted text-xs mb-1">
								<Icon name="zap" size={14} />
								{t('processing_tracker.fastest', 'Fastest (p10)')}
							</div>
							<p class="text-2xl font-bold text-green-600">
								{fmtDays(latestStat.p10_days)}
							</p>
						</div>

						<!-- Slowest (p90) -->
						<div class="p-4 bg-surface border border-border rounded-lg">
							<div class="flex items-center gap-2 text-muted text-xs mb-1">
								<Icon name="hourglass" size={14} />
								{t('processing_tracker.slowest', 'Slowest (p90)')}
							</div>
							<p class="text-2xl font-bold text-orange-600">
								{fmtDays(latestStat.p90_days)}
							</p>
						</div>
					</div>

					<!-- Meta row -->
					<div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted">
						{#if latestStat.consulate}
							<span class="flex items-center gap-1">
								<Icon name="building-2" size={12} />
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
						<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
							{t('processing_tracker.all_periods', 'All periods')}
						</h2>
						<table class="w-full text-sm">
							<thead>
								<tr class="text-start text-muted border-b border-border">
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
									<tr class="border-b border-border/50">
										<td class="py-2 pe-4 text-text">{fmtDate(stat.period_start)}</td>
										<td class="py-2 pe-4 text-text">{fmtDays(stat.avg_days)}</td>
										<td class="py-2 pe-4 text-text">{fmtDays(stat.median_days)}</td>
										<td class="py-2 pe-4 text-green-600">{fmtDays(stat.p10_days)}</td>
										<td class="py-2 pe-4 text-orange-600">{fmtDays(stat.p90_days)}</td>
										<td class="py-2 text-muted">{stat.sample_count}</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{/if}

				<!-- Trend chart -->
				<div class="mt-8">
					<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
						{t('processing_tracker.trend_title', '12-month trend (average days)')}
					</h2>
					{#if loadingTrend}
						<LoadingSkeleton height="240px" />
					{:else if chartData.length > 0}
						<div class="p-4 bg-surface border border-border rounded-lg">
							<div class="h-60 w-full">
								<LineChart
									data={chartData}
									x="period_start"
									y="avg_days"
									cRange={['var(--chart-1)']}
								/>
							</div>
						</div>
					{:else}
						<EmptyState
							icon="chart-line"
							title={t('processing_tracker.no_trend', 'No trend data available')}
							description={t(
								'processing_tracker.no_trend_desc',
								'Trend data will appear here once enough historical processing data is collected.'
							)}
						/>
					{/if}
				</div>
			{:else if !hasSearched}
				<EmptyState
					icon="timer"
					title={t('processing_tracker.select_prompt', 'Select a destination and visa category')}
					description={t(
						'processing_tracker.select_prompt_desc',
						'Choose your destination country and enter a visa category to view processing time statistics.'
					)}
				/>
			{/if}
		{/if}
	{/if}
</div>
