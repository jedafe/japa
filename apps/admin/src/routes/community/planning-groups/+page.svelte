<script lang="ts">
	// /community/planning-groups — admin HITL review queue for Planning Groups with extend action.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { planningGroupAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CalendarClock } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type PlanningGroupItem = {
		id: string;
		name: string;
		member_count: number;
		status: string;
		created_at: string;
		description?: string;
	};

	const canManage = $derived(hasPermission('planning_group.manage_any'));

	// ── State ───────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);
	let actionTarget = $state<PlanningGroupItem | null>(null);
	let showExtendModal = $state(false);
	let extendDays = $state(7);

	function openExtend(item: PlanningGroupItem) {
		actionTarget = item;
		extendDays = 7;
		showExtendModal = true;
	}

	async function confirmExtend() {
		if (!actionTarget) return;
		acting = true;
		pageError = '';
		try {
			const result = await planningGroupAdminApi.extend(actionTarget.id, { days: extendDays });
			if (result.ok) {
				showExtendModal = false;
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
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: PlanningGroupItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('app.planning_group.extend', 'Extend'),
				icon: CalendarClock,
				onclick: openExtend,
				show: () => true
			});
		}
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'name', label: t('app.planning_group.col_name', 'Name'), type: 'text' },
		{ key: 'member_count', label: t('app.planning_group.col_member_count', 'Members'), type: 'number' },
		{ key: 'status', label: t('app.planning_group.col_status', 'Status'), type: 'text' },
		{ key: 'description', label: t('app.planning_group.detail_description', 'Description'), type: 'textarea' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.planning_group.review_queue_heading', 'Planning Groups Review Queue'),
		description: t('app.planning_group.review_queue_desc', 'Review and manage planning groups'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('app.planning_group.col_name', 'Name'), sortable: true },
			{
				key: 'member_count',
				label: t('app.planning_group.col_member_count', 'Members'),
				sortable: true,
				render: (value: number) => (value != null ? String(value) : '—')
			},
			{
				key: 'status',
				label: t('app.planning_group.col_status', 'Status'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'created_at',
				label: t('app.planning_group.col_created_at', 'Created'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async () => {
			const result = await planningGroupAdminApi.list();
			if (result.ok) {
				const data = (result.data as PlanningGroupItem[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: PlanningGroupView
	});
</script>

<svelte:head>
	<title>{t('app.planning_group.review_queue_page_title', 'Planning Groups Review Queue — JapaConnect Admin')}</title>
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

<!-- ── Extend confirmation modal ─────────────────────────────────────────── -->
<Modal bind:open={showExtendModal} title={t('app.planning_group.modal_extend', 'Extend Planning Group')}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.planning_group.extend_confirm', 'Extend "{name}" by additional days.').replace('{name}', actionTarget?.name ?? '')}
		</p>
		<div>
			<label for="extend-days" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.planning_group.days', 'Days')}</label>
			<input
				id="extend-days"
				type="number"
				min="1"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={extendDays}
			/>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showExtendModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={confirmExtend} disabled={acting}>
			{acting ? t('app.planning_group.extending', 'Extending...') : t('app.planning_group.extend', 'Extend')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet PlanningGroupView({ row }: { row: PlanningGroupItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.planning_group.col_name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.planning_group.col_member_count', 'Members')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.member_count != null ? row.member_count : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.planning_group.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.planning_group.col_created_at', 'Created')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.planning_group.detail_description', 'Description')}</p>
			<p class="mt-1 whitespace-pre-wrap text-sm" style="color: var(--color-foreground)">{row.description || '—'}</p>
		</div>
	</div>
{/snippet}
