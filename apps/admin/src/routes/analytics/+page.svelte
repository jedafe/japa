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
	import { LineChart, AreaChart } from "layerchart";
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
	let security = $state<{ scans?: Array<Record<string, unknown>> } | null>(null);
	let userAnalytics = $state<Array<Record<string, unknown>> | null>(null);
	let contentAnalytics = $state<Array<Record<string, unknown>> | null>(null);
	let spaceAnalytics = $state<Array<Record<string, unknown>> | null>(null);
	let revenueAnalytics = $state<Array<Record<string, unknown>> | null>(null);
	let searchAnalytics = $state<Array<Record<string, unknown>> | null>(null);
	let loadAbortController: AbortController | null = null;
	
	async function loadAnalytics() {
		if (loadAbortController) loadAbortController.abort();
		loadAbortController = new AbortController();
		const signal = loadAbortController.signal;

		loading = true;
		error = "";
		try {
		const [overviewRes, securityRes, usersRes, contentRes, spacesRes, revenueRes, searchRes] = await Promise.allSettled([
			// Pass the ?from=&to= range when set; otherwise fetch the default window
			analyticsAdminApi.getOverview([from && `from=${from}`, to && `to=${to}`].filter(Boolean).join("&") || undefined),
			analyticsAdminApi.getSecurity(),
				analyticsAdminApi.getUsers(),
				analyticsAdminApi.getContent(),
				analyticsAdminApi.getSpaces(),
				analyticsAdminApi.getRevenue(),
				analyticsAdminApi.getSearch()
			]);
			
			if (signal.aborted) return;

			overview = overviewRes.status === 'fulfilled' && overviewRes.value.ok ? (overviewRes.value.data as OverviewData) : null;
			security = securityRes.status === 'fulfilled' && securityRes.value.ok ? (securityRes.value.data as { scans?: Array<Record<string, unknown>> }) : null;
			userAnalytics = usersRes.status === 'fulfilled' && usersRes.value.ok ? (usersRes.value.data as Array<Record<string, unknown>>) : null;
			contentAnalytics = contentRes.status === 'fulfilled' && contentRes.value.ok ? (contentRes.value.data as Array<Record<string, unknown>>) : null;
			spaceAnalytics = spacesRes.status === 'fulfilled' && spacesRes.value.ok ? (spacesRes.value.data as Array<Record<string, unknown>>) : null;
			revenueAnalytics = revenueRes.status === 'fulfilled' && revenueRes.value.ok ? (revenueRes.value.data as Array<Record<string, unknown>>) : null;
			searchAnalytics = searchRes.status === 'fulfilled' && searchRes.value.ok ? (searchRes.value.data as Array<Record<string, unknown>>) : null;
		} catch (e) {
			if (signal.aborted) return;
			error = e instanceof Error ? e.message : t('analytics.failed_to_load', 'Failed to load analytics');
		} finally {
			if (!signal.aborted) loading = false;
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
		from = page.url.searchParams.get("from") ?? "";
		to = page.url.searchParams.get("to") ?? "";
		loadAnalytics();

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
	
	const overviewCards = $derived([
		{ 
			label: t('analytics.daily_active_users', 'Daily Active Users'), 
			value: overview?.daily_active_users?.[overview.daily_active_users.length - 1]?.dau ?? 0, 
			change: 0,
			icon: Users, 
			color: "primary" 
		},
		{ 
			label: t('analytics.monthly_active_users', 'Monthly Active Users'), 
			value: overview?.monthly_active_users?.[overview.monthly_active_users.length - 1]?.mau ?? 0, 
			change: 0,
			icon: Users, 
			color: "success" 
		},
		{ 
			label: t('analytics.new_signups', 'New Signups (Latest)'), 
			value: overview?.user_growth?.[overview.user_growth.length - 1]?.new_users ?? 0, 
			change: 0,
			icon: UserPlus, 
			color: "info" 
		},
		{ 
			label: t('analytics.total_spaces', 'Total Spaces'), 
			value: overview?.space_stats ? new Set(overview.space_stats.map((s: any) => s.space_id)).size : 0, 
			change: 0,
			icon: Eye, 
			color: "warning" 
		},
		{ 
			label: t('analytics.content_count', 'Content Items'), 
			value: overview?.content_over_time?.reduce((sum, d) => sum + d.content_count, 0) ?? 0, 
			change: 0,
			icon: MessageSquare, 
			color: "primary" 
		},
		{ 
			label: t('analytics.event_count', 'Events'), 
			value: overview?.event_counts?.reduce((sum, e) => sum + e.count, 0) ?? 0, 
			change: 0,
			icon: TrendingUp, 
			color: "success" 
		}
	]);
	
	const securityColumns = [
		{ key: "id", label: t('analytics.scan_id', 'Scan ID'), sortable: true },
		{ key: "scan_type", label: t('analytics.type', 'Type'), sortable: true },
		{ 
			key: "severity", 
			label: t('analytics.status', 'Severity'), 
			sortable: true,
			render: (value: string) => {
				const colors: Record<string, string> = { low: 'bg-info-100 text-info-800', medium: 'bg-warning/15 text-warning', high: 'bg-danger/15 text-danger', critical: 'bg-danger text-white' };
				return `<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${colors[value] || 'bg-surface text-foreground'}">${value}</span>`;
			}
		},
		{ 
			key: "created_at", 
			label: t('analytics.date', 'Date'), 
			sortable: true,
			render: (value: string) => new Date(value).toLocaleString()
		},
		{ key: "finding", label: t('analytics.findings', 'Finding'), sortable: true }
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
								props={{
									spline: { curve: 'monotone-x', strokeWidth: 2, stroke: 'var(--color-primary)' },
									xAxis: { format: (d: Date) => d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) },
									yAxis: { format: (v: number) => v.toLocaleString() }
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
								props={{
									area: { curve: 'monotone-x', fill: 'var(--color-primary)', fillOpacity: 0.2, strokeWidth: 2, stroke: 'var(--color-primary)' },
									xAxis: { format: (d: Date) => d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) },
									yAxis: { format: (v: number) => v.toLocaleString() }
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
	{/if}
	
	{#if activeTab === "security"}
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.security_scans', 'Security Scans')}</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={securityColumns}
					data={security?.scans ?? []}
					pageSize={10}
					emptyMessage={t('analytics.no_security_scans', 'No security scans found')}
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
						{ key: "value", label: t('analytics.value', 'Value'), sortable: true },
						{ key: "change", label: t('analytics.change', 'Change'), sortable: true, render: (v: number) => `<span class="${v >= 0 ? 'text-success-600' : 'text-danger-600'}">${v >= 0 ? '+' : ''}${v}%</span>` }
					]}
					data={userAnalytics ?? []}
					emptyMessage={t('analytics.no_user_analytics', 'No user analytics data')}
				/>
			</CardContent>
		</Card>
	{/if}
	
	{#if activeTab === "content"}
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.content_analytics', 'Content Analytics')}</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={[
						{ key: "type", label: t('analytics.type', 'Type'), sortable: true },
						{ key: "count", label: t('analytics.count', 'Count'), sortable: true },
						{ key: "engagement", label: t('analytics.engagement', 'Engagement'), sortable: true }
					]}
					data={contentAnalytics ?? []}
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
						{ key: "space", label: t('analytics.space', 'Space'), sortable: true },
						{ key: "members", label: t('analytics.members', 'Members'), sortable: true },
						{ key: "activity", label: t('analytics.activity', 'Activity'), sortable: true }
					]}
					data={spaceAnalytics ?? []}
					emptyMessage={t('analytics.no_space_analytics', 'No space analytics data')}
				/>
			</CardContent>
		</Card>
	{/if}
	
	{#if activeTab === "revenue"}
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
					data={revenueAnalytics ?? []}
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
						{ key: "results", label: t('analytics.avg_results', 'Avg Results'), sortable: true }
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
