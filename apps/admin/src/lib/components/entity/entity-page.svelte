<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { page as sveltekitPage } from '$app/state';
	import { replaceState } from '$app/navigation';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { DataTable } from '$lib/components/ui/data-table';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { TableSkeleton } from '$lib/components/ui/skeleton';
	import Pagination from '$lib/components/ui/Pagination.svelte';
	import FilterPanel from '$lib/components/ui/data-table/filter-panel.svelte';
	import { Search } from 'lucide-svelte';
	import { Eye, Pencil, Trash2, RefreshCw, Plus, Download, RotateCcw, Trash, Grid, List } from 'lucide-svelte';
	import EntityForm from './entity-form.svelte';
	import type { FieldDef } from './index';
	import type { Column, ActionItem, ActionContext, FilterDef, BulkAction } from '$lib/components/ui/data-table';
	import { untrack } from 'svelte';
	import type { Snippet } from 'svelte';

	// URL params reserved for non-filter state (page/pageSize/q/sort/dir).
	// Every other query param is treated as a filter key.
	const RESERVED_PARAMS = new Set(['page', 'pageSize', 'q', 'sort', 'dir']);
	const DEFAULT_PAGE_SIZE = 25;

	// EntityPageConfig — declarative config the scaffold turns into a full CRUD page.
	export interface EntityPageController {
		reload: () => void;
		openView: (row: any) => void;
		openEdit: (row: any) => void;
		openDelete: (row: any) => void;
	}

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
		hardDeletePermission?: string;
		restorePermission?: string;
		fields: FieldDef[];
		readonly?: boolean;
		pageSize?: number;
		pageSizeOptions?: number[];
		headerActions?: Array<{
			label: string;
			icon?: any;
			onclick?: () => void;
			variant?: 'default' | 'destructive' | 'outline' | 'secondary' | 'ghost' | 'link';
		}>;
		load: (params: {
			page: number;
			pageSize: number;
			q: string;
			sort: string;
			filter: Record<string, string>;
		}) => Promise<{ data: any[]; total: number; totalPages: number } | any[]>;
		create?: (body: Record<string, any>) => Promise<{ ok: boolean; error?: string; id?: string }>;
		update?: (id: string, body: Record<string, any>) => Promise<{ ok: boolean; error?: string }>;
		remove?: (id: string) => Promise<{ ok: boolean; error?: string }>;
		/** Permanently delete a soft-deleted row. When provided, a "Hard Delete" action
		 *  appears for rows where `isDeleted(row)` returns true. */
		hardRemove?: (id: string) => Promise<{ ok: boolean; error?: string }>;
		/** Restore a soft-deleted row. When provided, a "Restore" action appears for
		 *  rows where `isDeleted(row)` returns true. */
		restore?: (id: string) => Promise<{ ok: boolean; error?: string }>;
		/** Predicate to detect soft-deleted rows. Default: `row.status === 'deleted' || row.deleted_at != null`. */
		isDeleted?: (row: any) => boolean;
		/** Show an Export button that downloads all filtered rows as CSV. */
		exportable?: boolean;
		/** Custom filename for the CSV export (without extension). Default: `${title}-export`. */
		exportFilename?: string;
		onCreated?: (id: string) => void;
		viewSnippet?: Snippet<[{ row: any }]>;
		formSnippet?: Snippet<[{ value: Record<string, any>; editing: any | null }]>;
		/** When provided, a grid/list view toggle appears in the header and the
		 *  grid snippet is rendered instead of the DataTable when grid mode is
		 *  active. The snippet receives each row and should render a card. */
		gridSnippet?: Snippet<[{ row: any }]>;
		/** Default view mode when gridSnippet is provided ('list' | 'grid'). Default: 'list'. */
		defaultViewMode?: 'list' | 'grid';
	}

	let { config, controller = $bindable() }: { config: EntityPageConfig; controller?: EntityPageController } = $props();

	// ── Owned UI state ──────────────────────────────────────────────────────
	// Seeded from the URL once at init so a reload restores page/pageSize/q/sort/filters
	// and the first `load` call uses the restored params (no double-fetch).
	const initialParams = sveltekitPage.url.searchParams;
	const initialFilters: Record<string, string> = {};
	for (const [key, value] of initialParams.entries()) {
		if (!RESERVED_PARAMS.has(key) && value) initialFilters[key] = value;
	}
	const initialPageSize = Number(initialParams.get('pageSize'));
	const pageSizeSeededFromUrl = Number.isFinite(initialPageSize) && initialPageSize > 0;

	let loading = $state(true);
	let error = $state('');
	let success = $state('');
	let rows = $state<any[]>([]);
	let totalRows = $state(0);
	let totalPages = $state(1);
	let currentPage = $state(
		initialParams.get('page') ? Math.max(0, Number(initialParams.get('page')) - 1) : 0
	);
	let pageSize = $state(pageSizeSeededFromUrl ? initialPageSize : DEFAULT_PAGE_SIZE);
	let searchQuery = $state(initialParams.get('q') ?? '');
	let filterValue = $state<Record<string, string>>(initialFilters);
	let sortKey = $state(initialParams.get('sort') ?? '');
	let sortDir = $state<'asc' | 'desc'>(
		initialParams.get('dir') === 'desc' ? 'desc' : 'asc'
	);
	let selectedIds = $state<Set<string>>(new Set());

	// Grid/list view mode — only relevant when config.gridSnippet is provided.
	// untrack: we deliberately only read the initial defaultViewMode once.
	let viewMode = $state<'list' | 'grid'>(untrack(() => config.defaultViewMode ?? 'list'));

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

	// Hard-delete confirm modal (separate, stronger warning)
	let hardDeleteConfirmOpen = $state(false);
	let hardDeleteTarget = $state<any | null>(null);

	// Restore confirm modal
	let restoreConfirmOpen = $state(false);
	let restoreTarget = $state<any | null>(null);

	// Export state
	let exporting = $state(false);

	// Soft-delete detection — default checks both `status === 'deleted'` and `deleted_at`.
	const defaultIsDeleted = (row: any) => row?.status === 'deleted' || row?.deleted_at != null;
	const isRowDeleted = $derived(config.isDeleted ?? defaultIsDeleted);

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
	const canHardDelete = $derived(
		!!config.hardRemove &&
			(!(config.hardDeletePermission ?? config.deletePermission ?? config.createPermission) ||
				hasPermission(config.hardDeletePermission ?? config.deletePermission ?? config.createPermission!))
	);
	const canRestore = $derived(
		!!config.restore &&
			(!(config.restorePermission ?? config.deletePermission ?? config.createPermission) ||
				hasPermission(config.restorePermission ?? config.deletePermission ?? config.createPermission!))
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

	// Reflect page/pageSize/filters in the URL. Sort/dir/q are written by the
	// DataTable (urlSync={true}); this effect only owns the state DataTable
	// does not, and preserves sort/dir/q when it updates the URL.
	const filterValueKey = $derived(JSON.stringify(filterValue));
	let urlSyncReady = false;
	$effect(() => {
		void currentPage;
		void pageSize;
		void filterValueKey;
		if (!urlSyncReady) {
			urlSyncReady = true;
			return;
		}
		syncToUrl();
	});

	function syncToUrl() {
		if (typeof window === 'undefined') return;
		// Build from the current URL so sort/dir/q (owned by the DataTable) are preserved.
		const params = new URLSearchParams(sveltekitPage.url.searchParams.toString());
		// Drop stale filter params (any non-reserved key) before re-writing current ones.
		for (const key of [...params.keys()]) {
			if (!RESERVED_PARAMS.has(key)) params.delete(key);
		}
		if (currentPage > 0) params.set('page', String(currentPage + 1));
		else params.delete('page');
		const sizeDefault = config.pageSize ?? DEFAULT_PAGE_SIZE;
		if (pageSize !== sizeDefault) params.set('pageSize', String(pageSize));
		else params.delete('pageSize');
		for (const [key, value] of Object.entries(filterValue)) {
			if (value) params.set(key, value);
		}
		const qs = params.toString();
		const url = `${sveltekitPage.url.pathname}${qs ? `?${qs}` : ''}`;
		// Prefers SvelteKit's replaceState (updates $app/state's page.url so seed
		// effects see new params), falling back to window.history.replaceState
		// if the SvelteKit router isn't initialized (e.g. in test environments).
		try {
			replaceState(url, {});
		} catch {
			window.history.replaceState({}, '', url);
		}
	}

	// Honour a parent-supplied default page size, but only when the URL did not
	// already restore one. After init, DataTable owns the value.
	// untrack(pageSize) so this effect only fires when config.pageSize changes,
	// not when the user (or Pagination) changes pageSize — otherwise it would
	// reset the user's selection back to the config default on every change.
	$effect(() => {
		if (pageSizeSeededFromUrl) return;
		const ps = config.pageSize;
		if (ps && ps !== untrack(() => pageSize)) pageSize = ps;
	});

	// ── Row actions: custom actions get an ActionContext; defaults open the slide-overs ──
	const actionCtx = $derived.by<ActionContext>(() => ({ openView, openEdit, openDelete, reload }));
	const rowActions = $derived.by<ActionItem[]>(() => {
		if (config.actions) {
			return config.actions.map((a) => ({
				...a,
				onclick: (row: any) => a.onclick(row, actionCtx)
			}));
		}
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: openView }
		];
		if (canEdit) {
			acts.push({
				label: t('common.edit', 'Edit'),
				icon: Pencil,
				onclick: openEdit,
				show: (row: any) => !isRowDeleted(row)
			});
		}
		if (canDelete) {
			acts.push({
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: openDelete,
				show: (row: any) => !isRowDeleted(row)
			});
		}
		if (canRestore) {
			acts.push({
				label: t('common.restore', 'Restore'),
				icon: RotateCcw,
				onclick: openRestore,
				show: (row: any) => isRowDeleted(row)
			});
		}
		if (canHardDelete) {
			acts.push({
				label: t('common.hardDelete', 'Hard Delete'),
				icon: Trash,
				variant: 'destructive',
				onclick: openHardDelete,
				show: (row: any) => isRowDeleted(row)
			});
		}
		return acts;
	});

	// Wrap bulk actions to auto-reload + clear selection after completion.
	const wiredBulkActions = $derived.by<BulkAction[]>(() => {
		if (!config.bulkActions) return [];
		return config.bulkActions.map((a) => ({
			...a,
			onclick: async (ids: Set<string>) => {
				await a.onclick(ids);
				selectedIds = new Set();
				await reload();
			}
		}));
	});

	// Expose controller so page-local code (modals, etc.) can trigger reload/view/edit/delete.
	$effect(() => {
		controller = { reload, openView, openEdit, openDelete };
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
				if (!editing) {
					const createdId = (res as { id?: string }).id;
					if (createdId) config.onCreated?.(createdId);
				}
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

	function openHardDelete(row: any) {
		hardDeleteTarget = row;
		hardDeleteConfirmOpen = true;
	}

	async function confirmHardDelete() {
		if (!hardDeleteTarget) return;
		try {
			const res = await config.hardRemove?.(hardDeleteTarget.id);
			if (res?.ok) {
				hardDeleteConfirmOpen = false;
				hardDeleteTarget = null;
				success = t('common.saved', 'Saved');
				await reload();
			} else {
				error = res?.error ?? t('common.error', 'An error occurred');
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('common.error', 'An error occurred');
		}
	}

	function openRestore(row: any) {
		restoreTarget = row;
		restoreConfirmOpen = true;
	}

	async function confirmRestore() {
		if (!restoreTarget) return;
		try {
			const res = await config.restore?.(restoreTarget.id);
			if (res?.ok) {
				restoreConfirmOpen = false;
				restoreTarget = null;
				success = t('common.saved', 'Saved');
				await reload();
			} else {
				error = res?.error ?? t('common.error', 'An error occurred');
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('common.error', 'An error occurred');
		}
	}

	// ── Export (CSV) ────────────────────────────────────────────────────────
	function csvEscape(value: any): string {
		if (value == null) return '';
		const s = typeof value === 'object' ? JSON.stringify(value) : String(value);
		if (/[",\n\r]/.test(s)) return `"${s.replace(/"/g, '""')}"`;
		return s;
	}

	async function exportCsv() {
		if (exporting) return;
		exporting = true;
		error = '';
		try {
			const allRows: any[] = [];
			let page = 0;
			const exportPageSize = 100;
			// Paginate through all filtered rows using the current search/sort/filter state.
			for (;;) {
				const result = await config.load({
					page,
					pageSize: exportPageSize,
					q: searchQuery,
					sort: sortKey ? `${sortKey}:${sortDir}` : '',
					filter: filterValue
				});
				if (Array.isArray(result)) {
					allRows.push(...result);
					break;
				}
				allRows.push(...(result.data ?? []));
				const tp = result.totalPages ?? 1;
				if (page + 1 >= tp) break;
				page++;
			}
			// Build CSV from visible columns.
			const cols = config.columns;
			const header = cols.map((c) => csvEscape(c.label)).join(',');
			const lines = allRows.map((row) =>
				cols.map((c) => csvEscape(row[c.key])).join(',')
			);
			const csv = [header, ...lines].join('\n');
			const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' });
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = `${config.exportFilename ?? `${config.title}-export`}.csv`;
			document.body.appendChild(a);
			a.click();
			document.body.removeChild(a);
			URL.revokeObjectURL(url);
			success = t('common.exported', 'Exported {n} rows').replace('{n}', String(allRows.length));
		} catch (e) {
			error = e instanceof Error ? e.message : t('common.error', 'An error occurred');
		} finally {
			exporting = false;
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

	// PageHeader actions: Create (when permitted) + Export (when enabled) + view toggle (when gridSnippet) + custom header actions + Refresh.
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
		if (config.exportable) {
			acts.push({
				label: t('common.export', 'Export'),
				icon: Download,
				variant: 'outline',
				onclick: exportCsv
			});
		}
		// Grid/list view toggle — only when a gridSnippet is provided.
		if (config.gridSnippet) {
			acts.push({
				label: 'Grid',
				icon: Grid,
				variant: viewMode === 'grid' ? 'default' : 'ghost',
				onclick: () => (viewMode = 'grid')
			});
			acts.push({
				label: 'List',
				icon: List,
				variant: viewMode === 'list' ? 'default' : 'ghost',
				onclick: () => (viewMode = 'list')
			});
		}
		if (config.headerActions) {
			acts.push(...config.headerActions);
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
		<!-- Only show skeleton on initial load (no data yet). On subsequent reloads
		     (search/sort/filter/pagination), keep the view mounted so its
		     internal state (search text, column widths, visibility, sort) is
		     preserved — showing stale-while-revalidate UX instead of unmounting. -->
		{#if loading && rows.length === 0}
			<TableSkeleton rows={8} columns={config.columns.length} />
		{:else}
			{#if loading}
				<div class="mb-2 flex items-center gap-2 text-sm" style="color: var(--color-muted)">
					<RefreshCw class="h-3.5 w-3.5 animate-spin" />
					{t('common.loading', 'Loading…')}
				</div>
			{/if}
			{#if viewMode === 'grid' && config.gridSnippet}
				<!-- Grid/card view: filter bar on top, then search bar, then grid of cards + pagination.
				     Mirrors the DataTable layout where FilterPanel sits above the search row. -->
				{#if config.filters && config.filters.length > 0}
					<div class="mb-3">
						<FilterPanel
							filters={config.filters}
							bind:value={filterValue}
							onapply={handleFilterApply}
							onclear={handleFilterClear}
						/>
					</div>
				{/if}
				<div class="mb-4 flex items-center gap-3">
					<div class="relative flex-1">
						<Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" style="color: var(--color-muted)" />
						<input
							type="text"
							placeholder={t('common.search', 'Search…')}
							value={searchQuery}
							oninput={(e) => handleSearch(e.currentTarget.value)}
							class="w-full rounded-lg border bg-surface py-2 pl-9 pr-3 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
							style="border-color: var(--color-border); color: var(--color-foreground)"
						/>
						{#if searchQuery}
							<button
								type="button"
								onclick={() => handleSearch('')}
								aria-label={t('common.clearSelection', 'Clear search')}
								class="absolute right-2 top-1/2 -translate-y-1/2 rounded p-1"
								style="color: var(--color-muted)"
							>
								✕
							</button>
						{/if}
					</div>
				</div>
				{#if rows.length === 0}
					<div class="py-12 text-center text-sm" style="color: var(--color-muted)">
						{t('common.noData', 'No data')}
					</div>
				{:else}
					<div class="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
						{#each rows as row (row.id)}
							{@render config.gridSnippet({ row })}
						{/each}
					</div>
					<div class="mt-4">
						<Pagination
							bind:page={currentPage}
							{totalPages}
							totalItems={totalRows}
							bind:pageSize
							pageSizeOptions={config.pageSizeOptions ?? [10, 25, 50, 100]}
							urlSync={false}
						/>
					</div>
				{/if}
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
					bulkActionsList={wiredBulkActions}
					selectable={!!config.bulkActions}
					bind:selectedIds
					urlSync
				/>
			{/if}
		{/if}
	</CardContent>
</Card>

<!-- Create/Edit slide-over -->
<SlideOver bind:open={formOpen} title={formTitle}>
	{#if config.formSnippet}
		{@render config.formSnippet({ value: formValue, editing })}
	{:else}
		<EntityForm fields={config.fields} bind:value={formValue} errors={formErrors} disabled={saving} {editing} onsubmit={submitForm} />
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

<!-- Delete-confirm modal (soft delete) -->
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

<!-- Hard-delete confirm modal (permanent erasure) -->
<Modal bind:open={hardDeleteConfirmOpen} title={t('common.hardDelete', 'Hard Delete')}>
	<p class="text-sm" style="color: var(--color-foreground)">
		{t('common.hardDeleteWarning', 'This will permanently erase the record and all related data. This action cannot be undone.')}.
	</p>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (hardDeleteConfirmOpen = false)}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button variant="destructive" onclick={confirmHardDelete}>
			{t('common.hardDelete', 'Hard Delete')}
		</Button>
	</div>
</Modal>

<!-- Restore confirm modal -->
<Modal bind:open={restoreConfirmOpen} title={t('common.restore', 'Restore')}>
	<p class="text-sm" style="color: var(--color-muted)">
		{t('common.restoreConfirm', 'Are you sure you want to restore this record?')}.
	</p>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (restoreConfirmOpen = false)}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button onclick={confirmRestore}>
			{t('common.restore', 'Restore')}
		</Button>
	</div>
</Modal>
