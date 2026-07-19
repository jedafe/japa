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
	
	let activeTab = $state("overview");
	let flushModalOpen = $state(false);
	let flushing = $state(false);
	let loading = $state(true);
	let error = $state("");
	// Date-range state sourced from the URL so it drives the analytics fetch
	let from = $state(page.url.searchParams.get("from") ?? "");
	let to = $state(page.url.searchParams.get("to") ?? "");
	
	// Analytics data
	let overview = $state<Record<string, unknown> | null>(null);
	let security = $state<{ scans?: Array<Record<string, unknown>> } | null>(null);
	let userAnalytics = $state<Array<Record<string, unknown>> | null>(null);
	let contentAnalytics = $state<Array<Record<string, unknown>> | null>(null);
	let spaceAnalytics = $state<Array<Record<string, unknown>> | null>(null);
	let revenueAnalytics = $state<Array<Record<string, unknown>> | null>(null);
	let searchAnalytics = $state<Array<Record<string, unknown>> | null>(null);
	
	async function loadAnalytics() {
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
			
			overview = overviewRes.status === 'fulfilled' && overviewRes.value.ok ? (overviewRes.value.data as Record<string, unknown>) : null;
			security = securityRes.status === 'fulfilled' && securityRes.value.ok ? (securityRes.value.data as { scans?: Array<Record<string, unknown>> }) : null;
			userAnalytics = usersRes.status === 'fulfilled' && usersRes.value.ok ? (usersRes.value.data as Array<Record<string, unknown>>) : null;
			contentAnalytics = contentRes.status === 'fulfilled' && contentRes.value.ok ? (contentRes.value.data as Array<Record<string, unknown>>) : null;
			spaceAnalytics = spacesRes.status === 'fulfilled' && spacesRes.value.ok ? (spacesRes.value.data as Array<Record<string, unknown>>) : null;
			revenueAnalytics = revenueRes.status === 'fulfilled' && revenueRes.value.ok ? (revenueRes.value.data as Array<Record<string, unknown>>) : null;
			searchAnalytics = searchRes.status === 'fulfilled' && searchRes.value.ok ? (searchRes.value.data as Array<Record<string, unknown>>) : null;
		} catch (e) {
			error = e instanceof Error ? e.message : "Failed to load analytics";
		} finally {
			loading = false;
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
	});
	
	const tabs = [
		{ value: "overview", label: "Overview", icon: TrendingUp },
		{ value: "users", label: "Users", icon: Users },
		{ value: "content", label: "Content", icon: MessageSquare },
		{ value: "spaces", label: "Spaces", icon: Eye },
		{ value: "revenue", label: "Revenue", icon: DollarSign },
		{ value: "search", label: "Search", icon: Search },
		{ value: "security", label: "Security", icon: Shield }
	];
	
	const overviewCards = $derived([
		{ 
			label: "Total Users", 
			value: (overview?.totalUsers as number) ?? 0, 
			change: (overview?.usersChange as number) ?? 0, 
			icon: Users, 
			color: "primary" 
		},
		{ 
			label: "Active Users", 
			value: (overview?.activeUsers as number) ?? 0, 
			change: (overview?.activeChange as number) ?? 0, 
			icon: Users, 
			color: "success" 
		},
		{ 
			label: "New Signups", 
			value: (overview?.newSignups as number) ?? 0, 
			change: (overview?.signupChange as number) ?? 0, 
			icon: UserPlus, 
			color: "info" 
		},
		{ 
			label: "Total Spaces", 
			value: (overview?.totalSpaces as number) ?? 0, 
			change: (overview?.spacesChange as number) ?? 0, 
			icon: Eye, 
			color: "warning" 
		},
		{ 
			label: "Total Posts", 
			value: (overview?.totalPosts as number) ?? 0, 
			change: (overview?.postsChange as number) ?? 0, 
			icon: MessageSquare, 
			color: "primary" 
		},
		{ 
			label: "Revenue", 
			value: `$${((overview?.revenue as number) ?? 0).toLocaleString()}`, 
			change: (overview?.revenueChange as number) ?? 0, 
			icon: DollarSign, 
			color: "success" 
		}
	]);
	
	const securityColumns = [
		{ key: "scanId", label: "Scan ID", sortable: true },
		{ key: "type", label: "Type", sortable: true },
		{ 
			key: "status", 
			label: "Status", 
			sortable: true,
			render: (value: string) => {
				const colors = value === 'passed' ? 'bg-success/15 text-success' : 'bg-danger/15 text-danger';
				return `<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${colors}">${value}</span>`;
			}
		},
		{ 
			key: "date", 
			label: "Date", 
			sortable: true,
			render: (value: string) => new Date(value).toLocaleString()
		},
		{ key: "findings", label: "Findings", sortable: true }
	];
	
	async function handleFlushCache() {
		flushing = true;
		try {
			await analyticsAdminApi.flushCache({ scope: "all" });
			flushModalOpen = false;
			goto("", { invalidateAll: true });
		} catch {
			error = "Failed to flush cache";
		} finally {
			flushing = false;
		}
	}
	
	function handleExport() {
		// TODO: Implement analytics export
	}
