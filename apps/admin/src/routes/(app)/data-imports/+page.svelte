<script lang="ts">
	// /data-imports — Platform Data Import (Section 67).
	// Migrated to the EntityPage scaffold per apps/admin/AGENTS.md.
	// Non-standard CRUD: no edit, custom create form (select/file-url),
	// status filter, summary cards, error log in view slide-over.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { t } from '$lib/stores/i18n.svelte';
	import { api, apiRaw, adminCatalogApi, type CatalogOption } from '$lib/api';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Select } from '$lib/components/ui/select';
	import { RefreshCw, Clock, CheckCircle, XCircle, Trash2, FileUp } from 'lucide-svelte';

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

	// ── Page-local state ──────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let pageSuccess = $state('');
	let summaryData = $state<DataImport[]>([]);
	let bulkDeleting = $state(false);
	let saving = $state(false);

	let importTypes = $state<CatalogOption[]>([
		{ value: 'users', label: t('app.data_imports.type_users', 'Users') },
		{ value: 'posts', label: t('app.data_imports.type_posts', 'Posts') },
		{ value: 'comments', label: t('app.data_imports.type_comments', 'Comments') },
		{ value: 'orders', label: t('app.data_imports.type_orders', 'Orders') },
		{ value: 'transactions', label: t('app.data_imports.type_transactions', 'Transactions') },
		{ value: 'analytics', label: t('app.data_imports.type_analytics', 'Analytics') },
		{ value: 'moderation', label: t('app.data_imports.type_moderation', 'Moderation Logs') }
	]);

	let importFormatOptions = $state<CatalogOption[]>([
		{ value: 'csv', label: t('app.data_imports.format_csv', 'CSV') },
		{ value: 'json', label: t('app.data_imports.format_json', 'JSON') }
	]);

	let importModeOptions = $state<CatalogOption[]>([
		{ value: 'create', label: t('app.data_imports.mode_create', 'Create (skip existing)') },
		{ value: 'upsert', label: t('app.data_imports.mode_upsert', 'Upsert (update or insert)') }
	]);

	let importStatusOptions = $state<CatalogOption[]>([
		{ value: 'pending', label: 'Pending' },
		{ value: 'processing', label: 'Processing' },
		{ value: 'completed', label: t('app.data_imports.tab_completed', 'Completed') },
		{ value: 'partial', label: t('app.data_imports.tab_partial', 'Partial') },
		{ value: 'failed', label: t('app.data_imports.tab_failed', 'Failed') }
	]);

	$effect(() => {
		void (async () => {
			const [typeRes, formatRes, modeRes, statusRes] = await Promise.all([
				adminCatalogApi.get('data_imports.type'),
				adminCatalogApi.get('data_imports.format'),
				adminCatalogApi.get('data_imports.mode'),
				adminCatalogApi.get('data_imports.status')
			]);
			if (typeRes.ok && typeRes.data?.length) importTypes = typeRes.data;
			if (formatRes.ok && formatRes.data?.length) importFormatOptions = formatRes.data;
			if (modeRes.ok && modeRes.data?.length) importModeOptions = modeRes.data;
			if (statusRes.ok && statusRes.data?.length) importStatusOptions = statusRes.data;
		})();
	});

	// ── Summary cards (page-local, above EntityPage) ──────────────────────
	async function loadSummary() {
		const result = await apiRaw<{ data: DataImport[]; pagination: { total: number } }>(
			'/admin/data-imports?page_size=100&page_offset=0'
		);
		if (result.ok) {
			summaryData = Array.isArray(result.data.data) ? result.data.data : [];
		}
	}

	$effect(() => {
		loadSummary();
	});

	// ── Helpers ───────────────────────────────────────────────────────────
	function statusBadgeClass(status: string): string {
		switch (status) {
			case 'completed': return 'bg-success/15 text-success border-success';
			case 'failed': return 'bg-danger/15 text-danger border-danger';
			case 'processing': return 'bg-warning/15 text-warning border-warning';
			case 'partial': return 'bg-warning/15 text-warning border-warning';
			default: return 'bg-surface text-foreground border';
		}
	}

	// ── Bulk delete (in-flight guard per AGENTS.md) ───────────────────────
	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return;
		bulkDeleting = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => api(`/admin/data-imports/${id}`, { method: 'DELETE' })));
			pageSuccess = t('app.data_imports.msg_bulk_deleted', `${idArr.length} import(s) deleted`);
			await loadSummary();
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('app.data_imports.msg_delete_failed', 'Failed to delete import');
		} finally {
			bulkDeleting = false;
		}
	}

	// ── EntityPage config ─────────────────────────────────────────────────
	const fields = $derived<FieldDef[]>([
		{ key: 'name', label: t('app.data_imports.import_name', 'Import Name'), type: 'text', required: true },
		{ key: 'type', label: t('app.data_imports.data_type', 'Data Type'), type: 'select', options: importTypes },
		{ key: 'format', label: t('app.data_imports.format', 'Format'), type: 'select', options: importFormatOptions },
		{ key: 'mode', label: t('app.data_imports.mode', 'Mode'), type: 'select', options: importModeOptions },
		{ key: 'file_url', label: t('app.data_imports.file_url', 'File URL'), type: 'text', required: true }
	]);

	const config = $derived<EntityPageConfig>({
		title: t('app.data_imports.title', 'Data Imports'),
		description: t('app.data_imports.description', 'Import platform data from CSV/JSON files'),
		createLabel: t('app.data_imports.action_new', 'New Import'),
		createPermission: 'gdpr.data_exports.view',
		deletePermission: 'gdpr.data_exports.view',
		readonly: true, // no edit — imports are fire-and-forget, not editable
		pageSize: 25,
		columns: [
			{
				key: 'name',
				label: t('app.data_imports.col_name', 'Name'),
				sortable: true,
				render: (_: any, row: any) => `
					<div>
						<p class="font-medium text-foreground">${row.name}</p>
						<p class="text-xs text-muted">${row.type} · ${row.format.toUpperCase()} · ${row.mode}</p>
					</div>`
			},
			{
				key: 'status',
				label: t('app.data_imports.col_status', 'Status'),
				sortable: true,
				render: (value: string) => {
					const cls = statusBadgeClass(value);
					return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold capitalize ${cls}">${value}</span>`;
				}
			},
			{
				key: 'importedRows',
				label: t('app.data_imports.col_progress', 'Progress'),
				sortable: true,
				render: (v: number, row: DataImport) => {
					const total = row.totalRows || 0;
					const failed = row.failedRows || 0;
					const failedTag = failed > 0
						? ` <span class="text-xs text-danger">· ${failed} ${t('app.data_imports.failed', 'failed')}</span>`
						: '';
					return `<span class="text-sm text-foreground-alt">${v}/${total} ${t('app.data_imports.imported', 'imported')}${failedTag}</span>`;
				}
			},
			{
				key: 'createdAt',
				label: t('app.data_imports.col_created', 'Created'),
				sortable: true,
				render: (value: string) => `<span class="text-sm text-foreground-alt">${new Date(value).toLocaleString()}</span>`
			},
			{
				key: 'completedAt',
				label: t('app.data_imports.col_completed', 'Completed'),
				sortable: true,
				render: (value: string | null) => value
					? `<span class="text-sm text-foreground-alt">${new Date(value).toLocaleString()}</span>`
					: `<span class="text-sm text-muted">—</span>`
			}
		],
		filters: [
			{
				key: 'status',
				label: t('app.data_imports.col_status', 'Status'),
				options: importStatusOptions
			},
			{
				key: 'type',
				label: t('app.data_imports.filter_import_type', 'Import Type'),
				options: importTypes
			}
		],
		bulkActions: [
			{
				label: t('app.data_imports.action_delete_selected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDelete
			}
		],
		fields,
		load: async ({ q, sort, filter, page, pageSize }) => {
			const params = new URLSearchParams();
			params.set('page_size', String(pageSize || 25));
			params.set('page_offset', String(((page || 1) - 1) * (pageSize || 25)));
			if (filter.status) params.set('status', filter.status);
			if (filter.type) params.set('type', filter.type);
			if (q) params.set('q', q);
			if (sort) params.set('sort', sort);
			const result = await apiRaw<{ data: DataImport[]; pagination: { total: number; page_size: number; page_offset: number } }>(
				`/admin/data-imports?${params.toString()}`
			);
			if (result.ok) {
				const data = (result.data.data as DataImport[]) ?? [];
				const total = result.data.pagination?.total ?? data.length;
				const ps = result.data.pagination?.page_size ?? (pageSize || 25);
				const totalPages = Math.max(1, Math.ceil(total / ps));
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('app.data_imports.msg_create_failed', 'Failed to load imports'));
		},
		create: async (body) => {
			saving = true;
			pageError = '';
			try {
				const result = await api('/admin/data-imports', {
					method: 'POST',
					body: {
						name: String(body.name ?? '').trim(),
						type: body.type ?? 'users',
						format: body.format ?? 'csv',
						file_url: String(body.file_url ?? '').trim(),
						mode: body.mode ?? 'create'
					}
				});
				if (result.ok) {
					pageSuccess = t('app.data_imports.msg_queued', 'Import job queued successfully');
					await loadSummary();
					return { ok: true };
				}
				return { ok: false, error: result.error ?? t('app.data_imports.msg_create_failed', 'Failed to create import') };
			} finally {
				saving = false;
			}
		},
		remove: async (id) => {
			const result = await api(`/admin/data-imports/${id}`, { method: 'DELETE' });
			if (result.ok) {
				pageSuccess = t('app.data_imports.msg_deleted', 'Import deleted');
				await loadSummary();
				return { ok: true };
			}
			return { ok: false, error: result.error ?? t('app.data_imports.msg_delete_failed', 'Failed to delete import') };
		},
		formSnippet: ImportForm,
		viewSnippet: ImportView
	});
</script>

<svelte:head>
	<title>{t('app.data_imports.page_title', 'Data Imports — JapaConnect Admin')}</title>
</svelte:head>

<PageHeader
	title={t('app.data_imports.title', 'Data Imports')}
	description={t('app.data_imports.description', 'Import platform data from CSV/JSON files')}
	actions={[
		{ label: t('app.data_imports.action_refresh', 'Refresh'), icon: RefreshCw, variant: 'ghost', onclick: () => { entityController?.reload(); loadSummary(); } }
	]}
/>

<div class="space-y-6">
	{#if pageError}
		<div class="rounded-lg border border-danger bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{pageError}
		</div>
	{/if}
	{#if pageSuccess}
		<div class="rounded-lg border border-success bg-success-50 px-4 py-3 text-sm text-success-700">
			{pageSuccess}
		</div>
	{/if}

	<!-- Summary Cards -->
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-4">
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-muted">{t('app.data_imports.total_imports', 'Total Imports')}</p>
						<p class="text-2xl font-bold text-foreground">{summaryData.length}</p>
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
						<p class="text-sm font-medium text-muted">{t('app.data_imports.in_progress', 'In Progress')}</p>
						<p class="text-2xl font-bold text-warning-600">
							{summaryData.filter((i) => i.status === 'pending' || i.status === 'processing').length}
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
						<p class="text-sm font-medium text-muted">{t('app.data_imports.completed', 'Completed')}</p>
						<p class="text-2xl font-bold text-success-600">{summaryData.filter((i) => i.status === 'completed').length}</p>
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
						<p class="text-sm font-medium text-muted">{t('app.data_imports.failed', 'Failed / Partial')}</p>
						<p class="text-2xl font-bold text-danger-600">
							{summaryData.filter((i) => i.status === 'failed' || i.status === 'partial').length}
						</p>
					</div>
					<div class="rounded-lg bg-danger/15 p-3">
						<XCircle class="h-6 w-6 text-danger-600" />
					</div>
				</div>
			</CardContent>
		</Card>
	</div>

	<EntityPage {config} bind:controller={entityController} />
</div>

<!-- ── Snippets ───────────────────────────────────────────────────────── -->

<!-- Create form (formSnippet) -->
{#snippet ImportForm({ value, editing }: { value: any; editing: boolean })}
	<div class="space-y-4">
		<div>
			<label for="import-name" class="mb-1 block text-sm font-medium text-foreground-alt">
				{t('app.data_imports.import_name', 'Import Name')}
			</label>
			<input
				id="import-name"
				type="text"
				class="w-full rounded-lg border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				style="border-color: var(--color-border)"
				placeholder={t('app.data_imports.name_placeholder', 'e.g., Users Q1 2024 Migration')}
				bind:value={value.name}
			/>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="import-type" class="mb-1 block text-sm font-medium text-foreground-alt">
					{t('app.data_imports.data_type', 'Data Type')}
				</label>
				<Select bind:value={value.type} options={importTypes} />
			</div>
			<div>
				<label for="import-format" class="mb-1 block text-sm font-medium text-foreground-alt">
					{t('app.data_imports.format', 'Format')}
				</label>
				<Select bind:value={value.format} options={importFormatOptions} />
			</div>
		</div>
		<div>
			<label for="import-mode" class="mb-1 block text-sm font-medium text-foreground-alt">
				{t('app.data_imports.mode', 'Mode')}
			</label>
			<Select bind:value={value.mode} options={importModeOptions} />
		</div>
		<div>
			<label for="import-file-url" class="mb-1 block text-sm font-medium text-foreground-alt">
				{t('app.data_imports.file_url', 'File URL')}
			</label>
			<input
				id="import-file-url"
				type="text"
				class="w-full rounded-lg border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				style="border-color: var(--color-border)"
				placeholder="/uploads/import-file.csv"
				bind:value={value.file_url}
			/>
			<p class="mt-1 text-xs text-muted">
				{t('app.data_imports.file_url_hint', 'Path to the uploaded file. Upload the file first via the Media uploader, then paste the path here.')}
			</p>
		</div>
		<div class="rounded-lg border p-3 text-xs" style="border-color: var(--color-border); color: var(--color-muted)">
			{t('app.data_imports.form_help', 'In "create" mode, rows that conflict with existing records are skipped. In "upsert" mode, existing records are updated and new ones are inserted.')}
		</div>
	</div>
{/snippet}

<!-- View slide-over body (viewSnippet) -->
{#snippet ImportView({ row }: { row: DataImport })}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.data_imports.detail_name', 'Name')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.data_imports.detail_type', 'Type')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.type} · {row.format.toUpperCase()} · {row.mode}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.data_imports.detail_status', 'Status')}</p>
				<p class="mt-1">
					<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold capitalize {statusBadgeClass(row.status)}">
						{row.status}
					</span>
				</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.data_imports.detail_progress', 'Progress')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">
					{row.importedRows}/{row.totalRows} {t('app.data_imports.imported', 'imported')}
					· {row.failedRows} {t('app.data_imports.failed', 'failed')}
				</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.data_imports.detail_created', 'Created')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{new Date(row.createdAt).toLocaleString()}</p>
			</div>
			{#if row.completedAt}
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.data_imports.detail_completed', 'Completed')}</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">{new Date(row.completedAt).toLocaleString()}</p>
				</div>
			{/if}
			<div class="col-span-2">
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.data_imports.detail_file_url', 'File URL')}</p>
				<p class="mt-1 break-all text-sm font-mono" style="color: var(--color-foreground)">{row.fileUrl}</p>
			</div>
		</div>

		{#if row.errorLog && row.errorLog.length > 0}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('app.data_imports.detail_error_log', 'Error Log')} ({row.errorLog.length})
				</p>
				<ul class="mt-2 space-y-2">
					{#each row.errorLog as err, i (i)}
						<li class="rounded-md border p-2 text-xs" style="border-color: var(--color-border)">
							<p class="font-medium" style="color: var(--color-foreground)">
								{t('app.data_imports.row', 'Row')} {err.row}
							</p>
							<p class="mt-0.5" style="color: var(--color-danger)">{err.error}</p>
						</li>
					{/each}
				</ul>
			</div>
		{/if}
	</div>
{/snippet}
