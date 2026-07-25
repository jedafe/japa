<script lang="ts">
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { Button } from '$lib/components/ui/button';
	import StatCard from '$lib/components/ui/StatCard.svelte';
	import { CardSkeleton } from '$lib/components/ui/skeleton';
	import { analyticsAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Users, TrendingUp, Shield, RefreshCw, Zap } from 'lucide-svelte';
	import { LineChart, AreaChart } from 'layerchart';
	import { curveMonotoneX } from 'd3-shape';

	type UserGrowthRow = { day: string; new_users: number };
	type ContentOverTimeRow = { day: string; content_count: number };
	type TopContentRow = {
		content_id: string;
		total_views: number;
		total_reactions: number;
		total_comments: number;
		total_unique_viewers: number;
	};
	type EventCountRow = { event_type: string; count: number };
	type DailyActiveRow = { day: string; dau: number };
	type MonthlyActiveRow = { month: string; mau: number };
	type TrafficSourceRow = { referrer: string | null; visits: number };
	type ContentStatRow = {
		day: string;
		tenant_id: string;
		content_id: string;
		views: number;
		reactions: number;
		comments: number;
		unique_viewers: number;
	};
	type SpaceStatRow = { name?: string; title?: string; member_count?: number };

	type OverviewData = {
		daily_active_users: DailyActiveRow[] | null;
		monthly_active_users: MonthlyActiveRow[] | null;
		event_counts: EventCountRow[] | null;
		content_over_time: ContentOverTimeRow[] | null;
		user_growth: UserGrowthRow[] | null;
		traffic_sources: TrafficSourceRow[] | null;
		top_content: TopContentRow[] | null;
		content_stats: ContentStatRow[] | null;
		space_stats: SpaceStatRow[] | null;
		pending_reports?: number;
		pending_tickets?: number;
		spam_queue?: number;
		generations_today?: number;
		hitl_pending?: number;
	};

	type UserAnalytics = {
		total_users: number;
		active_users: number;
		new_users_today: number;
	};

	let overview = $state<OverviewData>({
		daily_active_users: null,
		monthly_active_users: null,
		event_counts: null,
		content_over_time: null,
		user_growth: null,
		traffic_sources: null,
		top_content: null,
		content_stats: null,
		space_stats: null
	});
	let userStats = $state<UserAnalytics>({ total_users: 0, active_users: 0, new_users_today: 0 });
	let loading = $state(true);
	let error = $state('');
	let activeTab = $state('overview');
	let totalContent = $state(0);
	// Range controls are sourced from the URL so they survive navigation
	let period = $state(page.url.searchParams.get('period') ?? '7d');
	let from = $state(page.url.searchParams.get('from') ?? '');
	let to = $state(page.url.searchParams.get('to') ?? '');
	let loadAbortController: AbortController | null = null;

	const periods = [
		{ value: '24h', label: t('dashboard.period_24h', '24 Hours') },
		{ value: '7d', label: t('dashboard.period_7d', '7 Days') },
		{ value: '30d', label: t('dashboard.period_30d', '30 Days') },
		{ value: '90d', label: t('dashboard.period_90d', '90 Days') }
	];

	const tabs = [
		{ value: 'overview', label: t('dashboard.tab_overview', 'Overview'), icon: TrendingUp },
		{ value: 'community', label: t('dashboard.tab_community', 'Community'), icon: Users },
		{ value: 'moderation', label: t('dashboard.tab_moderation', 'Moderation'), icon: Shield },
		{ value: 'ai', label: t('dashboard.tab_ai', 'AI Usage'), icon: Zap }
	];

	const userGrowthData = $derived(
		(overview.user_growth ?? []).map((d) => ({
			date: new Date(d.day),
			count: d.new_users
		}))
	);

	const contentActivityData = $derived(
		(overview.content_over_time ?? []).map((d) => ({
			date: new Date(d.day),
			count: d.content_count
		}))
	);

	const topContent = $derived(overview.top_content ?? []);
	const eventCounts = $derived(overview.event_counts ?? []);

	const monthlyActiveData = $derived(
		(overview.monthly_active_users ?? []).map((d) => ({
			date: new Date(d.month),
			count: d.mau
		}))
	);

	const trafficSources = $derived(
		(overview.traffic_sources ?? [])
			.sort((a, b) => b.visits - a.visits)
			.slice(0, 10)
	);

	async function load() {
		// Abort any in-flight request to prevent race conditions
		if (loadAbortController) loadAbortController.abort();
		loadAbortController = new AbortController();
		const signal = loadAbortController.signal;

		loading = true;
		error = '';
		try {
		const params = [from && `from=${from}`, to && `to=${to}`].filter(Boolean).join('&') || `period=${period}`;
		const [overviewResult, usersResult] = await Promise.all([
			// Prefer an explicit date range; fall back to the period selector
			analyticsAdminApi.getOverview(params),
			analyticsAdminApi.getUsers()
		]);

			if (signal.aborted) return;

			if (overviewResult.ok && overviewResult.data) {
				const data = overviewResult.data as OverviewData;
				overview = data;
				totalContent = data.content_over_time?.reduce((sum, d) => sum + d.content_count, 0) ?? 0;
			} else if (!overviewResult.ok) {
				error = overviewResult.error ?? t('common.error', 'Failed to load data');
			}

			if (usersResult.ok && usersResult.data) {
				userStats = usersResult.data as UserAnalytics;
			}
		} catch (e) {
			if (signal.aborted) return;
			error = e instanceof Error ? e.message : t('common.error', 'Failed to load data');
		} finally {
			if (!signal.aborted) loading = false;
		}
	}

	function setPeriod(p: string) {
		// Selecting a period clears any explicit date range via the URL
		const url = new URL(page.url);
		url.searchParams.set('period', p);
		url.searchParams.delete('from');
		url.searchParams.delete('to');
		goto(url, { keepFocus: true, noScroll: true });
	}

	// Push the chosen date range to the URL (?from=&to=) so it drives the fetch
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
		from = page.url.searchParams.get('from') ?? '';
		to = page.url.searchParams.get('to') ?? '';
		period = page.url.searchParams.get('period') ?? '7d';
		load();

		return () => {
			if (loadAbortController) loadAbortController.abort();
		};
	});
