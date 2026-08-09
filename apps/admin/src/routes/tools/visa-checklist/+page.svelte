<script lang="ts">
	// /tools/visa-checklist — read-only monitoring page for visa document checklist user progress.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { visaChecklistAdminApi } from '$lib/api';

	type ChecklistItem = {
		id: string;
		user_email: string;
		dest_country: string;
		visa_type: string;
		items_total: number;
		items_completed: number;
		completion_pct: number;
		updated_at?: string;
	};

	const canManage = $derived(hasPermission('checklist.manage'));

	// ── State ───────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: ChecklistItem, ctx) => ctx?.openView(_row) }
		];
		return acts;
	});

	// `fields` is required by EntityPageConfig; readonly page so the form is never shown, but kept for shape completeness.
	const fields: FieldDef[] = [
		{ key: 'user_email', label: t('app.tools.col_user_email', 'User Email'), type: 'email' },
		{ key: 'dest_country', label: t('app.tools.col_dest_country', 'Destination Country'), type: 'text' },
		{ key: 'visa_type', label: t('app.tools.col_visa_type', 'Visa Type'), type: 'text' },
		{ key: 'items_total', label: t('app.tools.col_items_total', 'Items Total'), type: 'number' },
		{ key: 'items_completed', label: t('app.tools.col_items_completed', 'Items Completed'), type: 'number' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.tools.visa_checklist_heading', 'Visa Checklist Progress'),
		description: t('app.tools.visa_checklist_desc', 'Monitor user progress on visa document checklists'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'user_email', label: t('app.tools.col_user_email', 'User Email'), sortable: true },
			{ key: 'dest_country', label: t('app.tools.col_dest_country', 'Destination'), sortable: true },
			{ key: 'visa_type', label: t('app.tools.col_visa_type', 'Visa Type'), sortable: true },
			{
				key: 'items_total',
				label: t('app.tools.col_items_total', 'Items Total'),
				sortable: true,
				render: (value: number) => (value != null ? String(value) : '—')
			},
			{
				key: 'items_completed',
				label: t('app.tools.col_items_completed', 'Completed'),
				sortable: true,
				render: (value: number) => (value != null ? String(value) : '—')
			},
			{
				key: 'completion_pct',
				label: t('app.tools.col_completion_pct', 'Completion %'),
				sortable: true,
				render: (value: number) => (value != null ? `${value}%` : '—')
			},
			{
				key: 'updated_at',
				label: t('app.tools.col_updated_at', 'Updated'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async () => {
			const result = await visaChecklistAdminApi.listProgress();
			if (result.ok) {
				const data = (result.data as ChecklistItem[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: ChecklistView
	});
</script>

<svelte:head>
	<title>{t('app.tools.visa_checklist_page_title', 'Visa Checklist Progress — JapaConnect Admin')}</title>
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

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet ChecklistView({ row }: { row: ChecklistItem })}
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
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_visa_type', 'Visa Type')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.visa_type || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_items_total', 'Items Total')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.items_total != null ? String(row.items_total) : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_items_completed', 'Items Completed')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.items_completed != null ? String(row.items_completed) : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_completion_pct', 'Completion %')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.completion_pct != null ? `${row.completion_pct}%` : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_updated_at', 'Updated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.updated_at ? new Date(row.updated_at).toLocaleString() : '—'}</p>
		</div>
	</div>
{/snippet}
