<script lang="ts">
	// /community/mutual-aid — admin HITL review queue for Mutual Aid requests with status update.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { mutualAidAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, RefreshCw } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type MutualAidItem = {
		id: string;
		title: string;
		type: string;
		status: string;
		created_at: string;
		description?: string;
	};

	const canManage = $derived(hasPermission('mutual_aid.moderate'));

	const statusOptions = [
		{ value: 'active', label: t('mutual_aid.status.active', 'Active') },
		{ value: 'fulfilled', label: t('mutual_aid.status.fulfilled', 'Fulfilled') },
		{ value: 'closed', label: t('mutual_aid.status.closed', 'Closed') },
		{ value: 'expired', label: t('mutual_aid.status.expired', 'Expired') }
	];

	// ── State ───────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);
	let actionTarget = $state<MutualAidItem | null>(null);
	let showStatusModal = $state(false);
	let newStatus = $state('active');

	function openSetStatus(item: MutualAidItem) {
		actionTarget = item;
		newStatus = item.status || 'active';
		showStatusModal = true;
	}

	async function confirmSetStatus() {
		if (!actionTarget) return;
		acting = true;
		pageError = '';
		try {
			const result = await mutualAidAdminApi.updateStatus(actionTarget.id, { status: newStatus });
			if (result.ok) {
				showStatusModal = false;
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
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: MutualAidItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('app.mutual_aid.set_status', 'Set Status'),
				icon: RefreshCw,
				onclick: openSetStatus,
				show: () => true
			});
		}
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'title', label: t('app.mutual_aid.col_title', 'Title'), type: 'text' },
		{ key: 'type', label: t('app.mutual_aid.col_type', 'Type'), type: 'text' },
		{ key: 'status', label: t('app.mutual_aid.col_status', 'Status'), type: 'select', options: statusOptions },
		{ key: 'description', label: t('app.mutual_aid.detail_description', 'Description'), type: 'textarea' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.mutual_aid.review_queue_heading', 'Mutual Aid Review Queue'),
		description: t('app.mutual_aid.review_queue_desc', 'Review and moderate mutual aid requests'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'title', label: t('app.mutual_aid.col_title', 'Title'), sortable: true },
			{
				key: 'type',
				label: t('app.mutual_aid.col_type', 'Type'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'status',
				label: t('app.mutual_aid.col_status', 'Status'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'created_at',
				label: t('app.mutual_aid.col_created_at', 'Created'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async () => {
			const result = await mutualAidAdminApi.list();
			if (result.ok) {
				const data = (result.data as MutualAidItem[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: MutualAidView
	});
</script>

<svelte:head>
	<title>{t('app.mutual_aid.review_queue_page_title', 'Mutual Aid Review Queue — JapaConnect Admin')}</title>
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

<!-- ── Set Status modal ──────────────────────────────────────────────────── -->
<Modal bind:open={showStatusModal} title={t('app.mutual_aid.modal_set_status', 'Set Status')}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.mutual_aid.set_status_confirm', 'Update the status of "{title}".').replace('{title}', actionTarget?.title ?? '')}
		</p>
		<div>
			<label for="status-select" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.mutual_aid.col_status', 'Status')}</label>
			<select
				id="status-select"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={newStatus}
			>
				{#each statusOptions as opt}
					<option value={opt.value}>{opt.label}</option>
				{/each}
			</select>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showStatusModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={confirmSetStatus} disabled={acting}>
			{acting ? t('app.mutual_aid.updating', 'Updating...') : t('common.update', 'Update')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet MutualAidView({ row }: { row: MutualAidItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.mutual_aid.col_title', 'Title')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.mutual_aid.col_type', 'Type')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.type || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.mutual_aid.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.mutual_aid.col_created_at', 'Created')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.mutual_aid.detail_description', 'Description')}</p>
			<p class="mt-1 whitespace-pre-wrap text-sm" style="color: var(--color-foreground)">{row.description || '—'}</p>
		</div>
	</div>
{/snippet}
