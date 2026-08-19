<script lang="ts">
	// /moderation/alerts — migrated to the EntityPage scaffold; read-only list with
	// state actions (Acknowledge / Resolve) wired through the `actions` config.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { moderationAdminApi, apiRaw, adminCatalogApi, type CatalogOption } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, AlertCircle, CheckCircle } from 'lucide-svelte';

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

	// Permission node — same as the legacy page's manage gate.
	const canManage = $derived(hasPermission('moderation.review_queue'));

	// `fields` is required by EntityPage even on read-only pages (used to seed the
	// edit-form value); this page never opens create/edit, so an empty array is fine.
	const fields: FieldDef[] = [];

	let entityController = $state<EntityPageController | undefined>(undefined);

	let alertSeverityOptions = $state<CatalogOption[]>([
		{ value: 'low', label: t('app.moderation.alerts.severity.low', 'Low') },
		{ value: 'medium', label: t('app.moderation.alerts.severity.medium', 'Medium') },
		{ value: 'high', label: t('app.moderation.alerts.severity.high', 'High') },
		{ value: 'critical', label: t('app.moderation.alerts.severity.critical', 'Critical') }
	]);

	let alertStatusOptions = $state<CatalogOption[]>([
		{ value: 'open', label: t('app.moderation.alerts.status.open', 'Open') },
		{ value: 'acknowledged', label: t('app.moderation.alerts.status.acknowledged', 'Acknowledged') },
		{ value: 'resolved', label: t('app.moderation.alerts.status.resolved', 'Resolved') }
	]);

	$effect(() => {
		void (async () => {
			const [severityRes, statusRes] = await Promise.all([
				adminCatalogApi.get('moderation.alerts.severity'),
				adminCatalogApi.get('moderation.alerts.status')
			]);
			if (severityRes.ok && severityRes.data?.length) alertSeverityOptions = severityRes.data;
			if (statusRes.ok && statusRes.data?.length) alertStatusOptions = statusRes.data;
		})();
	});

	let pageError = $state('');

	async function acknowledgeAlert(row: ModerationAlert) {
		pageError = '';
		try {
			const res = await moderationAdminApi.acknowledgeAlert(row.id);
			if (res.ok) {
				entityController?.reload();
			} else {
				pageError = res.error ?? t('common.error', 'Failed to acknowledge alert');
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to acknowledge alert');
		}
	}

	async function resolveAlert(row: ModerationAlert) {
		pageError = '';
		try {
			const res = await moderationAdminApi.resolveAlert(row.id, { resolution: '' });
			if (res.ok) {
				entityController?.reload();
			} else {
				pageError = res.error ?? t('common.error', 'Failed to resolve alert');
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to resolve alert');
		}
	}

	const config = $derived<EntityPageConfig>({
		title: t('moderation.alerts_title', 'Moderation Alerts'),
		description: t('moderation.alerts.real_time_moderation_alerts_and_security', 'Real-time moderation alerts and security notifications'),
		readonly: true,
		pageSize: 25,
		columns: [
			{
				key: 'alert_type',
				label: t('app.moderation.alerts.type', 'Type'),
				sortable: true,
				render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${(v ?? '').replace('_', ' ')}</span>`
			},
			{ key: 'title', label: t('moderation.alerts_title', 'Alert'), sortable: true, render: (v: string) => `<span class="font-medium">${v}</span>` },
			{ key: 'severity', label: t('app.moderation.alerts.severity', 'Severity'), sortable: true, componentSnippet: SeverityBadge },
			{ key: 'status', label: t('app.moderation.alerts.status', 'Status'), sortable: true, componentSnippet: StatusBadge },
			{
				key: 'created_at',
				label: t('common.created', 'Created'),
				sortable: true,
				render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v ? new Date(v).toLocaleDateString() : '—'}</span>`
			},
			{
				key: 'acknowledged_at',
				label: t('app.moderation.alerts.acknowledged_at', 'Acknowledged At'),
				sortable: true,
				render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v ? new Date(v).toLocaleDateString() : '—'}</span>`
			},
			{
				key: 'resolved_at',
				label: t('app.moderation.alerts.resolved_at', 'Resolved At'),
				sortable: true,
				render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v ? new Date(v).toLocaleDateString() : '—'}</span>`
			}
		],
		filters: [
			{
				key: 'severity',
				label: t('app.moderation.alerts.severity', 'Severity'),
				options: alertSeverityOptions
			},
			{
				key: 'status',
				label: t('app.moderation.alerts.status', 'Status'),
				options: alertStatusOptions
			}
		],
		actions: [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (row: ModerationAlert, ctx) => ctx?.openView(row) },
			{
				label: t('app.moderation.alerts.acknowledge', 'Acknowledge'),
				icon: AlertCircle,
				onclick: (row: ModerationAlert) => acknowledgeAlert(row),
				show: (row: ModerationAlert) => !row.acknowledged_at && canManage
			},
			{
				label: t('app.moderation.alerts.resolve', 'Resolve'),
				icon: CheckCircle,
				onclick: (row: ModerationAlert) => resolveAlert(row),
				show: (row: ModerationAlert) => !row.resolved_at && canManage
			}
		],
		fields,
		load: async ({ page, pageSize, q, filter }) => {
			const params = new URLSearchParams();
			params.set('page_size', String(pageSize));
			params.set('page_offset', String(page * pageSize));
			if (filter.severity) params.set('severity', filter.severity);
			if (filter.status) params.set('status', filter.status);
			if (q) params.set('q', q);
			// Use apiRaw to access top-level total/total_pages alongside data.
			const result = await apiRaw<{ data?: ModerationAlert[]; alerts?: ModerationAlert[]; items?: ModerationAlert[]; total?: number; total_pages?: number }>(`/admin/moderation/alerts${params.toString() ? `?${params.toString()}` : ''}`);
			if (result.ok) {
				const d = result.data;
				const arr = (d?.data ?? d?.alerts ?? d?.items ?? []) as ModerationAlert[];
				return {
					data: arr,
					total: d?.total ?? arr.length,
					totalPages: d?.total_pages ?? 1
				};
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load moderation alerts'));
		},
		viewSnippet: AlertView
	});
</script>

<svelte:head>
	<title>{t('moderation.alerts_title', 'Moderation Alerts')} — JapaConnect Admin</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

{#snippet SeverityBadge({ value }: { value: string; row: ModerationAlert })}
	<span
		class="inline-block rounded-full px-2 py-0.5 text-xs font-medium
		{value === 'critical' || value === 'high' ? 'bg-danger/15 text-danger' :
		 value === 'medium' ? 'bg-warning/15 text-warning' :
		 'bg-muted/15 text-muted'}"
	>
		{value}
	</span>
{/snippet}

{#snippet StatusBadge({ value }: { value: string; row: ModerationAlert })}
	<span
		class="inline-block rounded-full px-2 py-0.5 text-xs font-medium
		{value === 'open' ? 'bg-danger/15 text-danger' :
		 value === 'acknowledged' ? 'bg-warning/15 text-warning' :
		 'bg-success/15 text-success'}"
	>
		{value}
	</span>
{/snippet}

<!-- View slide-over body (mirrors the legacy view modal) -->
{#snippet AlertView({ row }: { row: ModerationAlert })}
	<div class="space-y-3">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('moderation.alerts_title', 'Alert')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.moderation.alerts.type', 'Type')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.alert_type.replace('_', ' ')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.moderation.alerts.severity', 'Severity')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.severity}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.moderation.alerts.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.description', 'Description')}</p>
			<p class="mt-1 whitespace-pre-wrap text-sm" style="color: var(--color-foreground)">{row.description}</p>
		</div>
		{#if row.entity_type}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.moderation.alerts.entity_type', 'Entity Type')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.entity_type}</p>
			</div>
		{/if}
		{#if row.triggered_by}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.moderation.alerts.triggered_by', 'Triggered By')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.triggered_by}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.created', 'Created')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.moderation.alerts.acknowledged_at', 'Acknowledged At')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.acknowledged_at ? new Date(row.acknowledged_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.moderation.alerts.resolved_at', 'Resolved At')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.resolved_at ? new Date(row.resolved_at).toLocaleString() : '—'}</p>
		</div>
	</div>
{/snippet}