</script>

<PageHeader
	title="Analytics"
	description="Monitor platform performance and user engagement"
	actions={[
		{ label: "Export Report", icon: Download, onclick: handleExport, variant: "outline" },
		{ label: "Flush Cache", icon: RefreshCw, onclick: () => flushModalOpen = true, variant: "ghost" }
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
	
	{#if activeTab === "overview"}
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
					<h3 class="text-sm font-medium" style="color: var(--color-muted)">{t('analytics.user_growth')}</h3>
				</CardHeader>
				<CardContent>
					{#if loading}
						<div class="h-64 animate-pulse rounded-lg" style="background: var(--color-surface)"></div>
					{:else if ((overview?.userGrowth as Array<{ date: string; count: number }>) ?? []).length > 0}
						<div class="h-64">
							<LineChart
								data={((overview?.userGrowth as Array<{ date: string; count: number }>) ?? []).map((d) => ({ date: new Date(d.date), count: d.count }))}
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
							<p class="text-sm" style="color: var(--color-muted)">{t('analytics.no_data_available')}</p>
						</div>
					{/if}
				</CardContent>
			</Card>
			
			<Card>
				<CardHeader>
					<h3 class="text-sm font-medium" style="color: var(--color-muted)">{t('analytics.content_activity')}</h3>
				</CardHeader>
				<CardContent>
					{#if loading}
						<div class="h-64 animate-pulse rounded-lg" style="background: var(--color-surface)"></div>
					{:else if ((overview?.contentActivity as Array<{ date: string; count: number }>) ?? []).length > 0}
						<div class="h-64">
							<AreaChart
								data={((overview?.contentActivity as Array<{ date: string; count: number }>) ?? []).map((d) => ({ date: new Date(d.date), count: d.count }))}
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
							<p class="text-sm" style="color: var(--color-muted)">{t('analytics.no_data_available')}</p>
						</div>
					{/if}
				</CardContent>
			</Card>
		</div>
	{/if}
	
	{#if activeTab === "security"}
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.security_scans')}</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={securityColumns}
					data={security?.scans ?? []}
					pageSize={10}
					emptyMessage="No security scans found"
				/>
			</CardContent>
		</Card>
	{/if}
	
	{#if activeTab === "users"}
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.user_analytics')}</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={[
						{ key: "metric", label: "Metric", sortable: true },
						{ key: "value", label: "Value", sortable: true },
						{ key: "change", label: t('analytics.change', 'Change'), sortable: true, render: (v: number) => `<span class="${v >= 0 ? 'text-success-600' : 'text-danger-600'}">${v >= 0 ? '+' : ''}${v}%</span>` }
					]}
					data={userAnalytics ?? []}
					emptyMessage="No user analytics data"
				/>
			</CardContent>
		</Card>
	{/if}
	
	{#if activeTab === "content"}
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.content_analytics')}</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={[
						{ key: "type", label: "Type", sortable: true },
						{ key: "count", label: "Count", sortable: true },
						{ key: "engagement", label: "Engagement", sortable: true }
					]}
					data={contentAnalytics ?? []}
					emptyMessage="No content analytics data"
				/>
			</CardContent>
		</Card>
	{/if}
	
	{#if activeTab === "spaces"}
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.space_analytics')}</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={[
						{ key: "space", label: "Space", sortable: true },
						{ key: "members", label: "Members", sortable: true },
						{ key: "activity", label: "Activity", sortable: true }
					]}
					data={spaceAnalytics ?? []}
					emptyMessage="No space analytics data"
				/>
			</CardContent>
		</Card>
	{/if}
	
	{#if activeTab === "revenue"}
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.revenue_analytics')}</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={[
						{ key: "period", label: "Period", sortable: true },
						{ key: "revenue", label: t('analytics.revenue', 'Revenue'), sortable: true, render: (v: number) => `$${v.toLocaleString()}` },
						{ key: "transactions", label: "Transactions", sortable: true }
					]}
					data={revenueAnalytics ?? []}
					emptyMessage="No revenue analytics data"
				/>
			</CardContent>
		</Card>
	{/if}
	
	{#if activeTab === "search"}
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('analytics.search_analytics')}</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={[
						{ key: "query", label: "Query", sortable: true },
						{ key: "count", label: "Count", sortable: true },
						{ key: "results", label: "Avg Results", sortable: true }
					]}
					data={searchAnalytics ?? []}
					emptyMessage="No search analytics data"
				/>
			</CardContent>
		</Card>
	{/if}
</div>

<Modal
	bind:open={flushModalOpen}
	title="Flush Analytics Cache"
	description="This will clear all cached analytics data. The cache will rebuild automatically."
	size="sm"
>
	<div class="flex justify-end gap-3">
		<Button variant="outline" onclick={() => flushModalOpen = false}>{t('analytics.cancel')}</Button>
		<Button variant="destructive" onclick={handleFlushCache} disabled={flushing}>
			{#if flushing}
				<RefreshCw class="h-4 w-4 mr-2 animate-spin" />
				Flushing...
			{:else}
				Flush Cache
			{/if}
		</Button>
	</div>
</Modal>
