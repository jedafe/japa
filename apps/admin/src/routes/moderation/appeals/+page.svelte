<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /moderation/appeals — migrated to the EntityPage scaffold; read-only list with
	// state actions (uphold / overturn / dismiss) via page-local confirm Modals.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { moderationAdminApi, adminCatalogApi, type CatalogOption } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { escapeHtml } from '$lib/utils/html';
	import { Eye, Gavel, RotateCcw, XCircle } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type Appeal = {
		id: string;
		user_id: string;
		username?: string;
		display_name?: string;
		entity_type: string;
		entity_id: string;
		subject_label?: string;
		reason: string;
		status: string;
		resolution_note?: string;
		created_at: string;
		updated_at?: string;
	};

	const canReview = $derived(hasPermission('moderation.appeal.review'));

	// ── State-action confirmation modal (shared by uphold / overturn / dismiss) ──
	let entityController = $state<EntityPageController | undefined>(undefined);

	let appealStatusOptions = $state<CatalogOption[]>([
		{ value: 'pending', label: t('app.moderation.queue.pending', 'Pending') },
		{ value: 'upheld', label: t('moderation.upheld', 'Upheld') },
		{ value: 'overturn', label: t('moderation.overturn', 'Overturn') },
		{ value: 'dismissed', label: t('moderation.dismissed', 'Dismissed') }
	]);

	$effect(() => {
		void (async () => {
			const res = await adminCatalogApi.get('moderation.appeals.status');
			if (res.ok && res.data?.length) appealStatusOptions = res.data;
		})();
	});

	let pageError = $state('');
	let acting = $state(false);
	let actionTarget = $state<Appeal | null>(null);
	let actionType = $state<'upheld' | 'overturn' | 'dismissed'>('upheld');
	let resolutionNote = $state('');
	let showActionModal = $state(false);

	function openAction(appeal: Appeal, type: 'upheld' | 'overturn' | 'dismissed') {
		actionTarget = appeal;
		actionType = type;
		resolutionNote = '';
		showActionModal = true;
	}

	const actionModalTitle = $derived(
		actionType === 'upheld'
			? t('moderation.uphold_appeal', 'Uphold Appeal')
			: actionType === 'overturn'
				? t('moderation.overturn_appeal', 'Overturn Appeal')
				: t('moderation.dismiss_appeal', 'Dismiss Appeal')
	);

	const actionModalConfirmLabel = $derived(
		actionType === 'upheld'
			? t('moderation.uphold', 'Uphold')
			: actionType === 'overturn'
				? t('moderation.overturn', 'Overturn')
				: t('moderation.dismiss', 'Dismiss')
	);

	async function confirmAction() {
		if (!actionTarget) return;
		acting = true;
		pageError = '';
		try {
			const body: Record<string, unknown> = { status: actionType };
			if (resolutionNote.trim()) body.resolution_note = resolutionNote.trim();
			const result = await moderationAdminApi.updateAppeal(actionTarget.id, body);
			if (result.ok) {
				showActionModal = false;
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

	// ── Row actions: View + state actions (only when pending & reviewer) ──────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row, ctx) => ctx?.openView(_row) }
		];
		if (canReview) {
			acts.push({
				label: t('moderation.uphold', 'Uphold'),
				icon: Gavel,
				onclick: (row: Appeal) => openAction(row, 'upheld'),
				show: (row: Appeal) => row.status === 'pending'
			});
			acts.push({
				label: t('moderation.overturn', 'Overturn'),
				icon: RotateCcw,
				onclick: (row: Appeal) => openAction(row, 'overturn'),
				show: (row: Appeal) => row.status === 'pending'
			});
			acts.push({
				label: t('moderation.dismiss', 'Dismiss'),
				icon: XCircle,
				variant: 'destructive',
				onclick: (row: Appeal) => openAction(row, 'dismissed'),
				show: (row: Appeal) => row.status === 'pending'
			});
		}
		return acts;
	});

	function statusBadgeClass(status: string) {
		switch (status) {
			case 'pending': return 'bg-warning/15 text-warning dark:text-warning';
			case 'upheld': return 'bg-danger/15 text-danger dark:text-danger';
			case 'overturn':
			case 'overturned': return 'bg-success/15 text-success dark:text-success';
			case 'dismissed': return 'bg-muted/15 text-muted dark:text-muted';
			default: return 'bg-muted/15 text-muted dark:text-muted';
		}
	}

	// `fields` is required by EntityPage even on a read-only page (used for edit-form seeding); none are editable here.
	const fields: FieldDef[] = [
		{ key: 'entity_type', label: t('moderation.entity_type', 'Entity Type'), type: 'text' },
		{ key: 'reason', label: t('moderation.appeal_reason', 'Appeal Reason'), type: 'textarea' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('moderation.appeals', 'Appeals'),
		description: t('moderation.appeals_description', 'Review user appeals against moderation actions.'),
		readonly: true,
		pageSize: 25,
		columns: [
			{
				key: 'entity_type',
				label: t('moderation.subject', 'Subject'),
				sortable: true,
				render: (v: string, row: Appeal) =>
					`<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium bg-info/15 text-info dark:text-info">${v}</span>` +
					(row.subject_label
						? `<div class="mt-0.5 text-xs">${escapeHtml(row.subject_label)}</div>`
						: '')
			},
			{
				key: 'display_name',
				label: t('moderation.user', 'User'),
				sortable: true,
				render: (v: string, row: Appeal) => `<span class="text-sm">${v || row.username || 'anonymous'}</span>`
			},
			{
				key: 'reason',
				label: t('moderation.appeal_reason', 'Appeal Reason'),
				render: (v: string) => `<span class="text-xs max-w-xs truncate block">${v}</span>`
			},
			{
				key: 'status',
				label: t('moderation.status', 'Status'),
				sortable: true,
				render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${statusBadgeClass(v)}">${v}</span>`
			},
			{
				key: 'created_at',
				label: t('moderation.date', 'Date'),
				sortable: true,
				render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleDateString()}</span>`
			}
		],
		filters: [
			{
				key: 'status',
				label: t('moderation.status', 'Status'),
				options: appealStatusOptions
			}
		],
		actions: rowActions,
		fields,
		load: async ({ page, pageSize, q, filter }) => {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(page * pageSize)
			});
			if (filter.status) params.set('status', filter.status);
			if (q) params.set('q', q);
			const result = await moderationAdminApi.listAppeals(params.toString());
			if (result.ok) {
				const data = result.data as Appeal[] | { items: Appeal[]; total: number; total_pages: number };
				if (Array.isArray(data)) {
					const start = page * pageSize;
					const total = data.length;
					return { data: data.slice(start, start + pageSize), total, totalPages: Math.max(1, Math.ceil(total / pageSize)) };
				}
				return {
					data: data.items ?? [],
					total: data.total ?? 0,
					totalPages: data.total_pages ?? 1
				};
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load appeals'));
		},
		viewSnippet: AppealView
	});
</script>

<svelte:head>
	<title>{t('moderation.appeals', 'Appeals')} — {siteName()} Admin</title>
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

<!-- ── State-action confirmation modal (uphold / overturn / dismiss) ─────── -->

<Modal bind:open={showActionModal} title={actionModalTitle}>
	<p class="mb-3 text-sm" style="color: var(--color-muted)">
		{actionType === 'upheld'
			? t('moderation.uphold_appeal_desc', 'Upholding this appeal confirms the original moderation action stands.')
			: actionType === 'overturn'
				? t('moderation.overturn_appeal_desc', 'Overturning this appeal reverses the original moderation action.')
				: t('moderation.dismiss_appeal_desc', 'Dismissing this appeal closes it without changing the original action.')}
	</p>
	<textarea
		class="w-full rounded-md border px-3 py-2 text-sm"
		style="border-color: var(--color-border); background-color: var(--color-surface)"
		rows="3"
		placeholder={t('moderation.note_optional', 'Note (optional)')}
		bind:value={resolutionNote}
	></textarea>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showActionModal = false)} disabled={acting}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button
			variant={actionType === 'dismissed' ? 'destructive' : 'default'}
			onclick={confirmAction}
			disabled={acting}
		>
			{acting ? t('common.saving', 'Saving…') : actionModalConfirmLabel}
		</Button>
	</div>
</Modal>

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

{#snippet AppealView({ row }: { row: Appeal })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('moderation.subject', 'Subject')}</p>
			<p class="mt-1">
				<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium bg-info/15 text-info dark:text-info">{row.entity_type}</span>
				{#if row.subject_label}
					<span class="ms-2 text-sm" style="color: var(--color-foreground)">{row.subject_label}</span>
				{/if}
				<span class="ms-2 text-xs font-mono" style="color: var(--color-muted)">{row.entity_id}</span>
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('moderation.user', 'User')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.display_name || row.username || 'anonymous'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('moderation.appeal_reason', 'Appeal Reason')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.reason}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('moderation.status', 'Status')}</p>
			<p class="mt-1">
				<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {statusBadgeClass(row.status)}">{row.status}</span>
			</p>
		</div>
		{#if row.resolution_note}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('moderation.resolution_note', 'Resolution Note')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.resolution_note}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('moderation.date', 'Date')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{new Date(row.created_at).toLocaleString()}</p>
		</div>
		{#if row.updated_at}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('moderation.updated_at', 'Updated at')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{new Date(row.updated_at).toLocaleString()}</p>
			</div>
		{/if}
	</div>
{/snippet}
