<script lang="ts">
	import FilterBar from '$lib/components/FilterBar.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';

	const canManage = $derived(hasPermission('admin.moderation'));

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
	let page = $state(0);
	let totalPages = $state(1);
	let loading = $state(true);
	let filterStatus = $state('');
	let errorMessage = $state('');

	// Expandable detail
	let expandedId = $state<string | null>(null);

	// Action modal
	let showActionModal = $state(false);
	let actionReport = $state<Report | null>(null);
	let actionType = $state<'dismiss' | 'warn' | 'ban' | 'remove'>('dismiss');
	let actionReason = $state('');
	let acting = $state(false);

	const filterDefs = [
		{ key: 'status', label: t('moderation.status'), type: 'select' as const, options: ['pending', 'reviewed', 'dismissed'] }
	];

	async function load() {
		loading = true;
		errorMessage = '';
		const params = new URLSearchParams({ page: String(page) });
		if (filterStatus) params.set('status', filterStatus);
		try {
			const result = await api<{ items: Report[]; page: number; total_pages: number }>(`/admin/reports?${params}`);
			if (result.ok) {
				reports = result.data.items;
				totalPages = result.data.total_pages;
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
		filterStatus = values.status ?? '';
		page = 0;
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

	function handlePageChange(p: number) {
		page = p;
		load();
	}

	function statusBadge(status: string) {
		switch (status) {
			case 'pending': return 'bg-warning/15 text-warning dark:text-warning';
			case 'reviewed': return 'bg-blue-500/15 text-info dark:text-blue-400';
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

	$effect(() => {
		load();
	});
</script>

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
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderation.reporter')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderation.target')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderation.reason')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderation.status')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderation.date')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
				</tr>
			</thead>
			<tbody>
				{#each reports as report}
					<tr
						class="border-t cursor-pointer hover:bg-black/[0.02] dark:hover:bg-surface/[0.02]"
						style="border-color: var(--color-border)"
						onclick={() => toggleExpand(report.id)}
					>
						<td class="px-4 py-3">
							<p class="font-medium">{report.reporter_name}</p>
							<p class="text-xs" style="color: var(--color-muted)">{report.reporter_id.slice(0, 12)}</p>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">
							{report.target_content_type} &middot; {report.target_content_id.slice(0, 10)}
						</td>
						<td class="px-4 py-3 text-xs max-w-xs truncate">{report.reason}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {statusBadge(report.status)}">
								{report.status}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{report.created_at}</td>
						<td class="px-4 py-3" onclick={(e) => e.stopPropagation()}>
							{#if canManage && report.status === 'pending'}
								<div class="flex items-center gap-1">
									<button
										class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10"
										style="color: var(--color-muted)"
										onclick={() => openAction(report, 'dismiss')}
									>
										Dismiss
									</button>
									<button
										class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10"
										style="color: var(--color-warning)"
										onclick={() => openAction(report, 'warn')}
									>
										Warn
									</button>
									<button
										class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10"
										style="color: var(--color-danger)"
										onclick={() => openAction(report, 'remove')}
									>
										Remove
									</button>
								</div>
							{/if}
						</td>
					</tr>
					{#if expandedId === report.id}
						<tr style="border-color: var(--color-border)">
							<td colspan="6" class="px-4 py-4" style="background-color: var(--color-surface)">
								<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
									<div>
										<p class="text-xs font-medium" style="color: var(--color-muted)">{t('moderation.reports.reporter')}</p>
										<p class="mt-1 text-sm">{report.reporter_name}</p>
										{#if report.reporter_email}
											<p class="text-xs" style="color: var(--color-muted)">{report.reporter_email}</p>
										{/if}
									</div>
									<div>
										<p class="text-xs font-medium" style="color: var(--color-muted)">{t('moderation.reports.target_content')}</p>
										<p class="mt-1 text-sm">{report.target_content_type}</p>
										<p class="text-xs font-mono" style="color: var(--color-muted)">{report.target_content_id}</p>
									</div>
									<div>
										<p class="text-xs font-medium" style="color: var(--color-muted)">{t('moderation.reports.report_reason')}</p>
										<p class="mt-1 text-sm">{report.reason}</p>
									</div>
								</div>
								{#if report.content_preview}
									<div class="mt-3 rounded-md border p-3 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)">
										<p class="text-xs font-medium mb-1" style="color: var(--color-muted)">{t('moderation.reports.content_preview')}</p>
										{report.content_preview}
									</div>
								{/if}
								{#if canManage && report.status === 'pending'}
									<div class="mt-3 flex gap-2">
										<button
											class="rounded-md px-3 py-1.5 text-xs font-medium"
											style="background-color: var(--color-surface); border: 1px solid var(--color-border)"
											onclick={() => openAction(report, 'dismiss')}
										>
											Dismiss Report
										</button>
										<button
											class="rounded-md px-3 py-1.5 text-xs font-medium text-white"
											style="background-color: var(--color-warning)"
											onclick={() => openAction(report, 'warn')}
										>
											Warn Reporter
										</button>
										<button
											class="rounded-md px-3 py-1.5 text-xs font-medium text-white"
											style="background-color: var(--color-danger)"
											onclick={() => openAction(report, 'remove')}
										>
											Remove Content
										</button>
										<button
											class="rounded-md px-3 py-1.5 text-xs font-medium text-white"
											style="background-color: var(--color-danger)"
											onclick={() => openAction(report, 'ban')}
										>
											Ban Target
										</button>
									</div>
								{/if}
							</td>
						</tr>
					{/if}
				{:else}
					<tr>
						<td colspan="6" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							No reports found
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>

	<div class="mt-3 flex items-center justify-between">
		<p class="text-xs" style="color: var(--color-muted)">Page {page + 1} of {totalPages}</p>
		<div class="flex gap-1">
			<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-surface/10 disabled:opacity-40" disabled={page === 0} onclick={() => handlePageChange(page - 1)}>&larr;</button>
			<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-surface/10 disabled:opacity-40" disabled={page >= totalPages - 1} onclick={() => handlePageChange(page + 1)}>&rarr;</button>
		</div>
	</div>
{/if}

<!-- Action Modal -->
<Modal
	bind:open={showActionModal}
	title={actionLabel(actionType)}
	onConfirm={executeAction}
	confirmLabel={acting ? t('common.saving') : actionLabel(actionType)}
>
	<p class="text-sm mb-3" style="color: var(--color-muted)">
		{#if actionType === 'dismiss'}
			Dismiss this report and mark it as reviewed.
		{:else if actionType === 'warn'}
			Send a warning to the reporter for misuse.
		{:else if actionType === 'ban'}
			Ban the reported user from the platform.
		{:else}
			Permanently remove the reported content.
		{/if}
	</p>
	<textarea
		class="w-full rounded-md border px-3 py-2 text-sm"
		style="border-color: var(--color-border); background-color: var(--color-surface)"
		rows="3"
		placeholder="Reason (optional)"
		bind:value={actionReason}
	></textarea>
</Modal>
