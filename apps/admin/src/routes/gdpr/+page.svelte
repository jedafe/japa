<script lang="ts">
	import { api, gdprAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { RefreshCw, Trash2, Play } from 'lucide-svelte';

	type DeletionRequest = {
		id: string;
		user_id: string;
		status: string;
		reason: string;
		requested_at: string;
		executed_at?: string;
	};

	let requests = $state<DeletionRequest[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let searchQuery = $state('');

	const canManageDeletions = $derived(hasPermission('gdpr.deletions.view'));

	const columns = [
		{ key: 'user_id', label: t('app.gdpr.erasure.user', 'User ID'), sortable: true, render: (v: string) => `<span class="font-mono text-xs">${v}</span>` },
		{ key: 'reason', label: t('app.gdpr.erasure.reason', 'Reason'), render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v || '—'}</span>` },
		{ key: 'status', label: t('app.gdpr.erasure.status', 'Status'), render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${v === 'executed' ? 'bg-success/15 text-success dark:text-success' : v === 'pending' ? 'bg-warning/15 text-warning dark:text-warning' : 'bg-muted/15 text-muted dark:text-muted'}">${v}</span>` },
		{ key: 'requested_at', label: t('app.gdpr.erasure.requested_at', 'Requested At'), render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleString()}</span>` },
		{ key: 'actions', label: t('common.actions', 'Actions'), class: 'w-24', componentSnippet: DeletionActionsCell }
	];

	const filteredRequests = $derived(
		searchQuery
			? requests.filter((r) => r.user_id.toLowerCase().includes(searchQuery.toLowerCase()) || (r.reason || '').toLowerCase().includes(searchQuery.toLowerCase()))
			: requests
	);

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<DeletionRequest[]>('/admin/gdpr/deletion-requests');
			if (result.ok) {
				requests = result.data ?? [];
			} else {
				errorMessage = result.error ?? t('common.error', 'Failed to load deletion requests');
			}
		} catch {
			errorMessage = t('common.error', 'Failed to load deletion requests');
		} finally {
			loading = false;
		}
	}

	async function executeDeletion(id: string) {
		errorMessage = '';
		try {
			await api(`/admin/gdpr/deletion-requests/${id}/execute`, { method: 'POST', body: {} });
			load();
		} catch {
			errorMessage = t('common.error', 'Failed to execute deletion');
		}
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('app.gdpr.erasure.title', 'Data Erasure Requests')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('app.gdpr.erasure.title', 'Data Erasure Requests')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('app.gdpr.erasure.description', 'Manage user data deletion requests for GDPR compliance')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button class="btn btn-secondary flex items-center gap-1.5 text-xs" onclick={load}>
		<RefreshCw class="h-3 w-3" />
		{t('common.refresh', 'Refresh')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading', 'Loading...')}</p>
{:else}
	<DataTable
		columns={columns}
		data={filteredRequests}
		searchable={true}
		onsearch={(q: string) => { searchQuery = q; }}
		pageSize={25}
		emptyMessage={t('app.gdpr.erasure.empty', 'No deletion requests found')}
	/>
{/if}

{#snippet DeletionActionsCell({ row }: { value: any; row: DeletionRequest })}
	{#if row.status === 'pending' && canManageDeletions}
		<Button variant="ghost" size="sm" onclick={() => executeDeletion(row.id)} style="color: var(--color-danger)">
			<Play class="h-3.5 w-3.5" />
			{t('app.gdpr.erasure.execute', 'Execute')}
		</Button>
	{/if}
{/snippet}
