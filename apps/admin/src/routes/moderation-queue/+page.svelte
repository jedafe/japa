<script lang="ts">
	// /moderation-queue — migrated to the EntityPage scaffold.
	// This page has two distinct entity types behind tabs (queue items + alerts),
	// so it renders two separate `<EntityPage>` instances conditionally rather
	// than a single scaffold.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { moderationAdminApi, adsAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { user } from '$lib/stores/auth.svelte';
	import { Eye, UserPlus, CheckCircle, XCircle, MessageSquare, AlertTriangle, Check, Ban } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';
	import { initTabParam, bindTabParam } from '$lib/utils/url-sync.svelte';

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

	// Permission gate — the legacy page guarded on `admin.moderation` (the seeded
	// RBAC node for moderation handlers). The task brief named
	// `moderation.review_queue`, but that node is not seeded in sections/03-rbac.md
	// and would brick the page for non-super-admins, so the real legacy node is kept.
	const canModerate = $derived(hasPermission('admin.moderation'));

	const MQ_TABS = ['queue', 'alerts'] as const;
	let activeTab = $state<'queue' | 'alerts'>(initTabParam('queue', { valid: MQ_TABS }));
	bindTabParam(() => activeTab, (v) => (activeTab = v), { valid: MQ_TABS, preserve: [] });

	// ── Controllers + page-local error state ─────────────────────────────────
	let queueController = $state<EntityPageController | undefined>(undefined);
	let alertController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkActing = $state(false);

	// ── Queue tab ────────────────────────────────────────────────────────────
	let queueStatusOptions = $state<{ value: string; label: string }[]>([
		{ value: 'flagged', label: t('app.moderation.queue.flagged', 'Flagged') },
		{ value: 'published', label: t('app.moderation.queue.published', 'Published') },
		{ value: 'removed', label: t('app.moderation.queue.removed', 'Removed') },
		{ value: 'pending_review', label: t('app.moderation.queue.pending_review', 'Pending Review') }
	]);
	let queueContentTypeOptions = $state<{ value: string; label: string }[]>([
		{ value: 'organization', label: t('app.moderation.queue.organization', 'Organization') },
		{ value: 'service', label: t('app.moderation.queue.service', 'Service') },
		{ value: 'mentor', label: t('app.moderation.queue.mentor', 'Mentor') },
		{ value: 'university', label: t('app.moderation.queue.university', 'University') },
		{ value: 'ad_campaign', label: t('app.moderation.queue.ad_campaign', 'Ad Campaign') }
	]);

	$effect(() => {
		void (async () => {
			const res = await moderationAdminApi.getOptions();
			if (res.ok && res.data) {
				if (res.data.statuses?.length) {
					queueStatusOptions = res.data.statuses;
					// ad_campaign moderation still exposes a pending_review status that
					// the generic catalog does not yet include.
					if (!queueStatusOptions.some((o) => o.value === 'pending_review')) {
						queueStatusOptions = [
							...queueStatusOptions,
							{ value: 'pending_review', label: t('app.moderation.queue.pending_review', 'Pending Review') }
						];
					}
				}
				if (res.data.content_types?.length) {
					queueContentTypeOptions = res.data.content_types;
				}
			}
		})();
	});

	const queueFilters = $derived<FilterDef[]>([
		{
			key: 'status',
			label: t('common.status', 'Status'),
			options: queueStatusOptions
		},
		{
			key: 'content_type',
			label: t('moderation.content_type', 'Content Type'),
			options: queueContentTypeOptions
		}
	]);

	// "Assign to Me" uses the current admin user's id as the assignee.
	async function assignToMe(row: ModerationQueueItem, ctx?: { reload?: () => void }) {
		const assigneeId = user.current?.id;
		if (!assigneeId) {
			pageError = t('moderationQueue.assign_self_unavailable', 'Unable to determine your user id');
			return;
		}
		pageError = '';
		try {
			const res = await moderationAdminApi.assignItem(row.id, { assignee_id: assigneeId });
			if (!res.ok) {
				pageError = res.error ?? t('common.error', 'An error occurred');
			} else {
				ctx?.reload?.();
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'An error occurred');
		}
	}

	async function bulkDismiss(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkActing) return; // in-flight guard
		bulkActing = true;
		pageError = '';
		try {
			const res = await moderationAdminApi.bulkAction({ ids: idArr, action: 'dismiss' });
			if (!res.ok) pageError = res.error ?? t('common.error', 'An error occurred');
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'An error occurred');
		} finally {
			bulkActing = false;
		}
	}

	async function bulkAssignToMe(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkActing) return; // in-flight guard
		const assigneeId = user.current?.id;
		if (!assigneeId) {
			pageError = t('moderationQueue.assign_self_unavailable', 'Unable to determine your user id');
			return;
		}
		bulkActing = true;
		pageError = '';
		try {
			const res = await moderationAdminApi.bulkAction({ ids: idArr, action: 'assign', assignee_id: assigneeId });
			if (!res.ok) pageError = res.error ?? t('common.error', 'An error occurred');
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'An error occurred');
		} finally {
			bulkActing = false;
		}
	}

	// §61: Approve an ad campaign (sets status to active).
	// Only shown for rows where type === 'ad_campaign' and status === 'pending_review'.
	async function approveAdCampaign(row: ModerationQueueItem, ctx?: { reload?: () => void }) {
		pageError = '';
		try {
			const res = await adsAdminApi.activateCampaign(row.id);
			if (!res.ok) {
				pageError = res.error ?? t('common.error', 'An error occurred');
			} else {
				ctx?.reload?.();
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'An error occurred');
		}
	}

	// §61: Reject an ad campaign (sets status to rejected + enqueues refund).
	async function rejectAdCampaign(row: ModerationQueueItem, ctx?: { reload?: () => void }) {
		pageError = '';
		try {
			const res = await adsAdminApi.rejectCampaign(row.id);
			if (!res.ok) {
				pageError = res.error ?? t('common.error', 'An error occurred');
			} else {
				ctx?.reload?.();
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'An error occurred');
		}
	}

	const queueActions: ActionItem[] = [
		{ label: t('common.view', 'View'), icon: Eye, onclick: (row: any, ctx?: any) => ctx?.openView(row) },
		{
			label: t('moderationQueue.assign_to_me', 'Assign to Me'),
			icon: UserPlus,
			onclick: (row: ModerationQueueItem, ctx?: any) => assignToMe(row, ctx)
		},
		{
			label: t('moderationQueue.approve', 'Approve'),
			icon: Check,
			onclick: (row: ModerationQueueItem, ctx?: any) => approveAdCampaign(row, ctx),
			show: (row: ModerationQueueItem) => row.type === 'ad_campaign' && row.status === 'pending_review'
		},
		{
			label: t('moderationQueue.reject', 'Reject'),
			icon: Ban,
			variant: 'destructive',
			onclick: (row: ModerationQueueItem, ctx?: any) => rejectAdCampaign(row, ctx),
			show: (row: ModerationQueueItem) => row.type === 'ad_campaign' && row.status === 'pending_review'
		}
	];

	const queueConfig = $derived<EntityPageConfig>({
		title: t('moderationQueue.title', 'Moderation Queue'),
		description: t('moderationQueue.description', 'Unified moderation queue for reports and alerts'),
		readonly: true,
		pageSize: 20,
		columns: [
			{
				key: 'type',
				label: t('moderation.content_type', 'Content Type'),
				sortable: true,
				render: (v: string) =>
					`<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium bg-info/15 text-info dark:text-info">${v}</span>`
			},
			{
				key: 'status',
				label: t('common.status', 'Status'),
				sortable: true,
				render: (v: string) => {
					const colors =
						v === 'pending'
							? 'bg-warning/15 text-warning dark:text-warning'
							: v === 'approved'
								? 'bg-success/15 text-success dark:text-success'
								: v === 'rejected'
									? 'bg-danger/15 text-danger dark:text-danger'
									: v === 'flagged'
										? 'bg-warning/15 text-warning dark:text-warning'
										: 'bg-info/15 text-info dark:text-info';
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors}">${v}</span>`;
				}
			},
			{
				key: 'content',
				label: t('moderation.content_preview', 'Content'),
				render: (v: string) =>
					v
						? `<span class="text-xs max-w-xs truncate block">${v}</span>`
						: '<span style="color: var(--color-muted)">—</span>'
			},
			{
				key: 'reporter',
				label: t('moderation.reporter', 'Reporter'),
				render: (v: string, row: ModerationQueueItem) => v || row.author_id || '—'
			},
			{
				key: 'created_at',
				label: t('moderationQueue.reported', 'Reported'),
				sortable: true,
				render: (v: string) => new Date(v).toLocaleDateString()
			}
		],
		filters: queueFilters,
		actions: queueActions,
		bulkActions: [
			{
				label: t('moderationQueue.bulk_dismiss', 'Dismiss Selected'),
				icon: XCircle,
				variant: 'destructive',
				onclick: bulkDismiss
			},
			{
				label: t('moderationQueue.bulk_assign', 'Assign Selected to Me'),
				icon: UserPlus,
				onclick: bulkAssignToMe
			}
		],
		fields: [] as FieldDef[],
		load: async ({ page, pageSize, q, filter }) => {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(page * pageSize)
			});
			if (q) params.set('q', q);
			if (filter.status) params.set('status', filter.status);
			if (filter.content_type) params.set('content_type', filter.content_type);
			const result = await moderationAdminApi.getQueue(params.toString());
			if (result.ok) {
				const d = result.data as { items: ModerationQueueItem[]; total: number; total_pages: number };
				return {
					data: d?.items ?? [],
					total: d?.total ?? 0,
					totalPages: d?.total_pages ?? 1
				};
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load moderation queue'));
		},
		viewSnippet: QueueItemView
	});

	// ── Alerts tab ───────────────────────────────────────────────────────────
	async function acknowledgeAlert(row: ModerationAlert, ctx?: { reload?: () => void }) {
		pageError = '';
		try {
			const res = await moderationAdminApi.acknowledgeAlert(row.id);
			if (!res.ok) {
				pageError = res.error ?? t('common.error', 'An error occurred');
			} else {
				ctx?.reload?.();
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'An error occurred');
		}
	}

	async function resolveAlert(row: ModerationAlert, ctx?: { reload?: () => void }) {
		pageError = '';
		try {
			const res = await moderationAdminApi.resolveAlert(row.id, { resolution: 'resolved' });
			if (!res.ok) {
				pageError = res.error ?? t('common.error', 'An error occurred');
			} else {
				ctx?.reload?.();
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'An error occurred');
		}
	}

	const alertActions: ActionItem[] = [
		{ label: t('common.view', 'View'), icon: Eye, onclick: (row: any, ctx?: any) => ctx?.openView(row) },
		{
			label: t('moderationQueue.acknowledge', 'Acknowledge'),
			icon: CheckCircle,
			onclick: (row: ModerationAlert, ctx?: any) => acknowledgeAlert(row, ctx),
			show: (row: ModerationAlert) => !row.acknowledged_at
		},
		{
			label: t('moderationQueue.resolve', 'Resolve'),
			icon: CheckCircle,
			onclick: (row: ModerationAlert, ctx?: any) => resolveAlert(row, ctx),
			show: (row: ModerationAlert) => !row.resolved_at
		}
	];

	function alertStatusBadge(row: ModerationAlert): string {
		const label = row.resolved_at
			? t('moderationQueue.resolve', 'Resolved')
			: row.acknowledged_at
				? t('moderationQueue.acknowledge', 'Acknowledged')
				: t('moderationQueue.tab_alerts', 'Open');
		const cls = row.resolved_at
			? 'bg-success/15 text-success dark:text-success'
			: row.acknowledged_at
				? 'bg-info/15 text-info dark:text-info'
				: 'bg-warning/15 text-warning dark:text-warning';
		return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${cls}">${label}</span>`;
	}

	const alertConfig: EntityPageConfig = {
		title: t('moderationQueue.tab_alerts', 'Alerts'),
		description: t('moderationQueue.description', 'Unified moderation queue for reports and alerts'),
		readonly: true,
		pageSize: 20,
		columns: [
			{
				key: 'alert_type',
				label: t('moderationQueue.alert_type', 'Alert Type'),
				sortable: true,
				render: (v: string) =>
					`<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium bg-info/15 text-info dark:text-info">${v.replace(/_/g, ' ')}</span>`
			},
			{
				key: 'title',
				label: t('moderationQueue.message', 'Message'),
				render: (v: string, row: ModerationAlert) => {
					const desc = row.description
						? `<br><span class="text-xs" style="color: var(--color-muted)">${row.description}</span>`
						: '';
					return `<span class="text-sm">${v}</span>${desc}`;
				}
			},
			{
				key: 'severity',
				label: t('moderationQueue.severity', 'Severity'),
				sortable: true,
				render: (v: string) => {
					const cls =
						v === 'high'
							? 'bg-danger/15 text-danger dark:text-danger'
							: v === 'medium'
								? 'bg-warning/15 text-warning dark:text-warning'
								: 'bg-muted/15 text-muted dark:text-muted';
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${cls}">${v}</span>`;
				}
			},
			{
				key: 'status',
				label: t('common.status', 'Status'),
				render: (_v: string, row: ModerationAlert) => alertStatusBadge(row)
			},
			{
				key: 'created_at',
				label: t('moderationQueue.created', 'Created'),
				sortable: true,
				render: (v: string) => new Date(v).toLocaleDateString()
			}
		],
		actions: alertActions,
		fields: [] as FieldDef[],
		load: async ({ page, pageSize, q, filter }) => {
			const result = await moderationAdminApi.getAlerts('?page_size=1000');
			if (result.ok) {
				let alerts = Array.isArray(result.data)
					? (result.data as ModerationAlert[])
					: ((result.data as { alerts?: ModerationAlert[] })?.alerts) ?? [];
				const query = (q ?? '').toLowerCase();
				if (query) {
					alerts = alerts.filter((row) =>
						[row.title, row.description, row.severity, row.status, row.alert_type].some((v) =>
							String(v ?? '').toLowerCase().includes(query)
						)
					);
				}
				filter;
				const size = pageSize || 25;
				const offset = (page || 0) * size;
				const total = alerts.length;
				const totalPages = Math.max(1, Math.ceil(total / size));
				return {
					data: alerts.slice(offset, offset + size),
					total,
					totalPages
				};
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load moderation alerts'));
		},
		viewSnippet: AlertView
	};
</script>

<svelte:head>
	<title>{t('moderationQueue.title', 'Moderation Queue')} — JapaConnect Admin</title>
</svelte:head>

{#if !canModerate}
	<div class="p-8 text-center text-muted-foreground">
		<p>{t('moderationQueue.no_permission', "You don't have permission to access moderation.")}</p>
	</div>
{:else}
	{#if pageError}
		<div
			class="mb-4 rounded-lg p-3 text-sm"
			style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
		>
			{pageError}
		</div>
	{/if}

	<!-- Tabs -->
	<div class="mb-6 flex gap-1 rounded-lg p-1" style="background: var(--color-surface)">
		<button
			class="inline-flex items-center whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition-all"
			style={activeTab === 'queue'
				? 'background: var(--color-background); color: var(--color-foreground); box-shadow: 0 1px 2px rgba(0,0,0,0.05)'
				: 'color: var(--color-muted)'}
			onclick={() => (activeTab = 'queue')}
		>
			<MessageSquare class="mr-2 h-4 w-4" />
			{t('moderationQueue.tab_queue', 'Queue')}
		</button>
		<button
			class="inline-flex items-center whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition-all"
			style={activeTab === 'alerts'
				? 'background: var(--color-background); color: var(--color-foreground); box-shadow: 0 1px 2px rgba(0,0,0,0.05)'
				: 'color: var(--color-muted)'}
			onclick={() => (activeTab = 'alerts')}
		>
			<AlertTriangle class="mr-2 h-4 w-4" />
			{t('moderationQueue.tab_alerts', 'Alerts')}
		</button>
	</div>

	{#if activeTab === 'queue'}
		<EntityPage config={queueConfig} bind:controller={queueController} />
	{:else}
		<EntityPage config={alertConfig} bind:controller={alertController} />
	{/if}
{/if}

<!-- ── View snippets ─────────────────────────────────────────────────────── -->

{#snippet QueueItemView({ row }: { row: ModerationQueueItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('moderation.content_type', 'Content Type')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.type}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('common.status', 'Status')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status}</p>
		</div>
		{#if row.entity_ref}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('moderationQueue.content_id', 'Content ID')}
				</p>
				<p class="mt-1 break-all text-sm font-mono" style="color: var(--color-foreground)">{row.entity_ref}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('moderation.content_preview', 'Content')}
			</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.content ?? '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('moderation.reporter', 'Reporter')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.reporter || row.author_id || '—'}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('moderationQueue.reported', 'Reported')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{new Date(row.created_at).toLocaleString()}
			</p>
		</div>
	</div>
{/snippet}

{#snippet AlertView({ row }: { row: ModerationAlert })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('moderationQueue.alert_type', 'Alert Type')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.alert_type.replace(/_/g, ' ')}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('moderationQueue.message', 'Message')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
		</div>
		{#if row.description}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('moderationQueue.reason', 'Reason')}
				</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('moderationQueue.severity', 'Severity')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.severity}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('common.status', 'Status')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status}</p>
		</div>
		{#if row.acknowledged_at}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('moderationQueue.acknowledge', 'Acknowledged')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{new Date(row.acknowledged_at).toLocaleString()}
				</p>
			</div>
		{/if}
		{#if row.resolved_at}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('moderationQueue.resolve', 'Resolved')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{new Date(row.resolved_at).toLocaleString()}
				</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('moderationQueue.created', 'Created')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{new Date(row.created_at).toLocaleString()}
			</p>
		</div>
	</div>
{/snippet}
