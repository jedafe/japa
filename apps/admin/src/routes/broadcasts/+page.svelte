<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { page } from "$app/state";
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { Tabs } from "$lib/components/ui/tabs";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Badge } from "$lib/components/ui/badge";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { Send, Plus, Radio, Clock, CheckCircle, Eye } from "lucide-svelte";
	import { Select } from '$lib/components/ui/select';
	import { api } from "$lib/api";
	import { hasPermission } from '$lib/stores/permissions.svelte';

	const canManage = $derived(hasPermission('admin.broadcast'));

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

	// View modal
	let showViewModal = $state(false);
	let viewTarget = $state<Broadcast | null>(null);

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
					<p class="font-medium text-foreground">${row.title}</p>
					<p class="text-sm text-muted line-clamp-1">${row.body || ""}</p>
				</div>
			`
		},
		{
			key: "target_audience",
			label: "Audience",
			sortable: true,
			render: (value: string) => {
				const labels: Record<string, string> = { all: "All Users", active: "Active Users", premium: "Premium Users" };
				return `<span class="text-sm text-foreground-alt">${labels[value] || value}</span>`;
			}
		},
		{
			key: "status",
			label: "Status",
			sortable: true,
			render: (value: string) => {
				const variant = value === "sent" ? "success" : value === "scheduled" ? "info" : value === "failed" ? "destructive" : "secondary";
				const classes: Record<string, string> = {
					success: "bg-success/15 text-success border-success",
					info: "bg-info-100 text-info-800 border-info-200",
					destructive: "bg-danger/15 text-danger border-danger",
					secondary: "bg-surface text-foreground border"
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
			render: (value: string) => value ? new Date(value).toLocaleDateString("en-US", { year: "numeric", month: "short", day: "numeric" }) : '<span class="text-muted">—</span>'
		},
		{
			key: "actions",
			label: "",
			class: "w-24",
			component: BroadcastActionsCell
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
		// Apply the FilterBar audience filter (previously a no-op).
		if (filters.audience) {
			result = result.filter((b) => b.target_audience === filters.audience);
		}
		// Apply the FilterBar free-text search (previously a no-op).
		if (filters.search) {
			const q = filters.search.toLowerCase();
			result = result.filter(
				(b) => b.title.toLowerCase().includes(q) || (b.body ?? "").toLowerCase().includes(q)
			);
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

	function openView(broadcast: Broadcast) {
		viewTarget = broadcast;
		showViewModal = true;
	}

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
		...(canManage ? [{ label: "New Broadcast", icon: Plus, onclick: () => (showCreateModal = true) }] : [])
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
				<p class="py-8 text-center text-sm text-muted">{t('broadcasts.loading_broadcasts')}</p>
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
			<label for="bc-title" class="mb-1 block text-sm font-medium text-foreground">{t('broadcasts.title_2')}</label>
			<input
				id="bc-title"
				type="text"
				class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				placeholder="e.g. Platform Maintenance Notice"
				bind:value={newBroadcast.title}
			/>
		</div>
		<div>
			<label for="bc-body" class="mb-1 block text-sm font-medium text-foreground">{t('broadcasts.message')}</label>
			<textarea
				id="bc-body"
				class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				rows="5"
				placeholder="Write your broadcast message..."
				bind:value={newBroadcast.body}
			></textarea>
		</div>
		<div>
			<label for="bc-audience" class="mb-1 block text-sm font-medium text-foreground">{t('broadcasts.target_audience')}</label>
			<Select options={audienceOptions} bind:value={newBroadcast.target_audience} />
		</div>
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => (showCreateModal = false)}>{t('broadcasts.cancel')}</Button>
			{#if canManage}<Button onclick={createBroadcast} disabled={creating || !newBroadcast.title.trim() || !newBroadcast.body.trim()}>
				{#if creating}
					Creating...
				{:else}
					Create Broadcast
				{/if}
			</Button>{/if}
		</div>
	</div>
</Modal>

<!-- Send Confirmation Modal -->
<Modal bind:open={showSendModal} title="Send Broadcast" description="This message will be sent immediately to the target audience." size="sm">
	{#if sendTarget}
		<div class="rounded-lg bg-surface p-4">
			<p class="text-sm font-medium text-foreground">{sendTarget.title}</p>
			<p class="mt-1 text-sm text-muted">Audience: {sendTarget.target_audience === "all" ? "All Users" : sendTarget.target_audience === "active" ? "Active Users" : "Premium Users"}</p>
		</div>
	{/if}
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => (showSendModal = false)}>{t('broadcasts.cancel')}</Button>
		{#if canManage}<Button onclick={sendBroadcast} disabled={sending}>
		{#if sending}
			<Send class="h-4 w-4 mr-2 animate-spin" />
			Sending...
		{:else}
				<Send class="h-4 w-4 mr-2" />
				Send Now
			{/if}
		</Button>{/if}
	</div>
</Modal>

<!-- View Broadcast Modal -->
<Modal bind:open={showViewModal} title="Broadcast Details" size="lg">
	{#if viewTarget}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('broadcasts.title_2', 'Title')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.title}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('broadcasts.message', 'Message')}</p>
				<p class="mt-1 whitespace-pre-wrap text-sm" style="color: var(--color-foreground)">{viewTarget.body}</p>
			</div>
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('broadcasts.target_audience', 'Audience')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.target_audience}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.status', 'Status')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.status}</p>
				</div>
			</div>
		</div>
		<div class="mt-6 flex justify-end gap-2">
			{#if canManage && viewTarget.status === 'draft'}
				{@const bt = viewTarget}
				<Button onclick={() => { showViewModal = false; confirmSend(bt); }}>
					<Send class="mr-1 h-3.5 w-3.5" /> {t('common.send', 'Send')}
				</Button>
			{/if}
			<Button variant="outline" onclick={() => (showViewModal = false)}>{t('common.close', 'Close')}</Button>
		</div>
	{/if}
</Modal>

{#snippet BroadcastActionsCell(row: Broadcast)}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView(row)}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canManage && row.status === 'draft'}
			<Button variant="ghost" size="sm" onclick={() => confirmSend(row)} style="color: var(--color-primary)">
				<Send class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
