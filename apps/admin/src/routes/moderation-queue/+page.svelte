<script lang="ts">
	import { moderationAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, CheckCircle, XCircle, ShieldAlert, Users, MessageSquare, AlertTriangle } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';

	type ModerationQueueItem = {
		id: string;
		content_type: string;
		content_id: string;
		status: string;
		reason?: string;
		reported_at: string;
		assignee_id?: string;
		assignee_name?: string;
	};

	type ModerationAlert = {
		id: string;
		type: string;
		message: string;
		severity: string;
		acknowledged: boolean;
		created_at: string;
	};

	let items = $state<ModerationQueueItem[]>([]);
	let alerts = $state<ModerationAlert[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let activeTab = $state<'queue' | 'alerts'>('queue');

	// Bulk selection
	let selectedIds = $state<Set<string>>(new Set());
	let bulkLoading = $state(false);

	// Assign modal
	let showAssignModal = $state(false);
	let assignItemId = $state('');
	let assigneeId = $state('');
	let assignLoading = $state(false);

	// Appeal modal
	let showAppealModal = $state(false);
	let appealItemId = $state('');
	let appealReason = $state('');
	let appealLoading = $state(false);

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			if (activeTab === 'queue') {
				const result = await moderationAdminApi.getQueue();
				if (result.ok) {
					items = (result.data as ModerationQueueItem[]) ?? [];
				} else {
					errorMessage = result.error;
				}
			} else {
				const result = await moderationAdminApi.getAlerts();
				if (result.ok) {
					alerts = (result.data as ModerationAlert[]) ?? [];
				} else {
					errorMessage = result.error;
				}
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function approveItem(id: string) {
		errorMessage = '';
		try {
			await moderationAdminApi.updateItem(id, { status: 'approved' });
			load();
		} catch {
			errorMessage = t('common.error');
		}
	}

	async function rejectItem(id: string) {
		errorMessage = '';
		try {
			await moderationAdminApi.updateItem(id, { status: 'rejected' });
			load();
		} catch {
			errorMessage = t('common.error');
		}
	}

	function toggleSelect(id: string) {
		const next = new Set(selectedIds);
		if (next.has(id)) {
			next.delete(id);
		} else {
			next.add(id);
		}
		selectedIds = next;
	}

	function toggleSelectAll() {
		if (selectedIds.size === items.length) {
			selectedIds = new Set();
		} else {
			selectedIds = new Set(items.map((i) => i.id));
		}
	}

	async function bulkAction(action: 'approve' | 'reject') {
		if (selectedIds.size === 0) return;
		bulkLoading = true;
		errorMessage = '';
		try {
			await moderationAdminApi.bulkAction({
				ids: Array.from(selectedIds),
				action
			});
			selectedIds = new Set();
			load();
		} catch {
			errorMessage = t('common.error');
		} finally {
			bulkLoading = false;
		}
	}

	function openAssignModal(id: string) {
		assignItemId = id;
		assigneeId = '';
		showAssignModal = true;
	}

	async function assignModerator() {
		if (!assigneeId.trim()) return;
		assignLoading = true;
		errorMessage = '';
		try {
			await moderationAdminApi.assignItem(assignItemId, { assignee_id: assigneeId.trim() });
			showAssignModal = false;
			load();
		} catch {
			errorMessage = t('common.error');
		} finally {
			assignLoading = false;
		}
	}

	function openAppealModal(id: string) {
		appealItemId = id;
		appealReason = '';
		showAppealModal = true;
	}

	async function submitAppeal() {
		if (!appealReason.trim()) return;
		appealLoading = true;
		errorMessage = '';
		try {
			await moderationAdminApi.appealItem(appealItemId, { reason: appealReason.trim() });
			showAppealModal = false;
			load();
		} catch {
			errorMessage = t('common.error');
		} finally {
			appealLoading = false;
		}
	}

	async function acknowledgeAlert(id: string) {
		errorMessage = '';
		try {
			await moderationAdminApi.acknowledgeAlert(id);
			load();
		} catch {
			errorMessage = t('common.error');
		}
	}

	async function resolveAlert(id: string) {
		errorMessage = '';
		try {
			await moderationAdminApi.resolveAlert(id, { resolution: 'resolved' });
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
	<title>{t('moderationQueue.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('moderationQueue.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('moderationQueue.description')}</p>

<!-- Tabs -->
<div class="mb-6 flex gap-1 rounded-lg border p-1" style="border-color: var(--color-border)">
	<button
		class="flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'queue' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'queue'; selectedIds = new Set(); load(); }}
	>
		<MessageSquare class="h-4 w-4" />
		{t('moderationQueue.tab_queue')}
	</button>
	<button
		class="flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'alerts' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'alerts'; load(); }}
	>
		<AlertTriangle class="h-4 w-4" />
		{t('moderationQueue.tab_alerts')}
	</button>
</div>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex items-center justify-between">
	<div class="flex items-center gap-2">
		{#if activeTab === 'queue' && selectedIds.size > 0}
			<span class="text-xs" style="color: var(--color-muted)">{selectedIds.size} selected</span>
			<button
				class="flex items-center gap-1 rounded px-2 py-1 text-xs font-medium hover:bg-black/5 dark:hover:bg-surface/10 disabled:opacity-50"
				style="color: var(--color-success)"
				disabled={bulkLoading}
				onclick={() => bulkAction('approve')}
			>
				<CheckCircle class="h-3 w-3" />
				{t('moderationQueue.bulk_approve')}
			</button>
			<button
				class="flex items-center gap-1 rounded px-2 py-1 text-xs font-medium hover:bg-black/5 dark:hover:bg-surface/10 disabled:opacity-50"
				style="color: var(--color-danger)"
				disabled={bulkLoading}
				onclick={() => bulkAction('reject')}
			>
				<XCircle class="h-3 w-3" />
				{t('moderationQueue.bulk_reject')}
			</button>
		{/if}
	</div>
	<button class="btn btn-secondary flex items-center gap-1.5 text-xs" onclick={load}>
		<RefreshCw class="h-3 w-3" />
		{t('common.refresh')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else if activeTab === 'queue'}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">
						<input type="checkbox" checked={selectedIds.size === items.length && items.length > 0} onchange={toggleSelectAll} class="h-3.5 w-3.5 rounded" />
					</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderationQueue.type')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderationQueue.content_id')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderationQueue.reason')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderationQueue.assignee')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderationQueue.reported')}</th>
					<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each items as item (item.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3">
							<input type="checkbox" checked={selectedIds.has(item.id)} onchange={() => toggleSelect(item.id)} class="h-3.5 w-3.5 rounded" />
						</td>
						<td class="px-4 py-3 text-xs">{item.content_type}</td>
						<td class="px-4 py-3 text-xs font-mono" style="color: var(--color-muted)">{item.content_id.slice(0, 12)}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{item.reason || '—'}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{item.assignee_name || '—'}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(item.reported_at).toLocaleDateString()}</td>
						<td class="px-4 py-3 text-right">
							<div class="flex items-center justify-end gap-1">
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10" onclick={() => approveItem(item.id)} title={t('moderationQueue.approve')}>
									<CheckCircle class="h-3 w-3" style="color: var(--color-success)" />
								</button>
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10" onclick={() => rejectItem(item.id)} title={t('moderationQueue.reject')}>
									<XCircle class="h-3 w-3" style="color: var(--color-danger)" />
								</button>
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10" onclick={() => openAssignModal(item.id)} title={t('moderationQueue.assign')}>
									<Users class="h-3 w-3" style="color: var(--color-info)" />
								</button>
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10" onclick={() => openAppealModal(item.id)} title={t('moderationQueue.appeal')}>
									<ShieldAlert class="h-3 w-3" style="color: var(--color-warning)" />
								</button>
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="7" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('moderationQueue.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{:else if activeTab === 'alerts'}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderationQueue.alert_type')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderationQueue.message')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderationQueue.severity')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderationQueue.created')}</th>
					<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each alerts as alert (alert.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 text-xs font-medium">{alert.type}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{alert.message}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {alert.severity === 'high' ? 'bg-danger/15 text-danger dark:text-danger' : alert.severity === 'medium' ? 'bg-warning/15 text-warning dark:text-warning' : 'bg-muted/15 text-muted dark:text-muted'}">
								{alert.severity}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(alert.created_at).toLocaleDateString()}</td>
						<td class="px-4 py-3 text-right">
							<div class="flex items-center justify-end gap-1">
								{#if !alert.acknowledged}
									<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10" style="color: var(--color-info)" onclick={() => acknowledgeAlert(alert.id)}>
										{t('moderationQueue.acknowledge')}
									</button>
								{/if}
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10" style="color: var(--color-success)" onclick={() => resolveAlert(alert.id)}>
									{t('moderationQueue.resolve')}
								</button>
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('moderationQueue.no_alerts')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<!-- Assign Moderator Modal -->
<Modal bind:open={showAssignModal} title={t('moderationQueue.assign_moderator')} onConfirm={assignModerator} confirmLabel={assignLoading ? t('common.saving') : t('common.confirm')}>
	<div class="space-y-4">
		<div>
			<label for="assignee-id" class="mb-1 block text-sm font-medium">{t('moderationQueue.assignee_id')}</label>
			<input id="assignee-id" type="text" bind:value={assigneeId} placeholder="User ID" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
	</div>
</Modal>

<!-- Appeal Modal -->
<Modal bind:open={showAppealModal} title={t('moderationQueue.handle_appeal')} onConfirm={submitAppeal} confirmLabel={appealLoading ? t('common.saving') : t('common.confirm')}>
	<div class="space-y-4">
		<div>
			<label for="appeal-reason" class="mb-1 block text-sm font-medium">{t('moderationQueue.appeal_reason')}</label>
			<textarea id="appeal-reason" bind:value={appealReason} rows="3" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" placeholder={t('moderationQueue.appeal_reason_placeholder')}></textarea>
		</div>
	</div>
</Modal>
