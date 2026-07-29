<script lang="ts">
	import { t } from '$lib/i18n';
	import { Chart, Svg, Bars, Axis } from 'layerchart';
	import DashboardPanel from '$lib/components/hub/DashboardPanel.svelte';
	import MetricCard from '$lib/components/hub/MetricCard.svelte';
	import { LoadingSkeleton } from '$lib/components/ui';
	import { formatMetric } from '$lib/components/hub/charts/format';
	import type { MeAnalytics } from '$lib/types';

	interface AnalyticsData {
		analytics: MeAnalytics | null;
	}

	let { data }: { data: unknown } = $props();
	const payload = $derived(data as AnalyticsData | null);
	const analytics = $derived(payload?.analytics ?? null);

	// Content-status breakdown series for the bar chart — derived from the
	// single /me/analytics payload (no separate endpoint yet).
	const statusSeries = $derived(
		analytics
			? [
					{ x: 0, y: analytics.draft_content, label: t('dashboard.analytics.drafts', 'Drafts') },
					{ x: 1, y: analytics.published_content, label: t('dashboard.analytics.published', 'Published') },
					{ x: 2, y: Math.max(0, analytics.total_content - analytics.draft_content - analytics.published_content), label: t('dashboard.analytics.other', 'Other') }
				]
			: []
	);
</script>

<section class="space-y-6">
	<div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
		<MetricCard labelKey="dashboard.analytics.total_content" value={analytics?.total_content ?? 0} />
		<MetricCard labelKey="dashboard.analytics.published" value={analytics?.published_content ?? 0} />
		<MetricCard labelKey="dashboard.analytics.drafts" value={analytics?.draft_content ?? 0} />
		<MetricCard labelKey="dashboard.analytics.bookmarks" value={analytics?.total_bookmarks ?? 0} />
	</div>

	<DashboardPanel titleKey="dashboard.analytics.content_breakdown">
		{#if !analytics}
			<!-- H7: skeleton loading state while analytics loads. Distinguishes
			"loading" from "unavailable" — if the payload is null we show a
			skeleton rather than an error message, since the dashboard view
			resolver passes null until the API responds. -->
			<div class="h-56">
				<LoadingSkeleton variant="chart" />
			</div>
		{:else if statusSeries.length === 0}
			<p class="text-sm text-[var(--color-muted)]">{t('dashboard.analytics.unavailable', 'Analytics unavailable right now.')}</p>
		{:else}
			<div class="h-56">
				<Chart data={statusSeries} x="x" y="y">
					<Svg>
						<Axis placement="left" ticks={4} format={(v: number) => formatMetric(v)} />
						<Bars
							fill="var(--color-primary)"
							stroke="var(--color-surface)"
							strokeWidth={1}
						/>
					</Svg>
				</Chart>
			</div>
			<div class="mt-2 flex justify-around text-xs text-[var(--color-muted)]">
				{#each statusSeries as point (point.x)}
					<span>{point.label}</span>
				{/each}
			</div>
		{/if}
	</DashboardPanel>
</section>
