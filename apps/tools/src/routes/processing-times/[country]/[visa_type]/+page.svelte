<script lang="ts">
	import { untrack } from 'svelte';
	import { buildSeoMeta } from '$lib/seo';
	import SeoHead from '$lib/components/seo/SeoHead.svelte';
	import { t, siteName} from '$lib/i18n';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import { LineChart } from 'layerchart';
	import type { ProcessingStat, ProcessingTrendPoint } from '$lib/types';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	/**
	 * Aggregate corridor page — §83/§93.
	 * Public, SSR-rendered processing-time intelligence for one
	 * destination × visa-category corridor. SEO target; filters are
	 * query params so every filtered view stays linkable.
	 */
	let { data } = $props();

	let stats = $state<ProcessingStat[]>(untrack(() => data.stats));
	let trend = $state<ProcessingTrendPoint[]>(untrack(() => data.trend));

	const country = $derived(data.country);
	const visaLabel = $derived(
		data.visaCategory.replace(/_/g, ' ').replace(/\b\w/g, (c: string) => c.toUpperCase())
	);

	const seo = $derived(
		buildSeoMeta({
			title: t(
				'processing_tracker.aggregate_seo_title',
				'{visa} visa processing time — {country} | {site} Tools'
			)
				.replace('{visa}', visaLabel)
				.replace('{country}', country?.name ?? ''),
			description: t(
				'processing_tracker.aggregate_seo_desc',
				'Crowd-sourced {visa} visa processing times for {country} — average, median, and percentile wait times from real applicant timelines.'
			)
				.replace('{visa}', visaLabel)
				.replace('{country}', country?.name ?? ''),
			path: `/processing-times/${country?.slug}/${data.visaCategory.replace(/_/g, '-')}`,
			origin: page.url.origin,
			ogImage: `${page.url.origin}/og/og-processing-times.png`,
			jsonLd: {
				'@context': 'https://schema.org',
				'@type': 'Dataset',
				name: `${visaLabel} visa processing time — ${country?.name ?? ''}`,
				description: `Crowd-sourced ${visaLabel} visa processing times for ${country?.name ?? ''}.`,
				creator: { '@type': 'Organization', name: siteName() }
			}
		})
	);

	const latestStat = $derived.by(() => {
		if (stats.length === 0) return null;
		return [...stats].sort(
			(a, b) => new Date(b.period_start).getTime() - new Date(a.period_start).getTime()
		)[0];
	});

	const totalSamples = $derived(stats.reduce((sum, s) => sum + s.sample_count, 0));

	const chartData = $derived(
		trend
			.filter((p) => p.avg_days != null)
			.map((p) => ({
				period_start: new Date(p.period_start),
				avg_days: p.avg_days as number,
				median_days: p.median_days ?? null
			}))
	);

	function fmtDays(days?: number | null): string {
		if (days == null) return '—';
		return `${days.toFixed(0)} days`;
	}

	function fmtDate(iso: string): string {
		try {
			return new Date(iso).toLocaleDateString(undefined, {
				year: 'numeric',
				month: 'short',
				day: 'numeric'
			});
		} catch {
			return iso;
		}
	}

	function categorySlug(cat: string): string {
		return cat.toLowerCase().replace(/_/g, '-');
	}
</script>

<SeoHead seo={seo} ogType="website" twitterCard="summary" />

