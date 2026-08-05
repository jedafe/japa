<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { DataTable } from '$lib/components/ui/data-table';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { TableSkeleton } from '$lib/components/ui/skeleton';
	import { Eye, Pencil, Trash2, RefreshCw, Plus } from 'lucide-svelte';
	import EntityForm from './entity-form.svelte';
	import type { FieldDef } from './index';
	import type { Column, ActionItem, FilterDef, BulkAction } from '$lib/components/ui/data-table';
	import type { Snippet } from 'svelte';

	// EntityPageConfig — declarative config the scaffold turns into a full CRUD page.
	export interface EntityPageConfig {
		title: string;
		description?: string;
		columns: Column[];
		filters?: FilterDef[];
		actions?: ActionItem[];
		bulkActions?: BulkAction[];
		createLabel?: string;
		createPermission?: string;
		editPermission?: string;
		deletePermission?: string;
		fields: FieldDef[];
		readonly?: boolean;
		pageSize?: number;
		pageSizeOptions?: number[];
		load: (params: {
			page: number;
			pageSize: number;
			q: string;
			sort: string;
			filter: Record<string, string>;
		}) => Promise<{ data: any[]; total: number; totalPages: number } | any[]>;
		create?: (body: Record<string, any>) => Promise<{ ok: boolean; error?: string }>;
		update?: (id: string, body: Record<string, any>) => Promise<{ ok: boolean; error?: string }>;
		remove?: (id: string) => Promise<{ ok: boolean; error?: string }>;
		viewSnippet?: Snippet<[{ row: any }]>;
		formSnippet?: Snippet<[{ value: Record<string, any>; editing: any | null }]>;
	}

	let { config }: { config: EntityPageConfig } = $props();

	// ── Owned UI state ──────────────────────────────────────────────────────
	let loading = $state(true);
	let error = $state('');
	let success = $state('');
	let rows = $state<any[]>([]);
	let totalRows = $state(0);
	let totalPages = $state(1);
	let currentPage = $state(0);
	let pageSize = $state(25);
	let searchQuery = $state('');
	let filterValue = $state<Record<string, string>>({});
	let sortKey = $state('');
	let sortDir = $state<'asc' | 'desc'>('asc');
	let selectedIds = $state<Set<string>>(new Set());

	// Create/edit slide-over
	let formOpen = $state(false);
	let editing = $state<any | null>(null);
	let formValue = $state<Record<string, any>>({});
	let formErrors = $state<Record<string, string>>({});
	let saving = $state(false);

	// View slide-over
	let viewOpen = $state(false);
	let viewTarget = $state<any | null>(null);

	// Delete-confirm modal
	let confirmOpen = $state(false);
	let deleteTarget = $state<any | null>(null);

	// ── Permission derivation: omitted keys default to allowed; edit/delete fall back to createPermission ──
	const canCreate = $derived(
		!config.readonly && (!config.createPermission || hasPermission(config.createPermission))
	);
	const canEdit = $derived(
		!config.readonly &&
			(!(config.editPermission ?? config.createPermission) ||
				hasPermission(config.editPermission ?? config.createPermission!))
	);
	const canDelete = $derived(
		!(config.deletePermission ?? config.createPermission) ||
			hasPermission(config.deletePermission ?? config.createPermission!)
	);

	// ── Data loading ────────────────────────────────────────────────────────
	async function reload() {
		loading = true;
		error = '';
		try {
			const result = await config.load({
				page: currentPage,
				pageSize,
				q: searchQuery,
				sort: sortKey ? `${sortKey}:${sortDir}` : '',
				filter: filterValue
			});
			if (Array.isArray(result)) {
				rows = result;
				totalRows = result.length;
				totalPages = 1;
			} else {
				rows = result.data ?? [];
				totalRows = result.total ?? rows.length;
				totalPages = result.totalPages ?? 1;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('common.load_error', 'Failed to load');
		} finally {
			loading = false;
		}
	}

	// Re-fetch whenever any pagination/search/sort/filter state changes.
	$effect(() => {
		void currentPage;
		void pageSize;
		void searchQuery;
		void filterValue;
		void sortKey;
		void sortDir;
		void reload();
	});

	// Honour a parent-supplied default page size (read once; DataTable owns it after).
	$effect(() => {
		const ps = config.pageSize;
		if (ps && ps !== pageSize) pageSize = ps;
	});

	// ── Row actions: default View/Edit/Delete open the slide-overs when config.actions is omitted ──
	const rowActions = $derived.by<ActionItem[]>(() => {
		if (config.actions) return config.actions;
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: openView }
		];
		if (canEdit) {
			acts.push({ label: t('common.edit', 'Edit'), icon: Pencil, onclick: openEdit });
		}
		if (canDelete) {
			acts.push({
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: openDelete
			});
		}
		return acts;
	});

	function openView(row: any) {
		viewTarget = row;
		viewOpen = true;
	}

	function openCreate() {
		editing = null;
		formValue = {};
		formErrors = {};
		formOpen = true;
	}

	function openEdit(row: any) {
		editing = row;
		const seeded: Record<string, any> = {};
		for (const field of config.fields) {
			seeded[field.key] = row?.[field.key] ?? '';
		}
		formValue = seeded;
		formErrors = {};
		formOpen = true;
	}

	function openDelete(row: any) {
		deleteTarget = row;
		confirmOpen = true;
	}

	// ── Mutations ───────────────────────────────────────────────────────────
	async function submitForm() {
		saving = true;
		error = '';
		try {
			const body = { ...formValue };
			const res = editing
				? await config.update?.(editing.id, body)
				: await config.create?.(body);
			if (res?.ok) {
				formOpen = false;
				success = editing ? t('common.saved', 'Saved') : t('common.created', 'Created');
				await reload();
			} else {
				error = res?.error ?? t('common.error', 'An error occurred');
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('common.error', 'An error occurred');
		} finally {
			saving = false;
		}
	}

	async function confirmDelete() {
		if (!deleteTarget) return;
		try {
			const res = await config.remove?.(deleteTarget.id);
			if (res?.ok) {
				confirmOpen = false;
				deleteTarget = null;
				success = t('common.saved', 'Saved');
				await reload();
			} else {
				error = res?.error ?? t('common.error', 'An error occurred');
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('common.error', 'An error occurred');
		}
	}

	// ── DataTable wiring ────────────────────────────────────────────────────
	function handleSort(s: { key: string | null; dir: 'asc' | 'desc' }) {
		sortKey = s.key ?? '';
		sortDir = s.dir;
		currentPage = 0;
	}
	function handleSearch(q: string) {
		searchQuery = q;
		currentPage = 0;
	}
	function handleFilterApply() {
		currentPage = 0;
	}
	function handleFilterClear() {
		filterValue = {};
		currentPage = 0;
	}

	// PageHeader actions: Create (when permitted) + Refresh.
	const headerActions = $derived.by(() => {
		const acts: Array<{
			label: string;
			icon?: any;
			onclick?: () => void;
			variant?: 'default' | 'destructive' | 'outline' | 'secondary' | 'ghost' | 'link';
		}> = [];
		if (canCreate) {
			acts.push({ label: config.createLabel ?? t('common.create', 'Create'), icon: Plus, onclick: openCreate });
		}
		acts.push({ label: t('common.refresh', 'Refresh'), icon: RefreshCw, variant: 'ghost', onclick: reload });
		return acts;
	});

	const formTitle = $derived(editing ? t('common.edit', 'Edit') : (config.createLabel ?? t('common.create', 'Create')));
</script>

<PageHeader title={config.title} description={config.description} actions={headerActions} />

{#if error}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{error}
	</div>
{/if}
{#if success}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)"
	>
		{success}
	</div>
{/if}

<Card>
	<CardContent class="p-4">
		{#if loading}
			<TableSkeleton rows={8} columns={config.columns.length} />
		{:else}
			<DataTable
				columns={config.columns}
				data={rows}
				serverSide
				totalRows={totalRows}
				totalPages={totalPages}
				bind:page={currentPage}
				bind:pageSize
				pageSizeOptions={config.pageSizeOptions ?? [10, 25, 50, 100]}
				searchable
				onsortchange={handleSort}
				onsearch={handleSearch}
				filters={config.filters}
				bind:filterValue
				onfilterapply={handleFilterApply}
				onfilterclear={handleFilterClear}
				actions={rowActions}
				bulkActionsList={config.bulkActions}
				selectable={!!config.bulkActions}
				bind:selectedIds
				urlSync={false}
			/>
		{/if}
	</CardContent>
</Card>

<!-- Create/Edit slide-over -->
<SlideOver bind:open={formOpen} title={formTitle}>
	{#if config.formSnippet}
		{@render config.formSnippet({ value: formValue, editing })}
	{:else}
		<EntityForm fields={config.fields} bind:value={formValue} errors={formErrors} disabled={saving} onsubmit={submitForm} />
	{/if}

	{#snippet footer()}
		<Button variant="outline" onclick={() => (formOpen = false)} disabled={saving}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button onclick={submitForm} disabled={saving}>
			{t('common.save', 'Save Changes')}
		</Button>
	{/snippet}
</SlideOver>

<!-- View slide-over -->
<SlideOver bind:open={viewOpen} title={t('common.view', 'View')}>
	{#if viewTarget}
		{#if config.viewSnippet}
			{@render config.viewSnippet({ row: viewTarget })}
		{:else}
			<dl class="grid grid-cols-1 gap-4 sm:grid-cols-2">
				{#each config.columns as column (column.key)}
					<div class="flex flex-col gap-1">
						<dt class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
							{column.label}
						</dt>
						<dd class="text-sm font-medium" style="color: var(--color-foreground)">
							{viewTarget[column.key] ?? '—'}
						</dd>
					</div>
				{/each}
			</dl>
		{/if}
	{/if}

	{#snippet footer()}
		<Button variant="outline" onclick={() => (viewOpen = false)}>
			{t('common.close', 'Close')}
		</Button>
	{/snippet}
</SlideOver>

<!-- Delete-confirm modal -->
<Modal bind:open={confirmOpen} title={t('common.confirmDelete', 'Confirm Delete')}>
	<p class="text-sm" style="color: var(--color-muted)">
		{t('common.confirmDelete', 'Confirm Delete')}. {t('common.cannotUndo', 'This cannot be undone.')}.
	</p>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (confirmOpen = false)}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button variant="destructive" onclick={confirmDelete}>
			{t('common.confirm', 'Confirm')}
		</Button>
	</div>
</Modal>
