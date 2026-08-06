<script lang="ts">
	// /data-imports — Platform Data Import (Section 67).
	// Admin can import platform data (users, posts, comments, orders, etc.)
	// from CSV/JSON files. Supports both migration/restore and bulk seed.
	// Matches the data-exports page pattern.
	import { t } from '$lib/stores/i18n.svelte';
	import { api, apiRaw } from '$lib/api';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { DataTable } from '$lib/components/ui/data-table';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { Select } from '$lib/components/ui/select';
	import {
		Upload,
		Plus,
		RefreshCw,
		Clock,
		CheckCircle,
		XCircle,
		AlertTriangle,
		Eye,
		Trash2,
		FileUp
	} from 'lucide-svelte';

	type ImportErrorEntry = {
		row: number;
		error: string;
	};

	type DataImport = {
		id: string;
		name: string;
		type: string;
		format: string;
		status: 'pending' | 'processing' | 'completed' | 'failed' | 'partial';
		fileUrl: string;
		totalRows: number;
		importedRows: number;
		failedRows: number;
		errorLog: ImportErrorEntry[];
		mode: string;
		createdBy: string;
		createdAt: string;
		completedAt: string | null;
	};

	let imports = $state<DataImport[]>([]);
	let loading = $state(true);
	let activeTab = $state('all');
	let createSlideOverOpen = $state(false);
	let deleteConfirmOpen = $state(false);
	let deletingId = $state<string | null>(null);
	let saving = $state(false);
	let errorMessage = $state('');
	let successMessage = $state('');

	let showViewSlideOver = $state(false);
	let viewTarget = $state<DataImport | null>(null);
	let selectedIds = $state<Set<string>>(new Set());

	const canManage = $derived(hasPermission('gdpr.data_exports.view'));

	let form = $state({
		name: '',
		type: 'users',
		format: 'csv',
		fileUrl: '',
		mode: 'create'
	});

	const importTypes = [
		{ value: 'users', label: t('data_imports.type_users', 'Users') },
		{ value: 'posts', label: t('data_imports.type_posts', 'Posts') },
		{ value: 'comments', label: t('data_imports.type_comments', 'Comments') },
		{ value: 'orders', label: t('data_imports.type_orders', 'Orders') },
		{ value: 'transactions', label: t('data_imports.type_transactions', 'Transactions') },
		{ value: 'analytics', label: t('data_imports.type_analytics', 'Analytics') },
		{ value: 'moderation', label: t('data_imports.type_moderation', 'Moderation Logs') }
	];

	const formatOptions = [
		{ value: 'csv', label: t('data_imports.format_csv', 'CSV') },
		{ value: 'json', label: t('data_imports.format_json', 'JSON') }
	];

	const modeOptions = [
		{ value: 'create', label: t('data_imports.mode_create', 'Create (skip existing)') },
		{ value: 'upsert', label: t('data_imports.mode_upsert', 'Upsert (update or insert)') }
	];

	let tabs = $derived([
		{ value: 'all', label: t('data_imports.tab_all', 'All Imports'), icon: Upload, count: imports.length },
		{ value: 'active', label: t('data_imports.tab_in_progress', 'In Progress'), icon: Clock, count: imports.filter((i) => i.status === 'pending' || i.status === 'processing').length },
		{ value: 'completed', label: t('data_imports.tab_completed', 'Completed'), icon: CheckCircle, count: imports.filter((i) => i.status === 'completed').length },
		{ value: 'partial', label: t('data_imports.tab_partial', 'Partial'), icon: AlertTriangle, count: imports.filter((i) => i.status === 'partial').length },
		{ value: 'failed', label: t('data_imports.tab_failed', 'Failed'), icon: XCircle, count: imports.filter((i) => i.status === 'failed').length }
	]);

	let filteredImports = $derived.by(() => {
		let result = [...imports];
		if (activeTab === 'active') result = result.filter((i) => i.status === 'pending' || i.status === 'processing');
		if (activeTab === 'completed') result = result.filter((i) => i.status === 'completed');
		if (activeTab === 'partial') result = result.filter((i) => i.status === 'partial');
		if (activeTab === 'failed') result = result.filter((i) => i.status === 'failed');
		return result;
	});

	const importFilters: FilterDef[] = $derived([
		{
			key: 'type',
			label: t('data_imports.filter_import_type', 'Import Type'),
			options: importTypes
		}
	]);

	function statusVariant(status: string): string {
		switch (status) {
			case 'completed': return 'success';
			case 'failed': return 'destructive';
			case 'processing': return 'warning';
			case 'partial': return 'warning';
			default: return 'secondary';
		}
	}

	function statusBadgeClass(status: string): string {
		const variant = statusVariant(status);
		switch (variant) {
			case 'success': return 'bg-success/15 text-success border-success';
			case 'destructive': return 'bg-danger/15 text-danger border-danger';
			case 'warning': return 'bg-warning/15 text-warning border-warning';
			default: return 'bg-surface text-foreground border';
		}
	}

	const importColumns = [
		{
			key: 'name',
			label: t('data_imports.col_name', 'Name'),
			sortable: true,
			render: (_: any, row: DataImport) => `
				<div>
					<p class="font-medium text-foreground">${row.name}</p>
					<p class="text-xs text-muted">${row.type} · ${row.format.toUpperCase()} · ${row.mode}</p>
				</div>
			`
		},
		{
			key: 'status',
			label: t('data_imports.col_status', 'Status'),
			sortable: true,
			render: (value: string) => {
				const cls = statusBadgeClass(value);
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold capitalize ${cls}">${value}</span>`;
			}
		},
		{
			key: 'importedRows',
			label: t('data_imports.col_progress', 'Progress'),
			sortable: true,
			render: (v: number, row: DataImport) => {
				const total = row.totalRows || 0;
				const failed = row.failedRows || 0;
				const failedTag = failed > 0
					? ` <span class="text-xs text-danger">· ${failed} ${t('data_imports.failed', 'failed')}</span>`
					: '';
				return `<span class="text-sm text-foreground-alt">${v}/${total} ${t('data_imports.imported', 'imported')}${failedTag}</span>`;
			}
		},
		{
			key: 'createdAt',
			label: t('data_imports.col_created', 'Created'),
			sortable: true,
			render: (value: string) => `<span class="text-sm text-foreground-alt">${new Date(value).toLocaleString()}</span>`
		},
		{
			key: 'completedAt',
			label: t('data_imports.col_completed', 'Completed'),
			sortable: true,
			render: (value: string | null) => value ? `<span class="text-sm text-foreground-alt">${new Date(value).toLocaleString()}</span>` : `<span class="text-sm text-muted">—</span>`
		}
	];

	const importRowActions: ActionItem[] = [
		{ label: t('common.view', 'View'), icon: Eye, onclick: (row: DataImport) => openView(row) },
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row: DataImport) => openDeleteConfirm(row.id),
			show: () => canManage
		}
	];

	async function loadImports() {
		loading = true;
		const result = await apiRaw<{ data: DataImport[]; pagination: { total: number } }>(
			'/admin/data-imports?page=1&limit=100'
		);
		if (result.ok) {
			imports = Array.isArray(result.data.data) ? result.data.data : [];
		}
		loading = false;
	}

	async function submitImport() {
		if (!form.name.trim()) return;
		if (!form.fileUrl.trim()) {
			errorMessage = t('data_imports.msg_file_required', 'File URL is required');
			return;
		}
		saving = true;
		errorMessage = '';
		const result = await api('/admin/data-imports', {
			method: 'POST',
			body: {
				name: form.name.trim(),
				type: form.type,
				format: form.format,
				file_url: form.fileUrl.trim(),
				mode: form.mode
			}
		});
		if (result.ok) {
			createSlideOverOpen = false;
			resetForm();
			successMessage = t('data_imports.msg_queued', 'Import job queued successfully');
			loadImports();
		} else {
			errorMessage = result.error ?? t('data_imports.msg_create_failed', 'Failed to create import');
		}
		saving = false;
	}

	function openView(imp: DataImport) {
		viewTarget = imp;
		showViewSlideOver = true;
	}

	function openDeleteConfirm(id: string) {
		deletingId = id;
		deleteConfirmOpen = true;
	}

	async function confirmDelete() {
		if (!deletingId) return;
		saving = true;
		const result = await api(`/admin/data-imports/${deletingId}`, { method: 'DELETE' });
		if (result.ok) {
			successMessage = t('data_imports.msg_deleted', 'Import deleted');
			loadImports();
		} else {
			errorMessage = result.error ?? t('data_imports.msg_delete_failed', 'Failed to delete import');
		}
		deleteConfirmOpen = false;
		deletingId = null;
		saving = false;
	}

	async function bulkDelete() {
		if (!selectedIds.size) return;
		saving = true;
		const ids = [...selectedIds];
		for (const id of ids) {
			await api(`/admin/data-imports/${id}`, { method: 'DELETE' });
		}
		selectedIds = new Set();
		successMessage = t('data_imports.msg_bulk_deleted', `${ids.length} import(s) deleted`);
		loadImports();
		saving = false;
	}

	function resetForm() {
		form = { name: '', type: 'users', format: 'csv', fileUrl: '', mode: 'create' };
	}

	$effect(() => {
		loadImports();
	});
