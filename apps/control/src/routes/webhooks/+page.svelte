<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import AlertBanner from '$lib/components/ui/AlertBanner.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import WebhookEventMultiSelect from '$lib/components/webhooks/WebhookEventMultiSelect.svelte';
	import Modal from '$lib/components/ui/Modal.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { escapeHtml } from '$lib/utils';
	import { Webhook, Plus, Trash2, TestTube, Pencil, RefreshCw, Send } from 'lucide-svelte';
	import type { DataTableColumn, DataTableRowAction, DataTableBulkAction } from '$lib/api/types';

	let {} = $props();

	interface WebhookEndpoint {
		id: string;
		name: string;
		url: string;
		event_types: string[];
		is_active: boolean;
		verified_at: string | null;
		created_at: string;
	}

	let webhooks = $state<WebhookEndpoint[]>([]);
	let loading = $state(true);
	let message = $state('');
	let messageType = $state<'success' | 'error' | 'info'>('info');

	let page = $state(0);
	let pageSize = $state(10);
	let sortKey = $state('');
	let sortDirection = $state<'asc' | 'desc'>('asc');

	let createModalOpen = $state(false);
	let editingWebhook = $state<WebhookEndpoint | null>(null);
	let form = $state({ name: '', url: '', event_types: [] as string[], is_active: true });
	let saving = $state(false);

	let deleteModalOpen = $state(false);
	let deletingWebhook = $state<WebhookEndpoint | null>(null);

	let selectedIds = $state(new Set<string>());
	let confirmBulkDeleteOpen = $state(false);
	let pendingBulkIds = $state<string[]>([]);
	let bulkBusy = $state(false);

	let createdSecret = $state<string | null>(null);
	let showSecretModal = $state(false);

	const columns: DataTableColumn[] = [
		{
			key: 'name',
			label: t('webhooks.name', 'Name'),
			sortable: true,
			render: (val) =>
				`<span class="font-medium text-foreground">${escapeHtml(val) || '—'}</span>`
		},
		{
			key: 'url',
			label: t('webhooks.url', 'URL'),
			sortable: true,
			render: (val) =>
				`<span class="font-mono text-xs break-all">${escapeHtml(val) || '—'}</span>`
		},
		{
			key: 'event_types',
			label: t('webhooks.events', 'Events'),
			sortable: false,
			render: (val) => {
				const arr = Array.isArray(val) ? val : [];
				if (arr.length === 0) return `<span class="text-muted-foreground text-xs">—</span>`;
				return arr
					.map(
						(e) =>
							`<span class="bg-muted text-muted-foreground rounded-5px px-1.5 py-0.5 font-mono text-xs">${escapeHtml(e)}</span>`
					)
					.join(' ');
			}
		},
		{
			key: 'is_active',
			label: t('webhooks.status', 'Status'),
			sortable: true,
			render: (val) =>
				val
					? `<span class="bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 rounded-5px px-2 py-0.5 text-xs font-medium">${t('common.active', 'Active')}</span>`
					: `<span class="bg-muted text-muted-foreground rounded-5px px-2 py-0.5 text-xs font-medium">${t('common.inactive', 'Inactive')}</span>`
		},
		{
			key: 'created_at',
			label: t('webhooks.created', 'Created'),
			sortable: true,
			render: (val) =>
				`<span class="text-muted-foreground text-xs">${val ? escapeHtml(new Date(val as string).toLocaleString()) : '—'}</span>`
		}
	];

	async function load(quiet = false) {
		if (!quiet) loading = true;
		try {
			const result = await api<WebhookEndpoint[]>('/control/webhooks');
			if (result.ok) {
				webhooks = Array.isArray(result.data) ? result.data : [];
			} else {
				message = result.error || t('webhooks.load_error', 'Failed to load webhooks.');
				messageType = 'error';
			}
		} catch {
			message = t('webhooks.load_error', 'Failed to load webhooks.');
			messageType = 'error';
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		load();
		const interval = setInterval(() => {
			if (document.visibilityState === 'visible') {
				load(true);
			}
		}, 30000);
		return () => clearInterval(interval);
	});

	let sortedWebhooks = $derived.by(() => {
		const list = [...webhooks];
		if (sortKey) {
			list.sort((a, b) => {
				const row = a as unknown as Record<string, unknown>;
				const row2 = b as unknown as Record<string, unknown>;
				const av = row[sortKey];
				const bv = row2[sortKey];
				if (av == null && bv == null) return 0;
				if (av == null) return 1;
				if (bv == null) return -1;
				const cmp = String(av).localeCompare(String(bv));
				return sortDirection === 'asc' ? cmp : -cmp;
			});
		}
		return list;
	});

	let totalRows = $derived(sortedWebhooks.length);

	let formValid = $derived(
		form.name.trim() !== '' &&
			form.url.trim() !== '' &&
			/^https?:\/\//.test(form.url.trim()) &&
			form.event_types.length > 0
	);

	function openCreateModal() {
		editingWebhook = null;
		form = { name: '', url: '', event_types: [], is_active: true };
		createModalOpen = true;
	}

	function openEditModal(webhook: WebhookEndpoint) {
		editingWebhook = webhook;
		form = {
			name: webhook.name,
			url: webhook.url,
			event_types: Array.isArray(webhook.event_types) ? [...webhook.event_types] : [],
			is_active: webhook.is_active
		};
		createModalOpen = true;
	}

	function closeCreateModal() {
		createModalOpen = false;
		editingWebhook = null;
		form = { name: '', url: '', event_types: [], is_active: true };
	}

	async function submitWebhook() {
		if (!formValid) return;
		saving = true;
		message = '';
		try {
			const body = {
				name: form.name.trim(),
				url: form.url.trim(),
				event_types: form.event_types,
				is_active: form.is_active
			};
			if (editingWebhook) {
				const result = await api<WebhookEndpoint>(`/control/webhooks/${editingWebhook.id}`, {
					method: 'PATCH',
					body
				});
				if (result.ok) {
					closeCreateModal();
					await load();
					message = t('webhooks.update_success', 'Webhook updated successfully.');
					messageType = 'success';
				} else {
					message = result.error || t('webhooks.update_error', 'Failed to update webhook.');
					messageType = 'error';
				}
			} else {
				const result = await api<WebhookEndpoint & { secret?: string }>('/control/webhooks', {
					method: 'POST',
					body
				});
				if (result.ok) {
					createdSecret = result.data.secret ?? null;
					closeCreateModal();
					await load();
					message = t('webhooks.create_success', 'Webhook created successfully.');
					messageType = 'success';
					if (createdSecret) showSecretModal = true;
				} else {
					message = result.error || t('webhooks.create_error', 'Failed to create webhook.');
					messageType = 'error';
				}
			}
		} catch {
			message = t('webhooks.create_error', 'Failed to save webhook.');
			messageType = 'error';
		} finally {
			saving = false;
		}
	}

	function testWebhook(webhook: WebhookEndpoint) {
		void (async () => {
			message = '';
			try {
				const result = await api<{ ok: boolean }>(`/control/webhooks/${webhook.id}/test`, {
					method: 'POST',
					body: { event_type: 'webhook.test' }
				});
				if (result.ok) {
					message = t('webhooks.test_success', 'Test event sent successfully.');
					messageType = 'success';
				} else {
					message = result.error || t('webhooks.test_error', 'Failed to send test event.');
					messageType = 'error';
				}
			} catch {
				message = t('webhooks.test_error', 'Failed to send test event.');
				messageType = 'error';
			}
		})();
	}

	function confirmDelete(webhook: WebhookEndpoint) {
		deletingWebhook = webhook;
		deleteModalOpen = true;
	}

	async function handleDelete() {
		if (!deletingWebhook) return;
		message = '';
		try {
			const result = await api(`/control/webhooks/${deletingWebhook.id}`, { method: 'DELETE' });
			if (result.ok) {
				await load();
				message = t('webhooks.delete_success', 'Webhook deleted successfully.');
				messageType = 'success';
			} else {
				message = result.error || t('webhooks.delete_error', 'Failed to delete webhook.');
				messageType = 'error';
			}
		} catch {
			message = t('webhooks.delete_error', 'Failed to delete webhook.');
			messageType = 'error';
		} finally {
			deleteModalOpen = false;
			deletingWebhook = null;
		}
	}

	const rowActions: DataTableRowAction[] = [
		{
			label: t('webhooks.test', 'Send Test'),
			icon: TestTube,
			onClick: (row) => testWebhook(row as unknown as WebhookEndpoint)
		},
		{
			label: t('webhooks.edit', 'Edit'),
			icon: Pencil,
			onClick: (row) => openEditModal(row as unknown as WebhookEndpoint)
		},
		{
			label: t('webhooks.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onClick: (row) => confirmDelete(row as unknown as WebhookEndpoint)
		}
	];

	const bulkActions: DataTableBulkAction[] = [
		{
			label: t('webhooks.delete_selected', 'Delete Selected'),
			icon: Trash2,
			variant: 'destructive',
			onClick: (ids: string[]) => {
				pendingBulkIds = ids;
				confirmBulkDeleteOpen = true;
			}
		}
	];

	async function confirmBulkDelete() {
		const ids = pendingBulkIds;
		message = '';
		bulkBusy = true;
		try {
			const results = await Promise.allSettled(
				ids.map((id) => api(`/control/webhooks/${id}`, { method: 'DELETE' }))
			);
			const failed = results.filter(
				(r) => r.status === 'rejected' || (r.status === 'fulfilled' && !r.value.ok)
			).length;
			selectedIds = new Set();
			pendingBulkIds = [];
			confirmBulkDeleteOpen = false;
			await load();
			if (failed > 0) {
				message = t(
					'webhooks.bulk_delete_partial',
					'{n} webhook(s) could not be deleted.'
				).replace('{n}', String(failed));
				messageType = 'error';
			} else {
				message = t('webhooks.bulk_delete_success', 'Selected webhooks deleted successfully.');
				messageType = 'success';
			}
		} finally {
			bulkBusy = false;
		}
	}
</script>

<PageHeader
	title={t('webhooks.title', 'Webhooks')}
	description={t('webhooks.description', 'Webhook endpoint management')}
/>

<div class="mb-4 flex flex-wrap items-center gap-3">
	<div class="flex-1"></div>
	<button
		class="bg-foreground text-background flex items-center gap-1.5 rounded-9px px-3 py-2 text-xs font-medium transition-colors hover:opacity-90"
		onclick={openCreateModal}
	>
		<Plus class="size-3" />
		{t('webhooks.create', 'Create Webhook')}
	</button>
	<button
		class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px flex items-center gap-1.5 border px-3 py-2 text-xs font-medium transition-colors"
		onclick={() => { message = ''; load(); }}
		disabled={loading}
	>
		<RefreshCw class="size-3 {loading ? 'animate-spin' : ''}" />
		{t('common.refresh', 'Refresh')}
	</button>
</div>

{#if message}
	<div class="mb-4">
		<AlertBanner variant={messageType} {message} />
	</div>
{/if}

<DataTable
	{columns}
	rows={sortedWebhooks}
	{totalRows}
	bind:page
	bind:pageSize
	bind:sortKey
	bind:sortDirection
	{rowActions}
	{bulkActions}
	{loading}
	emptyMessage={t('webhooks.no_webhooks', 'No webhook endpoints configured.')}
	emptyIcon={Webhook}
	selectable={true}
	bind:selectedIds
/>

<Modal
	bind:open={createModalOpen}
	title={editingWebhook ? t('webhooks.edit_webhook', 'Edit Webhook') : t('webhooks.create_webhook', 'Create Webhook')}
	size="lg"
>
			<div class="mt-4 space-y-4">
				<div>
					<label for="webhook-name" class="text-foreground mb-1 block text-sm font-medium">
						{t('webhooks.name', 'Name')}
					</label>
					<p class="text-muted-foreground mt-1 text-xs">{t('webhooks.name_hint', 'A label so you remember this endpoint.')}</p>
					<input
						id="webhook-name"
						type="text"
						bind:value={form.name}
						class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						placeholder={t('webhooks.name_placeholder', 'My Webhook')}
					/>
				</div>
				<div>
					<label for="webhook-url" class="text-foreground mb-1 block text-sm font-medium">
						{t('webhooks.url', 'URL')}
					</label>
					<p class="text-muted-foreground mt-1 text-xs">{t('webhooks.url_hint', 'The URL that receives event payloads — must be HTTPS.')}</p>
					<input
						id="webhook-url"
						type="url"
						bind:value={form.url}
						class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						placeholder="https://example.com/webhook"
					/>
					<p class="text-muted-foreground mt-1 text-xs">
						{t('webhooks.url_hint', 'The endpoint that will receive event deliveries.')}
					</p>
				</div>
				<div>
					<label for="webhook-events" class="text-foreground mb-1 block text-sm font-medium">
						{t('webhooks.events', 'Events')}
					</label>
					<p class="text-muted-foreground mt-1 text-xs">{t('webhooks.events_hint', 'Which events trigger this webhook.')}</p>
					<WebhookEventMultiSelect
						value={form.event_types}
						onchange={(next) => (form.event_types = next)}
					/>
					<p class="text-muted-foreground mt-1 text-xs">
						{t('webhooks.events_hint', 'Select the events to subscribe to.')}
					</p>
				</div>
				<div class="rounded-9px border border-border-input p-3 text-xs text-muted-foreground">
					{t('webhooks.secret_hint', 'The signing secret is generated automatically on creation and shown once. To rotate, delete and recreate the webhook.')}
				</div>
				{#if editingWebhook}
					<label class="text-foreground flex items-center gap-2 text-sm font-medium">
						<input
							type="checkbox"
							bind:checked={form.is_active}
							class="border-border-input bg-background size-3.5 cursor-pointer rounded-sm accent-accent-foreground"
						/>
						{t('webhooks.active', 'Active')}
					</label>
				{/if}
			</div>
			<div class="mt-6 flex justify-end gap-3">
				<button
					type="button"
					class="border-border-input bg-background hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors"
					onclick={closeCreateModal}
				>
					{t('common.cancel', 'Cancel')}
				</button>
				<button
					class="bg-foreground text-background rounded-9px px-4 py-2 text-sm font-medium transition-colors hover:opacity-90 disabled:opacity-50"
					onclick={submitWebhook}
					disabled={saving || !formValid}
				>
					{saving
						? t('common.saving', 'Saving...')
						: editingWebhook
							? t('common.save', 'Save')
							: t('webhooks.create', 'Create')}
				</button>
			</div>
</Modal>

<ConfirmModal
	bind:open={deleteModalOpen}
	title={t('webhooks.delete_webhook', 'Delete Webhook')}
	consequence={t('webhooks.delete_confirm', 'This will permanently delete the webhook endpoint. This action cannot be undone.')}
	confirmText="DELETE"
	onConfirm={handleDelete}
/>

<ConfirmModal
	bind:open={confirmBulkDeleteOpen}
	title={t('webhooks.bulk_delete_title', 'Delete Selected Webhooks')}
	consequence={t('webhooks.bulk_delete_confirm', 'Are you sure you want to delete the selected webhook endpoints? This action cannot be undone.')}
	confirmText="DELETE"
	onConfirm={confirmBulkDelete}
	onCancel={() => { confirmBulkDeleteOpen = false; pendingBulkIds = []; }}
/>

<Modal
	bind:open={showSecretModal}
	onOpenChange={(isOpen) => { if (!isOpen) createdSecret = null; }}
	title={t('webhooks.secret_title', 'Signing secret — copy it now')}
	description={t('webhooks.secret_description', 'This secret is shown exactly once and is never stored in readable form. Copy it before closing.')}
>
	{#if createdSecret}
		<div class="mt-4">
			<label for="webhook-secret" class="text-foreground block text-sm font-medium">
				{t('webhooks.secret_label', 'Signing secret')}
			</label>
			<p class="text-muted-foreground mt-1 text-xs">{t('webhooks.secret_hint', 'Used to sign payloads — verify on your side.')}</p>
			<div class="mt-1 flex items-center gap-2">
				<input
					id="webhook-secret"
					type="text"
					readonly
					value={createdSecret}
					class="border-border-input bg-background text-foreground flex-1 rounded-9px border px-3 py-2 font-mono text-sm"
				/>
				<button
					type="button"
					class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-3 py-2 text-sm font-medium transition-colors"
					onclick={() => { if (createdSecret) void navigator.clipboard.writeText(createdSecret).catch(() => {}); }}
				>
					{t('common.copy', 'Copy')}
				</button>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end">
		<button
			type="button"
			class="bg-foreground text-background rounded-9px px-4 py-2 text-sm font-medium transition-colors hover:opacity-90"
			onclick={() => { showSecretModal = false; createdSecret = null; }}
		>
			{t('common.done', 'Done')}
		</button>
	</div>
</Modal>

<!-- RBAC CRUD: Webhook Endpoints management -->
