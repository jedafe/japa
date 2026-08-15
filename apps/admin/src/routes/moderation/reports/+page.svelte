<script lang="ts">
	// /moderation/reports — migrated to the EntityPage scaffold.
	// Distinct from /reports: this view is focused on resolving pending reports
	// via state actions (dismiss / warn / ban / remove content) issued through
	// PATCH /admin/reports/{id}. Read-only list + ActionMenu state actions +
	// page-local confirm Modal with an optional reason textarea.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { moderationAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, Ban, AlertTriangle, Trash2, XCircle } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type Report = {
		id: string;
		reporter_user_id: string;
		reporter_username?: string;
		reporter_display_name?: string;
		entity_type: string;
		entity_id: string;
		reason: string;
		notes?: string;
		evidence_urls?: string[];
		status: string;
		reviewed_by?: string;
		resolution_note?: string;
		resolved_at?: string;
		created_at: string;
	};

	type ActionType = 'dismiss' | 'warn' | 'ban' | 'remove';

	const canManage = $derived(hasPermission('reports.review'));

	function statusBadgeClass(status: string): string {
		switch (status) {
			case 'pending': return 'bg-warning/15 text-warning dark:text-warning';
			case 'reviewing': return 'bg-info/15 text-info dark:text-info';
			case 'actioned': return 'bg-success/15 text-success dark:text-success';
			case 'dismissed': return 'bg-muted/15 text-muted dark:text-muted';
			default: return 'bg-muted/15 text-muted dark:text-muted';
		}
	}

	function actionLabel(type: ActionType): string {
		switch (type) {
			case 'dismiss': return t('moderation.dismiss', 'Dismiss');
			case 'warn': return t('moderation.warn', 'Warn');
			case 'ban': return t('moderation.ban', 'Ban');
			case 'remove': return t('moderation.remove_content', 'Remove Content');
			default: return type;
		}
	}

	// ── State-action confirmation modal ─────────────────────────────────────
	// EntityPage's confirm Modal only covers default delete; state actions keep
	// a page-local Modal with richer copy + optional reason, then call
	// controller.reload() after the API call succeeds.
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);
	let actionTarget = $state<Report | null>(null);
	let actionType = $state<ActionType>('dismiss');
	let actionReason = $state('');
	let showActionModal = $state(false);

	function openAction(report: Report, action: ActionType) {
		actionTarget = report;
		actionType = action;
		actionReason = '';
		showActionModal = true;
	}

	async function executeAction() {
		if (!actionTarget) return;
		acting = true;
		pageError = '';
		try {
			const body: Record<string, unknown> = { action: actionType };
			if (actionReason.trim()) body.reason = actionReason.trim();
			const result = await moderationAdminApi.updateReport(actionTarget.id, body);
			if (result.ok) {
				showActionModal = false;
				actionTarget = null;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	// ── Row actions ─────────────────────────────────────────────────────────
	// Custom actions replace defaults, so View is included explicitly. State
	// actions only apply to pending reports and are permission-gated.
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('moderation.dismiss', 'Dismiss'),
				icon: XCircle,
				onclick: (row: Report) => openAction(row, 'dismiss'),
				show: (row: Report) => row.status === 'pending'
			});
			acts.push({
				label: t('moderation.warn', 'Warn'),
				icon: AlertTriangle,
				variant: 'destructive',
				onclick: (row: Report) => openAction(row, 'warn'),
				show: (row: Report) => row.status === 'pending'
			});
			acts.push({
				label: t('moderation.remove_content', 'Remove Content'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (row: Report) => openAction(row, 'remove'),
				show: (row: Report) => row.status === 'pending'
			});
			acts.push({
				label: t('moderation.ban', 'Ban'),
				icon: Ban,
				variant: 'destructive',
				onclick: (row: Report) => openAction(row, 'ban'),
				show: (row: Report) => row.status === 'pending'
			});
		}
		return acts;
	});

	// `fields` is required by EntityPage for edit-form seeding; this page is
	// read-only so the form is never shown, but the array must be present.
	const fields: FieldDef[] = [
		{ key: 'reason', label: t('moderation.reason', 'Reason'), type: 'text' }
	];

	// Config built as a $derived so rowActions updates (canManage / status)
	// propagate to EntityPage.
	const config = $derived<EntityPageConfig>({
		title: t('moderation.reports', 'Reports'),
		description: t('moderation.reports_description', 'Review and resolve reported content.'),
		readonly: true,
		pageSize: 25,
		columns: [
			{
				key: 'reporter_display_name',
				label: t('moderation.reporter', 'Reporter'),
				sortable: true,
				render: (v: string, row: Report) =>
					`<span class="text-sm">${v || row.reporter_username || 'anonymous'}</span>`
			},
			{
				key: 'entity_type',
				label: t('moderation.target', 'Target'),
				sortable: true,
				render: (_: any, row: Report) =>
					`<span class="text-xs" style="color: var(--color-muted)">${row.entity_type} &middot; ${(row.entity_id ?? '').slice(0, 10)}</span>`
			},
			{
				key: 'reason',
				label: t('moderation.reason', 'Reason'),
				render: (v: string) => `<span class="text-xs max-w-xs truncate block">${v}</span>`
			},
			{
				key: 'status',
				label: t('moderation.status', 'Status'),
				sortable: true,
				render: (v: string) =>
					`<span class="inline-block rounded-full px-2 py-0.5 text-xs ${statusBadgeClass(v)}">${v}</span>`
			},
			{
				key: 'created_at',
				label: t('moderation.date', 'Date'),
				sortable: true,
				render: (v: string) =>
					`<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleDateString()}</span>`
			}
		],
		filters: [
			{
				key: 'status',
				label: t('moderation.status', 'Status'),
				options: [
					{ value: 'pending', label: t('app.moderation.queue.pending', 'Pending') },
					{ value: 'reviewing', label: t('moderation.reviewing', 'Reviewing') },
					{ value: 'actioned', label: t('moderation.actioned', 'Actioned') },
					{ value: 'dismissed', label: t('app.moderation.queue.dismissed', 'Dismissed') }
				]
			}
		],
		actions: rowActions,
		fields,
		load: async ({ q, filter, page, pageSize }) => {
			const params = new URLSearchParams();
			params.set('page', String(page));
			params.set('page_size', String(pageSize));
			if (filter.status) params.set('status', filter.status);
			if (q) params.set('q', q);
			const result = await moderationAdminApi.listReports(params.toString() || undefined);
			if (result.ok) {
				const d = result.data as { items: Report[]; total?: number; total_pages?: number } | Report[];
				const items = Array.isArray(d) ? d : (d.items ?? []);
				const total = Array.isArray(d) ? items.length : (d.total ?? items.length);
				const totalPages = Array.isArray(d) ? 1 : (d.total_pages ?? Math.max(1, Math.ceil(total / pageSize)));
				return { data: items, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load reports'));
		},
		viewSnippet: ReportView
	});
</script>

<svelte:head>
	<title>{t('moderation.reports', 'Reports')} — JapaConnect Admin</title>
</svelte:head>

<div class="mb-4">
	<a href="/moderation" class="text-xs hover:underline" style="color: var(--color-muted)">&larr; {t('moderation.back', 'Back')}</a>
</div>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- ── State-action confirmation modal ─────────────────────────────────── -->
<Modal bind:open={showActionModal} title={actionLabel(actionType)}>
	<p class="mb-3 text-sm" style="color: var(--color-muted)">
		{t(`moderation.action_${actionType}_desc`, actionLabel(actionType))}
	</p>
	<textarea
		class="w-full rounded-md border px-3 py-2 text-sm"
		style="border-color: var(--color-border); background-color: var(--color-surface)"
		rows="3"
		placeholder={t('moderation.reason_optional', 'Reason (optional)')}
		bind:value={actionReason}
	></textarea>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showActionModal = false)}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button
			variant={actionType === 'dismiss' ? 'default' : 'destructive'}
			onclick={executeAction}
			disabled={acting}
		>
			{acting ? t('common.saving', 'Saving...') : actionLabel(actionType)}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body (mirrors the legacy expanded detail panel) ──── -->
{#snippet ReportView({ row }: { row: Report })}
	<div class="space-y-4">
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('moderation.reporter', 'Reporter')}</p>
				<p class="mt-1 text-sm font-mono">{row.reporter_user_id.slice(0, 12)}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('moderation.target', 'Target')}</p>
				<p class="mt-1 text-sm">{row.entity_type}</p>
				<p class="text-xs font-mono" style="color: var(--color-muted)">{row.entity_id}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('moderation.reason', 'Reason')}</p>
				<p class="mt-1 text-sm">{row.reason}</p>
			</div>
		</div>
		{#if row.notes}
			<div class="rounded-md border p-3 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)">
				<p class="mb-1 text-xs font-medium" style="color: var(--color-muted)">{t('moderation.notes', 'Notes')}</p>
				{row.notes}
			</div>
		{/if}
		{#if row.resolution_note}
			<div class="rounded-md border p-3 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)">
				<p class="mb-1 text-xs font-medium" style="color: var(--color-muted)">{t('moderation.resolution_note', 'Resolution Note')}</p>
				{row.resolution_note}
			</div>
		{/if}
		{#if row.evidence_urls && row.evidence_urls.length > 0}
			<div class="border-t pt-4" style="border-color: var(--color-border)">
				<p class="mb-2 text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('moderation.evidence', 'Evidence')}</p>
				<ul class="space-y-1">
					{#each row.evidence_urls as url (url)}
						<li><a href={url} target="_blank" rel="noopener" class="text-sm underline-offset-4 hover:underline" style="color: var(--color-primary)">{url}</a></li>
					{/each}
				</ul>
			</div>
		{/if}
	</div>
{/snippet}
