<script lang="ts">
	import { onMount } from 'svelte';
	import { api, webhooksAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Plus, Pencil, Trash2, Send, CheckCircle, RotateCcw, Eye } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';

	type Webhook = {
		id: string;
		url: string;
		events: string[];
		status: string;
		last_delivery_at?: string;
		created_at: string;
	};

	type Delivery = {
		id: string;
		status: string;
		status_code?: number;
		response_body?: string;
		created_at: string;
	};

	let webhooks = $state<Webhook[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let editingWebhook = $state<Webhook | null>(null);
	let saving = $state(false);
	let modalOpen = $state(false);

	let url = $state('');
	let events = $state('');

	// Deliveries
	let showDeliveriesModal = $state(false);
	let deliveriesWebhook = $state<Webhook | null>(null);
	let deliveries = $state<Delivery[]>([]);
	let deliveriesLoading = $state(false);
	let retryingId = $state('');

	// Verify
	let verifyingId = $state('');
	let verifyMessage = $state('');
	let verifyMessageType = $state<'success' | 'error'>('success');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<Webhook[]>('/admin/webhooks');
			if (result.ok) {
				webhooks = result.data ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		editingWebhook = null;
		url = '';
		events = '';
		modalOpen = true;
	}

	function openEdit(webhook: Webhook) {
		editingWebhook = webhook;
		url = webhook.url;
		events = webhook.events.join(', ');
		modalOpen = true;
	}

	async function createWebhook() {
		if (!url.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const body = { url: url.trim(), events: events.split(',').map((e) => e.trim()).filter(Boolean) };
			const result = await api('/admin/webhooks', { method: 'POST', body });
			if (result.ok) {
				modalOpen = false;
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			saving = false;
		}
	}

	async function updateWebhook() {
		if (!editingWebhook || !url.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const body = { url: url.trim(), events: events.split(',').map((e) => e.trim()).filter(Boolean) };
			const result = await webhooksAdminApiExtended.update(editingWebhook.id, body);
			if (result.ok) {
				modalOpen = false;
				editingWebhook = null;
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			saving = false;
		}
	}

	async function deleteWebhook(id: string) {
		errorMessage = '';
		try {
			await fetch(`/admin/webhooks/${id}`, { method: 'DELETE' });
			webhooks = webhooks.filter((w) => w.id !== id);
		} catch {
			errorMessage = t('common.error');
		}
	}

	async function testWebhook(id: string) {
		errorMessage = '';
		try {
			await webhooksAdminApiExtended.test(id);
			load();
		} catch {
			errorMessage = t('common.error');
		}
	}

	async function verifyWebhook(id: string) {
		verifyingId = id;
		verifyMessage = '';
		errorMessage = '';
		try {
			const result = await webhooksAdminApiExtended.verify(id);
			if (result.ok) {
				verifyMessageType = 'success';
				verifyMessage = t('webhooks.verify_success');
			} else {
				verifyMessageType = 'error';
				verifyMessage = result.error;
			}
			setTimeout(() => { verifyMessage = ''; }, 3000);
		} catch {
			errorMessage = t('common.error');
		} finally {
			verifyingId = '';
		}
	}

	async function viewDeliveries(webhook: Webhook) {
		deliveriesWebhook = webhook;
		deliveriesLoading = true;
		showDeliveriesModal = true;
		try {
			const result = await webhooksAdminApiExtended.getDeliveries(webhook.id);
			if (result.ok) {
				deliveries = (result.data as Delivery[]) ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			deliveriesLoading = false;
		}
	}

	async function retryDelivery(webhookId: string, deliveryId: string) {
		retryingId = deliveryId;
		try {
			await webhooksAdminApiExtended.retryDelivery(webhookId, deliveryId);
			viewDeliveries(deliveriesWebhook!);
		} catch {
			errorMessage = t('common.error');
		} finally {
			retryingId = '';
		}
	}

	function deliveryStatusColor(status: string) {
		switch (status) {
			case 'success': return 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400';
			case 'failed': return 'bg-red-500/15 text-red-700 dark:text-red-400';
			case 'pending': return 'bg-amber-500/15 text-amber-700 dark:text-amber-400';
			default: return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
		}
	}

	function statusColor(status: string) {
		return status === 'active'
			? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400'
			: 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('webhooks.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('webhooks.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('webhooks.description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

{#if verifyMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: {verifyMessageType === 'success' ? 'var(--color-success)' : 'var(--color-danger)'}; background-color: color-mix(in srgb, {verifyMessageType === 'success' ? 'var(--color-success)' : 'var(--color-danger)'} 8%, transparent); color: {verifyMessageType === 'success' ? 'var(--color-success)' : 'var(--color-danger)'}">
		{verifyMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button class="btn btn-primary flex items-center gap-1.5 text-xs" onclick={openCreate}>
		<Plus class="h-3 w-3" />
		{t('webhooks.create')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('webhooks.url')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('webhooks.events')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('webhooks.status')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('webhooks.last_delivery')}</th>
					<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each webhooks as webhook (webhook.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 text-xs font-mono max-w-xs truncate">{webhook.url}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">
							{webhook.events.slice(0, 3).join(', ')}{webhook.events.length > 3 ? ` +${webhook.events.length - 3}` : ''}
						</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {statusColor(webhook.status)}">
								{webhook.status}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">
							{webhook.last_delivery_at ? new Date(webhook.last_delivery_at).toLocaleString() : '—'}
						</td>
						<td class="px-4 py-3 text-right">
							<div class="flex items-center justify-end gap-1">
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => testWebhook(webhook.id)} title={t('webhooks.test')}>
									<Send class="h-3 w-3" style="color: var(--color-success)" />
								</button>
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => verifyWebhook(webhook.id)} title={t('webhooks.verify')}>
									{#if verifyingId === webhook.id}
										<RotateCcw class="h-3 w-3 animate-spin" style="color: var(--color-info)" />
									{:else}
										<CheckCircle class="h-3 w-3" style="color: var(--color-info)" />
									{/if}
								</button>
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => viewDeliveries(webhook)} title={t('webhooks.deliveries')}>
									<Eye class="h-3 w-3" style="color: var(--color-warning)" />
								</button>
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => openEdit(webhook)} title={t('common.edit')}>
									<Pencil class="h-3 w-3" style="color: var(--color-primary)" />
								</button>
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => deleteWebhook(webhook.id)} title={t('common.delete')}>
									<Trash2 class="h-3 w-3" style="color: var(--color-danger)" />
								</button>
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('webhooks.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<!-- Create/Edit Modal -->
<Modal bind:open={modalOpen} title={editingWebhook ? t('webhooks.edit') : t('webhooks.create')} onConfirm={editingWebhook ? updateWebhook : createWebhook} confirmLabel={saving ? t('common.saving') : editingWebhook ? t('common.save') : t('common.create')} onclose={() => { editingWebhook = null; }}>
	<div class="space-y-4">
		<div>
			<label for="wh-url" class="mb-1 block text-sm font-medium">{t('webhooks.url')}</label>
			<input id="wh-url" type="url" bind:value={url} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="wh-events" class="mb-1 block text-sm font-medium">{t('webhooks.events')}</label>
			<input id="wh-events" type="text" bind:value={events} placeholder="user.created, content.published" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
			<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('webhooks.events_help')}</p>
		</div>
	</div>
</Modal>

<!-- Deliveries Modal -->
<Modal bind:open={showDeliveriesModal} title={t('webhooks.delivery_history')}>
	{#if deliveriesLoading}
		<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
	{:else if deliveries.length === 0}
		<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('webhooks.no_deliveries')}</p>
	{:else}
		<div class="max-h-96 space-y-2 overflow-y-auto">
			{#each deliveries as delivery (delivery.id)}
				<div class="rounded-md border p-3" style="border-color: var(--color-border)">
					<div class="flex items-center justify-between">
						<div class="flex items-center gap-2">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {deliveryStatusColor(delivery.status)}">
								{delivery.status}
							</span>
							{#if delivery.status_code}
								<span class="text-xs font-mono" style="color: var(--color-muted)">HTTP {delivery.status_code}</span>
							{/if}
							<span class="text-xs" style="color: var(--color-muted)">{new Date(delivery.created_at).toLocaleString()}</span>
						</div>
						{#if delivery.status === 'failed'}
							<button
								class="flex items-center gap-1 rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-50"
								style="color: var(--color-primary)"
								disabled={retryingId === delivery.id}
								onclick={() => retryDelivery(deliveriesWebhook!.id, delivery.id)}
							>
								<RotateCcw class="h-3 w-3 {retryingId === delivery.id ? 'animate-spin' : ''}" />
								{t('webhooks.retry')}
							</button>
						{/if}
					</div>
					{#if delivery.response_body}
						<pre class="mt-2 max-h-24 overflow-y-auto rounded p-2 text-xs" style="background-color: var(--color-surface)">{delivery.response_body}</pre>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</Modal>
