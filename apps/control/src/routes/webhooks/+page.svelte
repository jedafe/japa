<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import AlertBanner from '$lib/components/ui/AlertBanner.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { Dialog, Popover } from 'bits-ui';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { escapeHtml } from '$lib/utils';
	import { Webhook, Plus, Trash2, TestTube, Pencil, RefreshCw, Send, ChevronDown, Check, Search, X } from 'lucide-svelte';
	import type { DataTableColumn, DataTableRowAction, DataTableBulkAction } from '$lib/api/types';

	let {} = $props();

	interface WebhookEndpoint {
		id: string;
		url: string;
		events: string[];
		secret: string;
		is_active: boolean;
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
	let form = $state({ url: '', events: [] as string[], secret: '', is_active: true });
	let saving = $state(false);
	let eventSearch = $state('');

	let deleteModalOpen = $state(false);
	let deletingWebhook = $state<WebhookEndpoint | null>(null);

	let selectedIds = $state(new Set<string>());
	let confirmBulkDeleteOpen = $state(false);
	let pendingBulkIds = $state<string[]>([]);

	interface WebhookEventDef {
		key: string;
		label: string;
		description: string;
	}

	let availableEvents = $state<WebhookEventDef[]>([]);

	function toggleEventInForm(eventKey: string) {
		const idx = form.events.indexOf(eventKey);
		if (idx >= 0) {
			form.events = form.events.filter((e) => e !== eventKey);
		} else {
			form.events = [...form.events, eventKey];
		}
	}

	function removeEventFromForm(eventKey: string) {
		form.events = form.events.filter((e) => e !== eventKey);
	}

	let filteredAvailableEvents = $derived.by(() => {
		const q = eventSearch.trim().toLowerCase();
		if (!q) return availableEvents;
		return availableEvents.filter(
			(e) => e.key.toLowerCase().includes(q) || e.label.toLowerCase().includes(q)
		);
	});

	let selectedEventLabels = $derived.by(() => {
		return form.events.map((key) => {
			const def = availableEvents.find((e) => e.key === key);
			return { key, label: def?.label ?? key };
		});
	});

	const columns: DataTableColumn[] = [
		{
			key: 'url',
			label: t('webhooks.url', 'URL'),
			sortable: true,
			render: (val) =>
				`<span class="font-mono text-xs break-all">${escapeHtml(val) || '—'}</span>`
		},
		{
			key: 'events',
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
					? `<span class="bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 rounded-5px px-2 py-0.5 text-xs font-medium">Active</span>`
					: `<span class="bg-muted text-muted-foreground rounded-5px px-2 py-0.5 text-xs font-medium">Inactive</span>`
		},
		{
			key: 'created_at',
			label: t('webhooks.created', 'Created'),
			sortable: true,
			render: (val) =>
				`<span class="text-muted-foreground text-xs">${val ? escapeHtml(new Date(val as string).toLocaleString()) : '—'}</span>`
		}
	];

	async function loadEvents() {
		const result = await api<WebhookEventDef[]>('/control/webhook-events');
		if (result.ok && Array.isArray(result.data)) {
			availableEvents = result.data;
		}
	}

	async function load() {
		loading = true;
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
		loadEvents();
		const interval = setInterval(() => {
			load();
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

	let paginatedWebhooks = $derived.by(() => {
		const start = page * pageSize;
		return sortedWebhooks.slice(start, start + pageSize);
	});

	let formValid = $derived(
		form.url.trim() !== '' &&
			/^https?:\/\//.test(form.url.trim()) &&
			form.events.length > 0
	);

	function openCreateModal() {
		editingWebhook = null;
		form = { url: '', events: [], secret: '', is_active: true };
		eventSearch = '';
		createModalOpen = true;
	}

	function openEditModal(webhook: WebhookEndpoint) {
		editingWebhook = webhook;
		form = {
			url: webhook.url,
			events: Array.isArray(webhook.events) ? [...webhook.events] : [],
			secret: webhook.secret,
			is_active: webhook.is_active
		};
		eventSearch = '';
		createModalOpen = true;
	}

	function closeCreateModal() {
		createModalOpen = false;
		editingWebhook = null;
		form = { url: '', events: [], secret: '', is_active: true };
		eventSearch = '';
	}

	async function submitWebhook() {
		if (!formValid) return;
		saving = true;
		message = '';
		try {
			const body = {
				url: form.url.trim(),
				events: form.events,
				secret: form.secret.trim(),
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
				const result = await api<WebhookEndpoint>('/control/webhooks', {
					method: 'POST',
					body
				});
				if (result.ok) {
					closeCreateModal();
					await load();
					message = t('webhooks.create_success', 'Webhook created successfully.');
					messageType = 'success';
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
					method: 'POST'
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
		const results = await Promise.allSettled(
			ids.map((id) => api(`/control/webhooks/${id}`, { method: 'DELETE' }))
		);
		const failed = results.filter((r) => r.status === 'rejected').length;
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
	rows={paginatedWebhooks}
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

<Dialog.Root bind:open={createModalOpen}>
	<Dialog.Portal>
		<Dialog.Overlay
			class="bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out fixed inset-0 z-50"
		/>
		<Dialog.Content
			class="border-border-card bg-background shadow-popover fixed left-1/2 top-1/2 z-50 w-full max-w-lg -translate-x-1/2 -translate-y-1/2 rounded-xl border p-6 focus:outline-none"
			role="dialog"
			aria-modal="true"
			onOpenAutoFocus={(e) => e.preventDefault()}
		>
			<Dialog.Title class="text-foreground text-lg font-semibold">
				{editingWebhook
					? t('webhooks.edit_webhook', 'Edit Webhook')
					: t('webhooks.create_webhook', 'Create Webhook')}
			</Dialog.Title>
			<div class="mt-4 space-y-4">
				<div>
					<label for="webhook-url" class="text-foreground mb-1 block text-sm font-medium">
						{t('webhooks.url', 'URL')}
					</label>
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
					<label class="text-foreground mb-1 block text-sm font-medium">
						{t('webhooks.events', 'Events')}
					</label>

					{#if selectedEventLabels.length > 0}
						<div class="mb-2 flex flex-wrap gap-1.5">
							{#each selectedEventLabels as ev (ev.key)}
								<span class="bg-muted text-muted-foreground flex items-center gap-1 rounded-5px px-2 py-0.5 font-mono text-xs">
									{ev.label}
									<button
										type="button"
										class="text-muted-foreground hover:text-foreground"
										onclick={() => removeEventFromForm(ev.key)}
										aria-label="Remove {ev.label}"
									>
										<X class="size-3" />
									</button>
								</span>
							{/each}
						</div>
					{/if}

					<Popover.Root>
						<Popover.Trigger
							class="border-border-input bg-background text-foreground hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus flex w-full items-center justify-between rounded-9px border px-3 py-2 text-sm transition-colors"
							aria-label={t('webhooks.events', 'Events')}
						>
							<span class={form.events.length === 0 ? 'text-muted-foreground' : ''}>
								{form.events.length === 0
									? t('webhooks.select_events', 'Select events...')
									: t('webhooks.events_selected', '{count} selected').replace('{count}', String(form.events.length))}
							</span>
							<ChevronDown class="text-muted-foreground size-4 shrink-0" />
						</Popover.Trigger>
						<Popover.Portal>
							<Popover.Content
								class="border-border-input bg-background shadow-popover z-50 w-[var(--bits-popover-anchor-width)] rounded-xl border p-0"
								sideOffset={4}
							>
								<div class="bg-muted/40 px-3 py-2">
									<div class="flex items-center gap-2">
										<Search class="text-muted-foreground size-4 shrink-0" />
										<input
											type="text"
											bind:value={eventSearch}
											class="text-foreground placeholder:text-muted-foreground/60 w-full bg-transparent text-sm outline-none"
											placeholder={t('webhooks.search_events', 'Search events...')}
										/>
										{#if eventSearch}
											<button
												type="button"
												class="text-muted-foreground hover:text-foreground shrink-0"
												onclick={() => { eventSearch = ''; }}
												aria-label="Clear search"
											>
												<X class="size-3.5" />
											</button>
										{/if}
									</div>
								</div>
								<div class="max-h-60 overflow-y-auto p-1">
									{#if filteredAvailableEvents.length === 0}
										<div class="text-muted-foreground px-3 py-4 text-center text-sm">
											{t('webhooks.no_events_found', 'No events found.')}
										</div>
									{:else}
										{#each filteredAvailableEvents as event (event.key)}
											<button
												type="button"
												class="hover:bg-accent flex w-full items-center gap-2 rounded-9px px-3 py-2 text-left text-sm transition-colors {form.events.includes(event.key) ? 'bg-accent' : ''}"
												onclick={() => toggleEventInForm(event.key)}
												title={event.description}
											>
												<span class="flex-1">
													<span class="text-foreground block font-medium">{event.label}</span>
													<span class="text-muted-foreground block font-mono text-xs">{event.key}</span>
												</span>
												{#if form.events.includes(event.key)}
													<Check class="text-foreground size-4 shrink-0" />
												{/if}
											</button>
										{/each}
									{/if}
								</div>
								{#if form.events.length > 0}
									<div class="border-border-input border-t p-2">
										<button
											type="button"
											class="text-muted-foreground hover:text-foreground w-full text-center text-xs font-medium"
											onclick={() => { form.events = []; }}
										>
											{t('webhooks.clear_all', 'Clear all')}
										</button>
									</div>
								{/if}
							</Popover.Content>
						</Popover.Portal>
					</Popover.Root>
					<p class="text-muted-foreground mt-1 text-xs">
						{t('webhooks.events_hint', 'Select the events to subscribe to.')}
					</p>
				</div>
				<div>
					<label for="webhook-secret" class="text-foreground mb-1 block text-sm font-medium">
						{t('webhooks.secret', 'Secret')}
					</label>
					<input
						id="webhook-secret"
						type="text"
						bind:value={form.secret}
						class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 font-mono text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						placeholder="whsec_..."
					/>
					<p class="text-muted-foreground mt-1 text-xs">
						{t('webhooks.secret_hint', 'Used to sign webhook payloads for verification.')}
					</p>
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
				<Dialog.Close
					class="border-border-input bg-background hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors"
					onclick={closeCreateModal}
				>
					{t('common.cancel', 'Cancel')}
				</Dialog.Close>
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
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

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

<!-- RBAC CRUD: Webhook Endpoints management -->
