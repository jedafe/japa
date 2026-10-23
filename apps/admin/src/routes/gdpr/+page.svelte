<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /gdpr — read-only deletion-request list with an "Execute" state action.
	// Backend: GET /admin/gdpr/deletion-requests → gdprHandlers.ListPendingDeletions (RBAC: gdpr.deletions.view)
	//          POST /admin/gdpr/deletion-requests/{id}/execute → gdprHandlers.ExecuteDeletion
	// Ref: sections/19-gdpr-ndpr-compliance.md, ADR-057
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { gdprAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, Play } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	// Matches gen.ListPendingDeletionRequestsRow JSON output.
	type DeletionRequest = {
		id: string;
		user_id: string;
		status: string;
		reason?: string;
		completed_at?: string;
		created_at: string;
		progress_step: number;
		username?: string;
		display_name?: string;
	};

	const canManageDeletions = $derived(hasPermission('gdpr.deletions.view'));

	// ── State-action confirmation modal ─────────────────────────────────────
	// EntityPage's confirm Modal only covers default delete; the Execute action keeps a page-local Modal with richer copy.
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let showExecuteConfirm = $state(false);
	let executeTarget = $state<DeletionRequest | null>(null);
	let executing = $state(false);

	function openExecute(req: DeletionRequest) {
		executeTarget = req;
		showExecuteConfirm = true;
	}

	async function confirmExecuteDeletion() {
		if (!executeTarget) return;
		if (executing) return; // in-flight guard
		executing = true;
		pageError = '';
		try {
			const result = await gdprAdminApiExtended.executeDeletion(executeTarget.id);
			if (result.ok) {
				showExecuteConfirm = false;
				executeTarget = null;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'Failed to execute deletion');
			}
		} catch {
			pageError = t('common.error', 'Failed to execute deletion');
		} finally {
			executing = false;
		}
	}

	// ── Row actions ─────────────────────────────────────────────────────────
	// Custom actions replace defaults, so View is included explicitly; Execute only shows for pending requests when permitted.
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (row: DeletionRequest, ctx) => ctx?.openView(row) }
		];
		if (canManageDeletions) {
			acts.push({
				label: t('app.gdpr.erasure.execute', 'Execute'),
				icon: Play,
				variant: 'destructive',
				onclick: openExecute,
				show: (row: DeletionRequest) => row.status === 'pending'
			});
		}
		return acts;
	});

	// `fields` is required by EntityPageConfig but unused (readonly page); provided as an empty array.
	const fields: FieldDef[] = [];

	const config = $derived<EntityPageConfig>({
		title: t('app.gdpr.erasure.title', 'Data Erasure Requests'),
		description: t('app.gdpr.erasure.description', 'Manage user data deletion requests for GDPR compliance'),
		readonly: true,
		pageSize: 25,
		columns: [
			{
				key: 'display_name',
				label: t('app.gdpr.erasure.user', 'User'),
				sortable: true,
				render: (v: string, row: DeletionRequest) => `<span class="text-sm">${v || row.username || 'anonymous'}</span>`
			},
			{
				key: 'reason',
				label: t('app.gdpr.erasure.reason', 'Reason'),
				render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v || '—'}</span>`
			},
			{
				key: 'status',
				label: t('app.gdpr.erasure.status', 'Status'),
				render: (v: string) => {
					const colors = v === 'executed' ? 'bg-success/15 text-success dark:text-success' : v === 'pending' ? 'bg-warning/15 text-warning dark:text-warning' : 'bg-muted/15 text-muted dark:text-muted';
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors}">${v}</span>`;
				}
			},
			{
				key: 'created_at',
				label: t('app.gdpr.erasure.requested_at', 'Requested At'),
				sortable: true,
				render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleString()}</span>`
			}
		],
		actions: rowActions,
		fields,
		load: async ({ page, pageSize, q, filter }) => {
			// Fetch the full list; search/filter/pagination are applied client-side.
			const result = await gdprAdminApiExtended.listDeletionRequests('?page_size=1000');
			if (result.ok) {
				let data = (result.data as DeletionRequest[]) ?? [];
				const lower = (q ?? '').toLowerCase();
				if (lower) {
					data = data.filter((r) =>
						[r.display_name, r.username, r.reason, r.status].some((v) =>
							String(v ?? '').toLowerCase().includes(lower)
						)
					);
				}
				filter;
				const size = pageSize || 25;
				const offset = (page || 0) * size;
				const total = data.length;
				const totalPages = Math.max(1, Math.ceil(total / size));
				return {
					data: data.slice(offset, offset + size),
					total,
					totalPages
				};
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load deletion requests'));
		},
		viewSnippet: DeletionRequestView
	});
</script>

<svelte:head>
	<title>{t('app.gdpr.erasure.title', 'Data Erasure Requests')} — {siteName()} Admin</title>
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

<!-- Execute deletion confirmation (destructive action behind Modal) -->
<Modal bind:open={showExecuteConfirm} title={t('app.gdpr.erasure.execute_confirm_title', 'Confirm Data Erasure')} size="sm">
	{#if executeTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.gdpr.erasure.execute_confirm_body', 'This will permanently delete all personal data for user')}
			<strong class="font-mono">{executeTarget.user_id}</strong>.
			{t('app.gdpr.erasure.execute_confirm_warning', 'This action cannot be undone.')}
		</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showExecuteConfirm = false)} disabled={executing}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button variant="destructive" onclick={confirmExecuteDeletion} disabled={executing}>
			<Play class="me-2 h-4 w-4" />
			{executing ? t('common.processing', 'Processing...') : t('app.gdpr.erasure.execute', 'Execute')}
		</Button>
	</div>
</Modal>

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

<!-- View slide-over body (mirrors the legacy row details) -->
{#snippet DeletionRequestView({ row }: { row: DeletionRequest })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.gdpr.erasure.user', 'User')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.display_name || row.username || 'anonymous'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.gdpr.erasure.user_id', 'User ID')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.user_id}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.gdpr.erasure.reason', 'Reason')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.reason || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.gdpr.erasure.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.gdpr.erasure.requested_at', 'Requested At')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{new Date(row.created_at).toLocaleString()}</p>
		</div>
		{#if row.completed_at}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.gdpr.erasure.completed_at', 'Completed At')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{new Date(row.completed_at).toLocaleString()}</p>
			</div>
		{/if}
	</div>
{/snippet}
