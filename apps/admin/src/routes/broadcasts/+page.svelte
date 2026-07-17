<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
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
	import { Send, Plus, Radio, Clock, CheckCircle } from "lucide-svelte";
	import { Select } from '$lib/components/ui/select';
	import { api } from "$lib/api";

	type Broadcast = {
		id: string;
		title: string;
		body: string;
		status: string;
		target_audience: string;
		sent_at?: string;
		scheduled_at?: string;
		recipient_count?: number;
		delivered_count?: number;
		created_at: string;
	};

	let { data } = $props();

	let activeTab = $state("all");
	let broadcasts = $state<Broadcast[]>([]);
	let loading = $state(true);
	let filters = $state<Record<string, string>>({});

	// Create modal
	let showCreateModal = $state(false);
	let newBroadcast = $state({ title: "", body: "", target_audience: "all" });
	let creating = $state(false);

	// Send confirmation
	let showSendModal = $state(false);
	let sendTarget = $state<Broadcast | null>(null);
	let sending = $state(false);

	const tabs = [
		{ value: "all", label: "All", icon: Radio },
		{ value: "draft", label: "Drafts", icon: Clock },
		{ value: "sent", label: "Sent", icon: CheckCircle }
	];

	const columns = [
		{
			key: "title",
			label: "Title",
			sortable: true,
			render: (_: any, row: any) => `
				<div>
					<p class="font-medium text-gray-900">${row.title}</p>
					<p class="text-sm text-gray-500 line-clamp-1">${row.body || ""}</p>
				</div>
			`
		},
		{
			key: "target_audience",
			label: "Audience",
			sortable: true,
			render: (value: string) => {
				const labels: Record<string, string> = { all: "All Users", active: "Active Users", premium: "Premium Users" };
				return `<span class="text-sm text-gray-700">${labels[value] || value}</span>`;
			}
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
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes[variant]}">${value}</span>`;
			}
		},
		{
			key: "recipient_count",
			label: "Recipients",
			sortable: true,
			render: (value: number) => `<span class="text-sm font-medium">${(value ?? 0).toLocaleString()}</span>`
		},
		{
			key: "sent_at",
			label: "Sent",
			sortable: true,
			render: (value: string) => value ? new Date(value).toLocaleDateString("en-US", { year: "numeric", month: "short", day: "numeric" }) : '<span class="text-gray-400">—</span>'
		},
		{
			key: "actions",
			label: "",
			class: "w-24",
			render: (_: any, row: any) => `
				<div class="flex items-center gap-1">
					<button onclick="window.__broadcastView('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100">${t("common.view")}</button>
					${row.status === "draft" ? `<button onclick="window.__broadcastSend('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-primary-700 hover:bg-primary-50">${t("common.send")}</button>` : ""}
				</div>
			`
		}
	];

	const audienceOptions = [
		{ value: 'all', label: 'All Users' },
		{ value: 'active', label: 'Active Users' },
		{ value: 'premium', label: 'Premium Users' }
	];

	let filteredBroadcasts = $derived.by(() => {
		let result = [...broadcasts];
		if (activeTab !== "all") {
			result = result.filter((b) => b.status === activeTab);
		}
		return result;
	});

	async function loadBroadcasts() {
		loading = true;
		try {
			const result = await api<Broadcast[]>("/admin/broadcasts");
			if (result.ok) {
				broadcasts = result.data ?? [];
			}
		} catch {
			// API not available
		} finally {
			loading = false;
		}
	}

	function handleFilter(values: Record<string, string>) {
		filters = values;
	}

	async function createBroadcast() {
		if (!newBroadcast.title.trim() || !newBroadcast.body.trim()) return;
		creating = true;
		try {
			const result = await api("/admin/broadcasts", {
				method: "POST",
				body: {
					title: newBroadcast.title.trim(),
					body: newBroadcast.body.trim(),
					target_audience: newBroadcast.target_audience
				}
			});
			if (result.ok) {
				showCreateModal = false;
				newBroadcast = { title: "", body: "", target_audience: "all" };
				loadBroadcasts();
			}
		} catch {
			// API not available
		} finally {
			creating = false;
		}
	}

	function confirmSend(broadcast: Broadcast) {
		sendTarget = broadcast;
		showSendModal = true;
	}

	async function sendBroadcast() {
		if (!sendTarget) return;
		sending = true;
		try {
			await api(`/admin/broadcasts/${sendTarget.id}/send`, { method: "POST", body: {} });
			showSendModal = false;
			sendTarget = null;
			loadBroadcasts();
		} catch {
			// API not available
		} finally {
			sending = false;
		}
	}

	// Expose window functions for DataTable inline actions
	$effect(() => {
		(window as any).__broadcastView = (id: string) => {
			goto(`/broadcasts/${id}`);
		};
		(window as any).__broadcastSend = (id: string) => {
			const bc = broadcasts.find((b) => b.id === id);
			if (bc) confirmSend(bc);
		};
	});

	let initialized = false;
	$effect(() => {
		if (!initialized) {
			initialized = true;
			loadBroadcasts();
		}
	});
