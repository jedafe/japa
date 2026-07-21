<script lang="ts">
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { TableSkeleton } from '$lib/components/ui/skeleton';
	import { RefreshCw, CheckCircle, XCircle, ShieldAlert, Users, MessageSquare, AlertTriangle } from 'lucide-svelte';
	import { moderationAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

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

	let selectedIds = $state<Set<string>>(new Set());
	let bulkLoading = $state(false);

	let showAssignModal = $state(false);
	let assignItemId = $state('');
	let assigneeId = $state('');
	let assignLoading = $state(false);

	let showAppealModal = $state(false);
	let appealItemId = $state('');
	let appealReason = $state('');
	let appealLoading = $state(false);

	const queueColumns = [
		{
			key: 'content_type',
			label: t('moderationQueue.type'),
			sortable: true
		},
		{
			key: 'content_id',
			label: t('moderationQueue.content_id'),
			render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v.slice(0, 12)}</span>`
		},
		{
			key: 'reason',
			label: t('moderationQueue.reason'),
			render: (v: string) => v || '—'
		},
		{
			key: 'assignee_name',
			label: t('moderationQueue.assignee'),
			render: (v: string) => v || '—'
		},
		{
			key: 'reported_at',
			label: t('moderationQueue.reported'),
			sortable: true,
			render: (v: string) => new Date(v).toLocaleDateString()
		},
		{
			key: 'actions',
			label: t('common.actions'),
			class: 'w-32',
			component: QueueActionsCell
		}
	];

	const alertColumns = [
		{
			key: 'type',
			label: t('moderationQueue.alert_type'),
			sortable: true
		},
		{
			key: 'message',
			label: t('moderationQueue.message')
		},
		{
			key: 'severity',
			label: t('moderationQueue.severity'),
			sortable: true,
			render: (v: string) => {
				const cls = v === 'high' ? 'bg-danger/15 text-danger dark:text-danger' : v === 'medium' ? 'bg-warning/15 text-warning dark:text-warning' : 'bg-muted/15 text-muted dark:text-muted';
				return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${cls}">${v}</span>`;
			}
		},
		{
			key: 'created_at',
			label: t('moderationQueue.created'),
			sortable: true,
			render: (v: string) => new Date(v).toLocaleDateString()
		},
		{
			key: 'actions',
			label: t('common.actions'),
			class: 'w-32',
			component: AlertActionsCell
		}
	];

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

<PageHeader
	title={t('moderationQueue.title')}
	description={t('moderationQueue.description')}
	actions={[
		{ label: t('common.refresh'), icon: RefreshCw, onclick: load }
	]}
/>