<div class="mx-auto max-w-5xl px-4 py-8">
	{#if !data.enabled}
		<ToolDisabled tool={data.tool} />
	{:else}
		<div class="mb-8">
			<a
				href="/processing-times"
				class="inline-flex items-center gap-1 text-sm text-muted hover:text-text mb-2"
			>
				<Icon name="arrow-left" size={14} />
				{t('processing_tracker.back', 'Processing Time Tracker')}
			</a>
			<div class="flex items-start justify-between gap-4">
				<div>
					<h1 class="text-2xl sm:text-3xl font-bold text-text">
						{visaLabel} — {country?.name}
					</h1>
					<p class="mt-2 text-text-subtle">
						{t(
							'processing_tracker.aggregate_subtitle',
							'Crowd-sourced processing times from real applicant timelines. Data is anonymised and aggregated.'
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

		<!-- Filter panel — GET form so filtered views stay shareable/SSR -->
		<form method="GET" class="mb-6 p-4 bg-surface border border-border rounded-lg grid sm:grid-cols-3 gap-3 items-end">
			<div>
				<label class="block text-xs font-medium text-muted mb-1" for="origin">
					{t('processing_tracker.origin', 'Origin country')}
				</label>
				<AutocompleteSelect
					id="origin"
					name="origin"
					value={data.originId ?? ''}
					options={[
						{ value: '', label: t('processing_tracker.any_origin', 'Any origin') },
						...data.origins.map((o) => ({ value: o.id, label: o.name }))
					]}
					class="w-full"
				/>
			</div>
			<div>
				<label class="block text-xs font-medium text-muted mb-1" for="consulate">
					{t('processing_tracker.consulate', 'Consulate')}
				</label>
				<input
					id="consulate"
					name="consulate"
					value={data.consulate ?? ''}
					placeholder={t('processing_tracker.consulate_placeholder', 'e.g. Lagos')}
					class="w-full px-3 py-2 text-sm bg-surface border border-border rounded-lg text-text focus:border-[var(--tool-accent)] focus:outline-none"
				/>
			</div>
			<button
				type="submit"
				class="px-4 py-2 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors"
			>
				{t('processing_tracker.apply_filters', 'Apply filters')}
			</button>
		</form>

		{#if stats.length === 0}
			<EmptyState
				icon="timer"
				title={t('processing_tracker.no_stats', 'No processing data available')}
				description={t(
					'processing_tracker.no_stats_corridor',
					'No one has shared a timeline for this corridor yet. Track your own application to contribute.'
				)}
			/>
		{:else if latestStat}
			<div class="mb-8">
				<div class="flex items-center justify-between mb-3">
					<h2 class="text-sm font-semibold text-muted uppercase tracking-wide">
						{t('processing_tracker.headline_stats', 'Processing statistics')}
					</h2>
					<span class="text-xs text-muted">
						{t('processing_tracker.total_samples', '{n} samples').replace('{n}', String(totalSamples))}
					</span>
				</div>

				<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
					<div class="p-4 bg-surface border border-border rounded-lg">
						<div class="flex items-center gap-2 text-muted text-xs mb-1">
							<Icon name="gauge" size={14} />
							{t('processing_tracker.avg_days', 'Average')}
						</div>
						<p class="text-2xl font-bold text-text">{fmtDays(latestStat.avg_days)}</p>
					</div>
					<div class="p-4 bg-surface border border-border rounded-lg">
						<div class="flex items-center gap-2 text-muted text-xs mb-1">
							<Icon name="bar-chart-3" size={14} />
							{t('processing_tracker.median_days', 'Median')}
						</div>
						<p class="text-2xl font-bold text-text">{fmtDays(latestStat.median_days)}</p>
					</div>
					<div class="p-4 bg-surface border border-border rounded-lg">
						<div class="flex items-center gap-2 text-muted text-xs mb-1">
							<Icon name="zap" size={14} />
							{t('processing_tracker.fastest', 'Fastest (p10)')}
						</div>
						<p class="text-2xl font-bold text-green-600">{fmtDays(latestStat.p10_days)}</p>
					</div>
					<div class="p-4 bg-surface border border-border rounded-lg">
						<div class="flex items-center gap-2 text-muted text-xs mb-1">
							<Icon name="hourglass" size={14} />
							{t('processing_tracker.slowest', 'Slowest (p90)')}
						</div>
						<p class="text-2xl font-bold text-orange-600">{fmtDays(latestStat.p90_days)}</p>
					</div>
				</div>

				<div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted">
					{#if latestStat.consulate}
						<span class="flex items-center gap-1">
							<Icon name="building-2" size={12} />
							{latestStat.consulate}
						</span>
					{/if}
					<span>{t('processing_tracker.period', 'Period:')} {fmtDate(latestStat.period_start)}</span>
					<span>{t('processing_tracker.sample_count', 'Sample:')} {latestStat.sample_count}</span>
					{#if latestStat.refreshed_at}
						<span>{t('processing_tracker.refreshed', 'Updated:')} {fmtDate(latestStat.refreshed_at)}</span>
					{/if}
				</div>
			</div>

			{#if stats.length > 1}
				<div class="mb-8 overflow-x-auto">
					<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
						{t('processing_tracker.all_periods', 'All periods')}
					</h2>
					<table class="w-full text-sm">
						<thead>
							<tr class="text-left text-muted border-b border-border">
								<th class="py-2 pr-4 font-medium">{t('processing_tracker.period', 'Period')}</th>
								<th class="py-2 pr-4 font-medium">{t('processing_tracker.avg_days', 'Average')}</th>
								<th class="py-2 pr-4 font-medium">{t('processing_tracker.median_days', 'Median')}</th>
								<th class="py-2 pr-4 font-medium">{t('processing_tracker.fastest', 'p10')}</th>
								<th class="py-2 pr-4 font-medium">{t('processing_tracker.slowest', 'p90')}</th>
								<th class="py-2 font-medium">{t('processing_tracker.sample_count', 'Samples')}</th>
							</tr>
						</thead>
						<tbody>
							{#each [...stats].sort((a, b) => new Date(b.period_start).getTime() - new Date(a.period_start).getTime()) as stat (stat.id ?? stat.period_start + stat.consulate)}
								<tr class="border-b border-border/50">
									<td class="py-2 pr-4 text-text">{fmtDate(stat.period_start)}</td>
									<td class="py-2 pr-4 text-text">{fmtDays(stat.avg_days)}</td>
									<td class="py-2 pr-4 text-text">{fmtDays(stat.median_days)}</td>
									<td class="py-2 pr-4 text-green-600">{fmtDays(stat.p10_days)}</td>
									<td class="py-2 pr-4 text-orange-600">{fmtDays(stat.p90_days)}</td>
									<td class="py-2 text-muted">{stat.sample_count}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}

			<div class="mt-8">
				<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
					{t('processing_tracker.trend_title', '12-month trend (average days)')}
				</h2>
				{#if chartData.length > 0}
					<div class="p-4 bg-surface border border-border rounded-lg">
						<div class="h-60 w-full">
							<LineChart data={chartData} x="period_start" y="avg_days" cRange={['var(--chart-1)']} />
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
		{/if}

		<!-- Sibling corridors — SEO interlinking (§83) -->
		{#if data.siblingCategories.length > 0 && country}
			<div class="mt-10 pt-6 border-t border-border">
				<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
					{t('processing_tracker.other_visas', 'Other visa types — {country}').replace(
						'{country}',
						country.name
					)}
				</h2>
				<div class="flex flex-wrap gap-2">
					{#each data.siblingCategories as cat (cat)}
						<a
							href="/processing-times/{country.slug}/{categorySlug(cat)}"
							class="px-3 py-1.5 text-sm bg-surface border border-border rounded-lg text-text hover:border-[var(--tool-accent)] transition-colors"
						>
							{cat.replace(/_/g, ' ')}
						</a>
					{/each}
				</div>
			</div>
		{/if}
	{/if}
</div>
