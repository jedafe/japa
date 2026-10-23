<script lang="ts">
	import { t, i18n } from '$lib/i18n';
	import { goto } from '$app/navigation';
	import { AreaChart, BarChart, LineChart } from 'layerchart';
	import { Tabs } from 'bits-ui';
	import DashboardPanel from '$lib/components/hub/DashboardPanel.svelte';
	import MetricCard from '$lib/components/hub/MetricCard.svelte';
	import { LoadingSkeleton } from '$lib/components/ui';
	import { formatMetric } from '$lib/components/hub/charts/format';
	import type { MeAnalytics, FollowerGrowthItem, TopContentItem, UserDashboard } from '$lib/types';

	interface AnalyticsData {
		period: '7d' | '30d' | '90d' | 'custom';
		start?: string;
		end?: string;
		analytics: MeAnalytics | null;
		deltas: {
			impressions: number;
			reactions: number;
			comments: number;
			followers_gained: number;
			posts: number;
			bookmarks: number;
			points: number;
		};
		followerGrowth: FollowerGrowthItem[];
		userDashboard: UserDashboard | null;
	}

	let { data }: { data: unknown } = $props();
	const payload = $derived(data as AnalyticsData);

	const analytics = $derived(payload.analytics ?? null);
	const userDashboard = $derived(payload.userDashboard ?? null);
	const followerGrowth = $derived(payload.followerGrowth ?? []);
	const period = $derived(payload.period ?? '30d');
	const start = $derived(payload.start);
	const end = $derived(payload.end);
	const deltas = $derived(payload.deltas ?? {
		impressions: 0,
		reactions: 0,
		comments: 0,
		followers_gained: 0,
		posts: 0,
		bookmarks: 0,
		points: 0
	});

	// Period selector state. Default to a 30-day custom range; the effect below
	// synchronises with the URL on load and after navigation.
	let selectedPeriod = $state<'7d' | '30d' | '90d' | 'custom'>('30d');
	let customStart = $state(new Date(Date.now() - 30 * 86400000).toISOString().slice(0, 10));
	let customEnd = $state(new Date().toISOString().slice(0, 10));

	$effect(() => {
		selectedPeriod = period;
		if (start) customStart = new Date(start).toISOString().slice(0, 10);
		if (end) customEnd = new Date(end).toISOString().slice(0, 10);
	});

	const periodOptions = [
		{ key: '7d', label: t('dashboard.analytics.last_7_days', 'Last 7 days') },
		{ key: '30d', label: t('dashboard.analytics.last_30_days', 'Last 30 days') },
		{ key: '90d', label: t('dashboard.analytics.last_90_days', 'Last 90 days') },
		{ key: 'custom', label: t('dashboard.analytics.custom_range', 'Custom') }
	] as const;

	// Section 40 dashboard tabs group related analytics panels so each panel can
	// render full-width (single column) without competing for horizontal space.
	let activeTab = $state('overview');
	const tabs = [
		{ value: 'overview', label: t('dashboard.analytics.tab_overview', 'Overview') },
		{ value: 'content', label: t('dashboard.analytics.tab_content', 'Content') },
		{ value: 'audience', label: t('dashboard.analytics.tab_audience', 'Audience') },
		{ value: 'engagement', label: t('dashboard.analytics.tab_engagement', 'Engagement') },
		{ value: 'ads', label: t('dashboard.analytics.tab_ads', 'Ads') }
	] as const;

	function applyPeriod() {
		if (selectedPeriod === 'custom') {
			const startTs = new Date(customStart + 'T00:00:00Z').toISOString();
			const endTs = new Date(customEnd + 'T23:59:59Z').toISOString();
			goto(`/dashboard/analytics?period=custom&start=${encodeURIComponent(startTs)}&end=${encodeURIComponent(endTs)}`, {
				invalidateAll: true
			});
		} else {
			goto(`/dashboard/analytics?period=${selectedPeriod}`, { invalidateAll: true });
		}
	}

	function formatDate(dateString: string | undefined): string {
		if (!dateString) return '';
		const d = new Date(dateString);
		if (Number.isNaN(d.getTime())) return '';
		return d.toLocaleDateString(i18n.locale, {
			month: 'short',
			day: 'numeric'
		});
	}

	// Label for the active period — used by chart captions so the summary text
	// reflects the selected window instead of a hardcoded "Last 30 days".
	const periodLabel = $derived(
		period === 'custom' && start && end
			? `${formatDate(start)} – ${formatDate(end)}`
			: period === '7d'
				? t('dashboard.analytics.last_7_days', 'Last 7 days')
				: period === '90d'
					? t('dashboard.analytics.last_90_days', 'Last 90 days')
					: t('dashboard.analytics.last_30_days', 'Last 30 days')
	);

	// Custom range validation — a start after the end produces a silently
	// empty window server-side; block Apply and tell the user instead.
	const customRangeInverted = $derived(!!customStart && !!customEnd && customStart > customEnd);
	const customRangeInvalid = $derived(!customStart || !customEnd || customRangeInverted);

	// Tick generator that forces the last tick to reach at least 95% of the
	// data maximum, preventing charts from showing empty space past the largest
	// bar (or from omitting the max value on the value axis).
	function valueTicks(scale: any) {
		if (typeof scale?.ticks !== 'function') return [];
		let domain: number[];
		try {
			domain = scale.domain();
		} catch {
			return scale.ticks(5);
		}
		if (!Array.isArray(domain) || domain.length < 2) return scale.ticks(5);
		const max = domain[1];
		if (typeof max !== 'number' || !Number.isFinite(max)) return scale.ticks(5);

		for (let count = 2; count <= 10; count++) {
			const ticks = scale.ticks(count);
			if (Array.isArray(ticks) && ticks.length > 0) {
				const last = ticks[ticks.length - 1] as number;
				if (last >= max * 0.95) return ticks;
			}
		}
		return scale.ticks(10);
	}

	const commonPadding = { top: 8, right: 12, bottom: 28 } as const;
	const valueXAxis = { format: 'integer' as const, ticks: valueTicks } as const;
	const valueYAxis = { format: 'integer' as const, ticks: valueTicks } as const;
	const categoryYAxis = {
		rule: false,
		tickMarks: false,
		tickLabelProps: { class: 'hidden' }
	} as const;

	function formatContentType(type: string): string {
		return type
			.replace(/_/g, ' ')
			.replace(/\b\w/g, (c) => c.toUpperCase());
	}

	// Hero chart: impressions over time as an AreaChart (ADR-059).
	// Convert API date strings to Date objects for time-axis rendering.
	const impressionsSeries = $derived(
		(analytics?.daily_impressions ?? []).map((item) => ({
			day: new Date(item.day),
			views: item.views ?? 0
		}))
	);

	// Readers by country — horizontal BarChart.
	const countrySeries = $derived(
		(analytics?.readers_by_country ?? []).map((item) => ({
			country: item.country_code || t('dashboard.analytics.unknown_country', 'Unknown'),
			readers: item.readers ?? 0
		}))
	);

	// Content status breakdown — vertical bar chart.
	const statusSeries = $derived(
		analytics
			? [
					{ x: t('dashboard.analytics.drafts', 'Drafts'), y: analytics.draft_content },
					{ x: t('dashboard.analytics.published', 'Published'), y: analytics.published_content },
					{ x: t('dashboard.analytics.other', 'Other'), y: Math.max(0, analytics.total_content - analytics.draft_content - analytics.published_content) }
				]
			: []
	);

	// Top posts — horizontal BarChart. `top_content` is period-scoped from
	// GET /me/analytics (Section 50). Truncate long titles for the y-axis.
	const topPostsSeries = $derived(
		(analytics?.top_content ?? []).map((item) => {
			const title = item.title?.trim() || t('dashboard.analytics.no_content', 'Untitled');
			return {
				id: item.id,
				title: title.length > 40 ? title.slice(0, 40) + '…' : title,
				fullTitle: title,
				views: item.view_count ?? 0,
				contentType: formatContentType(item.content_type ?? ''),
				createdAt: item.created_at,
				comments: item.comment_count ?? 0,
				shares: item.share_count ?? 0
			};
		})
	);

	// Follower growth — LineChart with date x-axis (follower + following).
	const followerSeries = $derived(
		followerGrowth
			.map((item) => ({
				day: new Date(item.day),
				new_followers: item.new_followers ?? 0,
				new_following: item.new_following ?? 0
			}))
			.slice()
			.sort((a, b) => a.day.getTime() - b.day.getTime())
	);

	// Section 40 user dashboard: user engagement over time (multi-series line).
	const engagementSeries = $derived(
		(userDashboard?.user_engagement ?? [])
			.map((item) => ({
				day: new Date(item.day),
				posts_created: item.posts_created ?? 0,
				reactions_given: item.reactions_given ?? 0,
				comments_made: item.comments_made ?? 0,
				views: item.views ?? 0
			}))
			.slice()
			.sort((a, b) => a.day.getTime() - b.day.getTime())
	);

	// Section 40 user dashboard: platform content creation volume over time.
	const contentVolumeSeries = $derived(
		(userDashboard?.content_over_time ?? [])
			.map((item) => ({
				day: new Date(item.day),
				content_count: item.content_count ?? 0
			}))
			.slice()
			.sort((a, b) => a.day.getTime() - b.day.getTime())
	);

	// Section 40 user dashboard: trending content on the platform (created in period).
	const trendingContentSeries = $derived(
		(userDashboard?.top_content ?? []).map((item) => {
			const title = item.content_title?.trim() || t('dashboard.analytics.no_content', 'Untitled');
			return {
				content_id: item.content_id,
				title: title.length > 40 ? title.slice(0, 40) + '…' : title,
				fullTitle: title,
				views: item.total_views ?? 0,
				comments: item.total_comments ?? 0,
				reactions: item.total_reactions ?? 0,
				shares: item.total_shares ?? 0
			};
		})
	);

	// Section 40 user dashboard: traffic sources for the user's content.
	const trafficSourcesSeries = $derived(
		(userDashboard?.traffic_sources ?? []).map((item) => ({
			referrer: item.referrer?.trim() || t('dashboard.analytics.unknown_source', 'Unknown'),
			visits: item.visits ?? 0
		}))
	);

	// Section 40 user dashboard: ad campaign performance (horizontal bar by CTR).
	const adPerformanceSeries = $derived(
		(userDashboard?.ad_performance ?? []).map((item) => {
			const name = item.campaign_name?.trim() || t('dashboard.analytics.unnamed_campaign', 'Unnamed');
			return {
				campaign_id: item.campaign_id,
				name: name.length > 40 ? name.slice(0, 40) + '…' : name,
				fullName: name,
				ctr: item.ctr ?? 0,
				impressions: item.impressions ?? 0,
				clicks: item.clicks ?? 0,
				spent: item.spent ?? 0,
				status: item.status ?? ''
			};
		})
	);
