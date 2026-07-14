<script lang="ts">
	import { page } from "$app/state";
	import { goto } from "$app/navigation";
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { Tabs } from "$lib/components/ui/tabs";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Badge } from "$lib/components/ui/badge";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { Mail, Plus, Send, Users, BarChart3, Trash2 } from "lucide-svelte";
	import { api } from "$lib/api";

	type Subscriber = {
		id: string;
		email: string;
		name?: string;
		status: string;
		subscribedAt: string;
		created_at: string;
	};

	type Campaign = {
		id: string;
		name: string;
		subject: string;
		preview?: string;
		status: string;
		sent_at?: string;
		sentAt?: string;
		recipient_count?: number;
		openRate?: number;
		open_rate?: number;
		click_rate?: number;
		created_at: string;
	};

	type NewsletterStats = {
		subscribers: number;
		campaigns: number;
		avgOpenRate: number;
		avgClickRate: number;
	};

	let { data } = $props();

	let activeTab = $state("subscribers");
	let subscribers = $state<Subscriber[]>([]);
	let campaigns = $state<Campaign[]>([]);
	let stats = $state<NewsletterStats>({ subscribers: 0, campaigns: 0, avgOpenRate: 0, avgClickRate: 0 });
	let loading = $state(true);
	let filters = $state<Record<string, string>>({});

	// Campaign creation modal
	let showCreateModal = $state(false);
	let newCampaign = $state({ name: "", subject: "", html: "" });
	let creating = $state(false);

	// Delete confirmation
	let showDeleteModal = $state(false);
	let deleteTarget = $state<Subscriber | null>(null);
	let deleting = $state(false);

	const tabs = [
		{ value: "subscribers", label: "Subscribers", icon: Users, count: stats.subscribers },
		{ value: "campaigns", label: "Campaigns", icon: Mail, count: stats.campaigns },
		{ value: "analytics", label: "Analytics", icon: BarChart3 }
	];

	const subscriberColumns = [
		{
			key: "email",
			label: "Email",
			sortable: true,
			render: (_: any, row: any) => `
				<div class="flex items-center gap-3">
					<div class="h-8 w-8 rounded-full bg-primary-100 flex items-center justify-center">
						<span class="text-sm font-medium text-primary-700">${(row.email || '?')[0].toUpperCase()}</span>
					</div>
					<div>
						<p class="font-medium text-gray-900">${row.email}</p>
						${row.name ? `<p class="text-sm text-gray-500">${row.name}</p>` : ''}
					</div>
				</div>
			`
		},
		{
			key: "status",
			label: "Status",
			sortable: true,
			render: (value: string) => {
				const variant = value === "active" ? "success" : value === "bounced" ? "destructive" : "secondary";
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${variant === "success" ? "bg-success-100 text-success-800 border-success-200" : variant === "destructive" ? "bg-danger-100 text-danger-800 border-danger-200" : "bg-gray-100 text-gray-800 border-gray-200"}">${value || "active"}</span>`;
			}
		},
		{
			key: "subscribedAt",
			label: "Subscribed",
			sortable: true,
			render: (value: string, row: any) => new Date(value || row.created_at).toLocaleDateString("en-US", { year: "numeric", month: "short", day: "numeric" })
		},
		{
			key: "actions",
			label: "",
			class: "w-24",
			render: (_: any, row: any) => `
				<div class="flex items-center gap-1">
					<button onclick="window.__newsletterEditSubscriber('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100">Edit</button>
					<button onclick="window.__newsletterDeleteSubscriber('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-danger-700 hover:bg-danger-50">Remove</button>
				</div>
			`
		}
	];

	const campaignColumns = [
		{
			key: "subject",
			label: "Subject",
			sortable: true,
			render: (_: any, row: any) => `
				<div>
					<p class="font-medium text-gray-900">${row.subject || row.name || "No subject"}</p>
					${row.preview ? `<p class="text-sm text-gray-500">${row.preview}</p>` : ""}
				</div>
			`
		},
		{
			key: "status",
			label: "Status",
			sortable: true,
			render: (value: string) => {
				const variant = value === "sent" ? "success" : value === "scheduled" ? "info" : value === "failed" ? "destructive" : "secondary";
				const classes: Record<string, string> = {
					success: "bg-success-100 text-success-800 border-success-200",
					info: "bg-info-100 text-info-800 border-info-200",
					destructive: "bg-danger-100 text-danger-800 border-danger-200",
					secondary: "bg-gray-100 text-gray-800 border-gray-200"
				};
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes[variant]}">${value || "draft"}</span>`;
			}
		},
		{
			key: "sentAt",
			label: "Sent",
			sortable: true,
			render: (value: string, row: any) => {
				const date = value || row.sent_at;
				return date ? new Date(date).toLocaleDateString("en-US", { year: "numeric", month: "short", day: "numeric" }) : '<span class="text-gray-400">—</span>';
			}
		},
		{
			key: "openRate",
			label: "Open Rate",
			sortable: true,
			render: (value: number, row: any) => {
				const rate = value ?? row.open_rate ?? 0;
				return `<span class="font-medium">${rate.toFixed(1)}%</span>`;
			}
		},
		{
			key: "actions",
			label: "",
			class: "w-24",
			render: (_: any, row: any) => `
				<div class="flex items-center gap-1">
					<button onclick="window.__newsletterViewCampaign('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100">View</button>
					${row.status === "draft" ? `<button onclick="window.__newsletterSendCampaign('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-primary-700 hover:bg-primary-50">Send</button>` : ""}
				</div>
			`
		}
	];

	async function loadSubscribers() {
		loading = true;
		try {
			const result = await api<Subscriber[]>("/admin/newsletter/subscribers");
			if (result.ok) {
				subscribers = result.data ?? [];
				stats.subscribers = subscribers.length;
			}
		} catch {
			// API not available
		} finally {
			loading = false;
		}
	}

	async function loadCampaigns() {
		loading = true;
		try {
			const result = await api<Campaign[]>("/admin/newsletter/campaigns");
			if (result.ok) {
				campaigns = result.data ?? [];
				stats.campaigns = campaigns.length;
			}
		} catch {
			// API not available
		} finally {
			loading = false;
		}
	}

	function handleTabChange() {
		if (activeTab === "subscribers") loadSubscribers();
		else if (activeTab === "campaigns") loadCampaigns();
	}

	function handleFilter(values: Record<string, string>) {
		filters = values;
	}

	async function createCampaign() {
		if (!newCampaign.name || !newCampaign.subject) return;
		creating = true;
		try {
			const result = await api("/admin/newsletter/campaigns", {
				method: "POST",
				body: newCampaign
			});
			if (result.ok) {
				showCreateModal = false;
				newCampaign = { name: "", subject: "", html: "" };
				loadCampaigns();
			}
		} catch {
			// API not available
		} finally {
			creating = false;
		}
	}

	async function sendCampaign(id: string) {
		try {
			await api(`/admin/newsletter/campaigns/${id}/send`, { method: "POST" });
			loadCampaigns();
		} catch {
			// API not available
		}
	}

	function confirmDeleteSubscriber(subscriber: Subscriber) {
		deleteTarget = subscriber;
		showDeleteModal = true;
	}

	async function deleteSubscriber() {
		if (!deleteTarget) return;
		deleting = true;
		try {
			await api(`/admin/newsletter/subscribers/${deleteTarget.id}`, { method: "DELETE" });
			subscribers = subscribers.filter((s) => s.id !== deleteTarget!.id);
			stats.subscribers = subscribers.length;
			showDeleteModal = false;
			deleteTarget = null;
		} catch {
			// API not available
		} finally {
			deleting = false;
		}
	}

	// Expose window functions for DataTable inline actions
	$effect(() => {
		(window as any).__newsletterEditSubscriber = (id: string) => {
			goto(`/newsletter/subscribers/${id}`);
		};
		(window as any).__newsletterDeleteSubscriber = (id: string) => {
			const sub = subscribers.find((s) => s.id === id);
			if (sub) confirmDeleteSubscriber(sub);
		};
		(window as any).__newsletterViewCampaign = (id: string) => {
			goto(`/newsletter/campaigns/${id}`);
		};
		(window as any).__newsletterSendCampaign = (id: string) => {
			sendCampaign(id);
		};
	});

	let initialized = false;
	$effect(() => {
		if (!initialized) {
			initialized = true;
			loadSubscribers();
		}
	});
