<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /broadcasts — migrated to shared components (ActionMenu, FilterPanel,
	// SlideOver, bulkActionsList). Tabbed page (all/drafts/sent); keeps
	// page-local Modals for create broadcast (form) and send confirmation.
	// View detail opens in a SlideOver.
	import { t } from '$lib/stores/i18n.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import { Select } from '$lib/components/ui/select';
	import type { ActionItem, BulkAction } from '$lib/components/ui/data-table';
	import { Send, Plus, Radio, Clock, CheckCircle, Trash2, Eye } from 'lucide-svelte';
	import { api } from '$lib/api';
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

	let activeTab = $state('all');
	let pageSize = $state(15);
	let broadcasts = $state<Broadcast[]>([]);
	let loading = $state(true);
	let filterValue = $state<Record<string, string>>({});
	let searchQuery = $state('');
	let selectedIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	// Create Modal (page-local — form)
	let showCreateModal = $state(false);
	let newBroadcast = $state({ title: '', body: '', target_audience: 'all' });
	let creating = $state(false);

	// Send confirmation Modal
	let showSendModal = $state(false);
	let sendTarget = $state<Broadcast | null>(null);
	let sending = $state(false);

	// View SlideOver
	let showViewSlideOver = $state(false);
	let viewTarget = $state<Broadcast | null>(null);

	const tabs = [
		{ value: 'all', label: t('broadcasts.tab_all', 'All'), icon: Radio },
		{ value: 'draft', label: t('broadcasts.tab_drafts', 'Drafts'), icon: Clock },
		{ value: 'sent', label: t('broadcasts.tab_sent', 'Sent'), icon: CheckCircle }
	];

	const audienceOptions = [
		{ value: 'all', label: t('broadcasts.all_users', 'All Users') },
		{ value: 'active', label: t('broadcasts.active_users', 'Active Users') },
		{ value: 'premium', label: t('broadcasts.premium_users', 'Premium Users') }
	];

	const columns = [
		{
			key: 'title',
			label: t('broadcasts.column_title', 'Title'),
			render: (_: any, row: any) => `
				<div>
					<p class="font-medium text-foreground">${row.title}</p>
					<p class="text-sm text-muted line-clamp-1">${row.body || ''}</p>
				</div>
			`
		},
		{
			key: 'target_audience',
			label: t('broadcasts.column_audience', 'Audience'),
			render: (v: string) => {
				const labels: Record<string, string> = { all: t('broadcasts.all_users', 'All Users'), active: t('broadcasts.active_users', 'Active Users'), premium: t('broadcasts.premium_users', 'Premium Users') };
				return `<span class="text-sm text-foreground-alt">${labels[v] || v}</span>`;
			}
		},
		{
			key: 'status',
			label: t('common.status', 'Status'),
			render: (v: string) => {
				const cls = v === 'sent' ? 'bg-success/15 text-success border-success' : v === 'scheduled' ? 'bg-info-100 text-info-800 border-info-200' : v === 'failed' ? 'bg-danger/15 text-danger border-danger' : 'bg-surface text-foreground border';
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${v}</span>`;
			}
		},
		{
			key: 'recipient_count',
			label: t('broadcasts.column_recipients', 'Recipients'),
			render: (v: number) => `<span class="text-sm font-medium">${(v ?? 0).toLocaleString()}</span>`
		},
		{
			key: 'sent_at',
			label: t('broadcasts.column_sent', 'Sent'),
			render: (v: string) => (v ? new Date(v).toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' }) : '<span class="text-muted">—</span>')
		}
	];

	const rowActions: ActionItem[] = [
		{ label: t('common.view', 'View'), icon: Eye, onclick: (row) => openView(row) },
		{
			label: t('common.send', 'Send'),
			icon: Send,
			onclick: (row) => confirmSend(row),
			show: (row: Broadcast) => canManage && row.status === 'draft'
		}
	];

	const bulkActions: BulkAction[] = $derived(canManage
		? [{ label: t('broadcasts.delete_selected', 'Delete Selected'), icon: Trash2, variant: 'destructive', onclick: bulkDelete }]
		: []);

	let filteredBroadcasts = $derived.by(() => {
		let result = [...broadcasts];
		if (activeTab !== 'all') result = result.filter((b) => b.status === activeTab);
		if (filterValue.audience) result = result.filter((b) => b.target_audience === filterValue.audience);
		if (searchQuery) {
			const q = searchQuery.toLowerCase();
			result = result.filter((b) => b.title.toLowerCase().includes(q) || (b.body ?? '').toLowerCase().includes(q));
		}
		return result;
	});

	async function loadBroadcasts() {
		loading = true;
		try {
			const result = await api<Broadcast[]>('/admin/broadcasts');
			if (result.ok) broadcasts = result.data ?? [];
		} catch { /* API not available */ } finally {
			loading = false;
		}
	}

	async function bulkDelete(ids: Set<string>) {
		if (ids.size === 0 || bulkDeleting) return;
		bulkDeleting = true;
		try {
			await Promise.all(Array.from(ids).map((id) => api(`/admin/broadcasts/${id}`, { method: 'DELETE' })));
		} catch { /* API not available */ } finally {
			bulkDeleting = false;
		}
	}

	async function createBroadcast() {
		if (!newBroadcast.title.trim() || !newBroadcast.body.trim()) return;
		creating = true;
		try {
			const result = await api('/admin/broadcasts', {
				method: 'POST',
				body: {
					title: newBroadcast.title.trim(),
					body: newBroadcast.body.trim(),
					target_audience: newBroadcast.target_audience
				}
			});
			if (result.ok) {
				showCreateModal = false;
				newBroadcast = { title: '', body: '', target_audience: 'all' };
				loadBroadcasts();
			}
		} catch { /* API not available */ } finally {
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
			await api(`/admin/broadcasts/${sendTarget.id}/send`, { method: 'POST', body: {} });
			showSendModal = false;
			sendTarget = null;
			loadBroadcasts();
		} catch { /* API not available */ } finally {
			sending = false;
		}
	}

	function openView(broadcast: Broadcast) {
		viewTarget = broadcast;
		showViewSlideOver = true;
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
	<title>{t('broadcasts.page_title', 'Broadcasts')} — {siteName()} Admin</title>
</svelte:head>

<PageHeader
	title={t('broadcasts.title', 'Broadcasts')}
	description={t('broadcasts.description', 'Send targeted messages to platform users')}
	actions={canManage ? [{ label: t('broadcasts.new_broadcast', 'New Broadcast'), icon: Plus, onclick: () => (showCreateModal = true) }] : []}
/>

<div class="space-y-6">
	<Tabs {tabs} bind:value={activeTab} tabParam="tab" tabPreserve={[]} />

	<Card>
		<CardContent class="p-0">
			<DataTable
				columns={columns}
				data={filteredBroadcasts}
				{loading}
				bind:pageSize
			pageSizeOptions={[10, 25, 50, 100]}
				emptyMessage={t('broadcasts.no_broadcasts_found', 'No broadcasts found')}
				searchable
				onsearch={(q) => { searchQuery = q; }}
				filters={[{ key: 'audience', label: t('broadcasts.filter_audience', 'Audience'), options: audienceOptions }]}
				bind:filterValue
				selectable={canManage}
				bind:selectedIds
				actions={rowActions}
				bulkActionsList={bulkActions}
			/>
		</CardContent>
	</Card>
</div>

<!-- Create Broadcast Modal (page-local — form) -->
<Modal bind:open={showCreateModal} title={t('broadcasts.new_broadcast', 'New Broadcast')} description={t('broadcasts.create_description', 'Create and send a broadcast message')} size="lg">
	<div class="space-y-4">
		<div>
			<label for="bc-title" class="mb-1 block text-sm font-medium text-foreground">{t('broadcasts.title_label', 'Title')}</label>
			<input id="bc-title" type="text" class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" placeholder={t('broadcasts.placeholder_title', 'e.g. Platform Maintenance Notice')} bind:value={newBroadcast.title} />
		</div>
		<div>
			<label for="bc-body" class="mb-1 block text-sm font-medium text-foreground">{t('broadcasts.message_label', 'Message')}</label>
			<textarea id="bc-body" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" rows="5" placeholder={t('broadcasts.placeholder_message', 'Write your broadcast message...')} bind:value={newBroadcast.body}></textarea>
		</div>
		<div>
			<label for="bc-audience" class="mb-1 block text-sm font-medium text-foreground">{t('broadcasts.target_audience_label', 'Target Audience')}</label>
			<Select options={audienceOptions} bind:value={newBroadcast.target_audience} />
		</div>
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => (showCreateModal = false)}>{t('broadcasts.cancel', 'Cancel')}</Button>
			<Button onclick={createBroadcast} disabled={creating || !newBroadcast.title.trim() || !newBroadcast.body.trim()}>
				{creating ? t('broadcasts.creating', 'Creating...') : t('broadcasts.create_broadcast', 'Create Broadcast')}
			</Button>
		</div>
	</div>
</Modal>

<!-- Send Confirmation Modal -->
<Modal bind:open={showSendModal} title={t('broadcasts.send_broadcast', 'Send Broadcast')} description={t('broadcasts.send_immediately_warning', 'This message will be sent immediately to the target audience.')} size="sm">
	{#if sendTarget}
		<div class="rounded-lg bg-surface p-4">
			<p class="text-sm font-medium text-foreground">{sendTarget.title}</p>
			<p class="mt-1 text-sm text-muted">{t('broadcasts.audience_prefix', 'Audience')}: {sendTarget.target_audience === 'all' ? t('broadcasts.all_users', 'All Users') : sendTarget.target_audience === 'active' ? t('broadcasts.active_users', 'Active Users') : t('broadcasts.premium_users', 'Premium Users')}</p>
		</div>
	{/if}
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => (showSendModal = false)}>{t('broadcasts.cancel', 'Cancel')}</Button>
		<Button onclick={sendBroadcast} disabled={sending}>
			<Send class="mr-2 h-4 w-4" />
			{sending ? t('broadcasts.sending', 'Sending...') : t('broadcasts.send_now', 'Send Now')}
		</Button>
	</div>
</Modal>

<!-- View Broadcast SlideOver -->
<SlideOver bind:open={showViewSlideOver} title={t('broadcasts.broadcast_details', 'Broadcast Details')} size="lg">
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
	{/if}
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			{#if canManage && viewTarget?.status === 'draft'}
				{@const bt = viewTarget}
				<Button onclick={() => { showViewSlideOver = false; confirmSend(bt); }}>
					<Send class="mr-1 h-3.5 w-3.5" /> {t('common.send', 'Send')}
				</Button>
			{/if}
			<Button variant="outline" onclick={() => (showViewSlideOver = false)}>{t('common.close', 'Close')}</Button>
		</div>
	{/snippet}
</SlideOver>