</script>

<section class="analytics-view space-y-6">
	<!-- Period selector (Section 40: date-range selectable) -->
	<div class="flex flex-wrap items-center gap-2">
		{#each periodOptions as opt (opt.key)}
			<button
				type="button"
				class="rounded-[var(--radius)] border px-3 py-1.5 text-sm transition-colors
					{selectedPeriod === opt.key
						? 'border-[var(--color-primary)] bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
						: 'border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'}"
				onclick={() => {
					selectedPeriod = opt.key;
					if (opt.key !== 'custom') applyPeriod();
				}}>{opt.label}</button
			>
		{/each}

		{#if selectedPeriod === 'custom'}
			<div class="flex items-center gap-2">
				<input
					type="date"
					bind:value={customStart}
					class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1.5 text-sm text-[var(--color-text)]"
				/>
				<span class="text-sm text-[var(--color-muted)]">-</span>
				<input
					type="date"
					bind:value={customEnd}
					class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1.5 text-sm text-[var(--color-text)]"
				/>
				<button
					type="button"
					disabled={customRangeInvalid}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm text-[var(--color-primary-contrast)] disabled:opacity-50"
					onclick={applyPeriod}>{t('common.apply', 'Apply')}</button
				>
				{#if customRangeInverted}
					<span class="text-xs text-[var(--color-danger)]">
						{t('dashboard.analytics.invalid_range', 'Start must be before end.')}
					</span>
				{/if}
			</div>
		{/if}
	</div>

	<!-- Metric cards (Section 50) — always visible above the tabbed charts. -->
	<div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
		<MetricCard
			labelKey="dashboard.analytics.impressions"
			value={analytics?.total_views ?? 0}
			delta={deltas.impressions}
		/>
		<MetricCard
			labelKey="dashboard.analytics.followers"
			value={analytics?.total_followers ?? 0}
			delta={analytics?.total_followers_gained ?? 0}
		/>
		<MetricCard
			labelKey="dashboard.analytics.posts"
			value={analytics?.published_content ?? 0}
			delta={analytics?.content_created?.published_count ?? 0}
		/>
		<MetricCard
			labelKey="dashboard.analytics.reactions"
			value={analytics?.total_reactions ?? 0}
			delta={deltas.reactions}
		/>
	</div>
	<div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
		<MetricCard
			labelKey="dashboard.analytics.comments"
			value={analytics?.total_comments ?? 0}
			delta={deltas.comments}
		/>
		<MetricCard labelKey="dashboard.analytics.following" value={analytics?.total_following ?? 0} />
		<MetricCard
			labelKey="dashboard.analytics.points_earned"
			value={analytics?.total_points_earned ?? 0}
			delta={deltas.points}
		/>
		<MetricCard labelKey="dashboard.analytics.bookmarks" value={analytics?.total_bookmarks ?? 0} />
	</div>

	<!-- Section 40 dashboard: tabbed analytics panels. Every panel is full-width. -->
	<Tabs.Root value={activeTab} onValueChange={(v) => { if (v) activeTab = v; }}>
		<Tabs.List
			class="flex gap-1 overflow-x-auto overflow-y-hidden border-b"
			style="border-color: var(--color-border)"
		>
			{#each tabs as tab (tab.value)}
				<Tabs.Trigger
					value={tab.value}
					class="relative flex shrink-0 cursor-pointer items-center px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-[1px] data-[state=active]:border-[var(--color-text)] data-[state=active]:text-[var(--color-text)] data-[state=inactive]:border-transparent data-[state=inactive]:text-[var(--color-muted)] data-[state=inactive]:hover:text-[var(--color-text)]"
				>
					{#snippet child({ props })}
						<span {...props}>{tab.label}</span>
					{/snippet}
				</Tabs.Trigger>
			{/each}
		</Tabs.List>

		<!-- Overview: impressions trend + content status breakdown -->
		<Tabs.Content value="overview" class="mt-6 space-y-6">
			{#if activeTab === 'overview'}
				<DashboardPanel titleKey="dashboard.analytics.impressions">
					{#if !analytics}
						<div class="h-56">
							<LoadingSkeleton variant="chart" />
						</div>
					{:else if impressionsSeries.length === 0}
						<p class="text-sm text-[var(--color-muted)]">
							{t('dashboard.analytics.no_impressions', 'No impressions data yet.')}
						</p>
					{:else}
						<div class="h-56">
							<AreaChart
								data={impressionsSeries}
								x="day"
								padding={{ ...commonPadding, left: 52 }}
								y="views"
								height={224}
								series={[
									{ key: 'impressions', value: 'views', color: 'var(--chart-1, var(--color-primary))' }
								]}
								grid
								props={{
									xAxis: { format: 'day' },
									yAxis: valueYAxis
								}}
							/>
						</div>
						<p class="mt-2 text-sm font-medium text-[var(--color-text)]">
							{formatMetric(impressionsSeries.reduce((sum, d) => sum + d.views, 0))}
							{t('dashboard.analytics.views', 'views')}
						</p>
					{/if}
				</DashboardPanel>

				<DashboardPanel titleKey="dashboard.analytics.content_breakdown">
					{#if !analytics}
						<div class="h-56">
							<LoadingSkeleton variant="chart" />
						</div>
					{:else if statusSeries.length === 0}
						<p class="text-sm text-[var(--color-muted)]">
							{t('dashboard.analytics.unavailable', 'Analytics unavailable right now.')}
						</p>
					{:else}
						<div class="h-56">
							<BarChart
								data={statusSeries}
								x="x"
								padding={{ ...commonPadding, left: 52 }}
								y="y"
								height={224}
								series={[{ key: 'content', value: 'y', color: 'var(--chart-4, var(--color-primary))' }]}
								grid
								props={{
									yAxis: valueYAxis
								}}
							/>
						</div>
					{/if}
				</DashboardPanel>
			{/if}
		</Tabs.Content>

		<!-- Content: top posts, content volume, trending content -->
		<Tabs.Content value="content" class="mt-6 space-y-6">
			{#if activeTab === 'content'}
				<DashboardPanel titleKey="dashboard.analytics.top_posts">
					{#if !analytics}
						<div class="h-56">
							<LoadingSkeleton variant="chart" />
						</div>
					{:else if topPostsSeries.length === 0}
						<p class="text-sm text-[var(--color-muted)]">
							{t('dashboard.analytics.no_content', 'No published content yet.')}
						</p>
					{:else}
						<div class="h-64">
							<BarChart
								data={topPostsSeries}
								x="views"
								y="title"
								orientation="horizontal"
								padding={{ ...commonPadding, left: 24 }}
								height={256}
								series={[
									{ key: 'views', value: 'views', color: 'var(--chart-1, var(--color-primary))' }
								]}
								grid
								props={{
									xAxis: valueXAxis,
									yAxis: categoryYAxis
								}}
							/>
						</div>
						<div class="mt-2 grid grid-cols-[minmax(0,1fr)_auto_auto] gap-x-4 gap-y-1 text-sm font-medium text-[var(--color-text)]">
							{#each topPostsSeries as point (point.id)}
								<div class="contents">
									<span class="truncate" title={point.fullTitle}>{point.fullTitle}</span>
									<span class="text-[var(--color-text-subtle)] whitespace-nowrap">
										{formatContentType(point.contentType)} · {formatDate(point.createdAt)}
									</span>
									<span class="text-end whitespace-nowrap">
										{formatMetric(point.views)} {t('profile.views', 'views')}
										{#if point.comments > 0}
											· {formatMetric(point.comments)} {t('dashboard.analytics.comments', 'comments')}
										{/if}
										{#if point.shares > 0}
											· {formatMetric(point.shares)} {t('profile.shares', 'shares')}
										{/if}
									</span>
								</div>
							{/each}
						</div>
					{/if}
				</DashboardPanel>

				<DashboardPanel titleKey="dashboard.analytics.content_volume">
					{#if !userDashboard}
						<div class="h-56">
							<LoadingSkeleton variant="chart" />
						</div>
					{:else if contentVolumeSeries.length === 0}
						<p class="text-sm text-[var(--color-muted)]">
							{t('dashboard.analytics.no_content_volume', 'No content volume data yet.')}
						</p>
					{:else}
						<div class="h-56">
							<AreaChart
								data={contentVolumeSeries}
								x="day"
								padding={{ ...commonPadding, left: 52 }}
								y="content_count"
								height={224}
								series={[
									{ key: 'content', value: 'content_count', color: 'var(--chart-5, var(--color-primary))' }
								]}
								grid
								props={{
									xAxis: { format: 'day' },
									yAxis: valueYAxis
								}}
							/>
						</div>
					{/if}
				</DashboardPanel>

				<DashboardPanel titleKey="dashboard.analytics.trending_content">
					{#if !userDashboard}
						<div class="h-56">
							<LoadingSkeleton variant="chart" />
						</div>
					{:else if trendingContentSeries.length === 0}
						<p class="text-sm text-[var(--color-muted)]">
							{t('dashboard.analytics.no_trending_content', 'No trending content yet.')}
						</p>
					{:else}
						<div class="h-64">
							<BarChart
								data={trendingContentSeries}
								x="views"
								y="title"
								orientation="horizontal"
								padding={{ ...commonPadding, left: 24 }}
								height={256}
								series={[{ key: 'views', value: 'views', color: 'var(--chart-6, var(--color-primary))' }]}
								grid
								props={{
									xAxis: valueXAxis,
									yAxis: categoryYAxis
								}}
							/>
						</div>
						<div class="mt-2 grid grid-cols-[minmax(0,1fr)_auto] gap-x-4 gap-y-1 text-sm font-medium text-[var(--color-text)]">
							{#each trendingContentSeries as point (point.content_id)}
								<div class="contents">
									<span class="truncate" title={point.fullTitle}>{point.fullTitle}</span>
									<span class="text-start whitespace-nowrap text-[var(--color-text-subtle)]">
										{formatMetric(point.views)} {t('dashboard.analytics.views', 'views')}
										{#if point.reactions > 0}
											· {formatMetric(point.reactions)} {t('dashboard.analytics.reactions', 'reactions')}
										{/if}
										{#if point.comments > 0}
											· {formatMetric(point.comments)} {t('dashboard.analytics.comments', 'comments')}
										{/if}
										{#if point.shares > 0}
											· {formatMetric(point.shares)} {t('profile.shares', 'shares')}
										{/if}
									</span>
								</div>
							{/each}
						</div>
					{/if}
				</DashboardPanel>
			{/if}
		</Tabs.Content>

		<!-- Audience: follower growth, readers by country, traffic sources -->
		<Tabs.Content value="audience" class="mt-6 space-y-6">
			{#if activeTab === 'audience'}
				<DashboardPanel titleKey="dashboard.analytics.follower_growth">
					{#if !analytics}
						<div class="h-56">
							<LoadingSkeleton variant="chart" />
						</div>
					{:else if followerSeries.length === 0}
						<p class="text-sm text-[var(--color-muted)]">
							{t('dashboard.analytics.no_growth_data', 'No follower growth data yet.')}
						</p>
					{:else}
						<div class="h-56">
							<LineChart
								data={followerSeries}
								x="day"
								padding={{ ...commonPadding, left: 52 }}
								series={[
									{
										key: 'followers',
										value: 'new_followers',
										color: 'var(--chart-2, var(--color-primary))'
									},
									{
										key: 'following',
										value: 'new_following',
										color: 'var(--chart-3, var(--color-text-subtle))'
									}
								]}
								height={224}
								grid
								props={{
									xAxis: { format: 'day' },
									yAxis: valueYAxis
								}}
							/>
						</div>
						<p class="mt-2 text-sm font-medium text-[var(--color-text)]">
							{periodLabel} ·
							{formatMetric(followerSeries.reduce((sum, d) => sum + d.new_followers, 0))}
							{t('dashboard.analytics.new_followers', 'new followers')}
							·
							{formatMetric(followerSeries.reduce((sum, d) => sum + d.new_following, 0))}
							{t('dashboard.analytics.new_following', 'new following')}
						</p>
					{/if}
				</DashboardPanel>

				<DashboardPanel titleKey="dashboard.analytics.readers_by_country">
					{#if !analytics}
						<div class="h-56">
							<LoadingSkeleton variant="chart" />
						</div>
					{:else if countrySeries.length === 0}
						<p class="text-sm text-[var(--color-muted)]">
							{t('dashboard.analytics.no_country_data', 'No country data yet.')}
						</p>
					{:else}
						<div class="h-64">
							<BarChart
								data={countrySeries}
								x="readers"
								y="country"
								orientation="horizontal"
								padding={{ ...commonPadding, left: 24 }}
								height={256}
								series={[
									{ key: 'readers', value: 'readers', color: 'var(--chart-3, var(--color-primary))' }
								]}
								grid
								props={{
									xAxis: valueXAxis,
									yAxis: categoryYAxis
								}}
							/>
						</div>
						<div class="mt-2 grid grid-cols-[minmax(0,1fr)_auto] gap-x-4 gap-y-1 text-sm font-medium text-[var(--color-text)]">
							{#each countrySeries as point (point.country)}
								<div class="contents">
									<span class="truncate" title={point.country}>{point.country}</span>
									<span class="text-end whitespace-nowrap">
										{formatMetric(point.readers)} {t('dashboard.analytics.readers', 'readers')}
									</span>
								</div>
							{/each}
						</div>
					{/if}
				</DashboardPanel>

				<DashboardPanel titleKey="dashboard.analytics.traffic_sources">
					{#if !userDashboard}
						<div class="h-56">
							<LoadingSkeleton variant="chart" />
						</div>
					{:else if trafficSourcesSeries.length === 0}
						<p class="text-sm text-[var(--color-muted)]">
							{t('dashboard.analytics.no_traffic_data', 'No traffic source data yet.')}
						</p>
					{:else}
						<div class="h-64">
							<BarChart
								data={trafficSourcesSeries}
								x="visits"
								y="referrer"
								orientation="horizontal"
								padding={{ ...commonPadding, left: 24 }}
								height={256}
								series={[{ key: 'visits', value: 'visits', color: 'var(--chart-7, var(--color-primary))' }]}
								grid
								props={{
									xAxis: valueXAxis,
									yAxis: categoryYAxis
								}}
							/>
						</div>
						<div class="mt-2 grid grid-cols-[minmax(0,1fr)_auto] gap-x-4 gap-y-1 text-sm font-medium text-[var(--color-text)]">
							{#each trafficSourcesSeries as point (point.referrer)}
								<div class="contents">
									<span class="truncate" title={point.referrer}>{point.referrer}</span>
									<span class="text-end whitespace-nowrap text-[var(--color-text-subtle)]">
										{formatMetric(point.visits)} {t('dashboard.analytics.visits', 'visits')}
									</span>
								</div>
							{/each}
						</div>
					{/if}
				</DashboardPanel>
			{/if}
		</Tabs.Content>

		<!-- Engagement: user engagement over time -->
		<Tabs.Content value="engagement" class="mt-6 space-y-6">
			{#if activeTab === 'engagement'}
				<DashboardPanel titleKey="dashboard.analytics.user_engagement">
					{#if !userDashboard}
						<div class="h-56">
							<LoadingSkeleton variant="chart" />
						</div>
					{:else if engagementSeries.length === 0}
						<p class="text-sm text-[var(--color-muted)]">
							{t('dashboard.analytics.no_engagement_data', 'No engagement data yet.')}
						</p>
					{:else}
						<div class="h-56">
							<LineChart
								data={engagementSeries}
								x="day"
								padding={{ ...commonPadding, left: 52 }}
								height={224}
								series={[
									{ key: 'posts_created', color: 'var(--chart-1, var(--color-primary))' },
									{ key: 'reactions_given', color: 'var(--chart-2, var(--color-primary))' },
									{ key: 'comments_made', color: 'var(--chart-3, var(--color-primary))' },
									{ key: 'views', color: 'var(--chart-4, var(--color-primary))' }
								]}
								grid
								props={{
									xAxis: { format: 'day' },
									yAxis: valueYAxis
								}}
							/>
						</div>
					{/if}
				</DashboardPanel>
			{/if}
		</Tabs.Content>

		<!-- Ads: ad campaign performance -->
		<Tabs.Content value="ads" class="mt-6 space-y-6">
			{#if activeTab === 'ads'}
				<DashboardPanel titleKey="dashboard.analytics.ad_performance">
					{#if !userDashboard}
						<div class="h-56">
							<LoadingSkeleton variant="chart" />
						</div>
					{:else if adPerformanceSeries.length === 0}
						<p class="text-sm text-[var(--color-muted)]">
							{t('dashboard.analytics.no_ad_data', 'No ad campaigns yet.')}
						</p>
					{:else}
						<div class="h-64">
							<BarChart
								data={adPerformanceSeries}
								x="ctr"
								y="name"
								orientation="horizontal"
								padding={{ ...commonPadding, left: 24 }}
								height={256}
								series={[{ key: 'ctr', value: 'ctr', color: 'var(--chart-8, var(--color-primary))' }]}
								grid
								props={{
									xAxis: valueXAxis,
									yAxis: categoryYAxis
								}}
							/>
						</div>
						<div class="mt-2 grid grid-cols-[minmax(0,1fr)_auto] gap-x-4 gap-y-1 text-sm font-medium text-[var(--color-text)]">
							{#each adPerformanceSeries as point (point.campaign_id)}
								<div class="contents">
									<span class="truncate" title={point.fullName}>{point.fullName}</span>
									<span class="text-start whitespace-nowrap text-[var(--color-text-subtle)]">
										{formatMetric(point.ctr)}% {t('dashboard.analytics.ctr', 'CTR')}
										· {formatMetric(point.impressions)} {t('dashboard.analytics.impressions', 'impressions')}
										· {formatMetric(point.clicks)} {t('dashboard.analytics.clicks', 'clicks')}
									</span>
								</div>
							{/each}
						</div>
					{/if}
				</DashboardPanel>
			{/if}
		</Tabs.Content>
	</Tabs.Root>
</section>

<style>
	.analytics-view :global(.lc-axis-tick-label),
	.analytics-view :global(.lc-axis-label) {
		font-size: 12px;
		font-weight: 500;
		fill: var(--color-text);
		stroke: none;
	}
</style>
