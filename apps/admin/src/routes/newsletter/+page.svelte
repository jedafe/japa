<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { page } from "$app/state";
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { Tabs } from "$lib/components/ui/tabs";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Badge } from "$lib/components/ui/badge";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { Select } from "$lib/components/ui/select";
	import { Mail, Plus, Send, Users, BarChart3, Trash2, Eye, Pencil } from "lucide-svelte";
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

	// RBAC: all newsletter mutations require the newsletter.admin node
	const canManageNewsletter = $derived(hasPermission('newsletter.admin'));

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

	// Bulk delete state
	let selectedSubscriberIds = $state<Set<string>>(new Set());
	let selectedCampaignIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	// Subscriber view/edit + campaign view modals
	let showSubscriberModal = $state(false);
	let subscriberTarget = $state<Subscriber | null>(null);
	let editSubscriberStatus = $state("");
	let showCampaignModal = $state(false);
	let campaignTarget = $state<Campaign | null>(null);

	const tabs = [
		{ value: "subscribers", label: t('app.newsletter.tab_subscribers', 'Subscribers'), icon: Users, count: stats.subscribers },
		{ value: "campaigns", label: t('app.newsletter.tab_campaigns', 'Campaigns'), icon: Mail, count: stats.campaigns },
		{ value: "analytics", label: t('app.newsletter.tab_analytics', 'Analytics'), icon: BarChart3 }
	];

	const subscriberColumns = [
		{
			key: "email",
			label: t('app.newsletter.col_email', 'Email'),
			sortable: true,
			render: (_: any, row: any) => `
				<div class="flex items-center gap-3">
					<div class="h-8 w-8 rounded-full bg-primary-100 flex items-center justify-center">
						<span class="text-sm font-medium text-primary-700">${(row.email || '?')[0].toUpperCase()}</span>
					</div>
					<div>
						<p class="font-medium text-foreground">${row.email}</p>
						${row.name ? `<p class="text-sm text-muted">${row.name}</p>` : ''}
					</div>
				</div>
			`
		},
		{
			key: "status",
			label: t('app.newsletter.col_status', 'Status'),
			sortable: true,
			render: (value: string) => {
				const variant = value === "active" ? "success" : value === "bounced" ? "destructive" : "secondary";
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${variant === "success" ? "bg-success/15 text-success border-success" : variant === "destructive" ? "bg-danger/15 text-danger border-danger" : "bg-surface text-foreground border"}">${value || "active"}</span>`;
			}
		},
		{
			key: "subscribedAt",
			label: t('app.newsletter.col_subscribed', 'Subscribed'),
			sortable: true,
			render: (value: string, row: any) => new Date(value || row.created_at).toLocaleDateString("en-US", { year: "numeric", month: "short", day: "numeric" })
		},
		{
			key: "actions",
			label: "",
			class: "w-24",
			component: SubscriberActionsCell
		}
	];

	const campaignColumns = [
		{
			key: "subject",
			label: t('app.newsletter.col_subject', 'Subject'),
			sortable: true,
			render: (_: any, row: any) => `
				<div>
					<p class="font-medium text-foreground">${row.subject || row.name || "No subject"}</p>
					${row.preview ? `<p class="text-sm text-muted">${row.preview}</p>` : ""}
				</div>
			`
		},
		{
			key: "status",
			label: t('app.newsletter.col_status', 'Status'),
			sortable: true,
			render: (value: string) => {
				const variant = value === "sent" ? "success" : value === "scheduled" ? "info" : value === "failed" ? "destructive" : "secondary";
				const classes: Record<string, string> = {
					success: "bg-success/15 text-success border-success",
					info: "bg-info-100 text-info-800 border-info-200",
					destructive: "bg-danger/15 text-danger border-danger",
					secondary: "bg-surface text-foreground border"
				};
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes[variant]}">${value || "draft"}</span>`;
			}
		},
		{
			key: "sentAt",
			label: t('app.newsletter.col_sent', 'Sent'),
			sortable: true,
			render: (value: string, row: any) => {
				const date = value || row.sent_at;
				return date ? new Date(date).toLocaleDateString("en-US", { year: "numeric", month: "short", day: "numeric" }) : '<span class="text-muted">—</span>';
			}
		},
		{
			key: "openRate",
			label: t('app.newsletter.col_open_rate', 'Open Rate'),
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
			component: CampaignActionsCell
		}
	];

	async function loadSubscribers() {
		loading = true;
		try {
			// Pass the FilterBar search/status through to the subscribers query.
			const params = new URLSearchParams();
			if (filters.search) params.set("search", filters.search);
			if (filters.status) params.set("status", filters.status);
			const query = params.toString();
			const result = await api<Subscriber[]>(`/admin/newsletter/subscribers${query ? `?${query}` : ""}`);
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
		// Previously a no-op — now refetch subscribers with the applied filters.
		loadSubscribers();
	}

	async function createCampaign() {
		if (!newCampaign.name || !newCampaign.subject) return;
		creating = true;
		const sanitizedHtml = newCampaign.html.replace(/<script[\s\S]*?<\/script>/gi, '')
			.replace(/<iframe[\s\S]*?<\/iframe>/gi, '')
			.replace(/\bon\w+\s*=\s*"[^"]*"/gi, '')
			.replace(/\bon\w+\s*=\s*'[^']*'/gi, '');
		try {
			const result = await api("/admin/newsletter/campaigns", {
				method: "POST",
				body: { ...newCampaign, html: sanitizedHtml }
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

	function openEditSubscriber(sub: Subscriber) {
		subscriberTarget = sub;
		editSubscriberStatus = sub.status;
		showSubscriberModal = true;
	}

	async function saveSubscriber() {
		if (!subscriberTarget) return;
		deleting = true;
		try {
			await api(`/admin/newsletter/subscribers/${subscriberTarget.id}`, { method: "PATCH", body: { status: editSubscriberStatus } });
			showSubscriberModal = false;
			loadSubscribers();
		} catch {
			// API not available
		} finally {
			deleting = false;
		}
	}

	function openViewCampaign(c: Campaign) {
		campaignTarget = c;
		showCampaignModal = true;
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

	function confirmDeleteSubscriber(subscriber: Subscriber) {
		deleteTarget = subscriber;
		showDeleteModal = true;
	}

	async function bulkDeleteCampaigns() {
		if (selectedCampaignIds.size === 0) return;
		bulkDeleting = true;
		try {
			for (const id of selectedCampaignIds) {
				await api(`/admin/newsletter/campaigns/${id}`, { method: 'DELETE' });
			}
			selectedCampaignIds = new Set();
			loadCampaigns();
		} catch {
			// API not available
		} finally {
			bulkDeleting = false;
		}
	}

	let initialized = false;
	$effect(() => {
		if (!initialized) {
			initialized = true;
			loadSubscribers();
		}
	});
</script>

<svelte:head>
	<title>{t('app.newsletter.page_title', 'Newsletter')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('app.newsletter.title', 'Newsletter')}
	description={t('app.newsletter.description', 'Manage email subscribers and campaigns')}
	actions={canManageNewsletter ? [
		{ label: t('app.newsletter.btn_new_campaign', 'New Campaign'), icon: Plus, onclick: () => (showCreateModal = true) },
		{ label: t('app.newsletter.btn_import_subscribers', 'Import Subscribers'), icon: Users, variant: "outline" }
	] : []}
/>

<div class="space-y-6">
	<Tabs {tabs} bind:value={activeTab} />

	{#if activeTab === "subscribers"}
		<FilterBar
			bind:value={filters}
			filters={[
				{
					key: "status",
					label: t('app.newsletter.filter_status', 'Status'),
					options: [
						{ value: "active", label: t('app.newsletter.status_active', 'Active') },
						{ value: "unsubscribed", label: t('app.newsletter.status_unsubscribed', 'Unsubscribed') },
						{ value: "bounced", label: t('app.newsletter.status_bounced', 'Bounced') }
					]
				}
			]}
			onsearch={handleFilter}
			onclear={() => { filters = {}; }}
		/>

		<Card>
			<CardContent class="p-0">
				{#if loading}
					<p class="py-8 text-center text-sm text-muted">{t('newsletter.loading_subscribers')}</p>
				{:else}
					<DataTable
						columns={subscriberColumns}
						data={subscribers}
						pageSize={20}
						selectable={canManageNewsletter}
						bind:selectedIds={selectedSubscriberIds}
						emptyMessage={t('app.newsletter.empty_subscribers', 'No subscribers yet')}
					/>
				{/if}
			</CardContent>
		</Card>
	{/if}

	{#if activeTab === "campaigns"}
		<Card>
			<CardContent class="p-0">
				{#if loading}
					<p class="py-8 text-center text-sm text-muted">{t('newsletter.loading_campaigns')}</p>
				{:else}
					<DataTable
						columns={campaignColumns}
						data={campaigns}
						pageSize={10}
						selectable={canManageNewsletter}
						bind:selectedIds={selectedCampaignIds}
						emptyMessage={t('app.newsletter.empty_campaigns', 'No campaigns yet')}
					>
						{#snippet bulkActions()}
							{#if selectedCampaignIds.size > 0 && canManageNewsletter}
								<Button variant="destructive" size="sm" onclick={bulkDeleteCampaigns} disabled={bulkDeleting}>
									<Trash2 class="mr-1 h-3.5 w-3.5" />
									{t('app.newsletter.btn_delete_selected', 'Delete Selected')} ({selectedCampaignIds.size})
								</Button>
							{/if}
						{/snippet}
					</DataTable>
				{/if}
			</CardContent>
		</Card>
	{/if}

	{#if activeTab === "analytics"}
		<div class="grid grid-cols-1 gap-6 md:grid-cols-3">
			<Card>
				<CardContent class="p-6">
					<p class="text-sm font-medium text-muted">{t('newsletter.total_subscribers')}</p>
					<p class="text-2xl font-bold text-foreground">{stats.subscribers}</p>
				</CardContent>
			</Card>
			<Card>
				<CardContent class="p-6">
					<p class="text-sm font-medium text-muted">{t('newsletter.avg_open_rate')}</p>
					<p class="text-2xl font-bold text-foreground">{stats.avgOpenRate}%</p>
				</CardContent>
			</Card>
			<Card>
				<CardContent class="p-6">
					<p class="text-sm font-medium text-muted">{t('newsletter.avg_click_rate')}</p>
					<p class="text-2xl font-bold text-foreground">{stats.avgClickRate}%</p>
				</CardContent>
			</Card>
		</div>

		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold text-foreground">{t('newsletter.campaign_performance')}</h3>
			</CardHeader>
			<CardContent>
				<div class="flex h-64 items-center justify-center rounded-lg bg-surface">
					<p class="text-muted">{t('newsletter.analytics_chart_placeholder_integrate_wi')}</p>
				</div>
			</CardContent>
		</Card>
	{/if}
</div>

<!-- Create Campaign Modal -->
<Modal bind:open={showCreateModal} title={t('app.newsletter.modal_create_campaign', 'Create Campaign')} description={t('app.newsletter.modal_create_campaign_desc', 'Compose a new newsletter campaign')} size="lg">
	<div class="space-y-4">
		<div>
			<label for="campaign-name" class="mb-1 block text-sm font-medium text-foreground">{t('newsletter.campaign_name')}</label>
			<input
				id="campaign-name"
				type="text"
				class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				placeholder={t('app.newsletter.placeholder_campaign_name', 'e.g. Weekly Digest #42')}
				bind:value={newCampaign.name}
			/>
		</div>
		<div>
			<label for="campaign-subject" class="mb-1 block text-sm font-medium text-foreground">{t('newsletter.subject_line')}</label>
			<input
				id="campaign-subject"
				type="text"
				class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				placeholder={t('app.newsletter.placeholder_subject', 'What subscribers will see in their inbox')}
				bind:value={newCampaign.subject}
			/>
		</div>
		<div>
			<label for="campaign-html" class="mb-1 block text-sm font-medium text-foreground">{t('newsletter.content_html')}</label>
			<textarea
				id="campaign-html"
				class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				rows="8"
				placeholder="<h1>{t('newsletter.hello')}</h1><p>{t('newsletter.your_newsletter_content_here')}</p>"
				bind:value={newCampaign.html}
			></textarea>
		</div>
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => (showCreateModal = false)}>{t('newsletter.cancel')}</Button>
			<Button onclick={createCampaign} disabled={creating || !newCampaign.name || !newCampaign.subject}>
				{#if creating}
					{t('app.newsletter.creating', 'Creating...')}
				{:else}
					{t('app.newsletter.btn_create_campaign', 'Create Campaign')}
				{/if}
			</Button>
		</div>
	</div>
</Modal>

<!-- Delete Subscriber Confirmation Modal -->
<Modal bind:open={showDeleteModal} title={t('app.newsletter.modal_remove_subscriber', 'Remove Subscriber')} description={t('app.newsletter.modal_remove_subscriber_desc', 'This subscriber will be removed from your mailing list.')} size="sm">
	{#if deleteTarget}
		<div class="rounded-lg bg-surface p-4">
			<p class="text-sm text-foreground-alt">
				{t('app.newsletter.confirm_remove_prefix', 'Are you sure you want to remove')} <strong>{deleteTarget.email}</strong>?
			</p>
		</div>
	{/if}
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => (showDeleteModal = false)}>{t('newsletter.cancel')}</Button>
		<Button variant="destructive" onclick={deleteSubscriber} disabled={deleting}>
			{#if deleting}
				{t('app.newsletter.removing', 'Removing...')}
			{:else}
				{t('app.newsletter.btn_remove', 'Remove')}
			{/if}
		</Button>
	</div>
</Modal>

<!-- Edit Subscriber Modal -->
<Modal bind:open={showSubscriberModal} title={t('app.newsletter.modal_edit_subscriber', 'Edit Subscriber')} size="sm">
	{#if subscriberTarget}
		<div class="space-y-4">
			<div>
				<p class="text-sm" style="color: var(--color-muted)">{subscriberTarget.email}</p>
			</div>
			<div>
				<label for="sub-status" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('common.status', 'Status')}</label>
				<Select
					options={[
						{ value: "active", label: t('app.newsletter.status_active', 'Active') },
						{ value: "unsubscribed", label: t('app.newsletter.status_unsubscribed', 'Unsubscribed') },
						{ value: "bounced", label: t('app.newsletter.status_bounced', 'Bounced') }
					]}
					value={editSubscriberStatus}
					onchange={(val) => (editSubscriberStatus = val)}
				/>
			</div>
		</div>
		<div class="mt-4 flex justify-end gap-2">
			<Button variant="ghost" onclick={() => (showSubscriberModal = false)}>{t('common.cancel', 'Cancel')}</Button>
			<Button onclick={saveSubscriber} disabled={deleting}>{t('common.save', 'Save')}</Button>
		</div>
	{/if}
</Modal>

<!-- View Campaign Modal -->
<Modal bind:open={showCampaignModal} title={t('app.newsletter.modal_campaign_details', 'Campaign Details')} size="lg">
	{#if campaignTarget}
		<div class="space-y-4">
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('newsletter.subject_line', 'Subject')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{campaignTarget.subject || campaignTarget.name}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.status', 'Status')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{campaignTarget.status}</p>
				</div>
			</div>
			{#if campaignTarget.preview}
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('newsletter.content_html', 'Preview')}</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">{campaignTarget.preview}</p>
				</div>
			{/if}
			{#if campaignTarget.recipient_count != null}
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('newsletter.recipients', 'Recipients')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{campaignTarget.recipient_count.toLocaleString()}</p>
				</div>
			{/if}
		</div>
		<div class="mt-6 flex justify-end gap-2">
			{#if campaignTarget.status === 'draft' && canManageNewsletter}
				{@const ct = campaignTarget}
				<Button onclick={() => { showCampaignModal = false; sendCampaign(ct.id); }}>
					<Send class="mr-1 h-3.5 w-3.5" /> {t('common.send', 'Send')}
				</Button>
			{/if}
			<Button variant="outline" onclick={() => (showCampaignModal = false)}>{t('common.close', 'Close')}</Button>
		</div>
	{/if}
</Modal>

{#snippet SubscriberActionsCell({ value, row }: { value: any; row: Subscriber })}
	<div class="flex items-center gap-1">
		{#if canManageNewsletter}
		<Button variant="ghost" size="sm" onclick={() => openEditSubscriber(row)}>
			<Pencil class="h-3.5 w-3.5" />
		</Button>
		<Button variant="ghost" size="sm" onclick={() => confirmDeleteSubscriber(row)} style="color: var(--color-danger)">
			<Trash2 class="h-3.5 w-3.5" />
		</Button>
		{/if}
	</div>
{/snippet}

{#snippet CampaignActionsCell({ value, row }: { value: any; row: Campaign })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openViewCampaign(row)}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if row.status === 'draft' && canManageNewsletter}
			<Button variant="ghost" size="sm" onclick={() => sendCampaign(row.id)} style="color: var(--color-primary)">
				<Send class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
