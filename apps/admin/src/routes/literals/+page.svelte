<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /literals — template literals CRUD using the EntityPage scaffold.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { api, apiRaw } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { Eye, Pencil, Trash2 } from 'lucide-svelte';

	type TemplateLiteral = {
		id: string;
		key: string;
		kind: 'expression' | 'static';
		value: string;
		fallback: string | null;
		label: string | null;
		description: string | null;
		category: string | null;
		is_system: boolean;
		is_active: boolean;
		deleted_at?: string | null;
		created_at: string;
		updated_at: string;
	};

	function mapLiteral(raw: any): TemplateLiteral {
		return { ...raw, id: raw.id };
	}

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkDeleting = $state(false);

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return;
		bulkDeleting = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => api(`/admin/literals/${id}`, { method: 'DELETE' })));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete template literals');
		} finally {
			bulkDeleting = false;
		}
	}

	// ── Force delete — single + bulk share one confirm modal ──
	// BulkAction.onclick receives only ids — resolve them against a map of
	// every row seen this session so deleted-row filtering keeps working.
	const literalRowMap = new Map<string, TemplateLiteral>();
	const literalRowsFor = (ids: Set<string>): TemplateLiteral[] =>
		Array.from(ids)
			.map((id) => literalRowMap.get(id))
			.filter((r): r is TemplateLiteral => !!r);

	let forceTargets = $state<TemplateLiteral[]>([]);
	let showForceModal = $state(false);

	function openForceDelete(row: TemplateLiteral) {
		forceTargets = [row];
		showForceModal = true;
	}

	function openBulkForceDelete(ids: Set<string>) {
		const rows = literalRowsFor(ids).filter((r) => !!r.deleted_at && !r.is_system);
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
				const res = await api(`/admin/literals/${row.id}?force=true`, { method: 'DELETE' });
				if (!res.ok) {
					failed++;
					lastErr = res.error ?? '';
				}
			}
			showForceModal = false;
			forceTargets = [];
			if (failed > 0) {
				pageError =
					t('common.bulkDeletePartial', '{failed} of {n} items could not be deleted')
						.replace('{failed}', String(failed))
						.replace('{n}', String(targets.length)) + (lastErr ? ` — ${lastErr}` : '');
			}
			entityController?.reload();
		} finally {
			bulkDeleting = false;
		}
	}

	const fields: FieldDef[] = [
		{
			key: 'key',
			label: t('literals.key', 'Key'),
			type: 'text',
			placeholder: 'Current_Year',
			required: true,
			readonlyInEdit: true,
			help: t('literals.key_hint', 'Letters, numbers, and underscores only. Cannot be changed after creation.')
		},
		{
			key: 'kind',
			label: t('literals.kind', 'Kind'),
			type: 'select',
			options: [
				{ value: 'expression', label: t('literals.kind_expression', 'Expression') },
				{ value: 'static', label: t('literals.kind_static', 'Static') }
			],
			required: true
		},
		{
			key: 'value',
			label: t('literals.value', 'Value'),
			type: 'textarea',
			placeholder: "year(Now)",
			required: true,
			help: t('literals.value_hint', 'For Expression, use the safe expression language. For Static, enter the literal text.')
		},
		{
			key: 'fallback',
			label: t('literals.fallback', 'Fallback'),
			type: 'text',
			placeholder: '2026'
		},
		{
			key: 'label',
			label: t('literals.label', 'Label'),
			type: 'text'
		},
		{
			key: 'description',
			label: t('literals.description', 'Description'),
			type: 'textarea'
		},
		{
			key: 'category',
			label: t('literals.category', 'Category'),
			type: 'text',
			placeholder: 'time'
		},
		{
			key: 'is_active',
			label: t('literals.active', 'Active'),
			type: 'checkbox'
		}
	];

	const config: EntityPageConfig = {
		title: t('literals.title', 'Template Literals'),
		description: t('literals.description', 'Manage global template literals for dynamic text replacement.'),
		createPermission: 'content.literals.manage',
		pageSize: 25,
		columns: [
			{ key: 'key', label: t('literals.key', 'Key'), sortable: true },
			{ key: 'kind', label: t('literals.kind', 'Kind'), componentSnippet: KindCell },
			{ key: 'value', label: t('literals.value', 'Value'), componentSnippet: ValueCell },
			{ key: 'label', label: t('literals.label', 'Label') },
			{ key: 'category', label: t('literals.category', 'Category') },
			{ key: 'is_active', label: t('literals.active', 'Active'), componentSnippet: StatusCell },
			{ key: 'is_system', label: t('literals.system', 'System'), componentSnippet: SystemCell }
		],
		filters: [
			{
				key: 'is_active',
				label: t('literals.status', 'Status'),
				options: [
					{ value: 'true', label: t('common.active', 'Active') },
					{ value: 'false', label: t('common.inactive', 'Inactive') }
				]
			},
			{
				key: 'kind',
				label: t('literals.kind', 'Kind'),
				options: [
					{ value: 'expression', label: t('literals.kind_expression', 'Expression') },
					{ value: 'static', label: t('literals.kind_static', 'Static') }
				]
			},
			{
				// Soft-deleted literals are hidden by default — this filter
				// surfaces them so admins can force-delete.
				key: 'deleted',
				label: t('common.filter_deleted', 'Deleted'),
				options: [
					{ value: 'only', label: t('common.filter_deleted_only', 'Deleted only') },
					{ value: 'all', label: t('common.filter_deleted_all', 'Include deleted') }
				]
			}
		],
		bulkActions: [
			{
				label: t('common.deleteSelected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDelete,
				// Rows already soft-deleted can't be deleted again.
				disabled: (_ids, rows) => (rows ?? []).some((r) => !!r.deleted_at)
			},
			{
				label: t('common.force_delete', 'Force Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: openBulkForceDelete,
				disabled: (ids, rows) =>
					bulkDeleting || !(rows ?? literalRowsFor(ids)).some((r) => !!r.deleted_at && !r.is_system)
			}
		],
		fields,
		load: async ({ page, pageSize, q, sort, filter }) => {
			const params = new URLSearchParams();
			params.set('page', String(page + 1));
			params.set('pageSize', String(pageSize));
			if (q) params.set('q', q);
			if (filter.is_active) params.set('is_active', filter.is_active);
			if (filter.kind) params.set('kind', filter.kind);
			if (filter.deleted) params.set('deleted', filter.deleted);
			if (sort) {
				const [sortKey, sortDir] = sort.split(':');
				if (sortKey) params.set('sort', sortKey);
				if (sortDir) params.set('dir', sortDir);
			}
			const result = await apiRaw<{ data: TemplateLiteral[]; total: number; total_pages: number }>(
				`/admin/literals?${params.toString()}`
			);
			if (result.ok) {
				const body = result.data;
				const items = (body.data as TemplateLiteral[]) ?? [];
				for (const row of items) literalRowMap.set(row.id, row);
				return {
					data: items.map(mapLiteral),
					total: body.total ?? items.length,
					totalPages: body.total_pages ?? 1
				};
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load template literals'));
		},
		create: async (body) => {
			const res = await api('/admin/literals', {
				method: 'POST',
				body: {
					key: String(body.key ?? '').trim(),
					kind: String(body.kind ?? 'expression'),
					value: String(body.value ?? ''),
					fallback: body.fallback ? String(body.fallback) : null,
					label: body.label ? String(body.label) : null,
					description: body.description ? String(body.description) : null,
					category: body.category ? String(body.category) : null,
					is_active: body.is_active === undefined ? true : !!body.is_active
				}
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await api(`/admin/literals/${id}`, {
				method: 'PATCH',
				body: {
					kind: String(body.kind ?? 'expression'),
					value: String(body.value ?? ''),
					fallback: body.fallback ? String(body.fallback) : null,
					label: body.label ? String(body.label) : null,
					description: body.description ? String(body.description) : null,
					category: body.category ? String(body.category) : null,
					is_active: body.is_active === undefined ? undefined : !!body.is_active
				}
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await api(`/admin/literals/${id}`, { method: 'DELETE' });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		actions: [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (row: TemplateLiteral, ctx) => ctx?.openView(row) },
			{
				label: t('common.edit', 'Edit'),
				icon: Pencil,
				onclick: (row: TemplateLiteral, ctx) => ctx?.openEdit(row),
				show: (row: TemplateLiteral) => !row.is_system && !row.deleted_at
			},
			{
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (row: TemplateLiteral, ctx) => ctx?.openDelete(row),
				show: (row: TemplateLiteral) => !row.is_system && !row.deleted_at
			},
			{
				// Permanent removal — only for already-soft-deleted rows.
				label: t('common.force_delete', 'Force Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (row: TemplateLiteral) => openForceDelete(row),
				show: (row: TemplateLiteral) => !!row.deleted_at && !row.is_system
			}
		],
		viewSnippet: LiteralView
	};
</script>

<svelte:head>
	<title>{t('literals.title', 'Template Literals')} — {siteName()} Admin</title>
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

<!-- Force Delete confirmation (single + bulk) — permanent erasure of soft-deleted rows -->
<Modal bind:open={showForceModal} title={t('common.force_delete', 'Force Delete')} size="sm">
	{#if forceTargets.length > 0}
		<p class="text-sm" style="color: var(--color-foreground)">
			{#if forceTargets.length === 1}
				{t('common.force_delete_confirm', 'Permanently delete "{name}"? This cannot be undone.').replace('{name}', forceTargets[0].key)}
			{:else}
				{t('common.force_delete_confirm_bulk', 'Permanently delete {count} selected items? This cannot be undone.').replace('{count}', String(forceTargets.length))}
			{/if}
		</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showForceModal = false)} disabled={bulkDeleting}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmForceDelete} disabled={bulkDeleting}>{bulkDeleting ? t('common.deleting', 'Deleting...') : t('common.force_delete', 'Force Delete')}</Button>
	</div>
</Modal>

<!-- ── Cell snippets ─────────────────────────────────────────────────────── -->
{#snippet KindCell({ value }: { value: string })}
	<span
		class="inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium {value === 'expression'
			? 'bg-primary/15 text-primary'
			: 'bg-muted/15 text-muted'}"
	>
		{value === 'expression' ? t('literals.kind_expression', 'Expression') : t('literals.kind_static', 'Static')}
	</span>
{/snippet}

{#snippet ValueCell({ value }: { value: string })}
	<span class="font-mono text-xs" style="color: var(--color-muted)">{value ?? ''}</span>
{/snippet}

{#snippet StatusCell({ value, row }: { value: boolean; row: TemplateLiteral })}
	{#if row.deleted_at}
		<span class="inline-block rounded-full bg-danger/15 px-2 py-0.5 text-xs font-medium text-danger">
			{t('common.filter_deleted', 'Deleted')}
		</span>
	{:else}
		<span
			class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {value
				? 'bg-success/15 text-success'
				: 'bg-muted/15 text-muted'}"
		>
			{value ? t('common.active', 'Active') : t('common.inactive', 'Inactive')}
		</span>
	{/if}
{/snippet}

{#snippet SystemCell({ value }: { value: boolean })}
	{#if value}
		<span class="inline-block rounded-full bg-warning/15 px-2 py-0.5 text-xs font-medium text-warning">
			{t('literals.system', 'System')}
		</span>
	{/if}
{/snippet}

<!-- ── View modal body ──────────────────────────────────────────────── -->
{#snippet LiteralView({ row }: { row: TemplateLiteral })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('literals.key', 'Key')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.key}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('literals.kind', 'Kind')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.kind}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('literals.value', 'Value')}</p>
			<pre
				class="mt-1 overflow-x-auto rounded border p-2 text-xs font-mono"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
			>{row.value}</pre>
		</div>
		{#if row.fallback}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('literals.fallback', 'Fallback')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.fallback}</p>
			</div>
		{/if}
		{#if row.label}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('literals.label', 'Label')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.label}</p>
			</div>
		{/if}
		{#if row.description}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('literals.description', 'Description')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description}</p>
			</div>
		{/if}
		{#if row.category}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('literals.category', 'Category')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.category}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('literals.active', 'Active')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.is_active ? t('common.yes', 'Yes') : t('common.no', 'No')}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('literals.system', 'System')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.is_system ? t('common.yes', 'Yes') : t('common.no', 'No')}
			</p>
		</div>
	</div>
{/snippet}
