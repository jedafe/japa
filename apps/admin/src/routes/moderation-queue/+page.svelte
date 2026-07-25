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
		type: string;
		status: string;
		content?: string;
		reporter?: string;
		author_id?: string;
		created_at: string;
		entity_ref?: string;
	};

	type ModerationAlert = {
		id: string;
		alert_type: string;
		title: string;
		description?: string;
		severity: string;
		status: string;
		acknowledged_at?: string;
		resolved_at?: string;
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

	const filterDefs: { key: string; label: string; type: 'select' | 'text'; options?: { value: string; label: string }[] }[] = [
		{ key: 'status', label: t('common.status', 'Status'), type: 'select', options: [
			{ value: 'flagged', label: t('app.moderation.queue.flagged', 'Flagged') },
			{ value: 'published', label: t('app.moderation.queue.published', 'Published') },
			{ value: 'removed', label: t('app.moderation.queue.removed', 'Removed') }
		]},
		{ key: 'content_type', label: t('moderation.content_type', 'Content Type'), type: 'select', options: [
			{ value: 'organization', label: t('app.moderation.queue.organization', 'Organization') },
			{ value: 'service', label: t('app.moderation.queue.service', 'Service') },
			{ value: 'mentor', label: t('app.moderation.queue.mentor', 'Mentor') },
			{ value: 'university', label: t('app.moderation.queue.university', 'University') }
		]},
		{ key: 'q', label: t('common.search', 'Search'), type: 'text' }
	];

	let filterValues = $state<Record<string, string>>({});

	function handleApplyFilters(values: Record<string, string>) {
		filterValues = values;
		currentPage = 0;
	}

	const queueColumns = [
		{
			key: 'status',
			label: t('common.status', 'Status'),
			sortable: true,
			render: (v: string) => {
				const colors = v === 'pending' ? 'bg-warning/15 text-warning dark:text-warning' : v === 'approved' ? 'bg-success/15 text-success dark:text-success' : v === 'rejected' ? 'bg-danger/15 text-danger dark:text-danger' : v === 'flagged' ? 'bg-warning/15 text-warning dark:text-warning' : 'bg-info/15 text-info dark:text-info';
				return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors}">${v}</span>`;
			}
		},
		{
			key: 'type',
			label: t('moderation.content_type', 'Content Type'),
			sortable: true,
			render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium bg-info/15 text-info dark:text-info">${v}</span>`
		},
		{
			key: 'content',
			label: t('moderation.content_preview', 'Content'),
			render: (v: string) => v ? `<span class="text-xs max-w-xs truncate block">${v}</span>` : '<span style="color: var(--color-muted)">—</span>'
		},
		{
			key: 'reporter',
			label: t('moderation.reporter', 'Reporter'),
			render: (v: string) => v || '—'
		},
		{
			key: 'created_at',
			label: t('moderationQueue.reported', 'Reported'),
			sortable: true,
			render: (v: string) => new Date(v).toLocaleDateString()
		},
		{
			key: 'actions',
			label: t('common.actions', 'Actions'),
			class: 'w-32',
			componentSnippet: QueueActionsCell
		}
	];

	const alertColumns = [
		{
			key: 'alert_type',
			label: t('moderationQueue.alert_type', 'Alert Type'),
			sortable: true,
			render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium bg-info/15 text-info dark:text-info">${v.replace(/_/g, ' ')}</span>`
		},
		{
			key: 'title',
			label: t('moderationQueue.message', 'Message'),
			render: (v: string, row: ModerationAlert) => {
				const desc = row.description ? `<br><span class="text-xs" style="color: var(--color-muted)">${row.description}</span>` : '';
				return `<span class="text-sm">${v}</span>${desc}`;
			}
		},
		{
			key: 'severity',
			label: t('moderationQueue.severity', 'Severity'),
			sortable: true,
			render: (v: string) => {
				const cls = v === 'high' ? 'bg-danger/15 text-danger dark:text-danger' : v === 'medium' ? 'bg-warning/15 text-warning dark:text-warning' : 'bg-muted/15 text-muted dark:text-muted';
				return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${cls}">${v}</span>`;
			}
		},
		{
			key: 'created_at',
			label: t('moderationQueue.created', 'Created'),
			sortable: true,
			render: (v: string) => new Date(v).toLocaleDateString()
		},
		{
			key: 'actions',
			label: t('common.actions', 'Actions'),
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
					page_size: String(itemsPerPage),
					page_offset: String(currentPage * itemsPerPage)
				});
				Object.entries(filterValues).forEach(([key, value]) => {
					if (value) params.set(key, value);
				});
				const result = await moderationAdminApi.getQueue(params.toString());
				if (result.ok) {
					const d = result.data as { items: ModerationQueueItem[]; total: number; total_pages: number };
					items = d?.items ?? [];
					totalPages = d?.total_pages ?? 0;
				} else {
					errorMessage = result.error;
				}
			} else {
				const result = await moderationAdminApi.getAlerts();
				if (result.ok) {
					alerts = Array.isArray(result.data) ? result.data : ((result.data as { alerts: ModerationAlert[] })?.alerts) ?? [];
				} else {
					errorMessage = result.error;
				}
			}
		} catch {
			errorMessage = t('common.error', 'An error occurred');
		} finally {
			loading = false;
		}
	}

	async function approveItem(id: string) {
		errorMessage = '';
		try {
			await moderationAdminApi.updateItem(id, { action: 'approve' });
			fetchData();
		} catch {
			errorMessage = t('common.error');
		}
	}

	async function rejectItem(id: string) {
		errorMessage = '';
		try {
			await moderationAdminApi.updateItem(id, { action: 'reject' });
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
		<p>{t('moderationQueue.no_permission', "You don't have permission to access moderation.")}</p>
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
			onclick={() => { activeTab = 'queue'; selectedIds = new Set(); currentPage = 0; }}
		>
			<MessageSquare class="mr-2 h-4 w-4" />
			{t('moderationQueue.tab_queue')}
		</button>
		<button
			class="inline-flex items-center whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition-all"
			style={activeTab === 'alerts' ? 'background: var(--color-background); color: var(--color-foreground); box-shadow: 0 1px 2px rgba(0,0,0,0.05)' : 'color: var(--color-muted)'}
			onclick={() => { activeTab = 'alerts'; currentPage = 0; }}
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
					pageSize={itemsPerPage}
					emptyMessage={t('moderationQueue.empty', 'No items in moderation queue')}
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
					pageSize={itemsPerPage}
					emptyMessage={t('moderationQueue.no_alerts', 'No alerts')}
				/>
			{/if}
		</CardContent>
	</Card>
{/if}

<!-- Assign Moderator Modal -->
<Modal bind:open={showAssignModal} title={t('moderationQueue.assign_moderator')}>
	<div class="space-y-4">
		<div>
			<label for="assignee-id" class="mb-1 block text-sm font-medium">{t('moderationQueue.assignee_id')}</label>
			<input id="assignee-id" type="text" bind:value={assigneeId} placeholder={t('moderationQueue.user_id_placeholder', 'User ID')} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
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
		{#if !row.acknowledged_at}
			<Button variant="ghost" size="sm" onclick={() => acknowledgeAlert(row.id)} style="color: var(--color-info)">
				{t('moderationQueue.acknowledge')}
			</Button>
		{/if}
		{#if !row.resolved_at}
			<Button variant="ghost" size="sm" onclick={() => resolveAlert(row.id)} style="color: var(--color-success)">
				{t('moderationQueue.resolve')}
			</Button>
		{/if}
	</div>
{/snippet}
