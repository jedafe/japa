<script lang="ts">
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { RefreshCw, CheckCircle, XCircle } from 'lucide-svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import PageHeader from '$lib/components/layout/page-header.svelte';

	// Ref: jc-admin-develop — RBAC gate for join request management
	const canManage = $derived(hasPermission('space.join_requests.manage'));

	type JoinRequest = {
		id: string;
		user_id: string;
		space_id: string;
		status: string;
		message?: string;
		requested_at: string;
		username?: string;
		display_name?: string;
	};

	let requests = $state<JoinRequest[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	let searchQuery = $state('');

	let spaceId = $derived(page.params.id);

	const columns = [
		{ key: 'display_name', label: t('joinRequests.user'), sortable: true,
			render: (v: string, row: JoinRequest) => v || row.username || 'anonymous' },
		{ key: 'message', label: t('joinRequests.message'), sortable: true, class: 'text-xs', render: (v: string) => `<span style="color: var(--color-muted)">${v || '—'}</span>` },
		{ key: 'status', label: t('joinRequests.status'), sortable: true, render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${v === 'approved' ? 'bg-success/15 text-success dark:text-success' : v === 'rejected' ? 'bg-danger/15 text-danger dark:text-danger' : 'bg-warning/15 text-warning dark:text-warning'}">${v}</span>` },
		{ key: 'requested_at', label: t('joinRequests.requested'), sortable: true, class: 'text-xs', render: (v: string) => `<span style="color: var(--color-muted)">${new Date(v).toLocaleDateString()}</span>` }
	];

	const rowActions: ActionItem[] = [
		{
			label: t('joinRequests.approve'),
			icon: CheckCircle,
			onclick: (row: JoinRequest) => updateRequest(row.id, 'approved'),
			show: (row: JoinRequest) => canManage && row.status === 'pending'
		},
		{
			label: t('joinRequests.reject'),
			icon: XCircle,
			variant: 'destructive',
			onclick: (row: JoinRequest) => updateRequest(row.id, 'rejected'),
			show: (row: JoinRequest) => canManage && row.status === 'pending'
		}
	];

	function filteredRequests(): JoinRequest[] {
		const q = searchQuery.toLowerCase().trim();
		if (!q) return requests;
		return requests.filter(r =>
			(r.display_name || r.username || '').toLowerCase().includes(q) ||
			r.status.toLowerCase().includes(q) ||
			(r.message ?? '').toLowerCase().includes(q)
		);
	}

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<JoinRequest[]>(`/admin/spaces/${spaceId}/join-requests`);
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

	async function updateRequest(requestId: string, status: string) {
		errorMessage = '';
		try {
			await api(`/admin/spaces/${spaceId}/join-requests/${requestId}`, {
				method: 'PATCH',
				body: { status }
			});
			load();
		} catch {
			errorMessage = t('common.error');
		}
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('joinRequests.title')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('joinRequests.title')}
	description={t('joinRequests.description')}
	actions={[{ label: t('common.refresh'), icon: RefreshCw, variant: 'outline' as const, onclick: load }]}
/>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<DataTable
	columns={columns}
	data={filteredRequests()}
	{loading}
	pageSize={25}
	emptyMessage={t('joinRequests.empty')}
	searchable
	searchPlaceholder={t('common.search')}
	onsearch={(q) => { searchQuery = q; }}
	actions={rowActions}
/>
