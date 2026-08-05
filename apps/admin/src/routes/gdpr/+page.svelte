<script lang="ts">
	import { api, gdprAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { RefreshCw, Trash2, Play } from 'lucide-svelte';

	type DeletionRequest = {
		id: string;
		user_id: string;
		username?: string;
		display_name?: string;
		status: string;
		reason: string;
		requested_at: string;
		executed_at?: string;
	};

	let requests = $state<DeletionRequest[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let searchQuery = $state('');
	let showExecuteConfirm = $state(false);
	let executeTarget = $state<DeletionRequest | null>(null);
	let executing = $state(false);

	const canManageDeletions = $derived(hasPermission('gdpr.deletions.view'));

	const columns = [
		{ key: 'display_name', label: t('app.gdpr.erasure.user', 'User'), sortable: true,
			render: (v: string, row: DeletionRequest) => `<span class="text-sm">${v || row.username || 'anonymous'}</span>` },
		{ key: 'reason', label: t('app.gdpr.erasure.reason', 'Reason'), render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v || '—'}</span>` },
		{ key: 'status', label: t('app.gdpr.erasure.status', 'Status'), render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${v === 'executed' ? 'bg-success/15 text-success dark:text-success' : v === 'pending' ? 'bg-warning/15 text-warning dark:text-warning' : 'bg-muted/15 text-muted dark:text-muted'}">${v}</span>` },
		{ key: 'requested_at', label: t('app.gdpr.erasure.requested_at', 'Requested At'), render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleString()}</span>` },
		{ key: 'actions', label: t('common.actions', 'Actions'), class: 'w-24', componentSnippet: DeletionActionsCell }
	];

	const filteredRequests = $derived(
		searchQuery
			? requests.filter((r) => (r.display_name || r.username || '').toLowerCase().includes(searchQuery.toLowerCase()) || (r.reason || '').toLowerCase().includes(searchQuery.toLowerCase()))
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

	async function executeDeletion(req: DeletionRequest) {
		executeTarget = req;
		showExecuteConfirm = true;
	}

	async function confirmExecuteDeletion() {
		if (!executeTarget) return;
		executing = true;
		errorMessage = '';
		try {
			await api(`/admin/gdpr/deletion-requests/${executeTarget.id}/execute`, { method: 'POST', body: {} });
			showExecuteConfirm = false;
			executeTarget = null;
			load();
		} catch {
			errorMessage = t('common.error', 'Failed to execute deletion');
		} finally {
			executing = false;
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
		<Button variant="ghost" size="sm" onclick={() => executeDeletion(row)} style="color: var(--color-danger)">
			<Play class="h-3.5 w-3.5" />
			{t('app.gdpr.erasure.execute', 'Execute')}
		</Button>
	{/if}
{/snippet}

<!-- Execute deletion confirmation (per jc-admin-develop: destructive actions behind Modal) -->
<Modal bind:open={showExecuteConfirm} title={t('app.gdpr.erasure.execute_confirm_title', 'Confirm Data Erasure')} size="sm">
	{#if executeTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.gdpr.erasure.execute_confirm_body', 'This will permanently delete all personal data for user')}
			<strong class="font-mono">{executeTarget.user_id}</strong>.
			{t('app.gdpr.erasure.execute_confirm_warning', 'This action cannot be undone.')}
		</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showExecuteConfirm = false)} disabled={executing}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button variant="destructive" onclick={confirmExecuteDeletion} disabled={executing}>
			<Play class="mr-2 h-4 w-4" />
			{executing ? t('common.processing', 'Processing...') : t('app.gdpr.erasure.execute', 'Execute')}
		</Button>
	</div>
</Modal>
