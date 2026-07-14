<script lang="ts">
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
	
	let { data } = $props();
	
	let activeTab = $state("overview");
	let flushModalOpen = $state(false);
	let flushing = $state(false);
	
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
			value: data.overview?.totalUsers ?? 0, 
			change: data.overview?.usersChange ?? 0, 
			icon: Users, 
			color: "primary" 
		},
		{ 
			label: "Active Users", 
			value: data.overview?.activeUsers ?? 0, 
			change: data.overview?.activeChange ?? 0, 
			icon: Users, 
			color: "success" 
		},
		{ 
			label: "New Signups", 
			value: data.overview?.newSignups ?? 0, 
			change: data.overview?.signupChange ?? 0, 
			icon: UserPlus, 
			color: "info" 
		},
		{ 
			label: "Total Spaces", 
			value: data.overview?.totalSpaces ?? 0, 
			change: data.overview?.spacesChange ?? 0, 
			icon: Eye, 
			color: "warning" 
		},
		{ 
			label: "Total Posts", 
			value: data.overview?.totalPosts ?? 0, 
			change: data.overview?.postsChange ?? 0, 
			icon: MessageSquare, 
			color: "primary" 
		},
		{ 
			label: "Revenue", 
			value: `$${(data.overview?.revenue ?? 0).toLocaleString()}`, 
			change: data.overview?.revenueChange ?? 0, 
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
				const colors = value === 'passed' ? 'bg-success-100 text-success-800' : 'bg-danger-100 text-danger-800';
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
			await analyticsAdminApi.flushCache({ targets: "all" });
			flushModalOpen = false;
			goto("", { invalidateAll: true });
		} catch (error) {
			console.error("Failed to flush cache:", error);
		} finally {
			flushing = false;
		}
	}
	
	function handleExport() {
		// TODO: Implement analytics export
		console.log("Export analytics");
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
	<Tabs {tabs} bind:value={activeTab} />
	
	{#if activeTab === "overview"}
		<!-- Stats Grid -->
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each overviewCards as card}
				<Card>
					<CardContent class="p-6">
						<div class="flex items-center justify-between">
							<div>
								<p class="text-sm font-medium text-gray-500">{card.label}</p>
								<p class="text-2xl font-bold text-gray-900">{card.value}</p>
								<p class="text-sm {card.change >= 0 ? 'text-success-600' : 'text-danger-600'}">
									{card.change >= 0 ? '+' : ''}{card.change}% from last period
								</p>
							</div>
							<div class="rounded-lg bg-{card.color}-100 p-3">
								<card.icon class="h-6 w-6 text-{card.color}-600" />
							</div>
						</div>
					</CardContent>
				</Card>
			{/each}
		</div>
		
		<!-- Charts Placeholder -->
		<div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
			<Card>
				<CardHeader>
					<h3 class="text-lg font-semibold text-gray-900">User Growth</h3>
				</CardHeader>
				<CardContent>
					<div class="flex h-64 items-center justify-center rounded-lg bg-gray-50">
						<p class="text-gray-500">Chart placeholder - integrate with chart library</p>
					</div>
				</CardContent>
			</Card>
			
			<Card>
				<CardHeader>
					<h3 class="text-lg font-semibold text-gray-900">Content Activity</h3>
				</CardHeader>
				<CardContent>
					<div class="flex h-64 items-center justify-center rounded-lg bg-gray-50">
						<p class="text-gray-500">Chart placeholder - integrate with chart library</p>
					</div>
				</CardContent>
			</Card>
		</div>
	{/if}
	
	{#if activeTab === "security"}
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold text-gray-900">Security Scans</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={securityColumns}
					data={data.security?.scans ?? []}
					pageSize={10}
					emptyMessage="No security scans found"
				/>
			</CardContent>
		</Card>
	{/if}
	
	{#if activeTab === "users"}
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold text-gray-900">User Analytics</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={[
						{ key: "metric", label: "Metric", sortable: true },
						{ key: "value", label: "Value", sortable: true },
						{ key: "change", label: "Change", sortable: true, render: (v: number) => `<span class="${v >= 0 ? 'text-success-600' : 'text-danger-600'}">${v >= 0 ? '+' : ''}${v}%</span>` }
					]}
					data={data.userAnalytics ?? []}
					emptyMessage="No user analytics data"
				/>
			</CardContent>
		</Card>
	{/if}
	
	{#if activeTab === "content"}
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold text-gray-900">Content Analytics</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={[
						{ key: "type", label: "Type", sortable: true },
						{ key: "count", label: "Count", sortable: true },
						{ key: "engagement", label: "Engagement", sortable: true }
					]}
					data={data.contentAnalytics ?? []}
					emptyMessage="No content analytics data"
				/>
			</CardContent>
		</Card>
	{/if}
	
	{#if activeTab === "spaces"}
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold text-gray-900">Space Analytics</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={[
						{ key: "space", label: "Space", sortable: true },
						{ key: "members", label: "Members", sortable: true },
						{ key: "activity", label: "Activity", sortable: true }
					]}
					data={data.spaceAnalytics ?? []}
					emptyMessage="No space analytics data"
				/>
			</CardContent>
		</Card>
	{/if}
	
	{#if activeTab === "revenue"}
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold text-gray-900">Revenue Analytics</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={[
						{ key: "period", label: "Period", sortable: true },
						{ key: "revenue", label: "Revenue", sortable: true, render: (v: number) => `$${v.toLocaleString()}` },
						{ key: "transactions", label: "Transactions", sortable: true }
					]}
					data={data.revenueAnalytics ?? []}
					emptyMessage="No revenue analytics data"
				/>
			</CardContent>
		</Card>
	{/if}
	
	{#if activeTab === "search"}
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold text-gray-900">Search Analytics</h3>
			</CardHeader>
			<CardContent>
				<DataTable
					columns={[
						{ key: "query", label: "Query", sortable: true },
						{ key: "count", label: "Count", sortable: true },
						{ key: "results", label: "Avg Results", sortable: true }
					]}
					data={data.searchAnalytics ?? []}
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
		<Button variant="outline" onclick={() => flushModalOpen = false}>Cancel</Button>
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
