<script lang="ts">
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, CheckCircle, XCircle } from 'lucide-svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';

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

	let spaceId = $derived(page.params.id);

	const columns = [
		{ key: 'user_id', label: t('joinRequests.user'), sortable: true, class: 'font-mono text-xs' },
		{ key: 'message', label: t('joinRequests.message'), sortable: true, class: 'text-xs', render: (v: string) => `<span style="color: var(--color-muted)">${v || '—'}</span>` },
		{ key: 'status', label: t('joinRequests.status'), sortable: true, render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${v === 'approved' ? 'bg-success/15 text-success dark:text-success' : v === 'rejected' ? 'bg-danger/15 text-danger dark:text-danger' : 'bg-warning/15 text-warning dark:text-warning'}">${v}</span>` },
		{ key: 'requested_at', label: t('joinRequests.requested'), sortable: true, class: 'text-xs', render: (v: string) => `<span style="color: var(--color-muted)">${new Date(v).toLocaleDateString()}</span>` },
		{ key: 'actions', label: t('common.actions'), class: 'w-32 text-right', componentSnippet: JoinRequestActionsCell }
	];

	function filteredRequests(): JoinRequest[] {
		const q = searchQuery.toLowerCase().trim();
		if (!q) return requests;
		return requests.filter(r =>
			r.user_id.toLowerCase().includes(q) ||
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

{#snippet JoinRequestActionsCell({ value, row }: { value: any; row: JoinRequest })}
	{#if row.status === 'pending'}
		<div class="flex items-center justify-end gap-1">
			<Button variant="ghost" size="sm" onclick={() => updateRequest(row.id, 'approved')}>
				<CheckCircle class="h-3 w-3 inline mr-1" style="color: var(--color-success)" />
				{t('joinRequests.approve')}
			</Button>
			<Button variant="ghost" size="sm" onclick={() => updateRequest(row.id, 'rejected')}>
				<XCircle class="h-3 w-3 inline mr-1" style="color: var(--color-danger)" />
				{t('joinRequests.reject')}
			</Button>
		</div>
	{/if}
{/snippet}

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

<div class="mb-4 flex justify-end">
	<button class="btn btn-secondary flex items-center gap-1.5 text-xs" onclick={load}>
		<RefreshCw class="h-3 w-3" />
		{t('common.refresh')}
	</button>
</div>

<DataTable
	columns={columns}
	data={filteredRequests()}
	{loading}
	pageSize={25}
	emptyMessage={t('joinRequests.empty')}
	searchable
	searchPlaceholder={t('common.search')}
	onsearch={(q) => { searchQuery = q; }}
/>
