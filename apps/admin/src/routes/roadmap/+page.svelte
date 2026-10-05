<script lang="ts">
	// /roadmap — EntityPage scaffold (§113). Items table with status/category
	// filters, soft-delete restore + hard delete, and a "Change Status" state
	// action that drives the lifecycle state machine via PATCH .../status.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { roadmapAdminApi, roadmapCategoriesAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, Pencil, Trash2, Trash, RotateCcw, ArrowRightLeft } from 'lucide-svelte';
	import type { ActionItem, ActionContext, FilterDef } from '$lib/components/ui/data-table';

	type RoadmapItem = {
		id: string;
		title: string;
		slug: string;
		summary: string | null;
		body: string | null;
		category_id: string | null;
		category_name: string | null;
		status: string;
		sort_order: number;
		is_flagship: boolean;
		target_window: string | null;
		feature_flag_key: string | null;
		vote_count: number;
		submitted_by: string | null;
		shipped_at: string | null;
		created_at: string;
		updated_at: string;
		deleted_at: string | null;
	};

	// Lifecycle statuses (§113). Enum-like — the backend validates the same set.
	const STATUS_OPTIONS = [
		{ value: 'pending', label: t('roadmap.status.pending', 'Pending') },
		{ value: 'under_review', label: t('roadmap.status.under_review', 'Under Review') },
		{ value: 'planned', label: t('roadmap.status.planned', 'Planned') },
		{ value: 'in_progress', label: t('roadmap.status.in_progress', 'In Progress') },
		{ value: 'shipped', label: t('roadmap.status.shipped', 'Shipped') },
		{ value: 'declined', label: t('roadmap.status.declined', 'Declined') },
		{ value: 'archived', label: t('roadmap.status.archived', 'Archived') }
	];

	const STATUS_COLORS: Record<string, string> = {
		pending: 'bg-warning/15 text-warning',
		under_review: 'bg-info/15 text-info',
		planned: 'bg-info/15 text-info',
		in_progress: 'bg-primary/15 text-primary',
		shipped: 'bg-success/15 text-success',
		declined: 'bg-danger/15 text-danger',
		archived: 'bg-muted/30 text-muted'
	};

	const canEdit = $derived(hasPermission('roadmap.edit'));
	const canDelete = $derived(hasPermission('roadmap.delete'));
	const canModerate = $derived(hasPermission('roadmap.moderate'));
	const canHardDelete = $derived(hasPermission('admin.hard_delete'));

	// ── Filter + select options ────────────────────────────────────────────
	// Status options are the fixed §113 lifecycle enum; category options come
	// from the categories API on mount (Rule 16 — no hardcoded lists).
	let categoryOptions = $state<Array<{ value: string; label: string }>>([]);
	let filterDefs = $state<FilterDef[]>([
		{ key: 'status', label: t('common.status', 'Status'), options: STATUS_OPTIONS },
		{ key: 'category_id', label: t('roadmap.category', 'Category'), options: [] }
	]);

	async function loadCategoryOptions() {
		const res = await roadmapCategoriesAdminApi.list();
		if (res.ok) {
			const cats = ((res.data as Array<{ id: string; name: string }>) ?? []).map((c) => ({
				value: c.id,
				label: c.name
			}));
			categoryOptions = cats;
			filterDefs = filterDefs.map((def) =>
				def.key === 'category_id' ? { ...def, options: cats } : def
			);
		}
	}

	$effect(() => {
		loadCategoryOptions();
	});

	// ── State-action confirmation modals ───────────────────────────────────
	// EntityPage's built-in confirms only cover the default action set; the
	// custom actions array keeps page-local modals for status changes,
	// restore, and hard delete (mirrors /users).
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);
	let actionTarget = $state<RoadmapItem | null>(null);

	let showStatusModal = $state(false);
	let statusChoice = $state('');
	let showRestoreConfirm = $state(false);
	let showHardDeleteConfirm = $state(false);

	function openStatusModal(row: RoadmapItem) {
		actionTarget = row;
		statusChoice = row.status;
		showStatusModal = true;
	}

	function openRestore(row: RoadmapItem) {
		actionTarget = row;
		showRestoreConfirm = true;
	}

	function openHardDelete(row: RoadmapItem) {
		actionTarget = row;
		showHardDeleteConfirm = true;
	}

	async function confirmStatusChange() {
		if (!actionTarget || !statusChoice) return;
		acting = true;
		try {
			const result = await roadmapAdminApi.setStatus(actionTarget.id, statusChoice);
			if (result.ok) {
				showStatusModal = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'Failed to update status');
			}
		} catch {
			pageError = t('common.error', 'Failed to update status');
		} finally {
			acting = false;
		}
	}

	async function confirmRestore() {
		if (!actionTarget) return;
		acting = true;
		try {
			const result = await roadmapAdminApi.restore(actionTarget.id);
			if (result.ok) {
				showRestoreConfirm = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'Failed to restore item');
			}
		} catch {
			pageError = t('common.error', 'Failed to restore item');
		} finally {
			acting = false;
		}
	}

	async function confirmHardDelete() {
		if (!actionTarget) return;
		acting = true;
		try {
			const result = await roadmapAdminApi.hardDelete(actionTarget.id);
			if (result.ok) {
				showHardDeleteConfirm = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'Failed to hard delete item');
			}
		} catch {
			pageError = t('common.error', 'Failed to hard delete item');
		} finally {
			acting = false;
		}
	}

	// ── Row actions ────────────────────────────────────────────────────────
	// Custom actions replace the defaults entirely — View/Edit/Delete are
	// re-declared and delegate to the scaffold via ActionContext; Restore and
	// Hard Delete run through page-local confirm modals (mirrors /users).
	const isDeleted = (row: RoadmapItem) => row.deleted_at != null;

	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (row: RoadmapItem, ctx?: ActionContext) => ctx?.openView(row) }
		];
		if (canEdit) {
			acts.push({
				label: t('common.edit', 'Edit'),
				icon: Pencil,
				onclick: (row: RoadmapItem, ctx?: ActionContext) => ctx?.openEdit(row),
				show: (row: RoadmapItem) => !isDeleted(row)
			});
		}
		if (canModerate) {
			acts.push({
				label: t('roadmap.change_status', 'Change Status'),
				icon: ArrowRightLeft,
				onclick: openStatusModal,
				show: (row: RoadmapItem) => !isDeleted(row)
			});
		}
		if (canDelete) {
			acts.push({
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (row: RoadmapItem, ctx?: ActionContext) => ctx?.openDelete(row),
				show: (row: RoadmapItem) => !isDeleted(row)
			});
		}
		if (canEdit) {
			acts.push({
				label: t('common.restore', 'Restore'),
				icon: RotateCcw,
				onclick: openRestore,
				show: (row: RoadmapItem) => isDeleted(row)
			});
		}
		if (canHardDelete) {
			acts.push({
				label: t('common.hardDelete', 'Hard Delete'),
				icon: Trash,
				variant: 'destructive',
				onclick: openHardDelete,
				show: (row: RoadmapItem) => isDeleted(row)
			});
		}
		return acts;
	});

	// ── Form fields ─────────────────────────────────────────────────────────
	// `fields` is $derived so the category select picks up options once the
	// categories API responds.
	const fields = $derived<FieldDef[]>([
		{ key: 'title', label: t('roadmap.title', 'Title'), type: 'text', required: true },
		{
			key: 'slug',
			label: t('roadmap.slug', 'Slug'),
			type: 'text',
			placeholder: 'auto',
			help: t('roadmap.slug_hint', 'Leave empty to auto-generate from the title.')
		},
		{ key: 'summary', label: t('roadmap.summary', 'Summary'), type: 'textarea' },
		{ key: 'body', label: t('roadmap.body', 'Body'), type: 'textarea' },
		{
			key: 'category_id',
			label: t('roadmap.category', 'Category'),
			type: 'select',
			options: categoryOptions,
			placeholder: t('roadmap.no_category', 'No category')
		},
		{ key: 'status', label: t('common.status', 'Status'), type: 'select', options: STATUS_OPTIONS },
		{ key: 'sort_order', label: t('roadmap.sort_order', 'Sort Order'), type: 'number' },
		{ key: 'is_flagship', label: t('roadmap.is_flagship', 'Flagship'), type: 'checkbox' },
		{
			key: 'target_window',
			label: t('roadmap.target_window', 'Target Window'),
			type: 'text',
			placeholder: 'Q3 2026'
		},
		{
			key: 'feature_flag_key',
			label: t('roadmap.feature_flag_key', 'Feature Flag Key'),
			type: 'text',
			placeholder: 'feature.example'
		}
	]);

	function itemBody(body: Record<string, any>) {
		return {
			title: String(body.title ?? '').trim(),
			slug: String(body.slug ?? '').trim(),
			summary: String(body.summary ?? '').trim(),
			body: String(body.body ?? '').trim(),
			category_id: String(body.category_id ?? ''),
			sort_order: Number(body.sort_order ?? 0),
			is_flagship: !!body.is_flagship,
			target_window: String(body.target_window ?? '').trim(),
			feature_flag_key: String(body.feature_flag_key ?? '').trim()
		};
	}

	// Config is $derived so filterDefs/categoryOptions updates propagate.
	const config = $derived<EntityPageConfig>({
		title: t('roadmap.title_plural', 'Roadmap'),
		description: t('roadmap.description', 'Manage public product roadmap items.'),
		createLabel: t('roadmap.create', 'Create Item'),
		createPermission: 'roadmap.create',
		editPermission: 'roadmap.edit',
		deletePermission: 'roadmap.delete',
		hardDeletePermission: 'admin.hard_delete',
		restorePermission: 'roadmap.edit',
		pageSize: 25,
		exportable: true,
		exportFilename: 'roadmap-items-export',
		columns: [
			{
				key: 'title',
				label: t('roadmap.title', 'Title'),
				render: (v: string, row: RoadmapItem) =>
					`<span class="font-medium" style="color: var(--color-primary)">${v}</span>` +
					(isDeleted(row)
						? ` <span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium bg-muted/30 text-muted">${t('common.deleted', 'deleted')}</span>`
						: '')
			},
			{
				key: 'status',
				label: t('common.status', 'Status'),
				render: (v: string) => {
					const label = STATUS_OPTIONS.find((s) => s.value === v)?.label ?? v;
					const colors = STATUS_COLORS[v] ?? 'bg-muted/30 text-muted';
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors}">${label}</span>`;
				}
			},
			{
				key: 'category_name',
				label: t('roadmap.category', 'Category'),
				render: (v: string | null) =>
					v ?? `<span style="color: var(--color-muted)">—</span>`
			},
			{ key: 'vote_count', label: t('roadmap.votes', 'Votes'), sortable: true, sortKey: 'votes' },
			{
				key: 'target_window',
				label: t('roadmap.target_window', 'Target Window'),
				render: (v: string | null) =>
					v ?? `<span style="color: var(--color-muted)">—</span>`
			},
			{
				key: 'is_flagship',
				label: t('roadmap.is_flagship', 'Flagship'),
				render: (v: boolean) =>
					v
						? `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium bg-warning/15 text-warning">★</span>`
						: `<span style="color: var(--color-muted)">—</span>`
			},
			{
				key: 'created_at',
				label: t('common.created', 'Created'),
				sortable: true,
				sortKey: 'recent',
				render: (value: string) => new Date(value).toLocaleDateString()
			}
		],
		filters: filterDefs,
		actions: rowActions,
		fields,
		isDeleted,
		load: async ({ page, pageSize, q, sort, filter }) => {
			// The backend takes page_size/page_offset, `q` for search, and sort
			// keywords (recent | votes | title) supplied via column sortKey.
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(page * pageSize),
				include_deleted: 'true'
			});
			if (q) params.set('q', q);
			if (filter.status) params.set('status', filter.status);
			if (filter.category_id) params.set('category_id', filter.category_id);
			if (sort) {
				const [key] = sort.split(':');
				params.set('sort', key);
			}
			const result = await roadmapAdminApi.list(params.toString());
			if (result.ok) {
				const d = result.data as { items: RoadmapItem[]; total: number; total_pages: number };
				return { data: d.items ?? [], total: d.total ?? 0, totalPages: d.total_pages ?? 1 };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load roadmap items'));
		},
		create: async (body) => {
			const res = await roadmapAdminApi.create({
				...itemBody(body),
				status: String(body.status ?? '') || undefined
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body, row) => {
			// Status is not part of the PATCH contract — the backend ignores it
			// there and only mutates it through the lifecycle endpoint. Route a
			// changed status through setStatus first so invalid transitions
			// surface their 409 instead of being silently dropped.
			const nextStatus = String(body.status ?? '');
			if (nextStatus && row && nextStatus !== row.status) {
				const statusRes = await roadmapAdminApi.setStatus(id, nextStatus);
				if (!statusRes.ok) return { ok: false, error: statusRes.error };
			}
			const res = await roadmapAdminApi.update(id, itemBody(body));
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await roadmapAdminApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: ItemView
	});
</script>

<svelte:head>
	<title>{t('roadmap.title_plural', 'Roadmap')} — JapaConnect Admin</title>
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

<!-- Change-status modal -->
<Modal bind:open={showStatusModal} title={t('roadmap.change_status', 'Change Status')}>
	{#if actionTarget}
		<p class="text-sm" style="color: var(--color-muted)">
			{actionTarget.title}
		</p>
		<div class="mt-3">
			<label for="roadmap-status" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">
				{t('common.status', 'Status')}
			</label>
			<select
				id="roadmap-status"
				bind:value={statusChoice}
				class="w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
			>
				{#each STATUS_OPTIONS as opt (opt.value)}
					<option value={opt.value}>{opt.label}</option>
				{/each}
			</select>
			<p class="mt-1 text-xs" style="color: var(--color-muted)">
				{t('roadmap.status_hint', 'Only valid lifecycle transitions are accepted.')}
			</p>
		</div>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showStatusModal = false)} disabled={acting}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button onclick={confirmStatusChange} disabled={acting || !statusChoice || statusChoice === actionTarget?.status}>
			{t('common.save', 'Save Changes')}
		</Button>
	</div>
</Modal>

<!-- Restore confirm -->
<Modal bind:open={showRestoreConfirm} title={t('common.restore', 'Restore')}>
	{#if actionTarget}
		<p class="text-sm" style="color: var(--color-muted)">
			{t('common.restoreConfirm', 'Are you sure you want to restore this record?')}
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showRestoreConfirm = false)} disabled={acting}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button onclick={confirmRestore} disabled={acting}>
			{t('common.restore', 'Restore')}
		</Button>
	</div>
