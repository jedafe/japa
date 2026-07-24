<script lang="ts">
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import Modal from '$lib/components/Modal.svelte';
	import { TableSkeleton } from '$lib/components/ui/skeleton';
	import { RefreshCw, CheckCircle, XCircle, ShieldAlert, Users, MessageSquare, AlertTriangle } from 'lucide-svelte';
	import { moderationAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import FilterBar from '$lib/components/FilterBar.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';

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

	let currentPage = $state(0);
	let itemsPerPage = $state(20);
	let totalPages = $state(0);

	const canModerate = $derived(hasPermission('admin.moderation'));

	const filterDefs: { key: string; label: string; type: 'select'; options: { value: string; label: string }[] }[] = [
		{ key: 'status', label: 'Status', type: 'select', options: [
			{ value: 'pending', label: 'Pending' },
			{ value: 'approved', label: 'Approved' },
			{ value: 'rejected', label: 'Rejected' },
			{ value: 'under_review', label: 'Under Review' },
			{ value: 'escalated', label: 'Escalated' }
		]},
		{ key: 'content_type', label: 'Content Type', type: 'select', options: [
			{ value: 'post', label: 'Post' },
			{ value: 'comment', label: 'Comment' },
			{ value: 'message', label: 'Message' },
			{ value: 'profile', label: 'Profile' }
		]},
		{ key: 'source', label: 'Source', type: 'select', options: [
			{ value: 'user_report', label: 'User Report' },
			{ value: 'auto_flag', label: 'Auto Flag' },
			{ value: 'admin', label: 'Admin' }
		]}
	];

	let filterValues = $state<Record<string, string>>({});

	function handleApplyFilters(values: Record<string, string>) {
		filterValues = values;
		currentPage = 0;
		fetchData();
	}

	const queueColumns = [
		{
			key: 'content_type',
			label: t('moderationQueue.type'),
			sortable: true
		},
		{
			key: 'content_id',
			label: t('moderationQueue.content_id'),
			render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${(v ?? '').slice(0, 12)}</span>`
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
			componentSnippet: QueueActionsCell
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
			componentSnippet: AlertActionsCell
		}
	];

	async function fetchData() {
		loading = true;
		errorMessage = '';
		try {
			if (activeTab === 'queue') {
				const params = new URLSearchParams({
					page: String(currentPage + 1),
					limit: String(itemsPerPage)
				});
				Object.entries(filterValues).forEach(([key, value]) => {
					if (value) params.set(key, value);
				});
				const result = await moderationAdminApi.getQueue(params.toString());
				if (result.ok) {
					const d = result.data as { items: ModerationQueueItem[]; total_pages: number };
					items = d?.items ?? [];
					totalPages = d?.total_pages ?? 0;
				} else {
					errorMessage = result.error;
				}
			} else {
				const result = await moderationAdminApi.getAlerts();
				if (result.ok) {
					alerts = ((result.data as { alerts: ModerationAlert[] })?.alerts) ?? [];
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
			fetchData();
		} catch {
			errorMessage = t('common.error');
		}
	}

	async function rejectItem(id: string) {
		errorMessage = '';
		try {
			await moderationAdminApi.updateItem(id, { status: 'rejected' });
			fetchData();
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
			fetchData();
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
			fetchData();
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
			fetchData();
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
			fetchData();
		} catch {
			errorMessage = t('common.error');
		}
	}

	async function resolveAlert(id: string) {
		errorMessage = '';
		try {
			await moderationAdminApi.resolveAlert(id, { resolution: 'resolved' });
			fetchData();
		} catch {
			errorMessage = t('common.error');
		}
	}

	$effect(() => {
		void activeTab;
		void currentPage;
		void itemsPerPage;
		void filterValues;
		fetchData();
	});
</script>

<svelte:head>
	<title>{t('moderationQueue.title')} — JapaConnect Admin</title>
</svelte:head>

{#if !canModerate}
	<div class="p-8 text-center text-muted-foreground">
		<p>You don't have permission to access moderation.</p>
	</div>
{:else}
	<PageHeader
		title={t('moderationQueue.title')}
		description={t('moderationQueue.description')}
		actions={[
			{ label: t('common.refresh'), icon: RefreshCw, onclick: fetchData }
		]}
	/>

	<FilterBar filters={filterDefs} values={filterValues} onApply={handleApplyFilters} />

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
			onclick={() => { activeTab = 'queue'; selectedIds = new Set(); currentPage = 0; fetchData(); }}
		>
			<MessageSquare class="mr-2 h-4 w-4" />
			{t('moderationQueue.tab_queue')}
		</button>
		<button
			class="inline-flex items-center whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition-all"
			style={activeTab === 'alerts' ? 'background: var(--color-background); color: var(--color-foreground); box-shadow: 0 1px 2px rgba(0,0,0,0.05)' : 'color: var(--color-muted)'}
			onclick={() => { activeTab = 'alerts'; currentPage = 0; fetchData(); }}
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
					showPagination={false}
					pageSize={itemsPerPage}
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
					showPagination={false}
					pageSize={itemsPerPage}
					emptyMessage={t('moderationQueue.no_alerts')}
				/>
			{/if}
		</CardContent>
	</Card>

	{#if totalPages > 1}
		<div class="flex items-center justify-between mt-4">
			<p class="text-sm text-muted-foreground">
				Page {currentPage + 1} of {totalPages}
			</p>
			<div class="flex gap-2">
				<button
					class="btn btn-outline btn-sm"
					disabled={currentPage === 0}
					onclick={() => { currentPage--; fetchData(); }}
				>
					Previous
				</button>
				<button
					class="btn btn-outline btn-sm"
					disabled={currentPage >= totalPages - 1}
					onclick={() => { currentPage++; fetchData(); }}
				>
					Next
				</button>
			</div>
		</div>
	{/if}
{/if}

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
{#snippet QueueActionsCell({ value, row }: { value: any; row: ModerationQueueItem })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => approveItem(row.id)} title={t('moderationQueue.approve')} style="color: var(--color-success)">
			<CheckCircle class="h-3.5 w-3.5" />
		</Button>
		<Button variant="ghost" size="sm" onclick={() => rejectItem(row.id)} title={t('moderationQueue.reject')} style="color: var(--color-danger)">
			<XCircle class="h-3.5 w-3.5" />
		</Button>
		<Button variant="ghost" size="sm" onclick={() => openAssignModal(row.id)} title={t('moderationQueue.assign')} style="color: var(--color-info)">
			<Users class="h-3.5 w-3.5" />
		</Button>
		<Button variant="ghost" size="sm" onclick={() => openAppealModal(row.id)} title={t('moderationQueue.appeal')} style="color: var(--color-warning)">
			<ShieldAlert class="h-3.5 w-3.5" />
		</Button>
	</div>
{/snippet}

<!-- Alert Action Cell Snippet -->
{#snippet AlertActionsCell({ value, row }: { value: any; row: ModerationAlert })}
	<div class="flex items-center gap-1">
		{#if !row.acknowledged}
			<Button variant="ghost" size="sm" onclick={() => acknowledgeAlert(row.id)} style="color: var(--color-info)">
				{t('moderationQueue.acknowledge')}
			</Button>
		{/if}
		<Button variant="ghost" size="sm" onclick={() => resolveAlert(row.id)} style="color: var(--color-success)">
			{t('moderationQueue.resolve')}
		</Button>
	</div>
{/snippet}
