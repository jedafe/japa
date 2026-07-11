<script lang="ts">
	import { onMount } from 'svelte';
	import Modal from '$lib/components/Modal.svelte';
	import Badge from '$lib/components/ui/Badge.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type GdprRequest = {
		id: string;
		user_id: string;
		username?: string;
		email?: string;
		request_type: 'erasure' | 'export';
		status: 'pending' | 'processing' | 'completed' | 'failed';
		requested_at: string;
		completed_at?: string;
		export_url?: string;
	};

	let requests = $state<GdprRequest[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let selectedRequest = $state<GdprRequest | null>(null);
	let showDetailModal = $state(false);
	let processing = $state(false);

	function statusVariant(status: string): 'warning' | 'info' | 'success' | 'danger' {
		switch (status) {
			case 'pending': return 'warning';
			case 'processing': return 'info';
			case 'completed': return 'success';
			case 'failed': return 'danger';
			default: return 'default' as any;
		}
	}

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<{ requests: GdprRequest[] }>('/admin/gdpr/deletion-requests');
			if (result.ok) {
				requests = result.data.requests;
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	function viewDetail(request: GdprRequest) {
		selectedRequest = request;
		showDetailModal = true;
	}

	async function processRequest(request: GdprRequest) {
		processing = true;
		try {
			const result = await api(`/admin/gdpr/deletion-requests/${request.id}/execute`, { method: 'POST' });
			if (result.ok) {
				showDetailModal = false;
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			processing = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('gdpr.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('gdpr.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('gdpr.description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else if requests.length === 0}
	<EmptyState title={t('gdpr.empty')} description="No GDPR requests have been submitted yet." />
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">User</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Type</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Status</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Requested</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Completed</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
				</tr>
			</thead>
			<tbody>
				{#each requests as request}
					<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
						<td class="px-4 py-3">
							<div class="text-sm font-medium">{request.username ?? request.user_id}</div>
							{#if request.email}
								<div class="text-xs" style="color: var(--color-muted)">{request.email}</div>
							{/if}
						</td>
						<td class="px-4 py-3">
							<Badge variant={request.request_type === 'erasure' ? 'danger' : 'info'}>
								{request.request_type}
							</Badge>
						</td>
						<td class="px-4 py-3">
							<Badge variant={statusVariant(request.status)}>
								{request.status}
							</Badge>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{request.requested_at}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{request.completed_at ?? '—'}</td>
						<td class="px-4 py-3">
							<button
								class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
								style="color: var(--color-primary)"
								onclick={() => viewDetail(request)}
							>
								{t('common.view')}
							</button>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

{#if selectedRequest}
	<Modal
		bind:open={showDetailModal}
		title={selectedRequest.request_type === 'erasure' ? 'GDPR Erasure Request' : 'GDPR Data Export'}
		onConfirm={() => selectedRequest && processRequest(selectedRequest)}
		confirmLabel={processing ? t('common.processing') : selectedRequest.status === 'pending' ? 'Process Request' : t('common.close')}
	>
		<div class="space-y-3 text-sm">
			<div class="grid grid-cols-2 gap-3">
				<div>
					<p class="text-xs font-medium" style="color: var(--color-muted)">User</p>
					<p>{selectedRequest.username ?? selectedRequest.user_id}</p>
				</div>
				<div>
					<p class="text-xs font-medium" style="color: var(--color-muted)">Type</p>
					<p>{selectedRequest.request_type}</p>
				</div>
				<div>
					<p class="text-xs font-medium" style="color: var(--color-muted)">Status</p>
					<Badge variant={statusVariant(selectedRequest.status)}>
						{selectedRequest.status}
					</Badge>
				</div>
				<div>
					<p class="text-xs font-medium" style="color: var(--color-muted)">Requested</p>
					<p>{selectedRequest.requested_at}</p>
				</div>
			</div>
			{#if selectedRequest.export_url}
				<div>
					<p class="text-xs font-medium" style="color: var(--color-muted)">Export URL</p>
					<a href={selectedRequest.export_url} target="_blank" rel="noopener" class="text-xs underline" style="color: var(--color-primary)">
						Download Export
					</a>
				</div>
			{/if}
			{#if selectedRequest.request_type === 'erasure' && selectedRequest.status === 'pending'}
				<div class="rounded-md border p-3 text-xs" style="border-color: var(--color-warning); background-color: color-mix(in srgb, var(--color-warning) 8%, transparent); color: var(--color-warning)">
					Warning: Erasure requests permanently delete user data after the grace period. This action cannot be undone.
				</div>
			{/if}
		</div>
	</Modal>
{/if}