{#if errorMessage}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<!-- Tabs -->
<div class="mb-6 flex gap-1 rounded-lg p-1" style="background: var(--color-surface)">
	<button
		class="inline-flex items-center whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition-all"
		style={activeTab === 'queue' ? 'background: var(--color-background); color: var(--color-foreground); box-shadow: 0 1px 2px rgba(0,0,0,0.05)' : 'color: var(--color-muted)'}
		onclick={() => { activeTab = 'queue'; selectedIds = new Set(); load(); }}
	>
		<MessageSquare class="mr-2 h-4 w-4" />
		{t('moderationQueue.tab_queue')}
	</button>
	<button
		class="inline-flex items-center whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition-all"
		style={activeTab === 'alerts' ? 'background: var(--color-background); color: var(--color-foreground); box-shadow: 0 1px 2px rgba(0,0,0,0.05)' : 'color: var(--color-muted)'}
		onclick={() => { activeTab = 'alerts'; load(); }}
	>
		<AlertTriangle class="mr-2 h-4 w-4" />
		{t('moderationQueue.tab_alerts')}
	</button>
</div>

<Card>
	<CardContent>
		{#if loading}
			<TableSkeleton rows={8} columns={activeTab === 'queue' ? queueColumns.length : alertColumns.length} />
		{:else if activeTab === 'queue'}
			<DataTable
				columns={queueColumns}
				data={items}
				selectable={true}
				bind:selectedIds
				pageSize={25}
				emptyMessage={t('moderationQueue.empty')}
			>
				{#snippet bulkActions()}
					<Button variant="ghost" size="sm" onclick={() => bulkAction('approve')} disabled={bulkLoading} style="color: var(--color-success)">
						<CheckCircle class="mr-1 h-3.5 w-3.5" />
						{t('moderationQueue.bulk_approve')}
					</Button>
					<Button variant="ghost" size="sm" onclick={() => bulkAction('reject')} disabled={bulkLoading} style="color: var(--color-danger)">
						<XCircle class="mr-1 h-3.5 w-3.5" />
						{t('moderationQueue.bulk_reject')}
					</Button>
				{/snippet}
			</DataTable>
		{:else}
			<DataTable
				columns={alertColumns}
				data={alerts}
				pageSize={25}
				emptyMessage={t('moderationQueue.no_alerts')}
			/>
		{/if}
	</CardContent>
</Card>

<!-- Assign Moderator Modal -->
<Modal bind:open={showAssignModal} title={t('moderationQueue.assign_moderator')}>
	<div class="space-y-4">
		<div>
			<label for="assignee-id" class="mb-1 block text-sm font-medium">{t('moderationQueue.assignee_id')}</label>
			<input id="assignee-id" type="text" bind:value={assigneeId} placeholder="User ID" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showAssignModal = false)}>{t('common.cancel')}</Button>
		<Button onclick={assignModerator} disabled={assignLoading}>{assignLoading ? t('common.saving') : t('common.confirm')}</Button>
	</div>
</Modal>

<!-- Appeal Modal -->
<Modal bind:open={showAppealModal} title={t('moderationQueue.handle_appeal')}>
	<div class="space-y-4">
		<div>
			<label for="appeal-reason" class="mb-1 block text-sm font-medium">{t('moderationQueue.appeal_reason')}</label>
			<textarea id="appeal-reason" bind:value={appealReason} rows="3" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" placeholder={t('moderationQueue.appeal_reason_placeholder')}></textarea>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showAppealModal = false)}>{t('common.cancel')}</Button>
		<Button onclick={submitAppeal} disabled={appealLoading}>{appealLoading ? t('common.saving') : t('common.confirm')}</Button>
	</div>
</Modal>

<!-- Queue Action Cell Snippet -->
{#snippet QueueActionsCell(item: ModerationQueueItem)}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => approveItem(item.id)} title={t('moderationQueue.approve')} style="color: var(--color-success)">
			<CheckCircle class="h-3.5 w-3.5" />
		</Button>
		<Button variant="ghost" size="sm" onclick={() => rejectItem(item.id)} title={t('moderationQueue.reject')} style="color: var(--color-danger)">
			<XCircle class="h-3.5 w-3.5" />
		</Button>
		<Button variant="ghost" size="sm" onclick={() => openAssignModal(item.id)} title={t('moderationQueue.assign')} style="color: var(--color-info)">
			<Users class="h-3.5 w-3.5" />
		</Button>
		<Button variant="ghost" size="sm" onclick={() => openAppealModal(item.id)} title={t('moderationQueue.appeal')} style="color: var(--color-warning)">
			<ShieldAlert class="h-3.5 w-3.5" />
		</Button>
	</div>
{/snippet}

<!-- Alert Action Cell Snippet -->
{#snippet AlertActionsCell(alert: ModerationAlert)}
	<div class="flex items-center gap-1">
		{#if !alert.acknowledged}
			<Button variant="ghost" size="sm" onclick={() => acknowledgeAlert(alert.id)} style="color: var(--color-info)">
				{t('moderationQueue.acknowledge')}
			</Button>
		{/if}
		<Button variant="ghost" size="sm" onclick={() => resolveAlert(alert.id)} style="color: var(--color-success)">
			{t('moderationQueue.resolve')}
		</Button>
	</div>
{/snippet}
