<script lang="ts">
	import { t } from '$lib/i18n';
	import { Chart, Svg, Bars, Axis, LineChart } from 'layerchart';
	import DashboardPanel from '$lib/components/hub/DashboardPanel.svelte';
	import MetricCard from '$lib/components/hub/MetricCard.svelte';
	import { LoadingSkeleton } from '$lib/components/ui';
	import { formatMetric } from '$lib/components/hub/charts/format';
	import type { MeAnalytics } from '$lib/types';

	interface ContentAnalyticsItem {
		id: string;
		title: string;
		content_type: string;
		status: string;
		view_count: number;
		comment_count: number;
		share_count: number;
		created_at: string;
	}

	interface FollowerGrowthItem {
		day: string;
		new_followers: number;
	}

	interface AnalyticsData {
		analytics: MeAnalytics | null;
		contentAnalytics?: ContentAnalyticsItem[];
		followerGrowth?: FollowerGrowthItem[];
	}

	let { data }: { data: unknown } = $props();
	const payload = $derived(data as AnalyticsData | null);
	const analytics = $derived(payload?.analytics ?? null);
	const contentAnalytics = $derived(payload?.contentAnalytics ?? []);
	const followerGrowth = $derived(payload?.followerGrowth ?? []);

	// Content-status breakdown series for the bar chart — derived from the
	// single /me/analytics payload (no separate endpoint yet). Uses label as
	// the x category so scaleBand renders proper bar widths.
	const statusSeries = $derived(
		analytics
			? [
					{ x: t('dashboard.analytics.drafts', 'Drafts'), y: analytics.draft_content },
					{ x: t('dashboard.analytics.published', 'Published'), y: analytics.published_content },
					{ x: t('dashboard.analytics.other', 'Other'), y: Math.max(0, analytics.total_content - analytics.draft_content - analytics.published_content) }
				]
			: []
	);

	// Top posts series — vertical BarChart of view_count per content title.
	// Phase 18.01a: "top posts BarChart (me/analytics/content)".
	// Ref: ADR-059. Uses truncated title as x category for scaleBand.
	const topPostsSeries = $derived(
		contentAnalytics
			.slice(0, 10)
			.map((item) => ({
				id: item.id,
				x: item.title?.length > 20 ? item.title.slice(0, 20) + '…' : item.title ?? item.id,
				y: item.view_count ?? 0,
				label: item.title?.length > 30 ? item.title.slice(0, 30) + '…' : item.title ?? item.id
			}))
	);

	// Follower growth series — LineChart of daily new followers over 30 days.
	// Phase 18.01a: "follower growth LineChart (me/analytics/followers)".
	// Ref: ADR-059
	const followerSeries = $derived(
		followerGrowth.map((item, i) => ({
			x: i,
			y: item.new_followers ?? 0,
			label: item.day
		}))
	);

	// Impressions over time series — LineChart of daily content views.
	// Phase 18.01a: "impressions over time (me/analytics)".
	// Ref: sections/40-platform-analytics-site-reports.md
	const impressionsSeries = $derived(
		(analytics?.daily_impressions ?? []).map((item, i) => ({
			x: i,
			y: item.views ?? 0,
			label: item.day
		}))
	);

	// Readers by country series — horizontal/country BarChart.
	const countrySeries = $derived(
		(analytics?.readers_by_country ?? []).map((item) => ({
			x: item.country_code || t('dashboard.analytics.unknown_country', 'Unknown'),
			y: item.readers ?? 0
		}))
	);
</script>

