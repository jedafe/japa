<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { page } from "$app/state";
	import { goto } from "$app/navigation";
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { Tabs } from "$lib/components/ui/tabs";
	import { DataTable } from "$lib/components/ui/data-table";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { Badge } from "$lib/components/ui/badge";
	import { 
		BarChart3, Users, Eye, MessageSquare, TrendingUp, 
		Download, RefreshCw, Shield, DollarSign, Search, UserPlus
	} from "lucide-svelte";
	import { analyticsAdminApi } from "$lib/api";
	import { LineChart, AreaChart, BarChart } from "layerchart";
	import { curveMonotoneX } from "d3-shape";
	import StatCard from "$lib/components/ui/StatCard.svelte";
	import { CardSkeleton } from "$lib/components/ui/skeleton";
	
	let activeTab = $state("overview");
	let flushModalOpen = $state(false);
	let flushing = $state(false);
	let loading = $state(true);
	let error = $state("");
	// Date-range state sourced from the URL so it drives the analytics fetch
	let from = $state(page.url.searchParams.get("from") ?? "");
	let to = $state(page.url.searchParams.get("to") ?? "");
	
	// Analytics data
	type OverviewData = {
		daily_active_users: Array<{ day: string; dau: number }> | null;
		monthly_active_users: Array<{ month: string; mau: number }> | null;
		event_counts: Array<{ event_type: string; count: number }> | null;
		content_over_time: Array<{ day: string; content_count: number }> | null;
		user_growth: Array<{ day: string; new_users: number }> | null;
		traffic_sources: Array<{ referrer: string | null; visits: number }> | null;
		top_content: Array<{ content_id: string; total_views: number; total_reactions: number; total_comments: number; total_unique_viewers: number }> | null;
		content_stats: unknown[] | null;
		space_stats: unknown[] | null;
	};

	let overview = $state<OverviewData | null>(null);
	let security = $state<Array<Record<string, unknown>> | null>(null);
	let userAnalytics = $state<{ total_users?: number; active_users?: number; new_users_today?: number } | null>(null);
	let contentAnalytics = $state<{ content_stats?: Array<Record<string, unknown>>; total_content?: number; total_views?: number; total_reactions?: number; total_comments?: number; total_unique_viewers?: number } | null>(null);
	let spaceAnalytics = $state<Array<Record<string, unknown>> | null>(null);
	let revenueAnalytics = $state<{ total_revenue?: number; monthly_revenue?: number; active_subscriptions?: number; periods?: Array<{ period: string; revenue: number; transactions: number }> } | null>(null);
	let searchAnalytics = $state<Array<Record<string, unknown>> | null>(null);
	let scanning = $state(false);
	let loadAbortController: AbortController | null = null;
	
	async function loadAnalytics(f = from, toVal = to) {
		if (loadAbortController) loadAbortController.abort();
		loadAbortController = new AbortController();
		const signal = loadAbortController.signal;

		loading = true;
		error = "";
		try {
		// Pass the ?from=&to= range to every windowed endpoint so the date
		// picker drives all tabs, not just the overview.
		const rangeParams = [f && `from=${f}`, toVal && `to=${toVal}`].filter(Boolean).join("&") || undefined;
		const [overviewRes, securityRes, usersRes, contentRes, spacesRes, revenueRes, searchRes] = await Promise.allSettled([
			analyticsAdminApi.getOverview(rangeParams),
			analyticsAdminApi.getSecurity(rangeParams),
			analyticsAdminApi.getUsers(rangeParams),
			analyticsAdminApi.getContent(rangeParams),
			analyticsAdminApi.getSpaces(rangeParams),
			analyticsAdminApi.getRevenue(rangeParams),
			analyticsAdminApi.getSearch(rangeParams)
		]);
			
			if (signal.aborted) return;

			overview = overviewRes.status === 'fulfilled' && overviewRes.value.ok ? (overviewRes.value.data as OverviewData) : null;
			security = securityRes.status === 'fulfilled' && securityRes.value.ok ? (securityRes.value.data as Array<Record<string, unknown>>) : null;
			userAnalytics = usersRes.status === 'fulfilled' && usersRes.value.ok ? (usersRes.value.data as { total_users?: number; active_users?: number; new_users_today?: number }) : null;
			contentAnalytics = contentRes.status === 'fulfilled' && contentRes.value.ok ? (contentRes.value.data as { content_stats?: Array<Record<string, unknown>>; total_content?: number; total_views?: number; total_reactions?: number; total_comments?: number; total_unique_viewers?: number }) : null;
			spaceAnalytics = spacesRes.status === 'fulfilled' && spacesRes.value.ok ? (spacesRes.value.data as Array<Record<string, unknown>>) : null;
			revenueAnalytics = revenueRes.status === 'fulfilled' && revenueRes.value.ok ? (revenueRes.value.data as { total_revenue?: number; monthly_revenue?: number; active_subscriptions?: number; periods?: Array<{ period: string; revenue: number; transactions: number }> }) : null;
			searchAnalytics = searchRes.status === 'fulfilled' && searchRes.value.ok ? (searchRes.value.data as Array<Record<string, unknown>>) : null;
		} catch (e) {
			if (signal.aborted) return;
			error = e instanceof Error ? e.message : t('analytics.failed_to_load', 'Failed to load analytics');
		} finally {
			if (!signal.aborted) loading = false;
		}
	}

	async function runSecurityScan() {
		scanning = true;
		try {
			const res = await analyticsAdminApi.runSecurityScan({});
			if (!res.ok) throw new Error('Scan failed');
			// Wait a moment for the async scan to process, then reload security data
			await new Promise(r => setTimeout(r, 2000));
			const params = [from && `from=${from}`, to && `to=${to}`].filter(Boolean).join("&") || undefined;
			const secRes = await analyticsAdminApi.getSecurity(params);
			security = secRes.ok ? (secRes.data as Array<Record<string, unknown>>) : [];
		} catch (e) {
			error = e instanceof Error ? e.message : t('analytics.scan_failed', 'Scan failed');
		} finally {
			scanning = false;
		}
	}

	async function resolveScan(id: string) {
		try {
			await analyticsAdminApi.resolveSecurityItem(id, {});
			security = (security ?? []).map((s) =>
				s.id === id ? { ...s, resolved_at: new Date().toISOString() } : s
			);
		} catch (e) {
			error = e instanceof Error ? e.message : t('analytics.resolve_failed', 'Failed to resolve');
		}
	}

	// Push the chosen date range to the URL (?from=&to=) to drive the reload
	function applyRange() {
		const url = new URL(page.url);
		if (from) url.searchParams.set("from", from);
		else url.searchParams.delete("from");
		if (to) url.searchParams.set("to", to);
		else url.searchParams.delete("to");
		goto(url, { keepFocus: true, noScroll: true });
	}

	$effect(() => {
		// Re-sync controls from the URL and reload whenever the range changes
		const f = page.url.searchParams.get("from") ?? "";
		const t = page.url.searchParams.get("to") ?? "";
		from = f;
		to = t;
		loadAnalytics(f, t);

		return () => {
			if (loadAbortController) loadAbortController.abort();
		};
	});
	
	const tabs = [
		{ value: "overview", label: t('analytics.tab_overview', 'Overview'), icon: TrendingUp },
		{ value: "users", label: t('analytics.tab_users', 'Users'), icon: Users },
		{ value: "content", label: t('analytics.tab_content', 'Content'), icon: MessageSquare },
		{ value: "spaces", label: t('analytics.tab_spaces', 'Spaces'), icon: Eye },
		{ value: "revenue", label: t('analytics.tab_revenue', 'Revenue'), icon: DollarSign },
		{ value: "search", label: t('analytics.tab_search', 'Search'), icon: Search },
		{ value: "security", label: t('analytics.tab_security', 'Security'), icon: Shield }
	];
	
	// pctChange computes the percentage change between the last and
	// second-to-last values of a time series. Returns undefined when there
	// is not enough data or the previous value is zero (avoid div-by-zero).
	function pctChange<T>(series: T[] | null | undefined, pick: (item: T) => number): number | undefined {
		if (!series || series.length < 2) return undefined;
		const last = pick(series[series.length - 1]);
		const prev = pick(series[series.length - 2]);
		if (prev === 0) return undefined;
		return Math.round(((last - prev) / prev) * 100);
	}

	const overviewCards = $derived([
		{
			label: t('analytics.daily_active_users', 'Daily Active Users'),
			value: overview?.daily_active_users?.[overview.daily_active_users.length - 1]?.dau ?? 0,
			change: pctChange(overview?.daily_active_users, (d) => d.dau),
			icon: Users,
			color: "primary"
		},
		{
			label: t('analytics.monthly_active_users', 'Monthly Active Users'),
			value: overview?.monthly_active_users?.[overview.monthly_active_users.length - 1]?.mau ?? 0,
			change: pctChange(overview?.monthly_active_users, (d) => d.mau),
			icon: Users,
			color: "success"
		},
		{
			label: t('analytics.new_signups', 'New Signups (Latest)'),
			value: overview?.user_growth?.[overview.user_growth.length - 1]?.new_users ?? 0,
			change: pctChange(overview?.user_growth, (d) => d.new_users),
			icon: UserPlus,
			color: "info"
		},
		{
			label: t('analytics.total_spaces', 'Total Spaces'),
			value: overview?.space_stats ? new Set(overview.space_stats.map((s: any) => s.space_id)).size : 0,
			change: undefined,
			icon: Eye,
			color: "warning"
		},
		{
			label: t('analytics.content_count', 'Content Items'),
			value: overview?.content_over_time?.reduce((sum, d) => sum + d.content_count, 0) ?? 0,
			change: undefined,
			icon: MessageSquare,
			color: "primary"
		},
		{
			label: t('analytics.event_count', 'Events'),
			value: overview?.event_counts?.reduce((sum, e) => sum + e.count, 0) ?? 0,
			change: undefined,
			icon: TrendingUp,
			color: "success"
		}
	]);

	// Top Content table columns for the Overview tab.
	const topContentColumns = [
		{ key: "content_title", label: t('analytics.content_title', 'Content Title'), sortable: true, render: (v: string) => v ? `<span class="font-medium">${v}</span>` : '—' },
		{ key: "total_views", label: t('analytics.views', 'Views'), sortable: true },
		{ key: "total_reactions", label: t('analytics.reactions', 'Reactions'), sortable: true },
		{ key: "total_comments", label: t('analytics.comments', 'Comments'), sortable: true },
		{ key: "total_unique_viewers", label: t('analytics.unique_viewers', 'Unique Viewers'), sortable: true }
	];

	// Map the user analytics summary object into metric/value rows for the
	// Users tab DataTable (backend returns an object, not an array).
	const userRows = $derived([
		{ metric: t('analytics.total_users', 'Total Users'), value: userAnalytics?.total_users ?? 0 },
		{ metric: t('analytics.active_users', 'Active Users'), value: userAnalytics?.active_users ?? 0 },
		{ metric: t('analytics.new_users_today', 'New Users Today'), value: userAnalytics?.new_users_today ?? 0 }
	]);

	// LayerChart Axis format: dates on the x-axis, numbers on the y-axis.
	// Typed `any` to satisfy the chart's union `axis` prop type.
	const chartAxisFormat: any = {
		format: (v: any) =>
			v instanceof Date
				? v.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
				: Number(v).toLocaleString()
	};

	const securityColumns = [
		{ key: "scan_type", label: t('analytics.type', 'Type'), sortable: true },
		{
			key: "severity",
			label: t('analytics.severity', 'Severity'),
			sortable: true,
			render: (value: string) => {
				const colors: Record<string, string> = { info: 'bg-info-100 text-info-800', warning: 'bg-warning/15 text-warning', low: 'bg-info-100 text-info-800', medium: 'bg-warning/15 text-warning', high: 'bg-danger/15 text-danger', critical: 'bg-danger text-white' };
				return `<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${colors[value] || 'bg-surface text-foreground'}">${value}</span>`;
			}
		},
		{
			key: "created_at",
			label: t('analytics.date', 'Date'),
			sortable: true,
			render: (value: string) => value ? new Date(value).toLocaleString() : '—'
		},
		{ key: "finding", label: t('analytics.findings', 'Finding'), sortable: true },
		{
			key: "resolved_at",
			label: t('analytics.resolved', 'Resolved'),
			sortable: true,
			render: (value: string) => value ? new Date(value).toLocaleString() : `<span class="text-warning font-medium">${t('analytics.unresolved', 'Unresolved')}</span>`
		}
	];
	
	async function handleFlushCache() {
		flushing = true;
		try {
			await analyticsAdminApi.flushCache({ scope: "all" });
			flushModalOpen = false;
			await goto('/analytics', { invalidateAll: true });
		} catch {
			error = t('analytics.failed_to_flush_cache', 'Failed to flush cache');
		} finally {
			flushing = false;
		}
	}
	
	function handleExport() {
		// Build CSV from overview data
		const rows: string[] = ['Metric,Value'];
		if (overview?.daily_active_users?.length) {
			const latest = overview.daily_active_users[overview.daily_active_users.length - 1];
			rows.push(`Daily Active Users,${latest.dau}`);
		}
		if (overview?.monthly_active_users?.length) {
			const latest = overview.monthly_active_users[overview.monthly_active_users.length - 1];
			rows.push(`Monthly Active Users,${latest.mau}`);
		}
		if (overview?.user_growth?.length) {
			const latest = overview.user_growth[overview.user_growth.length - 1];
			rows.push(`New Signups (Latest),${latest.new_users}`);
		}
		if (overview?.content_over_time?.length) {
			const total = overview.content_over_time.reduce((sum, d) => sum + d.content_count, 0);
			rows.push(`Total Content Items,${total}`);
		}
		if (overview?.event_counts?.length) {
			const total = overview.event_counts.reduce((sum, e) => sum + e.count, 0);
			rows.push(`Total Events,${total}`);
		}
		const csv = rows.join('\n');
		const blob = new Blob([csv], { type: 'text/csv' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = `analytics-report-${new Date().toISOString().slice(0, 10)}.csv`;
		a.click();
		URL.revokeObjectURL(url);
	}
</script>

<PageHeader
	title={t('analytics.page_title', 'Analytics')}
	description={t('analytics.page_description', 'Monitor platform performance and user engagement')}
	actions={[
		{ label: t('analytics.export_report', 'Export Report'), icon: Download, onclick: handleExport, variant: "outline" },
		{ label: t('analytics.flush_cache', 'Flush Cache'), icon: RefreshCw, onclick: () => flushModalOpen = true, variant: "ghost" }
	]}
/>

<div class="space-y-6">
	<!-- Date-range picker that writes ?from=&to= to the URL and refetches -->
	<div class="flex flex-wrap items-end gap-2">
		<div>
			<label for="analytics-from" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('common.from', 'From')}</label>
			<input
				id="analytics-from"
				type="date"
				bind:value={from}
				class="rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
			/>
		</div>
		<div>
			<label for="analytics-to" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('common.to', 'To')}</label>
			<input
				id="analytics-to"
				type="date"
				bind:value={to}
				class="rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
			/>
		</div>
		<Button variant="outline" size="sm" onclick={applyRange}>{t('common.apply', 'Apply')}</Button>
	</div>

	<Tabs {tabs} bind:value={activeTab} />
	
	{#if loading}
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each Array(6) as _}
				<CardSkeleton />
			{/each}
		</div>
		<div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
			{#each Array(2) as _}
				<Card>
					<CardHeader>
						<div class="h-4 w-32 animate-pulse rounded" style="background: var(--color-surface)"></div>
					</CardHeader>
					<CardContent>
						<div class="h-64 animate-pulse rounded-lg" style="background: var(--color-surface)"></div>
					</CardContent>
				</Card>
			{/each}
		</div>
	{:else if activeTab === "overview"}
		<!-- Stats Grid -->
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each overviewCards as card}
				<StatCard label={card.label} value={card.value} delta={card.change} />
			{/each}
		</div>
		
		<!-- Charts -->
		<div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
			<Card>
				<CardHeader>
					<h3 class="text-sm font-medium" style="color: var(--color-muted)">{t('analytics.user_growth', 'User Growth')}</h3>
				</CardHeader>
				<CardContent>
					{#if loading}
						<div class="h-64 animate-pulse rounded-lg" style="background: var(--color-surface)"></div>
					{:else if ((overview?.user_growth) ?? []).length > 0}
						<div class="h-64">
							<LineChart
								data={(overview?.user_growth ?? []).map((d) => ({ date: new Date(d.day), count: d.new_users }))}
								x="date"
								y="count"
								cRange={['var(--chart-1)']}
								axis={chartAxisFormat}
								props={{
									spline: { curve: curveMonotoneX, strokeWidth: 2, stroke: 'var(--chart-1)' }
								}}
							/>
						</div>
					{:else}
						<div class="flex h-64 items-center justify-center">
							<p class="text-sm" style="color: var(--color-muted)">{t('analytics.no_data_available', 'No data available')}</p>
						</div>
					{/if}
				</CardContent>
			</Card>
			
			<Card>
				<CardHeader>
					<h3 class="text-sm font-medium" style="color: var(--color-muted)">{t('analytics.content_activity', 'Content Activity')}</h3>
				</CardHeader>
				<CardContent>
					{#if loading}
						<div class="h-64 animate-pulse rounded-lg" style="background: var(--color-surface)"></div>
					{:else if ((overview?.content_over_time) ?? []).length > 0}
						<div class="h-64">
							<AreaChart
								data={(overview?.content_over_time ?? []).map((d) => ({ date: new Date(d.day), count: d.content_count }))}
								x="date"
								y="count"
								cRange={['var(--chart-1)']}
								axis={chartAxisFormat}
								props={{
									area: { curve: curveMonotoneX, fill: 'var(--chart-1)', fillOpacity: 0.2, strokeWidth: 2, stroke: 'var(--chart-1)', line: { curve: curveMonotoneX } }
								}}
							/>
						</div>
					{:else}
						<div class="flex h-64 items-center justify-center">
							<p class="text-sm" style="color: var(--color-muted)">{t('analytics.no_data_available', 'No data available')}</p>
						</div>
					{/if}
				</CardContent>
			</Card>
		</div>

		<!-- Traffic Sources + Top Content (data the backend already returns) -->
		<div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
			<Card>
				<CardHeader>
					<h3 class="text-sm font-medium" style="color: var(--color-muted)">{t('analytics.traffic_sources', 'Traffic Sources')}</h3>
				</CardHeader>
				<CardContent>
					{#if loading}
						<div class="h-48 animate-pulse rounded-lg" style="background: var(--color-surface)"></div>
					{:else if (overview?.traffic_sources ?? []).length > 0}
						<div class="h-48">
							<BarChart
								data={(overview?.traffic_sources ?? []).map((d) => ({ source: d.referrer ?? t('analytics.direct', 'Direct'), visits: d.visits }))}
								x="source"
								y="visits"
								xPadding={[0.1, 0.1]}
								cRange={['var(--chart-2)']}
								axis={chartAxisFormat}
							/>
						</div>
					{:else}
						<div class="flex h-48 items-center justify-center">
							<p class="text-sm" style="color: var(--color-muted)">{t('analytics.no_data_available', 'No data available')}</p>
						</div>
					{/if}
				</CardContent>
			</Card>

			<Card>
				<CardHeader>
					<h3 class="text-sm font-medium" style="color: var(--color-muted)">{t('analytics.top_content', 'Top Content')}</h3>
				</CardHeader>
				<CardContent>
					<DataTable
						columns={topContentColumns}
						data={overview?.top_content ?? []}
						pageSize={5}
						emptyMessage={t('analytics.no_top_content', 'No top content data')}
					/>
				</CardContent>
			</Card>
		</div>
	{/if}

	{#if activeTab === "security"}
		<Card>
			<CardHeader>
				<div class="flex items-center justify-between">
					<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.security_scans', 'Security Scans')}</h3>
					<Button onclick={runSecurityScan} disabled={scanning}>
						{#if scanning}
							<RefreshCw class="mr-2 h-4 w-4 animate-spin" />
							{t('analytics.scanning', 'Scanning...')}
						{:else}
							<Shield class="mr-2 h-4 w-4" />
							{t('analytics.run_scan', 'Run Scan')}
						{/if}
					</Button>
				</div>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={securityColumns}
					data={security ?? []}
					pageSize={10}
					emptyMessage={t('analytics.no_security_scans', 'No security scans found. Click "Run Scan" to start a security scan.')}
				/>
			</CardContent>
		</Card>
	{/if}
	
	{#if activeTab === "users"}
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.user_analytics', 'User Analytics')}</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={[
						{ key: "metric", label: t('analytics.metric', 'Metric'), sortable: true },
						{ key: "value", label: t('analytics.value', 'Value'), sortable: true, render: (v: number) => Number(v).toLocaleString() }
					]}
					data={userRows}
					emptyMessage={t('analytics.no_user_analytics', 'No user analytics data')}
				/>
			</CardContent>
		</Card>
	{/if}
	
	{#if activeTab === "content"}
		<div class="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
			<StatCard label={t('analytics.total_content', 'Total Content')} value={contentAnalytics?.total_content ?? 0} />
			<StatCard label={t('analytics.views', 'Views')} value={contentAnalytics?.total_views ?? 0} />
			<StatCard label={t('analytics.reactions', 'Reactions')} value={contentAnalytics?.total_reactions ?? 0} />
			<StatCard label={t('analytics.comments', 'Comments')} value={contentAnalytics?.total_comments ?? 0} />
			<StatCard label={t('analytics.unique_viewers', 'Unique Viewers')} value={contentAnalytics?.total_unique_viewers ?? 0} />
		</div>
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.content_analytics', 'Content Analytics')}</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={[
						{ key: "day", label: t('analytics.day', 'Day'), sortable: true, render: (v: string) => v ? new Date(v).toLocaleDateString() : '—' },
						{ key: "content_title", label: t('analytics.content_title', 'Content Title'), sortable: true, render: (v: string) => v ? `<span class="font-medium">${v}</span>` : '—' },
						{ key: "views", label: t('analytics.views', 'Views'), sortable: true },
						{ key: "reactions", label: t('analytics.reactions', 'Reactions'), sortable: true },
						{ key: "comments", label: t('analytics.comments', 'Comments'), sortable: true },
						{ key: "unique_viewers", label: t('analytics.unique_viewers', 'Unique Viewers'), sortable: true }
					]}
					data={contentAnalytics?.content_stats ?? []}
					emptyMessage={t('analytics.no_content_analytics', 'No content analytics data')}
				/>
			</CardContent>
		</Card>
	{/if}
	
	{#if activeTab === "spaces"}
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.space_analytics', 'Space Analytics')}</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={[
						{ key: "space_name", label: t('analytics.space_name', 'Space Name'), sortable: true },
						{ key: "total_joins", label: t('analytics.total_joins', 'Total Joins'), sortable: true },
						{ key: "total_posts", label: t('analytics.total_posts', 'Total Posts'), sortable: true }
					]}
					data={spaceAnalytics ?? []}
					pageSize={10}
					emptyMessage={t('analytics.no_space_analytics', 'No space analytics data')}
				/>
			</CardContent>
		</Card>
	{/if}
	
	{#if activeTab === "revenue"}
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
			<StatCard label={t('analytics.total_revenue', 'Total Revenue')} value={`$${(revenueAnalytics?.total_revenue ?? 0).toLocaleString()}`} />
			<StatCard label={t('analytics.monthly_revenue', 'Monthly Revenue')} value={`$${(revenueAnalytics?.monthly_revenue ?? 0).toLocaleString()}`} />
			<StatCard label={t('analytics.active_subscriptions', 'Active Subscriptions')} value={revenueAnalytics?.active_subscriptions ?? 0} />
		</div>
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.revenue_analytics', 'Revenue Analytics')}</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={[
						{ key: "period", label: t('analytics.period', 'Period'), sortable: true },
						{ key: "revenue", label: t('analytics.revenue', 'Revenue'), sortable: true, render: (v: number) => `$${v.toLocaleString()}` },
						{ key: "transactions", label: t('analytics.transactions', 'Transactions'), sortable: true }
					]}
					data={revenueAnalytics?.periods ?? []}
					emptyMessage={t('analytics.no_revenue_analytics', 'No revenue analytics data')}
				/>
			</CardContent>
		</Card>
	{/if}
	
	{#if activeTab === "search"}
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.search_analytics', 'Search Analytics')}</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={[
						{ key: "query", label: t('analytics.query', 'Query'), sortable: true },
						{ key: "count", label: t('analytics.count', 'Count'), sortable: true },
						{ key: "avg_results", label: t('analytics.avg_results', 'Avg Results'), sortable: true }
					]}
					data={searchAnalytics ?? []}
					emptyMessage={t('analytics.no_search_analytics', 'No search analytics data')}
				/>
			</CardContent>
		</Card>
	{/if}
</div>

<Modal
	bind:open={flushModalOpen}
	title={t('analytics.flush_analytics_cache', 'Flush Analytics Cache')}
	description={t('analytics.flush_cache_description', 'This will clear all cached analytics data. The cache will rebuild automatically.')}
	size="sm"
>
	<div class="flex justify-end gap-3">
		<Button variant="outline" onclick={() => flushModalOpen = false}>{t('analytics.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={handleFlushCache} disabled={flushing}>
			{#if flushing}
				<RefreshCw class="h-4 w-4 mr-2 animate-spin" />
				{t('analytics.flushing', 'Flushing...')}
			{:else}
				{t('analytics.flush_cache', 'Flush Cache')}
			{/if}
		</Button>
	</div>
</Modal>