</script>

<svelte:head>
	<title>{t('data_imports.page_title', 'Data Imports — JapaConnect Admin')}</title>
</svelte:head>

<PageHeader
	title={t('data_imports.title', 'Data Imports')}
	description={t('data_imports.description', 'Import platform data from CSV/JSON files')}
	actions={[
		{ label: t('data_imports.action_new', 'New Import'), icon: Plus, onclick: () => (createSlideOverOpen = true) },
		{ label: t('data_imports.action_refresh', 'Refresh'), icon: RefreshCw, variant: 'ghost', onclick: loadImports }
	]}
/>

<div class="space-y-6">
	{#if errorMessage}
		<div class="rounded-lg border border-danger bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{errorMessage}
		</div>
	{/if}
	{#if successMessage}
		<div class="rounded-lg border border-success bg-success-50 px-4 py-3 text-sm text-success-700">
			{successMessage}
		</div>
	{/if}

	<!-- Summary Cards -->
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-4">
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-muted">{t('data_imports.total_imports', 'Total Imports')}</p>
						<p class="text-2xl font-bold text-foreground">{imports.length}</p>
					</div>
					<div class="rounded-lg bg-primary-100 p-3">
						<FileUp class="h-6 w-6 text-primary-600" />
					</div>
				</div>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-muted">{t('data_imports.in_progress', 'In Progress')}</p>
						<p class="text-2xl font-bold text-warning-600">
							{imports.filter((i) => i.status === 'pending' || i.status === 'processing').length}
						</p>
					</div>
					<div class="rounded-lg bg-warning/15 p-3">
						<Clock class="h-6 w-6 text-warning-600" />
					</div>
				</div>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-muted">{t('data_imports.completed', 'Completed')}</p>
						<p class="text-2xl font-bold text-success-600">{imports.filter((i) => i.status === 'completed').length}</p>
					</div>
					<div class="rounded-lg bg-success/15 p-3">
						<CheckCircle class="h-6 w-6 text-success-600" />
					</div>
				</div>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-muted">{t('data_imports.failed', 'Failed / Partial')}</p>
						<p class="text-2xl font-bold text-danger-600">
							{imports.filter((i) => i.status === 'failed' || i.status === 'partial').length}
						</p>
					</div>
					<div class="rounded-lg bg-danger/15 p-3">
						<XCircle class="h-6 w-6 text-danger-600" />
					</div>
				</div>
			</CardContent>
		</Card>
	</div>

	<Tabs {tabs} bind:value={activeTab} />

	<Card>
		<CardContent class="p-0">
			{#if loading}
				<p class="py-8 text-center text-sm text-muted">{t('data_imports.loading', 'Loading imports…')}</p>
			{:else}
				<DataTable
					columns={importColumns}
					data={filteredImports}
					filters={importFilters}
					actions={importRowActions}
					pageSize={20}
					emptyMessage={t('data_imports.empty', 'No data imports found')}
					selectable
					bind:selectedIds
				>
					{#snippet bulkActions()}
						{#if canManage}
						<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={saving}>
							<Trash2 class="mr-1 h-3.5 w-3.5" />
							{t('data_imports.action_delete_selected', 'Delete Selected')} ({selectedIds.size})
						</Button>
						{/if}
					{/snippet}
				</DataTable>
			{/if}
		</CardContent>
	</Card>
</div>

<!-- Create Import SlideOver -->
<SlideOver bind:open={createSlideOverOpen} title={t('data_imports.modal_new_title', 'New Data Import')}>
	<div class="space-y-4">
		{#if errorMessage}
			<div class="rounded-lg border border-danger bg-danger-50 px-4 py-3 text-sm text-danger-700">
				{errorMessage}
			</div>
		{/if}
		<div>
			<label for="import-name" class="mb-1 block text-sm font-medium text-foreground-alt">
				{t('data_imports.import_name', 'Import Name')}
			</label>
			<input
				id="import-name"
				type="text"
				class="w-full rounded-lg border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				style="border-color: var(--color-border)"
				placeholder={t('data_imports.name_placeholder', 'e.g., Users Q1 2024 Migration')}
				bind:value={form.name}
			/>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="import-type" class="mb-1 block text-sm font-medium text-foreground-alt">
					{t('data_imports.data_type', 'Data Type')}
				</label>
				<Select bind:value={form.type} options={importTypes} />
			</div>
			<div>
				<label for="import-format" class="mb-1 block text-sm font-medium text-foreground-alt">
					{t('data_imports.format', 'Format')}
				</label>
				<Select bind:value={form.format} options={formatOptions} />
			</div>
		</div>
		<div>
			<label for="import-mode" class="mb-1 block text-sm font-medium text-foreground-alt">
				{t('data_imports.mode', 'Mode')}
			</label>
			<Select bind:value={form.mode} options={modeOptions} />
		</div>
		<div>
			<label for="import-file-url" class="mb-1 block text-sm font-medium text-foreground-alt">
				{t('data_imports.file_url', 'File URL')}
			</label>
			<input
				id="import-file-url"
				type="text"
				class="w-full rounded-lg border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				style="border-color: var(--color-border)"
				placeholder="/uploads/import-file.csv"
				bind:value={form.fileUrl}
			/>
			<p class="mt-1 text-xs text-muted">
				{t('data_imports.file_url_hint', 'Path to the uploaded file. Upload the file first via the Media uploader, then paste the path here.')}
			</p>
		</div>
		<div class="rounded-lg border p-3 text-xs" style="border-color: var(--color-border); color: var(--color-muted)">
			{t('data_imports.form_help', 'In "create" mode, rows that conflict with existing records are skipped. In "upsert" mode, existing records are updated and new ones are inserted.')}
		</div>
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => { createSlideOverOpen = false; resetForm(); }}>
				{t('data_imports.cancel', 'Cancel')}
			</Button>
			<Button onclick={submitImport} disabled={saving || !form.name.trim() || !form.fileUrl.trim()}>
				{saving ? t('data_imports.btn_queuing', 'Queuing…') : t('data_imports.btn_start', 'Start Import')}
			</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- View SlideOver -->
<SlideOver bind:open={showViewSlideOver} title={t('data_imports.modal_details_title', 'Import Details')}>
	{#if viewTarget}
		<div class="space-y-4">
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('data_imports.detail_name', 'Name')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.name}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('data_imports.detail_type', 'Type')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.type} · {viewTarget.format.toUpperCase()} · {viewTarget.mode}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('data_imports.detail_status', 'Status')}</p>
					<p class="mt-1">
						<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold capitalize {statusBadgeClass(viewTarget.status)}">
							{viewTarget.status}
						</span>
					</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('data_imports.detail_progress', 'Progress')}</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">
						{viewTarget.importedRows}/{viewTarget.totalRows} {t('data_imports.imported', 'imported')}
						· {viewTarget.failedRows} {t('data_imports.failed', 'failed')}
					</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('data_imports.detail_created', 'Created')}</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">{new Date(viewTarget.createdAt).toLocaleString()}</p>
				</div>
				{#if viewTarget.completedAt}
					<div>
						<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('data_imports.detail_completed', 'Completed')}</p>
						<p class="mt-1 text-sm" style="color: var(--color-foreground)">{new Date(viewTarget.completedAt).toLocaleString()}</p>
					</div>
				{/if}
				<div class="col-span-2">
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('data_imports.detail_file_url', 'File URL')}</p>
					<p class="mt-1 break-all text-sm font-mono" style="color: var(--color-foreground)">{viewTarget.fileUrl}</p>
				</div>
			</div>

			{#if viewTarget.errorLog && viewTarget.errorLog.length > 0}
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t('data_imports.detail_error_log', 'Error Log')} ({viewTarget.errorLog.length})
					</p>
					<ul class="mt-2 space-y-2">
						{#each viewTarget.errorLog as err, i (i)}
							<li class="rounded-md border p-2 text-xs" style="border-color: var(--color-border)">
								<p class="font-medium" style="color: var(--color-foreground)">
									{t('data_imports.row', 'Row')} {err.row}
								</p>
								<p class="mt-0.5" style="color: var(--color-danger)">{err.error}</p>
							</li>
						{/each}
					</ul>
				</div>
			{/if}
		</div>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end">
			<Button variant="outline" onclick={() => (showViewSlideOver = false)}>
				{t('data_imports.btn_close', 'Close')}
			</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- Delete Confirmation Modal -->
<Modal bind:open={deleteConfirmOpen} title={t('data_imports.modal_delete_title', 'Confirm Delete')} description={t('data_imports.modal_delete_description', 'This action cannot be undone')}>
	<div class="space-y-4">
		<p class="text-sm text-foreground-alt">
			{t('data_imports.modal_delete_body', 'Are you sure you want to delete this import record? The imported data will remain in the database.')}
		</p>
		<div class="flex justify-end gap-3">
			<Button variant="outline" onclick={() => (deleteConfirmOpen = false)}>{t('data_imports.cancel', 'Cancel')}</Button>
			<Button variant="destructive" onclick={confirmDelete} disabled={saving}>
				{saving ? t('data_imports.btn_deleting', 'Deleting…') : t('data_imports.btn_delete_import', 'Delete Import')}
			</Button>
		</div>
	</div>
</Modal>
