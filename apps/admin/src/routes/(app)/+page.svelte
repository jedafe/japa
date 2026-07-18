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
	type TopContentRow = { title: string; views?: number; content_type?: string };
	type EventCountRow = { event_type: string; count: number };
	type DailyActiveRow = { day: string; dau: number };

	type OverviewData = {
		daily_active_users: DailyActiveRow[] | null;
		monthly_active_users: unknown[] | null;
		event_counts: EventCountRow[] | null;
		content_over_time: ContentOverTimeRow[] | null;
		user_growth: UserGrowthRow[] | null;
		traffic_sources: unknown[] | null;
		top_content: TopContentRow[] | null;
		content_stats: unknown[] | null;
		space_stats: unknown[] | null;
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
	// Range controls are sourced from the URL so they survive navigation
	let period = $state(page.url.searchParams.get('period') ?? '7d');
	let from = $state(page.url.searchParams.get('from') ?? '');
	let to = $state(page.url.searchParams.get('to') ?? '');

	const periods = [
		{ value: '24h', label: '24 Hours' },
		{ value: '7d', label: '7 Days' },
		{ value: '30d', label: '30 Days' },
		{ value: '90d', label: '90 Days' }
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

	async function load() {
		loading = true;
		error = '';
		try {
		const [overviewResult, usersResult] = await Promise.all([
			// Prefer an explicit date range; fall back to the period selector
			analyticsAdminApi.getOverview([from && `from=${from}`, to && `to=${to}`].filter(Boolean).join('&') || `period=${period}`),
			analyticsAdminApi.getUsers()
		]);

			if (overviewResult.ok && overviewResult.data) {
				overview = overviewResult.data as OverviewData;
			} else if (!overviewResult.ok) {
				error = overviewResult.error ?? t('common.error', 'Failed to load data');
			}

			if (usersResult.ok && usersResult.data) {
				userStats = usersResult.data as UserAnalytics;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('common.error', 'Failed to load data');
		} finally {
			loading = false;
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
						value={(contentActivityData.length).toLocaleString()}
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
								props={{
									// Use the design-system color instead of the LayerChart default
									spline: { curve: curveMonotoneX, strokeWidth: 2, stroke: 'var(--color-primary)' }
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
									props={{
										area: {
											curve: curveMonotoneX,
											fill: 'var(--color-primary)',
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
								<div class="flex items-center justify-between rounded-lg px-3 py-2">
									<div class="min-w-0 flex-1">
										<p class="truncate text-sm font-medium" style="color: var(--color-foreground)">
											{item.title}
										</p>
										{#if item.content_type}
											<p class="text-xs" style="color: var(--color-muted)">{item.content_type}</p>
										{/if}
									</div>
									{#if item.views !== undefined}
										<span class="text-sm" style="color: var(--color-muted)">
											{item.views.toLocaleString()} views
										</span>
									{/if}
								</div>
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
										<span class="text-sm" style="color: var(--color-foreground)">{JSON.stringify(space)}</span>
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
						value="0"
					/>
				</a>
				<a href="/tickets">
					<StatCard
						label={t('dashboard.pending_tickets', 'Pending Tickets')}
						value="0"
					/>
				</a>
				<a href="/moderation?tab=spam">
					<StatCard
						label={t('dashboard.spam_queue', 'Spam Queue')}
						value="0"
					/>
				</a>
			</div>
		{/if}

		{#if activeTab === 'ai'}
			<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
				<StatCard
					label={t('dashboard.generations_today', 'Generations Today')}
					value="0"
				/>
				<StatCard
					label={t('dashboard.hitl_pending', 'HITL Pending Review')}
					value="0"
				/>
				<StatCard
					label={t('dashboard.new_today', 'New Users Today')}
					value={(userStats.new_users_today ?? 0).toLocaleString()}
				/>
			</div>
		{/if}
	</div>
{/if}
