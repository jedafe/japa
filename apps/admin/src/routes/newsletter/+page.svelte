<script lang="ts">
	// /newsletter — migrated to shared components (ActionMenu, FilterPanel,
	// SlideOver, bulkActionsList). Tabbed page (subscribers + campaigns +
	// analytics); keeps page-local Modals for create campaign and import
	// subscribers (forms with rich inputs). View campaign and edit subscriber
	// open in SlideOver panels.
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import { Select } from '$lib/components/ui/select';
	import type { ActionItem, BulkAction } from '$lib/components/ui/data-table';
	import { Mail, Plus, Send, Users, BarChart3, Trash2, Eye, Pencil, Upload } from 'lucide-svelte';
	import { api } from '$lib/api';

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

	const canManageNewsletter = $derived(hasPermission('newsletter.admin'));

	let activeTab = $state('subscribers');
	let subscribers = $state<Subscriber[]>([]);
	let campaigns = $state<Campaign[]>([]);
	let stats = $state<NewsletterStats>({ subscribers: 0, campaigns: 0, avgOpenRate: 0, avgClickRate: 0 });
	let loading = $state(true);
	let filterValue = $state<Record<string, string>>({});
	let searchQuery = $state('');

	// Campaign creation Modal (page-local — form with HTML textarea)
	let showCreateModal = $state(false);
	let newCampaign = $state({ name: '', subject: '', html: '' });
	let creating = $state(false);

	// Delete confirmation Modal
	let showDeleteModal = $state(false);
	let deleteTarget = $state<Subscriber | null>(null);
	let deleting = $state(false);

	// Bulk delete
	let selectedSubscriberIds = $state<Set<string>>(new Set());
	let selectedCampaignIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	// Edit subscriber SlideOver
	let showEditSlideOver = $state(false);
	let subscriberTarget = $state<Subscriber | null>(null);
	let editSubscriberStatus = $state('');

	// View campaign SlideOver
	let showCampaignSlideOver = $state(false);
	let campaignTarget = $state<Campaign | null>(null);

	// Import subscribers Modal (page-local — form with textarea)
	let showImportModal = $state(false);
	let importText = $state('');
	let importing = $state(false);
	let importResult = $state<{ success: number; failed: number; errors: string[] } | null>(null);

	const tabs = [
		{ value: 'subscribers', label: t('app.newsletter.tab_subscribers', 'Subscribers'), icon: Users, count: stats.subscribers },
		{ value: 'campaigns', label: t('app.newsletter.tab_campaigns', 'Campaigns'), icon: Mail, count: stats.campaigns },
		{ value: 'analytics', label: t('app.newsletter.tab_analytics', 'Analytics'), icon: BarChart3 }
	];

	const subscriberStatusOptions = [
		{ value: 'active', label: t('app.newsletter.status_active', 'Active') },
		{ value: 'unsubscribed', label: t('app.newsletter.status_unsubscribed', 'Unsubscribed') },
		{ value: 'bounced', label: t('app.newsletter.status_bounced', 'Bounced') }
	];

	const subscriberColumns = [
		{
			key: 'email',
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
			key: 'status',
			label: t('app.newsletter.col_status', 'Status'),
			sortable: true,
			render: (v: string) => {
				const cls = v === 'active' ? 'bg-success/15 text-success border-success' : v === 'bounced' ? 'bg-danger/15 text-danger border-danger' : 'bg-surface text-foreground border';
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${v || 'active'}</span>`;
			}
		},
		{
			key: 'subscribedAt',
			label: t('app.newsletter.col_subscribed', 'Subscribed'),
			sortable: true,
			render: (v: string, row: any) => new Date(v || row.created_at).toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' })
		}
	];

	const campaignColumns = [
		{
			key: 'subject',
			label: t('app.newsletter.col_subject', 'Subject'),
			sortable: true,
			render: (_: any, row: any) => `
				<div>
					<p class="font-medium text-foreground">${row.subject || row.name || 'No subject'}</p>
					${row.preview ? `<p class="text-sm text-muted">${row.preview}</p>` : ''}
				</div>
			`
		},
		{
			key: 'status',
			label: t('app.newsletter.col_status', 'Status'),
			sortable: true,
			render: (v: string) => {
				const cls = v === 'sent' ? 'bg-success/15 text-success border-success' : v === 'scheduled' ? 'bg-info-100 text-info-800 border-info-200' : v === 'failed' ? 'bg-danger/15 text-danger border-danger' : 'bg-surface text-foreground border';
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${v || 'draft'}</span>`;
			}
		},
		{
			key: 'sentAt',
			label: t('app.newsletter.col_sent', 'Sent'),
			sortable: true,
			render: (v: string, row: any) => {
				const date = v || row.sent_at;
				return date ? new Date(date).toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' }) : '<span class="text-muted">—</span>';
			}
		},
		{
			key: 'openRate',
			label: t('app.newsletter.col_open_rate', 'Open Rate'),
			sortable: true,
			render: (v: number, row: any) => {
				const rate = v ?? row.open_rate ?? 0;
				return `<span class="font-medium">${rate.toFixed(1)}%</span>`;
			}
		}
	];

	// ── Row actions via ActionMenu ──────────────────────────────────────────
	const subscriberActions: ActionItem[] = [
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row) => openEditSubscriber(row),
			show: () => canManageNewsletter
		},
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row) => confirmDeleteSubscriber(row),
			show: () => canManageNewsletter
		}
	];

	const campaignActions: ActionItem[] = [
		{ label: t('common.view', 'View'), icon: Eye, onclick: (row) => openViewCampaign(row) },
		{
			label: t('common.send', 'Send'),
			icon: Send,
			onclick: (row) => sendCampaign(row.id),
			show: (row: Campaign) => row.status === 'draft' && canManageNewsletter
		}
	];

	// ── Bulk actions via BulkBar ────────────────────────────────────────────
	const campaignBulkActions: BulkAction[] = [
		{ label: t('app.newsletter.btn_delete_selected', 'Delete Selected'), icon: Trash2, variant: 'destructive', onclick: bulkDeleteCampaigns }
	];

	async function loadSubscribers() {
		loading = true;
		try {
			const params = new URLSearchParams();
			if (searchQuery) params.set('search', searchQuery);
			if (filterValue.status) params.set('status', filterValue.status);
			const query = params.toString();
			const result = await api<Subscriber[]>(`/admin/newsletter/subscribers${query ? `?${query}` : ''}`);
			if (result.ok) {
				subscribers = result.data ?? [];
				stats.subscribers = subscribers.length;
			}
		} catch { /* API not available */ } finally {
			loading = false;
		}
	}

	async function loadCampaigns() {
		loading = true;
		try {
			const result = await api<Campaign[]>('/admin/newsletter/campaigns');
			if (result.ok) {
				campaigns = result.data ?? [];
				stats.campaigns = campaigns.length;
			}
		} catch { /* API not available */ } finally {
			loading = false;
		}
	}

	async function createCampaign() {
		if (!newCampaign.name || !newCampaign.subject) return;
		creating = true;
		const sanitizedHtml = newCampaign.html
			.replace(/<script[\s\S]*?<\/script>/gi, '')
			.replace(/<iframe[\s\S]*?<\/iframe>/gi, '')
			.replace(/\bon\w+\s*=\s*"[^"]*"/gi, '')
			.replace(/\bon\w+\s*=\s*'[^']*'/gi, '');
		try {
			const result = await api('/admin/newsletter/campaigns', { method: 'POST', body: { ...newCampaign, html: sanitizedHtml } });
			if (result.ok) {
				showCreateModal = false;
				newCampaign = { name: '', subject: '', html: '' };
				loadCampaigns();
			}
		} catch { /* API not available */ } finally {
			creating = false;
		}
	}

	async function sendCampaign(id: string) {
		try {
			await api(`/admin/newsletter/campaigns/${id}/send`, { method: 'POST' });
			loadCampaigns();
		} catch { /* API not available */ }
	}

	function openEditSubscriber(sub: Subscriber) {
		subscriberTarget = sub;
		editSubscriberStatus = sub.status;
		showEditSlideOver = true;
	}

	async function saveSubscriber() {
		if (!subscriberTarget) return;
		deleting = true;
		try {
			await api(`/admin/newsletter/subscribers/${subscriberTarget.id}`, { method: 'PATCH', body: { status: editSubscriberStatus } });
			showEditSlideOver = false;
			loadSubscribers();
		} catch { /* API not available */ } finally {
			deleting = false;
		}
	}

	function openViewCampaign(c: Campaign) {
		campaignTarget = c;
		showCampaignSlideOver = true;
	}

	async function deleteSubscriber() {
		if (!deleteTarget) return;
		deleting = true;
		try {
			await api(`/admin/newsletter/subscribers/${deleteTarget.id}`, { method: 'DELETE' });
			showDeleteModal = false;
			deleteTarget = null;
			loadSubscribers();
		} catch { /* API not available */ } finally {
			deleting = false;
		}
	}

	function confirmDeleteSubscriber(subscriber: Subscriber) {
		deleteTarget = subscriber;
		showDeleteModal = true;
	}

	async function bulkDeleteCampaigns(ids: Set<string>) {
		if (ids.size === 0) return;
		if (bulkDeleting) return; // in-flight guard
		bulkDeleting = true;
		try {
			await Promise.all(Array.from(ids).map((id) => api(`/admin/newsletter/campaigns/${id}`, { method: 'DELETE' })));
		} catch { /* API not available */ } finally {
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

	// Reload when tab changes
	$effect(() => {
		void activeTab;
		if (initialized) {
			if (activeTab === 'subscribers') loadSubscribers();
			else if (activeTab === 'campaigns') loadCampaigns();
		}
	});

	function openImportModal() {
		importText = '';
		importResult = null;
		showImportModal = true;
	}

	async function importSubscribers() {
		const lines = importText.split('\n').map((l) => l.trim()).filter((l) => l && l.includes('@'));
		if (lines.length === 0) return;
		importing = true;
		importResult = { success: 0, failed: 0, errors: [] };
		for (const line of lines) {
			const [email, name] = line.split(',').map((s) => s.trim());
			try {
				const result = await api('/admin/newsletter/subscribers', { method: 'POST', body: { email, ...(name ? { name } : {}) } });
				if (result.ok) importResult.success++;
				else { importResult.failed++; importResult.errors.push(`${email}: ${result.error ?? 'failed'}`); }
			} catch (e) {
				importResult.failed++;
				importResult.errors.push(`${email}: ${e instanceof Error ? e.message : 'failed'}`);
			}
		}
		importing = false;
		if (importResult.success > 0) loadSubscribers();
	}
</script>

<svelte:head>
	<title>{t('app.newsletter.page_title', 'Newsletter')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('app.newsletter.title', 'Newsletter')}
	description={t('app.newsletter.description', 'Manage email subscribers and campaigns')}
	actions={canManageNewsletter ? [
		{ label: t('app.newsletter.btn_new_campaign', 'New Campaign'), icon: Plus, onclick: () => (showCreateModal = true) },
		{ label: t('app.newsletter.btn_import_subscribers', 'Import Subscribers'), icon: Upload, variant: 'outline', onclick: openImportModal }
	] : []}
/>

<div class="space-y-6">
	<Tabs {tabs} bind:value={activeTab} />

	{#if activeTab === 'subscribers'}
		<Card>
			<CardContent class="p-0">
				<DataTable
					columns={subscriberColumns}
					data={subscribers}
					pageSize={20}
					{loading}
					selectable={canManageNewsletter}
					bind:selectedIds={selectedSubscriberIds}
					emptyMessage={t('app.newsletter.empty_subscribers', 'No subscribers yet')}
					searchable
					onsearch={(q) => { searchQuery = q; loadSubscribers(); }}
					filters={[{ key: 'status', label: t('app.newsletter.filter_status', 'Status'), options: subscriberStatusOptions }]}
					bind:filterValue
					onfilterapply={loadSubscribers}
					actions={subscriberActions}
				/>
			</CardContent>
		</Card>
	{/if}

	{#if activeTab === 'campaigns'}
		<Card>
			<CardContent class="p-0">
				<DataTable
					columns={campaignColumns}
					data={campaigns}
					pageSize={10}
					{loading}
					selectable={canManageNewsletter}
					bind:selectedIds={selectedCampaignIds}
					emptyMessage={t('app.newsletter.empty_campaigns', 'No campaigns yet')}
					actions={campaignActions}
					bulkActionsList={canManageNewsletter ? campaignBulkActions : undefined}
				/>
			</CardContent>
		</Card>
	{/if}

	{#if activeTab === 'analytics'}
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

<!-- Create Campaign Modal (page-local — form with HTML textarea) -->
<Modal bind:open={showCreateModal} title={t('app.newsletter.modal_create_campaign', 'Create Campaign')} description={t('app.newsletter.modal_create_campaign_desc', 'Compose a new newsletter campaign')} size="lg">
	<div class="space-y-4">
		<div>
			<label for="campaign-name" class="mb-1 block text-sm font-medium text-foreground">{t('newsletter.campaign_name')}</label>
			<input id="campaign-name" type="text" class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" placeholder={t('app.newsletter.placeholder_campaign_name', 'e.g. Weekly Digest #42')} bind:value={newCampaign.name} />
		</div>
		<div>
			<label for="campaign-subject" class="mb-1 block text-sm font-medium text-foreground">{t('newsletter.subject_line')}</label>
			<input id="campaign-subject" type="text" class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" placeholder={t('app.newsletter.placeholder_subject', 'What subscribers will see in their inbox')} bind:value={newCampaign.subject} />
		</div>
		<div>
			<label for="campaign-html" class="mb-1 block text-sm font-medium text-foreground">{t('newsletter.content_html')}</label>
			<textarea id="campaign-html" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" rows="8" placeholder="<h1>Hello</h1><p>Your newsletter content here</p>" bind:value={newCampaign.html}></textarea>
		</div>
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => (showCreateModal = false)}>{t('newsletter.cancel')}</Button>
			<Button onclick={createCampaign} disabled={creating || !newCampaign.name || !newCampaign.subject}>
				{creating ? t('app.newsletter.creating', 'Creating...') : t('app.newsletter.btn_create_campaign', 'Create Campaign')}
			</Button>
		</div>
	</div>
</Modal>

<!-- Delete Subscriber Confirmation Modal -->
<Modal bind:open={showDeleteModal} title={t('app.newsletter.modal_remove_subscriber', 'Remove Subscriber')} description={t('app.newsletter.modal_remove_subscriber_desc', 'This subscriber will be removed from your mailing list.')} size="sm">
	{#if deleteTarget}
		<div class="rounded-lg bg-surface p-4">
			<p class="text-sm text-foreground-alt">{t('app.newsletter.confirm_remove_prefix', 'Are you sure you want to remove')} <strong>{deleteTarget.email}</strong>?</p>
		</div>
	{/if}
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => (showDeleteModal = false)}>{t('newsletter.cancel')}</Button>
		<Button variant="destructive" onclick={deleteSubscriber} disabled={deleting}>
			{deleting ? t('app.newsletter.removing', 'Removing...') : t('app.newsletter.btn_remove', 'Remove')}
		</Button>
	</div>
</Modal>

<!-- Edit Subscriber SlideOver -->
<SlideOver bind:open={showEditSlideOver} title={t('app.newsletter.modal_edit_subscriber', 'Edit Subscriber')}>
	{#if subscriberTarget}
		<div class="space-y-4">
			<div>
				<p class="text-sm" style="color: var(--color-muted)">{subscriberTarget.email}</p>
			</div>
			<div>
				<label for="sub-status" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('common.status', 'Status')}</label>
				<Select options={subscriberStatusOptions} value={editSubscriberStatus} onchange={(val) => (editSubscriberStatus = val)} />
			</div>
		</div>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="ghost" onclick={() => (showEditSlideOver = false)}>{t('common.cancel', 'Cancel')}</Button>
			<Button onclick={saveSubscriber} disabled={deleting}>{t('common.save', 'Save')}</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- View Campaign SlideOver -->
<SlideOver bind:open={showCampaignSlideOver} title={t('app.newsletter.modal_campaign_details', 'Campaign Details')} size="lg">
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
	{/if}
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			{#if campaignTarget?.status === 'draft' && canManageNewsletter}
				{@const ct = campaignTarget}
				<Button onclick={() => { showCampaignSlideOver = false; sendCampaign(ct.id); }}>
					<Send class="mr-1 h-3.5 w-3.5" /> {t('common.send', 'Send')}
				</Button>
			{/if}
			<Button variant="outline" onclick={() => (showCampaignSlideOver = false)}>{t('common.close', 'Close')}</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- Import Subscribers Modal (page-local — form with textarea) -->
<Modal bind:open={showImportModal} title={t('app.newsletter.modal_import_subscribers', 'Import Subscribers')} size="lg">
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-muted)">
			{t('app.newsletter.import_hint', 'Paste email addresses below, one per line. Optionally add a name after a comma: email@example.com, John Doe')}
		</p>
		<textarea class="w-full rounded-lg border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" rows="10" bind:value={importText} placeholder={"user1@example.com\nuser2@example.com, Jane Doe\nuser3@example.com"}></textarea>
		{#if importResult}
			<div class="rounded-lg border p-3 text-sm" style="border-color: var(--color-border); background: var(--color-surface)">
				<p style="color: var(--color-success)">{t('app.newsletter.import_success', 'Imported')}: {importResult.success}</p>
				{#if importResult.failed > 0}
					<p style="color: var(--color-danger)">{t('app.newsletter.import_failed', 'Failed')}: {importResult.failed}</p>
					<div class="mt-2 max-h-32 overflow-y-auto">
						{#each importResult.errors as err}
							<p class="text-xs" style="color: var(--color-muted)">{err}</p>
						{/each}
					</div>
				{/if}
			</div>
		{/if}
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showImportModal = false)}>{t('common.close', 'Close')}</Button>
		<Button onclick={importSubscribers} disabled={importing || !importText.trim()}>
			{importing ? t('common.saving', 'Importing...') : t('app.newsletter.btn_import', 'Import')}
		</Button>
	</div>
</Modal>
