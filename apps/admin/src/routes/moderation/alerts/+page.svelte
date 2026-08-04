<script lang="ts">
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { TableSkeleton } from '$lib/components/ui/skeleton';
	import { CheckCircle, AlertCircle, RefreshCw, Eye } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';

	const canManage = $derived(hasPermission('moderation.alerts.manage'));

	type ModerationAlert = {
		id: string;
		alert_type: 'content_report' | 'ai_flag' | 'security_scan' | 'spam_burst' | 'ban_request';
		severity: 'low' | 'medium' | 'high' | 'critical';
		title: string;
		description: string;
		entity_type?: string;
		entity_id?: string;
		triggered_by?: string;
		status: 'open' | 'acknowledged' | 'resolved';
		created_at: string;
		acknowledged_at?: string;
		resolved_at?: string;
	};

	let alerts = $state<ModerationAlert[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let searchQuery = $state('');
	let filterValues = $state<Record<string, string>>({});

	// Action confirmation dialogs
	let showAckConfirm = $state(false);
	let showResolveConfirm = $state(false);
	let actionTarget = $state<ModerationAlert | null>(null);
	let actionLoading = $state(false);

	// View modal
	let showViewModal = $state(false);
	let viewTarget = $state<ModerationAlert | null>(null);

	const columns = [
		{ key: 'title', label: t('moderation.alerts_title', 'Alert'), sortable: true, render: (v: string) => `<span class="font-medium">${v}</span>` },
		{ key: 'alert_type', label: t('app.moderation.alerts.type', 'Type'), sortable: true, render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v.replace('_', ' ')}</span>` },
		{ key: 'severity', label: t('app.moderation.alerts.severity', 'Severity'), sortable: true, componentSnippet: SeverityBadge },
		{ key: 'status', label: t('app.moderation.alerts.status', 'Status'), sortable: true, componentSnippet: StatusBadge },
		{ key: 'created_at', label: t('common.created', 'Created'), sortable: true, render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleDateString()}</span>` },
		{ key: 'actions', label: t('common.actions', 'Actions'), class: 'w-36', componentSnippet: AlertActionsCell }
	];

	const filtered = $derived(
		searchQuery
			? alerts.filter((a) => a.title.toLowerCase().includes(searchQuery.toLowerCase()) || a.description.toLowerCase().includes(searchQuery.toLowerCase()) || (a.entity_type ?? '').toLowerCase().includes(searchQuery.toLowerCase()))
			: alerts
	);

	async function load() {
		loading = true;
		errorMessage = '';
		const params = new URLSearchParams();
		if (filterValues.severity) params.set('severity', filterValues.severity);
		if (filterValues.status) params.set('status', filterValues.status);
		try {
			const result = await api<ModerationAlert[]>(`/admin/moderation/alerts?${params}`);
			if (result.ok) {
				alerts = result.data ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	function openView(alert: ModerationAlert) {
		viewTarget = alert;
		showViewModal = true;
	}

	function promptAcknowledge(alert: ModerationAlert) {
		actionTarget = alert;
		showAckConfirm = true;
	}

	function promptResolve(alert: ModerationAlert) {
		actionTarget = alert;
		showResolveConfirm = true;
	}

	async function confirmAcknowledge() {
		if (!actionTarget) return;
		actionLoading = true;
		try {
			const result = await api(`/admin/moderation/alerts/${actionTarget.id}/acknowledge`, { method: 'POST' });
			if (result.ok) {
				showAckConfirm = false;
				actionTarget = null;
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			actionLoading = false;
		}
	}

	async function confirmResolve() {
		if (!actionTarget) return;
		actionLoading = true;
		try {
			const result = await api(`/admin/moderation/alerts/${actionTarget.id}/resolve`, { method: 'POST' });
			if (result.ok) {
				showResolveConfirm = false;
				actionTarget = null;
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			actionLoading = false;
		}
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('moderation.alerts_title')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('moderation.alerts_title')}
	description={t('moderation.alerts.real_time_moderation_alerts_and_security')}
	actions={[
		{ label: t('common.refresh', 'Refresh'), icon: RefreshCw, variant: 'ghost', onclick: load }
	]}
/>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

{#if loading}
	<TableSkeleton rows={6} columns={6} />
{:else}
	<DataTable
		{columns}
		data={filtered}
		searchable={true}
		searchPlaceholder={t('app.moderation.alerts.search', 'Search alerts...')}
		onsearch={(q: string) => { searchQuery = q; }}
		pageSize={25}
		emptyMessage={t('app.moderation.alerts.empty_title', 'No moderation alerts')}
	/>
{/if}

<!-- View Modal -->
<Modal bind:open={showViewModal} title={t('app.moderation.alerts.details', 'Alert Details')} size="sm">
	{#if viewTarget}
		<div class="space-y-3">
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('moderation.alerts_title', 'Alert')}</p><p class="mt-1 text-sm font-medium">{viewTarget.title}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.moderation.alerts.type', 'Type')}</p><p class="mt-1 text-sm">{viewTarget.alert_type.replace('_', ' ')}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.moderation.alerts.severity', 'Severity')}</p><p class="mt-1 text-sm font-medium">{viewTarget.severity}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.moderation.alerts.status', 'Status')}</p><p class="mt-1 text-sm font-medium">{viewTarget.status}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.description', 'Description')}</p><p class="mt-1 text-sm whitespace-pre-wrap">{viewTarget.description}</p></div>
			{#if viewTarget.entity_type}
				<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.moderation.alerts.entity_type', 'Entity Type')}</p><p class="mt-1 text-sm">{viewTarget.entity_type}</p></div>
			{/if}
			{#if viewTarget.triggered_by}
				<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.moderation.alerts.triggered_by', 'Triggered By')}</p><p class="mt-1 text-sm font-mono">{viewTarget.triggered_by}</p></div>
			{/if}
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.created', 'Created')}</p><p class="mt-1 text-sm">{new Date(viewTarget.created_at).toLocaleString()}</p></div>
			{#if viewTarget.acknowledged_at}
				<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.moderation.alerts.acknowledged_at', 'Acknowledged At')}</p><p class="mt-1 text-sm">{new Date(viewTarget.acknowledged_at).toLocaleString()}</p></div>
			{/if}
			{#if viewTarget.resolved_at}
				<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.moderation.alerts.resolved_at', 'Resolved At')}</p><p class="mt-1 text-sm">{new Date(viewTarget.resolved_at).toLocaleString()}</p></div>
			{/if}
		</div>
	{/if}
	<div class="mt-6 flex justify-end">
		<Button variant="outline" onclick={() => (showViewModal = false)}>{t('common.close', 'Close')}</Button>
	</div>
</Modal>

<!-- Acknowledge confirmation -->
<Modal bind:open={showAckConfirm} title={t('app.moderation.alerts.acknowledge', 'Acknowledge Alert')} size="sm">
	{#if actionTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.moderation.alerts.acknowledge_confirm', 'Acknowledge this alert?')}
			<strong>{actionTarget.title}</strong>
		</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showAckConfirm = false)} disabled={actionLoading}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button onclick={confirmAcknowledge} disabled={actionLoading}>
			<CheckCircle class="mr-2 h-4 w-4" />
			{actionLoading ? t('common.processing', 'Processing...') : t('app.moderation.alerts.acknowledge', 'Acknowledge')}
		</Button>
	</div>
</Modal>

<!-- Resolve confirmation -->
<Modal bind:open={showResolveConfirm} title={t('app.moderation.alerts.resolve', 'Resolve Alert')} size="sm">
	{#if actionTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.moderation.alerts.resolve_confirm', 'Resolve this alert?')}
			<strong>{actionTarget.title}</strong>
		</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showResolveConfirm = false)} disabled={actionLoading}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button onclick={confirmResolve} disabled={actionLoading} style="color: var(--color-success)">
			<CheckCircle class="mr-2 h-4 w-4" />
			{actionLoading ? t('common.processing', 'Processing...') : t('app.moderation.alerts.resolve', 'Resolve')}
		</Button>
	</div>
</Modal>

{#snippet SeverityBadge({ value }: { value: string; row: ModerationAlert })}
	<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium
		{value === 'critical' || value === 'high' ? 'bg-danger/15 text-danger' :
		 value === 'medium' ? 'bg-warning/15 text-warning' :
		 'bg-muted/15 text-muted'}">
		{value}
	</span>
{/snippet}

{#snippet StatusBadge({ value }: { value: string; row: ModerationAlert })}
	<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium
		{value === 'open' ? 'bg-danger/15 text-danger' :
		 value === 'acknowledged' ? 'bg-warning/15 text-warning' :
		 'bg-success/15 text-success'}">
		{value}
	</span>
{/snippet}

{#snippet AlertActionsCell({ row }: { value: any; row: ModerationAlert })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView(row)} aria-label={t('common.view', 'View')}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canManage && row.status === 'open'}
			<Button variant="ghost" size="sm" onclick={() => promptAcknowledge(row)} style="color: var(--color-warning)" aria-label={t('app.moderation.alerts.acknowledge', 'Acknowledge')}>
				<AlertCircle class="h-3.5 w-3.5" />
			</Button>
		{/if}
		{#if canManage && row.status !== 'resolved'}
			<Button variant="ghost" size="sm" onclick={() => promptResolve(row)} style="color: var(--color-success)" aria-label={t('app.moderation.alerts.resolve', 'Resolve')}>
				<CheckCircle class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