</script>

<svelte:head>
	<title>{t('dashboard.title', 'Dashboard')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('dashboard.title', 'Dashboard')}
	description={t('dashboard.description', "Welcome back! Here's what's happening on your platform.")}
	actions={[
		{
			label: t('common.refresh', 'Refresh'),
			icon: RefreshCw,
			variant: 'outline',
			onclick: load
		}
	]}
/>

{#if error}
	<div
		role="alert"
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{error}
	</div>
{/if}

<!-- Period + date-range controls -->
<div class="mb-6 flex flex-wrap items-end gap-4">
	<div class="flex items-center gap-2">
		<span class="text-sm" style="color: var(--color-muted)">{t('common.period', 'Period')}:</span>
		{#each periods as p}
			<Button
				variant={period === p.value ? 'default' : 'ghost'}
				size="sm"
				onclick={() => setPeriod(p.value)}
			>
				{p.label}
			</Button>
		{/each}
	</div>
	<!-- Date-range picker that writes ?from=&to= to the URL and refetches -->
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
</div>

{#if loading}
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
		{#each Array(4) as _}
			<CardSkeleton />
		{/each}
	</div>
{:else}
	<div class="space-y-6">
		<Tabs {tabs} bind:value={activeTab} />

		{#if activeTab === 'overview'}
			<!-- KPI Stats -->
			<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
				<a href="/users">
					<StatCard
						label={t('dashboard.total_users', 'Total Users')}
						value={(userStats.total_users ?? 0).toLocaleString()}
					/>
				</a>
				<a href="/users">
					<StatCard
						label={t('dashboard.active_users', 'Active Users')}
						value={(userStats.active_users ?? 0).toLocaleString()}
					/>
				</a>
				<a href="/users">
					<StatCard
						label={t('dashboard.new_today', 'New Today')}
						value={(userStats.new_users_today ?? 0).toLocaleString()}
					/>
				</a>
				<a href="/content">
					<StatCard
						label={t('dashboard.total_content', 'Total Content')}
						value={totalContent.toLocaleString()}
					/>
				</a>
			</div>

			<!-- Charts -->
			<div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
				<!-- User Growth Chart -->
				<Card>
					<CardHeader>
						<h3 class="text-sm font-medium" style="color: var(--color-muted)">
							{t('dashboard.user_growth', 'User Growth')}
						</h3>
					</CardHeader>
					<CardContent>
						{#if userGrowthData.length > 0}
							<div class="h-64">
							<LineChart
								data={userGrowthData}
								x="date"
								y="count"
								cRange={['var(--chart-1)']}
								props={{
									// Ref: japaconnect-engineering/adr/ADR-059-*.md (LayerChart theming)
									spline: { curve: curveMonotoneX, strokeWidth: 2, stroke: 'var(--chart-1)' }
								}}
							/>
							</div>
						{:else}
							<div class="flex h-64 items-center justify-center">
								<p class="text-sm" style="color: var(--color-muted)">
									{t('dashboard.no_data', 'No data available')}
								</p>
							</div>
						{/if}
					</CardContent>
				</Card>

				<!-- Content Activity Chart -->
				<Card>
					<CardHeader>
						<h3 class="text-sm font-medium" style="color: var(--color-muted)">
							{t('dashboard.content_activity', 'Content Activity')}
						</h3>
					</CardHeader>
					<CardContent>
						{#if contentActivityData.length > 0}
							<div class="h-64">
								<AreaChart
									data={contentActivityData}
									x="date"
									y="count"
									cRange={['var(--chart-1)']}
									props={{
										area: {
											curve: curveMonotoneX,
											fill: 'var(--chart-1)',
											fillOpacity: 0.2,
											strokeWidth: 2
										}
									}}
								/>
							</div>
						{:else}
							<div class="flex h-64 items-center justify-center">
								<p class="text-sm" style="color: var(--color-muted)">
									{t('dashboard.no_data', 'No data available')}
								</p>
							</div>
						{/if}
					</CardContent>
				</Card>
			</div>

			<!-- Monthly Active Users + Traffic Sources -->
			<div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
				<!-- Monthly Active Users Chart -->
				<Card>
					<CardHeader>
						<h3 class="text-sm font-medium" style="color: var(--color-muted)">
							{t('dashboard.monthly_active_users', 'Monthly Active Users')}
						</h3>
					</CardHeader>
					<CardContent>
						{#if monthlyActiveData.length > 0}
							<div class="h-64">
								<LineChart
									data={monthlyActiveData}
									x="date"
									y="count"
									cRange={['var(--chart-2)']}
									props={{
										spline: { curve: curveMonotoneX, strokeWidth: 2, stroke: 'var(--chart-2)' }
									}}
								/>
							</div>
						{:else}
							<div class="flex h-64 items-center justify-center">
								<p class="text-sm" style="color: var(--color-muted)">
									{t('dashboard.no_data', 'No data available')}
								</p>
							</div>
						{/if}
					</CardContent>
				</Card>

				<!-- Traffic Sources -->
				<Card>
					<CardHeader>
						<h3 class="text-sm font-medium" style="color: var(--color-muted)">
							{t('dashboard.traffic_sources', 'Traffic Sources')}
						</h3>
					</CardHeader>
					<CardContent>
						{#if trafficSources.length > 0}
							<div class="space-y-3">
								{#each trafficSources as source}
									<div class="flex items-center justify-between rounded-lg px-3 py-2">
										<span class="text-sm" style="color: var(--color-foreground)">
											{source.referrer ?? t('dashboard.direct', 'Direct')}
										</span>
										<span class="text-sm" style="color: var(--color-muted)">
											{source.visits.toLocaleString()} {t('dashboard.visits', 'visits')}
										</span>
									</div>
								{/each}
							</div>
						{:else}
							<div class="flex h-64 items-center justify-center">
								<p class="text-sm" style="color: var(--color-muted)">
									{t('dashboard.no_data', 'No data available')}
								</p>
							</div>
						{/if}
					</CardContent>
				</Card>
			</div>

			<!-- Top Content -->
			<Card>
				<CardHeader>
					<div class="flex items-center justify-between">
						<h3 class="text-sm font-medium" style="color: var(--color-muted)">
							{t('dashboard.top_content', 'Top Content')}
						</h3>
						<Button variant="ghost" size="sm" href="/content">{t('common.viewAll', 'View All')}</Button>
					</div>
				</CardHeader>
				<CardContent>
					{#if topContent.length > 0}
						<div class="space-y-3">
							{#each topContent as item}
								<a href="/content/{item.content_id}" class="flex items-center justify-between rounded-lg px-3 py-2 hover:bg-[color-mix(in_srgb,var(--color-muted)_5%,transparent)]">
									<div class="min-w-0 flex-1">
										<p class="truncate text-sm font-medium" style="color: var(--color-foreground)">
											{item.content_id.slice(0, 8)}...
										</p>
										<div class="mt-1 flex items-center gap-3 text-xs" style="color: var(--color-muted)">
											<span>{item.total_views.toLocaleString()} {t('dashboard.views', 'views')}</span>
											<span>{item.total_reactions.toLocaleString()} {t('dashboard.reactions', 'reactions')}</span>
											<span>{item.total_comments.toLocaleString()} {t('dashboard.comments', 'comments')}</span>
											<span>{item.total_unique_viewers.toLocaleString()} {t('dashboard.viewers', 'viewers')}</span>
										</div>
									</div>
								</a>
							{/each}
						</div>
					{:else}
						<p class="py-4 text-center text-sm" style="color: var(--color-muted)">
							{t('dashboard.no_data', 'No data available')}
						</p>
					{/if}
				</CardContent>
			</Card>
		{/if}

		{#if activeTab === 'community'}
			<div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
				<Card>
					<CardHeader>
						<h3 class="text-sm font-medium" style="color: var(--color-muted)">
							{t('dashboard.community_health', 'Community Health')}
						</h3>
					</CardHeader>
					<CardContent>
						<div class="space-y-4">
							<div class="flex items-center justify-between">
								<span class="text-sm" style="color: var(--color-muted)"
									>{t('dashboard.total_users', 'Total Users')}</span
								>
								<span class="font-medium" style="color: var(--color-foreground)"
									>{(userStats.total_users ?? 0).toLocaleString()}</span
								>
							</div>
							<div class="flex items-center justify-between">
								<span class="text-sm" style="color: var(--color-muted)"
									>{t('dashboard.new_today', 'New Today')}</span
								>
								<span class="font-medium" style="color: var(--color-foreground)"
									>{(userStats.new_users_today ?? 0).toLocaleString()}</span
								>
							</div>
							<div class="flex items-center justify-between">
								<span class="text-sm" style="color: var(--color-muted)"
									>{t('dashboard.active_users', 'Active Users')}</span
								>
								<span class="font-medium" style="color: var(--color-foreground)"
									>{(userStats.active_users ?? 0).toLocaleString()}</span
								>
							</div>
							<div class="flex items-center justify-between">
								<span class="text-sm" style="color: var(--color-muted)"
									>{t('dashboard.event_types', 'Event Types')}</span
								>
								<span class="font-medium" style="color: var(--color-foreground)"
									>{eventCounts.length}</span
								>
							</div>
						</div>
					</CardContent>
				</Card>

				<Card>
					<CardHeader>
						<h3 class="text-sm font-medium" style="color: var(--color-muted)">
							{t('dashboard.top_spaces', 'Top Spaces')}
						</h3>
					</CardHeader>
					<CardContent>
						{#if overview.space_stats && overview.space_stats.length > 0}
							<div class="space-y-3">
							{#each overview.space_stats.slice(0, 5) as space}
								<div class="flex items-center justify-between rounded-lg px-3 py-2">
									<span class="text-sm" style="color: var(--color-foreground)">{space.name ?? space.title ?? t('dashboard.space', 'Space')}</span>
									{#if space.member_count !== undefined}
										<span class="text-xs" style="color: var(--color-muted)">{space.member_count} {t('dashboard.members', 'members')}</span>
									{/if}
								</div>
							{/each}
							</div>
						{:else}
							<p class="py-4 text-center text-sm" style="color: var(--color-muted)">
								{t('dashboard.space_data_coming', 'Space data coming soon')}
							</p>
						{/if}
					</CardContent>
				</Card>
			</div>
		{/if}

	{#if activeTab === 'moderation'}
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
			<a href="/moderation?tab=pending">
				<StatCard
					label={t('dashboard.pending_reports', 'Pending Reports')}
					value={(overview.pending_reports ?? 0).toLocaleString()}
				/>
			</a>
			<a href="/tickets">
				<StatCard
					label={t('dashboard.pending_tickets', 'Pending Tickets')}
					value={(overview.pending_tickets ?? 0).toLocaleString()}
				/>
			</a>
			<a href="/moderation?tab=spam">
				<StatCard
					label={t('dashboard.spam_queue', 'Spam Queue')}
					value={(overview.spam_queue ?? 0).toLocaleString()}
				/>
			</a>
		</div>
	{/if}

	{#if activeTab === 'ai'}
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
			<StatCard
				label={t('dashboard.generations_today', 'Generations Today')}
				value={(overview.generations_today ?? 0).toLocaleString()}
			/>
			<StatCard
				label={t('dashboard.hitl_pending', 'HITL Pending Review')}
				value={(overview.hitl_pending ?? 0).toLocaleString()}
			/>
			<StatCard
				label={t('dashboard.new_today', 'New Users Today')}
				value={(userStats.new_users_today ?? 0).toLocaleString()}
			/>
		</div>
	{/if}
	</div>
{/if}