</script>

<svelte:head>
	<title>Broadcasts — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title="Broadcasts"
	description="Send targeted messages to platform users"
	actions={[
		{ label: "New Broadcast", icon: Plus, onclick: () => (showCreateModal = true) }
	]}
/>

<div class="space-y-6">
	<Tabs {tabs} bind:value={activeTab} />

	<FilterBar
		bind:value={filters}
		filters={[
			{
				key: "audience",
				label: "Audience",
				options: [
					{ value: "all", label: "All Users" },
					{ value: "active", label: "Active Users" },
					{ value: "premium", label: "Premium Users" }
				]
			}
		]}
		onsearch={handleFilter}
		onclear={() => { filters = {}; }}
	/>

	<Card>
		<CardContent class="p-0">
			{#if loading}
				<p class="py-8 text-center text-sm text-gray-500">{t('broadcasts.loading_broadcasts')}</p>
			{:else}
				<DataTable
					columns={columns}
					data={filteredBroadcasts}
					pageSize={15}
					emptyMessage="No broadcasts found"
				/>
			{/if}
		</CardContent>
	</Card>
</div>

<!-- Create Broadcast Modal -->
<Modal bind:open={showCreateModal} title="New Broadcast" description="Create and send a broadcast message" size="lg">
	<div class="space-y-4">
		<div>
			<label for="bc-title" class="mb-1 block text-sm font-medium text-gray-900">{t('broadcasts.title_2')}</label>
			<input
				id="bc-title"
				type="text"
				class="h-10 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				placeholder="e.g. Platform Maintenance Notice"
				bind:value={newBroadcast.title}
			/>
		</div>
		<div>
			<label for="bc-body" class="mb-1 block text-sm font-medium text-gray-900">{t('broadcasts.message')}</label>
			<textarea
				id="bc-body"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				rows="5"
				placeholder="Write your broadcast message..."
				bind:value={newBroadcast.body}
			></textarea>
		</div>
		<div>
			<label for="bc-audience" class="mb-1 block text-sm font-medium text-gray-900">{t('broadcasts.target_audience')}</label>
			<Select options={audienceOptions} bind:value={newBroadcast.target_audience} />
		</div>
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => (showCreateModal = false)}>{t('broadcasts.cancel')}</Button>
			<Button onclick={createBroadcast} disabled={creating || !newBroadcast.title.trim() || !newBroadcast.body.trim()}>
				{#if creating}
					Creating...
				{:else}
					Create Broadcast
				{/if}
			</Button>
		</div>
	</div>
</Modal>

<!-- Send Confirmation Modal -->
<Modal bind:open={showSendModal} title="Send Broadcast" description="This message will be sent immediately to the target audience." size="sm">
	{#if sendTarget}
		<div class="rounded-lg bg-gray-50 p-4">
			<p class="text-sm font-medium text-gray-900">{sendTarget.title}</p>
			<p class="mt-1 text-sm text-gray-500">Audience: {sendTarget.target_audience === "all" ? "All Users" : sendTarget.target_audience === "active" ? "Active Users" : "Premium Users"}</p>
		</div>
	{/if}
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => (showSendModal = false)}>{t('broadcasts.cancel')}</Button>
		<Button onclick={sendBroadcast} disabled={sending}>
		{#if sending}
			<Send class="h-4 w-4 mr-2 animate-spin" />
			Sending...
		{:else}
				<Send class="h-4 w-4 mr-2" />
				Send Now
			{/if}
		</Button>
	</div>
</Modal>