<section class="space-y-6">
	<!-- Metric cards row — Phase 18.01a -->
	<div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
		<MetricCard labelKey="dashboard.analytics.views" value={analytics?.total_views ?? 0} />
		<MetricCard labelKey="dashboard.analytics.reactions" value={analytics?.total_reactions ?? 0} />
		<MetricCard labelKey="dashboard.analytics.comments" value={analytics?.total_comments ?? 0} />
		<MetricCard labelKey="dashboard.analytics.followers_gained" value={analytics?.total_followers_gained ?? 0} />
	</div>

	<!-- Impressions over time — LineChart -->
	<DashboardPanel titleKey="dashboard.analytics.impressions">
		{#if !analytics}
			<div class="h-56">
				<LoadingSkeleton variant="chart" />
			</div>
		{:else if impressionsSeries.length === 0}
			<p class="text-sm text-[var(--color-muted)]">{t('dashboard.analytics.no_impressions', 'No impressions data yet.')}</p>
		{:else}
			<div class="h-56">
				<LineChart
					data={impressionsSeries}
					x="x"
					y="y"
					props={{
						line: { class: 'stroke-[var(--chart-1,var(--color-primary))]' },
						area: { class: 'fill-[var(--chart-1,var(--color-primary))]/10' }
					}}
				/>
			</div>
			<p class="mt-2 text-xs text-[var(--color-muted)]">
				{impressionsSeries.reduce((sum, d) => sum + d.y, 0)} {t('dashboard.analytics.views', 'views')}
			</p>
		{/if}
	</DashboardPanel>

	<!-- Readers by country — BarChart -->
	<DashboardPanel titleKey="dashboard.analytics.readers_by_country">
		{#if !analytics}
			<div class="h-56">
				<LoadingSkeleton variant="chart" />
			</div>
		{:else if countrySeries.length === 0}
			<p class="text-sm text-[var(--color-muted)]">{t('dashboard.analytics.no_country_data', 'No country data yet.')}</p>
		{:else}
			<div class="h-56">
				<Chart data={countrySeries} x="x" y="y" bandPadding={0.3} padding={{ left: 48, bottom: 60 }}>
					<Svg>
						<Axis placement="left" ticks={4} format={(v: number) => formatMetric(v)} grid rule />
						<Axis placement="bottom" format={(v: string) => v} />
						<Bars
							fill="var(--chart-3, var(--color-primary))"
							stroke="var(--color-surface)"
							strokeWidth={1}
							radius={4}
						/>
					</Svg>
				</Chart>
			</div>
		{/if}
	</DashboardPanel>

	<!-- Content status breakdown — existing bar chart -->
	<DashboardPanel titleKey="dashboard.analytics.content_breakdown">
		{#if !analytics}
			<div class="h-56">
				<LoadingSkeleton variant="chart" />
			</div>
		{:else if statusSeries.length === 0}
			<p class="text-sm text-[var(--color-muted)]">{t('dashboard.analytics.unavailable', 'Analytics unavailable right now.')}</p>
		{:else}
			<div class="h-56">
				<Chart data={statusSeries} x="x" y="y" bandPadding={0.4} padding={{ left: 48, bottom: 24 }}>
					<Svg>
						<Axis placement="left" ticks={4} format={(v: number) => formatMetric(v)} grid rule />
						<Axis placement="bottom" />
						<Bars
							fill="var(--color-primary)"
							stroke="var(--color-surface)"
							strokeWidth={1}
							radius={4}
						/>
					</Svg>
				</Chart>
			</div>
		{/if}
	</DashboardPanel>

	<!-- Top posts — vertical BarChart (Phase 18.01a) -->
	<DashboardPanel titleKey="dashboard.analytics.top_posts">
		{#if !analytics}
			<div class="h-56">
				<LoadingSkeleton variant="chart" />
			</div>
		{:else if topPostsSeries.length === 0}
			<p class="text-sm text-[var(--color-muted)]">{t('dashboard.analytics.no_content', 'No published content yet.')}</p>
		{:else}
			<div class="h-64">
				<Chart data={topPostsSeries} x="x" y="y" bandPadding={0.3} padding={{ left: 48, bottom: 60 }}>
					<Svg>
						<Axis placement="left" ticks={4} format={(v: number) => formatMetric(v)} grid rule />
						<Axis placement="bottom" format={(v: string) => v} />
						<Bars
							fill="var(--chart-1, var(--color-primary))"
							stroke="var(--color-surface)"
							strokeWidth={1}
							radius={4}
						/>
					</Svg>
				</Chart>
			</div>
			<div class="mt-2 space-y-1 text-xs text-[var(--color-muted)]">
				{#each topPostsSeries as point (point.id)}
					<div class="flex items-center justify-between">
						<span class="truncate">{point.label}</span>
						<span class="shrink-0 pl-2">{formatMetric(point.y)} {t('profile.views', 'views')}</span>
					</div>
				{/each}
			</div>
		{/if}
	</DashboardPanel>

	<!-- Follower growth — LineChart (Phase 18.01a) -->
	<DashboardPanel titleKey="dashboard.analytics.follower_growth">
		{#if !analytics}
			<div class="h-56">
				<LoadingSkeleton variant="chart" />
			</div>
		{:else if followerSeries.length === 0}
			<p class="text-sm text-[var(--color-muted)]">{t('dashboard.analytics.no_growth_data', 'No follower growth data yet.')}</p>
		{:else}
			<div class="h-56">
				<LineChart
					data={followerSeries}
					x="x"
					y="y"
					props={{
						line: { class: 'stroke-[var(--chart-2,var(--color-primary))]' },
						area: { class: 'fill-[var(--chart-2,var(--color-primary))]/10' }
					}}
				/>
			</div>
			<p class="mt-2 text-xs text-[var(--color-muted)]">
				{t('dashboard.analytics.last_30_days', 'Last 30 days')} · {followerSeries.reduce((sum, d) => sum + d.y, 0)} {t('dashboard.analytics.new_followers', 'new followers')}
			</p>
		{/if}
	</DashboardPanel>
</section>
