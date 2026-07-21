<script lang="ts">
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { RefreshCw, CheckCircle, XCircle } from 'lucide-svelte';

	type JoinRequest = {
		id: string;
		user_id: string;
		space_id: string;
		status: string;
		message?: string;
		requested_at: string;
	};

	let requests = $state<JoinRequest[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let searchQuery = $state('');
	let selectedRequest = $state<JoinRequest | null>(null);
	let showDetailModal = $state(false);
	let selectedIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	const canModerateJoins = $derived(hasPermission('spaces.moderate'));

	const columns = [
		{ key: 'user_id', label: t('joinRequests.user', 'User'), sortable: true },
		{ key: 'space_id', label: t('joinRequests.space', 'Space'), sortable: true },
		{ key: 'message', label: t('joinRequests.message', 'Message'), sortable: false,
			render: (v: string) => v || '—'
		},
		{ key: 'status', label: t('joinRequests.status', 'Status'), sortable: true,
			render: (v: string) => {
				const cls = v === 'approved' ? 'bg-success/15 text-success' : v === 'rejected' ? 'bg-danger/15 text-danger' : 'bg-warning/15 text-warning';
				return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${cls}">${v}</span>`;
			}
		},
		{ key: 'requested_at', label: t('joinRequests.requested', 'Requested'), sortable: true,
			render: (v: string) => new Date(v).toLocaleDateString()
		}
	];

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const params = new URLSearchParams();
			if (searchQuery.trim()) params.set('q', searchQuery.trim());
			const qs = params.toString();
			const result = await api<JoinRequest[]>(`/admin/join-requests${qs ? `?${qs}` : ''}`);
			if (result.ok) {
				requests = result.data ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function approveRequest(id: string) {
		errorMessage = '';
		try {
			await api(`/admin/join-requests/${id}/approve`, { method: 'POST', body: {} });
			load();
		} catch {
			errorMessage = t('common.error');
		}
	}

	async function rejectRequest(id: string) {
		errorMessage = '';
		try {
			await api(`/admin/join-requests/${id}/reject`, { method: 'POST', body: {} });
			load();
		} catch {
			errorMessage = t('common.error');
		}
	}

	async function bulkDelete() {
		if (selectedIds.size === 0) return;
		bulkDeleting = true;
		errorMessage = '';
		try {
			for (const id of selectedIds) {
				await api(`/admin/join-requests/${id}`, { method: 'DELETE' });
			}
			selectedIds = new Set();
			load();
		} catch {
			errorMessage = t('common.error');
		}
		bulkDeleting = false;
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('joinRequests.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('joinRequests.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('joinRequests.description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex items-center justify-end">
	<Button variant="ghost" size="sm" onclick={load}>
		<RefreshCw class="mr-1.5 h-3.5 w-3.5" />
		{t('common.refresh')}
	</Button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<DataTable
		{columns}
		data={requests}
		pageSize={25}
		searchable
		searchPlaceholder={t('joinRequests.search', 'Search by user or space...')}
		onsearch={(q) => { searchQuery = q; load(); }}
		onrowclick={(row) => { selectedRequest = row; showDetailModal = true; }}
		emptyMessage={t('joinRequests.empty', 'No join requests found')}
		selectable
		bind:selectedIds
	>
		{#snippet bulkActions()}
			<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting || selectedIds.size === 0}>
				<XCircle class="h-3.5 w-3.5 mr-1" />
				{bulkDeleting ? t('common.deleting', 'Deleting...') : t('common.deleteSelected', 'Delete Selected')} ({selectedIds.size})
			</Button>
		{/snippet}
	</DataTable>
{/if}

<Modal bind:open={showDetailModal} title={t('joinRequests.details', 'Join Request Details')}>
	{#if selectedRequest}
		<div class="space-y-4">
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('joinRequests.user')}</p>
					<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{selectedRequest.user_id}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('joinRequests.space')}</p>
					<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{selectedRequest.space_id}</p>
				</div>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('joinRequests.message')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{selectedRequest.message || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('joinRequests.status')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{selectedRequest.status}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('joinRequests.requested')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{new Date(selectedRequest.requested_at).toLocaleString()}</p>
			</div>
			{#if selectedRequest.status === 'pending' && canModerateJoins}
				<div class="flex gap-2 pt-4 border-t" style="border-color: var(--color-border)">
					<Button variant="default" size="sm" onclick={() => { const id = selectedRequest!.id; showDetailModal = false; selectedRequest = null; approveRequest(id); }}>
						<CheckCircle class="mr-1.5 h-4 w-4" />
						{t('joinRequests.approve')}
					</Button>
					<Button variant="destructive" size="sm" onclick={() => { const id = selectedRequest!.id; showDetailModal = false; selectedRequest = null; rejectRequest(id); }}>
						<XCircle class="mr-1.5 h-4 w-4" />
						{t('joinRequests.reject')}
					</Button>
				</div>
			{/if}
		</div>
	{/if}
</Modal>