</Modal>

<!-- Hard-delete confirm -->
<Modal bind:open={showHardDeleteConfirm} title={t('common.hardDelete', 'Hard Delete')}>
	{#if actionTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('common.hardDeleteWarning', 'This will permanently erase the record and all related data. This action cannot be undone.')}.
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showHardDeleteConfirm = false)} disabled={acting}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button variant="destructive" onclick={confirmHardDelete} disabled={acting}>
			{t('common.hardDelete', 'Hard Delete')}
		</Button>
	</div>
</Modal>

<!-- View slide-over body -->
{#snippet ItemView({ row }: { row: RoadmapItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('roadmap.title', 'Title')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('roadmap.slug', 'Slug')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{STATUS_OPTIONS.find((s) => s.value === row.status)?.label ?? row.status}
			</p>
		</div>
		{#if row.summary}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('roadmap.summary', 'Summary')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.summary}</p>
			</div>
		{/if}
		{#if row.body}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('roadmap.body', 'Body')}</p>
				<p class="mt-1 whitespace-pre-wrap text-sm" style="color: var(--color-foreground)">{row.body}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('roadmap.category', 'Category')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.category_name ?? '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('roadmap.votes', 'Votes')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.vote_count}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('roadmap.target_window', 'Target Window')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.target_window ?? '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('roadmap.feature_flag_key', 'Feature Flag Key')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.feature_flag_key ?? '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('roadmap.is_flagship', 'Flagship')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_flagship ? '★' : '—'}</p>
		</div>
		{#if row.submitted_by}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('roadmap.submitted_by', 'Submitted By')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.submitted_by}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.created', 'Created')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(row.created_at).toLocaleString()}</p>
		</div>
	</div>
{/snippet}
