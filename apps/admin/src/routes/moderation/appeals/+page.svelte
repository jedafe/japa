<script lang="ts">
	import FilterBar from '$lib/components/FilterBar.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';

	const canManage = $derived(hasPermission('reports.review'));

	type Appeal = {
		id: string;
		user_id: string;
		username: string;
		original_action: string;
		original_reason: string;
		appeal_reason: string;
		status: string;
		created_at: string;
		reviewed_at?: string;
	};

	let appeals = $state<Appeal[]>([]);
	let loading = $state(true);
	let filterStatus = $state('');
	let searchQuery = $state('');
	let errorMessage = $state('');

	let expandedId = $state<string | null>(null);

	let showActionModal = $state(false);
	let actionAppeal = $state<Appeal | null>(null);
	let actionType = $state<'approve' | 'deny'>('approve');
	let actionNote = $state('');
	let acting = $state(false);

	const filterDefs = [
		{ key: 'status', label: t('moderation.status'), type: 'select' as const, options: ['pending', 'approved', 'denied'] }
	];

	function statusBadge(status: string) {
		switch (status) {
			case 'pending': return 'bg-warning/15 text-warning dark:text-warning';
			case 'approved': return 'bg-success/15 text-success dark:text-success';
			case 'denied': return 'bg-danger/15 text-danger dark:text-danger';
			default: return 'bg-muted/15 text-muted dark:text-muted';
		}
	}

	function actionBadgeColor(action: string) {
		switch (action) {
			case 'ban': return 'bg-danger/15 text-danger dark:text-danger';
			case 'suspend': return 'bg-warning/15 text-warning dark:text-warning';
			case 'remove_content': return 'bg-warning/15 text-warning dark:text-warning';
			default: return 'bg-muted/15 text-muted dark:text-muted';
		}
	}

	const columns = [
		{
			key: 'username',
			label: t('moderation.user'),
			sortable: true,
			render: (_: any, row: Appeal) => `<p class="font-medium">${row.username}</p><p class="text-xs" style="color: var(--color-muted)">${row.user_id.slice(0, 12)}</p>`
		},
		{
			key: 'original_action',
			label: t('moderation.original_action'),
			sortable: true,
			render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs ${actionBadgeColor(v)}">${v}</span>`
		},
		{
			key: 'appeal_reason',
			label: t('moderation.appeal_reason'),
			render: (v: string) => `<span class="text-xs max-w-xs truncate block">${v}</span>`
		},
		{
			key: 'status',
			label: t('moderation.status'),
			sortable: true,
			render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs ${statusBadge(v)}">${v}</span>`
		},
		{
			key: 'created_at',
			label: t('moderation.date'),
			sortable: true,
			render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v}</span>`
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			componentSnippet: AppealActionsCell
		}
	];

	async function load() {
		loading = true;
		errorMessage = '';
		const params = new URLSearchParams();
		if (filterStatus) params.set('status', filterStatus);
		if (searchQuery) params.set('q', searchQuery);
		try {
			const result = await api<{ items: Appeal[] }>(`/admin/appeals?${params}`);
			if (result.ok) {
				appeals = result.data?.items ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	function handleFilter(values: Record<string, string>) {
		searchQuery = values.search ?? '';
		filterStatus = values.status ?? '';
		load();
	}

	function toggleExpand(id: string) {
		expandedId = expandedId === id ? null : id;
	}

	function openAction(appeal: Appeal, type: 'approve' | 'deny') {
		actionAppeal = appeal;
		actionType = type;
		actionNote = '';
		showActionModal = true;
	}

	async function executeAction() {
		if (!actionAppeal) return;
		acting = true;
		errorMessage = '';
		try {
			const endpoint = `/admin/appeals/${actionAppeal.id}`;
			const body: Record<string, unknown> = { status: actionType === 'approve' ? 'approved' : 'denied' };
			if (actionNote.trim()) body.note = actionNote.trim();
			const result = await api(endpoint, { method: 'PATCH', body });
			if (result.ok) {
				showActionModal = false;
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			acting = false;
		}
	}

	$effect(() => {
		load();
	});
</script>

{#snippet AppealActionsCell({ value, row }: { value: any; row: Appeal })}
	{#if canManage && row.status === 'pending'}
		<div class="flex items-center gap-1">
			<button
				class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10"
				style="color: var(--color-success)"
				onclick={(e: MouseEvent) => { e.stopPropagation(); openAction(row, 'approve'); }}
			>
				{t('moderation.approve')}
			</button>
			<button
				class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10"
				style="color: var(--color-danger)"
				onclick={(e: MouseEvent) => { e.stopPropagation(); openAction(row, 'deny'); }}
			>
				{t('moderation.deny')}
			</button>
		</div>
	{/if}
{/snippet}

<svelte:head>
	<title>{t('moderation.appeals')} — JapaConnect Admin</title>
</svelte:head>

<div class="mb-4">
	<a href="/moderation" class="text-xs hover:underline" style="color: var(--color-muted)">&larr; {t('moderation.back')}</a>
</div>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('moderation.appeals')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('moderation.appeals_description')}</p>

<FilterBar filters={filterDefs} onApply={handleFilter} />

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<DataTable {columns} data={appeals} pageSize={25} searchable emptyMessage="No appeals found" onrowclick={(row) => toggleExpand(row.id)} />

	{#if expandedId}
		{@const expanded = appeals.find((a) => a.id === expandedId)}
		{#if expanded}
			<div class="mt-3 rounded-lg border px-4 py-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
				<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
					<div>
						<p class="text-xs font-medium" style="color: var(--color-muted)">{t('moderation.appeals.original_action')}</p>
						<p class="mt-1">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {actionBadgeColor(expanded.original_action)}">
								{expanded.original_action}
							</span>
						</p>
						<p class="mt-1 text-sm" style="color: var(--color-muted)">{expanded.original_reason || 'No reason provided'}</p>
					</div>
					<div>
						<p class="text-xs font-medium" style="color: var(--color-muted)">{t('moderation.appeals.appeal_reason')}</p>
						<p class="mt-1 text-sm">{expanded.appeal_reason}</p>
					</div>
				</div>
				{#if expanded.reviewed_at}
					<p class="mt-3 text-xs" style="color: var(--color-muted)">Reviewed at: {expanded.reviewed_at}</p>
				{/if}
				{#if canManage && expanded.status === 'pending'}
					<div class="mt-3 flex gap-2">
						<button
							class="rounded-md px-3 py-1.5 text-xs font-medium text-white"
							style="background-color: var(--color-success)"
							onclick={() => openAction(expanded, 'approve')}
						>
							{t('moderation.approve_appeal')}
						</button>
						<button
							class="rounded-md px-3 py-1.5 text-xs font-medium text-white"
							style="background-color: var(--color-danger)"
							onclick={() => openAction(expanded, 'deny')}
						>
							{t('moderation.deny_appeal')}
						</button>
					</div>
				{/if}
			</div>
		{/if}
	{/if}
{/if}

<!-- Action Modal -->
<Modal
	bind:open={showActionModal}
	title={actionType === 'approve' ? t('moderation.approve_appeal') : t('moderation.deny_appeal')}
	onConfirm={executeAction}
	confirmLabel={acting ? t('common.saving') : actionType === 'approve' ? t('moderation.approve') : t('moderation.deny')}
>
	<p class="text-sm mb-3" style="color: var(--color-muted)">
		{actionType === 'approve'
			? t('moderation.approve_appeal_desc')
			: t('moderation.deny_appeal_desc')}
	</p>
	<textarea
		class="w-full rounded-md border px-3 py-2 text-sm"
		style="border-color: var(--color-border); background-color: var(--color-surface)"
		rows="3"
		placeholder="Note (optional)"
		bind:value={actionNote}
	></textarea>
</Modal>
