<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { comparisonAdminApi, type ComparisonEntry, type ComparisonType } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { escapeHtml } from '$lib/utils/html';
	import { Trash2 } from 'lucide-svelte';
	import type { FilterDef, BulkAction } from '$lib/components/ui/data-table';
	import type { Snippet } from 'svelte';

	type Comparison = ComparisonEntry;

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkDeleting = $state(false);

	let types = $state<ComparisonType[]>([]);
	let visibilityOptions = $state<{ value: string; label: string }[]>([]);
	let statusOptions = $state<{ value: string; label: string }[]>([]);

	const canCreateComparison = $derived(hasPermission('comparison.create'));
	const canEditComparison = $derived(
		hasPermission('comparison.edit_any') || hasPermission('comparison.edit_own')
	);
	const canDeleteComparison = $derived(
		hasPermission('comparison.delete_any') || hasPermission('comparison.delete_own')
	);
	// Rule 11 amendment: permanent delete requires the dedicated node.
	const canHardDelete = $derived(hasPermission('admin.hard_delete'));

	async function loadOptions() {
		const [typesResult, visResult, statusResult] = await Promise.all([
			comparisonAdminApi.listTypes(),
			comparisonAdminApi.getVisibilityOptions(),
			comparisonAdminApi.getStatusOptions()
		]);

		if (typesResult.ok) {
			types = (typesResult.data as ComparisonType[]) ?? [];
		}

		if (visResult.ok && visResult.data) {
			visibilityOptions = (visResult.data as { value: string; label: string }[]).map((o) => ({
				value: o.value,
				label: t(`comparisons.vis_${o.value}`, o.label)
			}));
		}

		if (statusResult.ok && statusResult.data) {
			statusOptions = (statusResult.data as { value: string; label: string }[]).map((o) => ({
				value: o.value,
				label: t(`comparisons.status_${o.value}`, o.label)
			}));
		}
	}

	$effect(() => {
		loadOptions();
	});

	function slugify(text: string): string {
		return text
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-|-$/g, '');
	}

	const typeOptions = $derived([
		{ value: '', label: t('comparisons.type_none', 'No type (generic)') },
		...types.map((tp) => ({ value: tp.id, label: tp.name }))
	]);

	const fields = $derived<FieldDef[]>([
		{ key: 'title', label: t('comparisons.col_title', 'Title'), type: 'text', required: true },
		{ key: 'slug', label: t('comparisons.col_slug', 'Slug'), type: 'text', required: true },
		{
			key: 'type_id',
			label: t('comparisons.type', 'Comparison Type'),
			type: 'select',
			options: typeOptions,
			readonlyInEdit: true
		},
		{
			key: 'visibility',
			label: t('comparisons.col_visibility', 'Visibility'),
			type: 'select',
			options: visibilityOptions.length ? visibilityOptions : [{ value: 'members', label: 'Members' }],
			required: true
		},
		{
			key: 'status',
			label: t('comparisons.col_status', 'Status'),
			type: 'select',
			options: statusOptions.length
				? statusOptions
				: [
						{ value: 'draft', label: 'Draft' },
						{ value: 'published', label: 'Published' }
			  ]
		},
		{ key: 'body', label: t('comparisons.body', 'Body'), type: 'textarea' }
	]);

	const filters = $derived<FilterDef[]>([
		{
			key: 'visibility',
			label: t('comparisons.col_visibility', 'Visibility'),
			options: visibilityOptions.length
				? visibilityOptions
				: [
						{ value: 'public', label: t('comparisons.vis_public', 'Public') },
						{ value: 'members', label: t('comparisons.vis_members', 'Members') },
						{ value: 'private', label: t('comparisons.vis_private', 'Private') }
				  ]
		},
		{
			key: 'status',
			label: t('comparisons.col_status', 'Status'),
			options: statusOptions.length
				? statusOptions
				: [
						{ value: 'draft', label: 'Draft' },
						{ value: 'published', label: 'Published' }
				  ]
		},
		{
			key: 'depth',
			label: t('comparisons.col_depth', 'Depth'),
			options: [
				{ value: 'quick', label: t('comparisons.depth_quick', 'Quick') },
				{ value: 'detailed', label: t('comparisons.depth_detailed', 'Detailed') }
			]
		},
		// Soft-deleted comparisons are hidden by default — this filter
		// surfaces them so the Hard Delete action has targets.
		{
			key: 'deleted',
			label: t('common.filter_deleted', 'Deleted'),
			options: [
				{ value: 'only', label: t('common.filter_deleted_only', 'Deleted only') },
				{ value: 'all', label: t('common.filter_deleted_all', 'Include deleted') }
			]
		}
	]);

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return;

		bulkDeleting = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => comparisonAdminApi.deleteComparison(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete comparisons');
		} finally {
			bulkDeleting = false;
		}
	}

	// Row map so bulk actions can resolve ids → rows after EntityPage clears
	// the selection.
	const comparisonRowMap = new Map<string, Comparison>();
	const comparisonRowsFor = (ids: Set<string>): Comparison[] =>
		Array.from(ids)
			.map((id) => comparisonRowMap.get(id))
			.filter((r): r is Comparison => !!r);

	// ── Force delete (admin.hard_delete) — bulk shares one confirm modal ──
	let forceTargets = $state<Comparison[]>([]);
	let showForceModal = $state(false);

	function openBulkForceDelete(ids: Set<string>) {
		const rows = comparisonRowsFor(ids).filter((r) => !!r.deleted_at);
		if (rows.length === 0) return;
		forceTargets = rows;
		showForceModal = true;
	}

	async function confirmForceDelete() {
		if (bulkDeleting || forceTargets.length === 0) return;
		bulkDeleting = true;
		pageError = '';
		try {
			const targets = forceTargets;
			let failed = 0;
			let lastErr = '';
			for (const row of targets) {
				const res = await comparisonAdminApi.hardDeleteComparison(row.id);
				if (!res.ok) {
					failed++;
					lastErr = res.error ?? '';
				}
			}
			showForceModal = false;
			forceTargets = [];
			if (failed > 0) {
				pageError =
					t('common.bulk_partial', '{failed} of {n} items could not be completed')
						.replace('{failed}', String(failed))
						.replace('{n}', String(targets.length)) + (lastErr ? ` — ${lastErr}` : '');
			}
			entityController?.reload();
		} finally {
			bulkDeleting = false;
		}
	}

	const config = $derived<EntityPageConfig>({
		title: t('comparisons.title', 'Comparisons'),
		description: t('comparisons.subtitle', 'Manage comparison content entries'),
		createLabel: t('comparisons.create', 'Create Comparison'),
		createPermission: canCreateComparison ? 'comparison.create' : undefined,
		editPermission: canEditComparison ? 'comparison.edit_any' : undefined,
		deletePermission: canDeleteComparison ? 'comparison.delete_any' : undefined,
		hardDeletePermission: 'admin.hard_delete',
		pageSize: 100,
		columns: [
			{ key: 'title', label: t('comparisons.col_title', 'Title') },
			{
				key: 'slug',
				label: t('comparisons.col_slug', 'Slug'),
				render: (v: string) =>
					`<span class="text-xs font-mono" style="color: var(--color-muted)">${escapeHtml(v)}</span>`
			},
			{
				key: 'visibility',
				label: t('comparisons.col_visibility', 'Visibility'),
				render: (v: string) => {
					const opt = visibilityOptions.find((o) => o.value === v);
					const label = opt?.label ?? v;
					const color =
						v === 'public'
							? 'bg-success/15 text-success dark:text-success'
							: v === 'members'
								? 'bg-muted/15 text-muted dark:text-muted'
								: 'bg-danger/15 text-danger dark:text-danger';
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${color}">${escapeHtml(label)}</span>`;
				}
			},
			{
				key: 'status',
				label: t('comparisons.col_status', 'Status'),
				render: (v: string) => {
					const color =
						v === 'published'
							? 'bg-success/15 text-success dark:text-success'
							: 'bg-muted/15 text-muted dark:text-muted';
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${color}">${escapeHtml(v)}</span>`;
				}
			},
			{
				key: 'depth',
				label: t('comparisons.col_depth', 'Depth'),
				render: (v: string) => {
					const color =
						v === 'detailed'
							? 'bg-success/15 text-success dark:text-success'
							: 'bg-muted/15 text-muted dark:text-muted';
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${color}">${escapeHtml(v ?? '—')}</span>`;
				}
			},
			{
				key: 'promoted_from_id',
				label: t('comparisons.col_promoted_from', 'Promoted From'),
				render: (v: string | null) => {
					if (!v) return '—';
					return `<a href="?q=${encodeURIComponent(v)}" class="text-xs font-mono" style="color: var(--color-primary)">${escapeHtml(v)}</a>`;
				}
			},
			{
				key: 'created_at',
				label: t('comparisons.col_created', 'Created'),
				render: (v: string) =>
					`<span class="text-xs" style="color: var(--color-muted)">${v ? new Date(v).toLocaleDateString() : '—'}</span>`
			},
			{
				key: 'deleted_at',
				label: t('common.filter_deleted', 'Deleted'),
				render: (v: string | null) =>
					v
						? `<span class="inline-block rounded-full px-2 py-0.5 text-xs bg-danger/15 text-danger">${t('common.filter_deleted', 'Deleted')}</span>`
						: `<span style="color: var(--color-muted)">—</span>`
			}
		],
		filters,
		bulkActions: [
			{
				label: t('common.deleteSelected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDelete,
				// Rows already soft-deleted can't be deleted again.
				disabled: (_ids, rows) => (rows ?? []).some((r) => !!r.deleted_at)
			},
			...(canHardDelete
				? [
						{
							label: t('common.force_delete', 'Force Delete Selected'),
							icon: Trash2,
							variant: 'destructive' as const,
							onclick: openBulkForceDelete,
							disabled: (ids: Set<string>) =>
								bulkDeleting || !comparisonRowsFor(ids).some((r) => !!r.deleted_at)
						}
					]
				: [])
		],
		fields,
		isDeleted: (row: Comparison) => !!row.deleted_at,
		load: async ({ q, filter }) => {
			const params = new URLSearchParams({ page_size: '100' });
			if (filter.depth) params.set('depth', filter.depth);
			if (filter.status) params.set('status', filter.status);
			if (filter.visibility) params.set('visibility', filter.visibility);
			if (filter.deleted) params.set('deleted', filter.deleted);

			const result = await comparisonAdminApi.listComparisonsAdmin(params.toString());
			if (!result.ok) {
				throw new Error(result.error ?? t('common.error', 'Failed to load comparisons'));
			}

			let rows = (result.data as Comparison[]) ?? [];
			for (const r of rows) comparisonRowMap.set(r.id, r);

			if (q) {
				const qLower = q.toLowerCase();
				rows = rows.filter(
					(c) =>
						c.title.toLowerCase().includes(qLower) ||
						c.slug.toLowerCase().includes(qLower)
				);
			}

			if (filter.visibility) {
				rows = rows.filter((c) => c.visibility === filter.visibility);
			}

			if (filter.status) {
				rows = rows.filter((c) => c.status === filter.status);
			}

			if (filter.depth) {
				rows = rows.filter((c) => c.depth === filter.depth);
			}

			return rows;
		},
		create: async (body) => {
			const title = String(body.title ?? '').trim();
			const slug = String(body.slug ?? '').trim() || slugify(title);
			const res = await comparisonAdminApi.createComparison({
				title,
				slug,
				body: String(body.body ?? '').trim() || undefined,
				visibility: String(body.visibility ?? 'members'),
				type_id: String(body.type_id ?? '') || undefined
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const title = String(body.title ?? '').trim();
			const slug = String(body.slug ?? '').trim() || slugify(title);
			const res = await comparisonAdminApi.updateComparison(id, {
				title,
				slug,
				body: String(body.body ?? '').trim() || undefined,
				visibility: String(body.visibility ?? 'members')
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await comparisonAdminApi.deleteComparison(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		hardRemove: async (id) => {
			const res = await comparisonAdminApi.hardDeleteComparison(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: ComparisonView
	});
</script>

<svelte:head>
	<title>{t('comparisons.title', 'Comparisons')} — {siteName()} Admin</title>
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

<!-- ── Force Delete confirmation (bulk) — permanent erasure of soft-deleted rows ── -->
<Modal bind:open={showForceModal} title={t('common.force_delete', 'Force Delete')} size="sm">
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{#if forceTargets.length === 1}
				{t('common.force_delete_confirm', 'Permanently delete "{name}"? This cannot be undone.').replace('{name}', forceTargets[0].title)}
			{:else}
				{t('common.force_delete_confirm_bulk', 'Permanently delete {count} selected items? This cannot be undone.').replace('{count}', String(forceTargets.length))}
			{/if}
		</p>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => { showForceModal = false; forceTargets = []; }}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmForceDelete} disabled={bulkDeleting}>
			{bulkDeleting ? t('common.deleting', 'Deleting...') : t('common.force_delete', 'Force Delete')}
		</Button>
	</div>
</Modal>

<!-- View modal body -->
{#snippet ComparisonView({ row }: { row: Comparison })}
	<div class="space-y-4">
		<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('comparisons.col_title', 'Title')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('comparisons.col_depth', 'Depth')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.depth ?? '—'}</p>
			</div>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('comparisons.col_slug', 'Slug')}
			</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('comparisons.col_visibility', 'Visibility')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{visibilityOptions.find((o) => o.value === row.visibility)?.label ?? row.visibility}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('comparisons.col_status', 'Status')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status}</p>
		</div>
		{#if row.promoted_from_id}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('comparisons.col_promoted_from', 'Promoted From')}
				</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.promoted_from_id}</p>
			</div>
		{/if}
		{#if row.body}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('comparisons.body', 'Body')}
				</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.body}</p>
			</div>
		{/if}
	</div>
{/snippet}
