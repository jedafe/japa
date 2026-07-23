<script lang="ts">
	import FilterBar from '$lib/components/FilterBar.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';

	const canManage = $derived(hasPermission('reports.review'));

	type Report = {
		id: string;
		reporter_id: string;
		reporter_name: string;
		target_content_type: string;
		target_content_id: string;
		reason: string;
		status: string;
		created_at: string;
		reporter_email?: string;
		content_preview?: string;
	};

	let reports = $state<Report[]>([]);
	let loading = $state(true);
	let filterStatus = $state('');
	let searchQuery = $state('');
	let errorMessage = $state('');

	let expandedId = $state<string | null>(null);

	let showActionModal = $state(false);
	let actionReport = $state<Report | null>(null);
	let actionType = $state<'dismiss' | 'warn' | 'ban' | 'remove'>('dismiss');
	let actionReason = $state('');
	let acting = $state(false);

	const filterDefs = [
		{ key: 'status', label: t('moderation.status'), type: 'select' as const, options: ['pending', 'reviewing', 'actioned', 'dismissed'] }
	];

	function statusBadge(status: string) {
		switch (status) {
			case 'pending': return 'bg-warning/15 text-warning dark:text-warning';
			case 'reviewing': return 'bg-info/15 text-info dark:text-info';
			case 'actioned': return 'bg-success/15 text-success dark:text-success';
			case 'dismissed': return 'bg-muted/15 text-muted dark:text-muted';
			default: return 'bg-muted/15 text-muted dark:text-muted';
		}
	}

	function actionLabel(type: string) {
		switch (type) {
			case 'dismiss': return t('moderation.dismiss');
			case 'warn': return t('moderation.warn');
			case 'ban': return t('moderation.ban');
			case 'remove': return t('moderation.remove_content');
			default: return type;
		}
	}

	function actionColor(type: string) {
		switch (type) {
			case 'dismiss': return 'var(--color-muted)';
			case 'warn': return 'var(--color-warning)';
			case 'ban': return 'var(--color-danger)';
			case 'remove': return 'var(--color-danger)';
			default: return 'var(--color-muted)';
		}
	}

	const columns = [
		{
			key: 'reporter_name',
			label: t('moderation.reporter'),
			sortable: true,
			render: (_: any, row: Report) => `<p class="font-medium">${row.reporter_name}</p><p class="text-xs" style="color: var(--color-muted)">${row.reporter_id.slice(0, 12)}</p>`
		},
		{
			key: 'target_content_type',
			label: t('moderation.target'),
			sortable: true,
			render: (_: any, row: Report) => `<span class="text-xs" style="color: var(--color-muted)">${row.target_content_type} &middot; ${row.target_content_id.slice(0, 10)}</span>`
		},
		{
			key: 'reason',
			label: t('moderation.reason'),
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
			class: 'w-32',
			component: ReportActionsCell
		}
	];

	async function load() {
		loading = true;
		errorMessage = '';
		const params = new URLSearchParams();
		if (filterStatus) params.set('status', filterStatus);
		if (searchQuery) params.set('q', searchQuery);
		try {
			const result = await api<{ items: Report[] }>(`/admin/reports?${params}`);
			if (result.ok) {
				reports = result.data.items;
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

	function openAction(report: Report, action: 'dismiss' | 'warn' | 'ban' | 'remove') {
		actionReport = report;
		actionType = action;
		actionReason = '';
		showActionModal = true;
	}

	async function executeAction() {
		if (!actionReport) return;
		acting = true;
		errorMessage = '';
		try {
			const body = { action: actionType, ...(actionReason.trim() ? { reason: actionReason.trim() } : {}) };
			const result = await api(`/admin/reports/${actionReport.id}`, { method: 'PATCH', body });
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

{#snippet ReportActionsCell({ value, row }: { value: any; row: Report })}
	{#if canManage && row.status === 'pending'}
		<div class="flex items-center gap-1">
			<button
				class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10"
				style="color: var(--color-muted)"
				onclick={(e: MouseEvent) => { e.stopPropagation(); openAction(row, 'dismiss'); }}
			>
				{t('moderation.dismiss')}
			</button>
			<button
				class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10"
				style="color: var(--color-warning)"
				onclick={(e: MouseEvent) => { e.stopPropagation(); openAction(row, 'warn'); }}
			>
				{t('moderation.warn')}
			</button>
			<button
				class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10"
				style="color: var(--color-danger)"
				onclick={(e: MouseEvent) => { e.stopPropagation(); openAction(row, 'remove'); }}
			>
				{t('moderation.remove_content')}
			</button>
		</div>
	{/if}
{/snippet}

<svelte:head>
	<title>{t('moderation.reports')} — JapaConnect Admin</title>
</svelte:head>

<div class="mb-4">
	<a href="/moderation" class="text-xs hover:underline" style="color: var(--color-muted)">&larr; {t('moderation.back')}</a>
</div>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('moderation.reports')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('moderation.reports_description')}</p>

<FilterBar filters={filterDefs} onApply={handleFilter} />

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<DataTable {columns} data={reports} pageSize={25} searchable emptyMessage="No reports found" onrowclick={(row) => toggleExpand(row.id)} />

	{#if expandedId}
		{@const expanded = reports.find((r) => r.id === expandedId)}
		{#if expanded}
			<div class="mt-3 rounded-lg border px-4 py-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
				<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
					<div>
						<p class="text-xs font-medium" style="color: var(--color-muted)">{t('moderation.reports.reporter')}</p>
						<p class="mt-1 text-sm">{expanded.reporter_name}</p>
						{#if expanded.reporter_email}
							<p class="text-xs" style="color: var(--color-muted)">{expanded.reporter_email}</p>
						{/if}
					</div>
					<div>
						<p class="text-xs font-medium" style="color: var(--color-muted)">{t('moderation.reports.target_content')}</p>
						<p class="mt-1 text-sm">{expanded.target_content_type}</p>
						<p class="text-xs font-mono" style="color: var(--color-muted)">{expanded.target_content_id}</p>
					</div>
					<div>
						<p class="text-xs font-medium" style="color: var(--color-muted)">{t('moderation.reports.report_reason')}</p>
						<p class="mt-1 text-sm">{expanded.reason}</p>
					</div>
				</div>
				{#if expanded.content_preview}
					<div class="mt-3 rounded-md border p-3 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)">
						<p class="text-xs font-medium mb-1" style="color: var(--color-muted)">{t('moderation.reports.content_preview')}</p>
						{expanded.content_preview}
					</div>
				{/if}
				{#if canManage && expanded.status === 'pending'}
					<div class="mt-3 flex gap-2">
						<button
							class="rounded-md px-3 py-1.5 text-xs font-medium"
							style="background-color: var(--color-surface); border: 1px solid var(--color-border)"
							onclick={() => openAction(expanded, 'dismiss')}
						>
							{t('moderation.dismiss_report')}
						</button>
						<button
							class="rounded-md px-3 py-1.5 text-xs font-medium text-white"
							style="background-color: var(--color-warning)"
							onclick={() => openAction(expanded, 'warn')}
						>
							{t('moderation.warn_reporter')}
						</button>
						<button
							class="rounded-md px-3 py-1.5 text-xs font-medium text-white"
							style="background-color: var(--color-danger)"
							onclick={() => openAction(expanded, 'remove')}
						>
							{t('moderation.remove_content')}
						</button>
						<button
							class="rounded-md px-3 py-1.5 text-xs font-medium text-white"
							style="background-color: var(--color-danger)"
							onclick={() => openAction(expanded, 'ban')}
						>
							{t('moderation.ban_target')}
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
	title={actionLabel(actionType)}
	onConfirm={executeAction}
	confirmLabel={acting ? t('common.saving') : actionLabel(actionType)}
>
	<p class="text-sm mb-3" style="color: var(--color-muted)">
		{t(`moderation.action_${actionType}_desc`)}
	</p>
	<textarea
		class="w-full rounded-md border px-3 py-2 text-sm"
		style="border-color: var(--color-border); background-color: var(--color-surface)"
		rows="3"
		placeholder="Reason (optional)"
		bind:value={actionReason}
	></textarea>
</Modal>
