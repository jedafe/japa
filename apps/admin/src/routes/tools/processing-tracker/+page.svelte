<script lang="ts">
	// /tools/processing-tracker — management page for user-submitted visa processing timeline entries.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, Trash2 } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { processingTrackerAdminApi } from '$lib/api';

	type TrackerItem = {
		id: string;
		user_email: string;
		dest_country: string;
		visa_category: string;
		submitted_date?: string;
		status: string;
		processing_days?: number;
	};

	const canManage = $derived(hasPermission('visa_timeline.manage_any'));

	// ── State ───────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);
	let actionTarget = $state<TrackerItem | null>(null);
	let showDeleteModal = $state(false);

	function openDelete(item: TrackerItem) {
		actionTarget = item;
		showDeleteModal = true;
	}

	async function confirmDelete() {
		if (!actionTarget) return;
		acting = true;
		pageError = '';
		try {
			const result = await processingTrackerAdminApi.delete(actionTarget.id);
			if (result.ok) {
				showDeleteModal = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: TrackerItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: openDelete,
				show: () => true
			});
		}
		return acts;
	});

	// `fields` is required by EntityPageConfig; readonly page so the form is never shown, but kept for shape completeness.
	const fields: FieldDef[] = [
		{ key: 'user_email', label: t('app.tools.col_user_email', 'User Email'), type: 'email' },
		{ key: 'dest_country', label: t('app.tools.col_dest_country', 'Destination Country'), type: 'text' },
		{ key: 'visa_category', label: t('app.tools.col_visa_category', 'Visa Category'), type: 'text' },
		{ key: 'status', label: t('app.tools.col_status', 'Status'), type: 'text' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.tools.processing_tracker_heading', 'Processing Time Tracker'),
		description: t('app.tools.processing_tracker_desc', 'Manage user-submitted visa processing timeline entries'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'user_email', label: t('app.tools.col_user_email', 'User Email'), sortable: true },
			{ key: 'dest_country', label: t('app.tools.col_dest_country', 'Destination'), sortable: true },
			{ key: 'visa_category', label: t('app.tools.col_visa_category', 'Visa Category'), sortable: true },
			{
				key: 'submitted_date',
				label: t('app.tools.col_submitted_date', 'Submitted'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			},
			{
				key: 'status',
				label: t('app.tools.col_status', 'Status'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'processing_days',
				label: t('app.tools.col_processing_days', 'Processing Days'),
				sortable: true,
				render: (value: number) => (value != null ? String(value) : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async () => {
			const result = await processingTrackerAdminApi.list();
			if (result.ok) {
				const data = (result.data as TrackerItem[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: TrackerView
	});
</script>

<svelte:head>
	<title>{t('app.tools.processing_tracker_page_title', 'Processing Time Tracker — JapaConnect Admin')}</title>
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

<!-- ── Delete confirmation modal ─────────────────────────────────────────── -->
<Modal bind:open={showDeleteModal} title={t('app.tools.modal_delete_entry', 'Delete Entry')}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.tools.delete_confirm', 'Are you sure you want to delete this processing timeline entry? This cannot be undone.')}
		</p>
		{#if actionTarget}
			<div
				class="rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
			>
				<p><strong>{actionTarget.user_email || '—'}</strong></p>
				<p class="mt-1" style="color: var(--color-muted)">{actionTarget.dest_country || '—'} · {actionTarget.visa_category || '—'}</p>
			</div>
		{/if}
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmDelete} disabled={acting}>
			{acting ? t('common.deleting', 'Deleting...') : t('common.delete', 'Delete')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet TrackerView({ row }: { row: TrackerItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_user_email', 'User Email')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.user_email || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_dest_country', 'Destination Country')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.dest_country || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_visa_category', 'Visa Category')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.visa_category || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_submitted_date', 'Submitted Date')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.submitted_date ? new Date(row.submitted_date).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_processing_days', 'Processing Days')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.processing_days != null ? String(row.processing_days) : '—'}</p>
		</div>
	</div>
{/snippet}