</script>

<svelte:head>
	<title>Newsletter — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title="Newsletter"
	description="Manage email subscribers and campaigns"
	actions={[
		{ label: "New Campaign", icon: Plus, onclick: () => (showCreateModal = true) },
		{ label: "Import Subscribers", icon: Users, variant: "outline" }
	]}
/>

<div class="space-y-6">
	<Tabs {tabs} bind:value={activeTab} />

	{#if activeTab === "subscribers"}
		<FilterBar
			bind:value={filters}
			filters={[
				{
					key: "status",
					label: "Status",
					options: [
						{ value: "active", label: "Active" },
						{ value: "unsubscribed", label: "Unsubscribed" },
						{ value: "bounced", label: "Bounced" }
					]
				}
			]}
			onsearch={handleFilter}
			onclear={() => { filters = {}; }}
		/>

		<Card>
			<CardContent class="p-0">
				{#if loading}
					<p class="py-8 text-center text-sm text-gray-500">Loading subscribers...</p>
				{:else}
					<DataTable
						columns={subscriberColumns}
						data={subscribers}
						pageSize={20}
						selectable={true}
						emptyMessage="No subscribers yet"
					/>
				{/if}
			</CardContent>
		</Card>
	{/if}

	{#if activeTab === "campaigns"}
		<Card>
			<CardContent class="p-0">
				{#if loading}
					<p class="py-8 text-center text-sm text-gray-500">Loading campaigns...</p>
				{:else}
					<DataTable
						columns={campaignColumns}
						data={campaigns}
						pageSize={10}
						emptyMessage="No campaigns yet"
					/>
				{/if}
			</CardContent>
		</Card>
	{/if}

	{#if activeTab === "analytics"}
		<div class="grid grid-cols-1 gap-6 md:grid-cols-3">
			<Card>
				<CardContent class="p-6">
					<p class="text-sm font-medium text-gray-500">Total Subscribers</p>
					<p class="text-2xl font-bold text-gray-900">{stats.subscribers}</p>
				</CardContent>
			</Card>
			<Card>
				<CardContent class="p-6">
					<p class="text-sm font-medium text-gray-500">Avg Open Rate</p>
					<p class="text-2xl font-bold text-gray-900">{stats.avgOpenRate}%</p>
				</CardContent>
			</Card>
			<Card>
				<CardContent class="p-6">
					<p class="text-sm font-medium text-gray-500">Avg Click Rate</p>
					<p class="text-2xl font-bold text-gray-900">{stats.avgClickRate}%</p>
				</CardContent>
			</Card>
		</div>

		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold text-gray-900">Campaign Performance</h3>
			</CardHeader>
			<CardContent>
				<div class="flex h-64 items-center justify-center rounded-lg bg-gray-50">
					<p class="text-gray-500">Analytics chart placeholder — integrate with chart library</p>
				</div>
			</CardContent>
		</Card>
	{/if}
</div>

<!-- Create Campaign Modal -->
<Modal bind:open={showCreateModal} title="Create Campaign" description="Compose a new newsletter campaign" size="lg">
	<div class="space-y-4">
		<div>
			<label for="campaign-name" class="mb-1 block text-sm font-medium text-gray-900">Campaign Name</label>
			<input
				id="campaign-name"
				type="text"
				class="h-10 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				placeholder="e.g. Weekly Digest #42"
				bind:value={newCampaign.name}
			/>
		</div>
		<div>
			<label for="campaign-subject" class="mb-1 block text-sm font-medium text-gray-900">Subject Line</label>
			<input
				id="campaign-subject"
				type="text"
				class="h-10 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				placeholder="What subscribers will see in their inbox"
				bind:value={newCampaign.subject}
			/>
		</div>
		<div>
			<label for="campaign-html" class="mb-1 block text-sm font-medium text-gray-900">Content (HTML)</label>
			<textarea
				id="campaign-html"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				rows="8"
				placeholder="<h1>Hello!</h1><p>Your newsletter content here...</p>"
				bind:value={newCampaign.html}
			></textarea>
		</div>
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => (showCreateModal = false)}>Cancel</Button>
			<Button onclick={createCampaign} disabled={creating || !newCampaign.name || !newCampaign.subject}>
				{#if creating}
					Creating...
				{:else}
					Create Campaign
				{/if}
			</Button>
		</div>
	</div>
</Modal>

<!-- Delete Subscriber Confirmation Modal -->
<Modal bind:open={showDeleteModal} title="Remove Subscriber" description="This subscriber will be removed from your mailing list." size="sm">
	{#if deleteTarget}
		<div class="rounded-lg bg-gray-50 p-4">
			<p class="text-sm text-gray-700">
				Are you sure you want to remove <strong>{deleteTarget.email}</strong>?
			</p>
		</div>
	{/if}
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => (showDeleteModal = false)}>Cancel</Button>
		<Button variant="destructive" onclick={deleteSubscriber} disabled={deleting}>
			{#if deleting}
				Removing...
			{:else}
				Remove
			{/if}
		</Button>
	</div>
</Modal>
