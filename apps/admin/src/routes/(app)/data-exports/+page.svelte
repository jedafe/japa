<script lang="ts">
	// /data-exports — Platform Data Export (Section 67).
	// Migrated to the EntityPage scaffold per apps/admin/AGENTS.md.
	// Non-standard CRUD: no edit, custom create form (select/checkbox),
	// status filter, summary cards, download link in view modal.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { t } from '$lib/stores/i18n.svelte';
	import { api, apiRaw, adminCatalogApi, type CatalogOption } from '$lib/api';
	import { escapeHtml } from '$lib/utils/html';
	import { apiFetch } from '$lib/api/client';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Select } from '$lib/components/ui/select';
	import { Download, RefreshCw, Clock, CheckCircle, XCircle, Trash2, FileDown } from 'lucide-svelte';

	type DataExport = {
		id: string;
		name: string;
		type: string;
		format: string;
		status: 'pending' | 'processing' | 'completed' | 'failed';
		fileUrl: string | null;
		fileSize: number | null;
		includeMetadata: boolean;
		errorMessage: string | null;
		createdBy: string;
		createdAt: string;
		completedAt: string | null;
	};

	// ── Page-local state ──────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let pageSuccess = $state('');
	let summaryData = $state<DataExport[]>([]);
	let summaryTotal = $state(0);
	let bulkDeleting = $state(false);
	let saving = $state(false);

	let exportTypes = $state<CatalogOption[]>([
		{ value: 'users', label: t('app.data_exports.type_users', 'Users') },
		{ value: 'posts', label: t('app.data_exports.type_posts', 'Posts') },
		{ value: 'comments', label: t('app.data_exports.type_comments', 'Comments') },
		{ value: 'orders', label: t('app.data_exports.type_orders', 'Orders') },
		{ value: 'transactions', label: t('app.data_exports.type_transactions', 'Transactions') },
		{ value: 'analytics', label: t('app.data_exports.type_analytics', 'Analytics') },
		{ value: 'moderation', label: t('app.data_exports.type_moderation', 'Moderation Logs') }
	]);

	let exportFormatOptions = $state<CatalogOption[]>([
		{ value: 'csv', label: t('app.data_exports.format_csv', 'CSV') },
		{ value: 'json', label: t('app.data_exports.format_json', 'JSON') }
	]);

	let exportStatusOptions = $state<CatalogOption[]>([
		{ value: 'pending', label: t('app.data_exports.tab_all', 'Pending') },
		{ value: 'processing', label: t('app.data_exports.tab_in_progress', 'Processing') },
		{ value: 'completed', label: t('app.data_exports.tab_completed', 'Completed') },
		{ value: 'failed', label: t('app.data_exports.tab_failed', 'Failed') }
	]);

	$effect(() => {
		void (async () => {
			const [typeRes, formatRes, statusRes] = await Promise.all([
				adminCatalogApi.get('data_exports.type'),
				adminCatalogApi.get('data_exports.format'),
				adminCatalogApi.get('data_exports.status')
			]);
			if (typeRes.ok && typeRes.data?.length) exportTypes = typeRes.data;
			if (formatRes.ok && formatRes.data?.length) exportFormatOptions = formatRes.data;
			if (statusRes.ok && statusRes.data?.length) exportStatusOptions = statusRes.data;
		})();
	});

	// ── Summary cards (page-local, above EntityPage) ──────────────────────
	async function loadSummary() {
		const result = await apiRaw<{ data: DataExport[]; pagination: { total: number } }>(
			'/admin/data-exports?page_size=100&page_offset=0'
		);
		if (result.ok) {
			summaryData = Array.isArray(result.data.data) ? result.data.data : [];
			summaryTotal = result.data.pagination?.total ?? summaryData.length;
		}
	}

	$effect(() => {
		loadSummary();
	});

	// ── Helpers ───────────────────────────────────────────────────────────
	function formatFileSize(bytes: number | null): string {
		if (!bytes) return '—';
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}

	function statusBadgeClass(status: string): string {
		switch (status) {
			case 'completed': return 'bg-success/15 text-success border-success';
			case 'failed': return 'bg-danger/15 text-danger border-danger';
			case 'processing': return 'bg-warning/15 text-warning border-warning';
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
			await Promise.all(idArr.map((id) => api(`/admin/data-exports/${id}`, { method: 'DELETE' })));
			pageSuccess = t('app.data_exports.msg_bulk_deleted', `${idArr.length} export(s) deleted`);
			await loadSummary();
			bulkDeleting = false;
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('app.data_exports.msg_delete_failed', 'Failed to delete export');
			bulkDeleting = false;
		}
	}

	// ── Authenticated file download ───────────────────────────────────────
	// The backend returns fileUrl as a relative API path
	// (/api/v1/admin/data-exports/{id}/download), but the admin SPA is served
	// from a different origin than the API and a raw <a href> cannot attach
	// the Bearer token. Fetch the file via apiFetch (blob-capable) so the
	// request gets the same CSRF-retry / 401-refresh coalescing as every
	// other admin API call, and trigger a blob download instead.
	let downloading = $state(false);

	async function downloadExport(row: DataExport) {
		if (downloading) return;
		downloading = true;
		pageError = '';
		try {
			const result = await apiFetch<Blob>(`/admin/data-exports/${row.id}/download`, {
				method: 'GET'
			}, undefined, 'blob');
			if (result.error || !(result.data instanceof Blob)) {
				pageError = t('app.data_exports.msg_download_failed', 'Failed to download export file');
				downloading = false;
				return;
			}
			const blob = result.data;
			const objUrl = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = objUrl;
			a.download = result.filename ?? `${row.name}.${row.format}`;
			document.body.appendChild(a);
			a.click();
			document.body.removeChild(a);
			URL.revokeObjectURL(objUrl);
			downloading = false;
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('app.data_exports.msg_download_failed', 'Failed to download export file');
			downloading = false;
		}
	}

	// ── EntityPage config ─────────────────────────────────────────────────
	// `fields` is required for EntityForm seeding even though we use
	// formSnippet for the create form (the export create flow has select
	// inputs and a checkbox that EntityForm supports, but we keep the
	// custom snippet for layout control).
	const fields = $derived<FieldDef[]>([
		{ key: 'name', label: t('app.data_exports.export_name', 'Export Name'), type: 'text', required: true },
		{ key: 'type', label: t('app.data_exports.data_type', 'Data Type'), type: 'select', options: exportTypes },
		{ key: 'format', label: t('app.data_exports.format', 'Format'), type: 'select', options: exportFormatOptions },
		{ key: 'includeMetadata', label: t('app.data_exports.include_metadata_timestamps_ids', 'Include metadata (timestamps, IDs)'), type: 'checkbox' }
	]);

	const config = $derived<EntityPageConfig>({
		title: t('app.data_exports.title', 'Data Exports'),
		description: t('app.data_exports.description', 'Export platform data in various formats'),
		createLabel: t('app.data_exports.action_new', 'New Export'),
		createPermission: 'gdpr.data_exports.view',
		deletePermission: 'gdpr.data_exports.view',
		readonly: true, // no edit — exports are fire-and-forget, not editable
		pageSize: 25,
		columns: [
			{
				key: 'name',
				label: t('app.data_exports.col_name', 'Name'),
				sortable: true,
				render: (_: any, row: any) => `
					<div>
						<p class="font-medium text-foreground">${escapeHtml(row.name)}</p>
						<p class="text-xs text-muted">${escapeHtml(row.type)} · ${escapeHtml(row.format.toUpperCase())}</p>
					</div>`
			},
			{
				key: 'status',
				label: t('app.data_exports.col_status', 'Status'),
				sortable: true,
				render: (value: string) => {
					const cls = statusBadgeClass(value);
					const label = String(value ?? '');
					const safe = escapeHtml(label.charAt(0).toUpperCase() + label.slice(1));
					return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${safe}</span>`;
				}
			},
			{
				key: 'fileSize',
				label: t('app.data_exports.col_size', 'Size'),
				sortable: true,
				render: (value: number | null) => `<span class="text-sm text-foreground-alt">${formatFileSize(value)}</span>`
			},
			{
				key: 'createdAt',
				label: t('app.data_exports.col_created', 'Created'),
				sortable: true,
				render: (value: string) => `<span class="text-sm text-foreground-alt">${new Date(value).toLocaleString()}</span>`
			},
			{
				key: 'completedAt',
				label: t('app.data_exports.col_completed', 'Completed'),
				sortable: true,
				render: (value: string | null) => value
					? `<span class="text-sm text-foreground-alt">${new Date(value).toLocaleString()}</span>`
					: `<span class="text-sm text-muted">—</span>`
			}
		],
		filters: [
			{
				key: 'status',
				label: t('app.data_exports.col_status', 'Status'),
				options: exportStatusOptions
			},
			{
				key: 'type',
				label: t('app.data_exports.filter_export_type', 'Export Type'),
				options: exportTypes
			}
		],
		bulkActions: [
			{
				label: t('app.data_exports.action_delete_selected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDelete
			}
		],
		fields,
		load: async ({ q, sort, filter, page, pageSize }) => {
			const params = new URLSearchParams();
			params.set('page_size', String(pageSize || 25));
			// page_offset is a row offset (api-response-format.md), not a page number.
			params.set('page_offset', String(page * (pageSize || 25)));
			if (filter.status) params.set('status', filter.status);
			if (filter.type) params.set('type', filter.type);
			if (q) params.set('q', q);
			if (sort) params.set('sort', sort);
			const result = await apiRaw<{ data: DataExport[]; pagination: { total: number; page_size: number; page_offset: number } }>(
				`/admin/data-exports?${params.toString()}`
			);
			if (result.ok) {
				const data = (result.data.data as DataExport[]) ?? [];
				const total = result.data.pagination?.total ?? data.length;
				const ps = result.data.pagination?.page_size ?? (pageSize || 25);
				const totalPages = Math.max(1, Math.ceil(total / ps));
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('app.data_exports.msg_create_failed', 'Failed to load exports'));
		},
		create: async (body) => {
			saving = true;
			pageError = '';
			try {
				const result = await api('/admin/data-exports', {
					method: 'POST',
					body: {
						name: String(body.name ?? '').trim(),
						type: body.type ?? 'users',
						format: body.format ?? 'csv',
						includeMetadata: body.includeMetadata ?? true
					}
				});
				if (result.ok) {
					pageSuccess = t('app.data_exports.msg_queued', 'Export job queued successfully');
					await loadSummary();
					saving = false;
					return { ok: true };
				}
				saving = false;
				return { ok: false, error: result.error ?? t('app.data_exports.msg_create_failed', 'Failed to create export') };
			} catch (e) {
				saving = false;
				return { ok: false, error: e instanceof Error ? e.message : t('app.data_exports.msg_create_failed', 'Failed to create export') };
			}
		},
		remove: async (id) => {
			const result = await api(`/admin/data-exports/${id}`, { method: 'DELETE' });
			if (result.ok) {
				pageSuccess = t('app.data_exports.msg_deleted', 'Export deleted');
				await loadSummary();
				return { ok: true };
			}
			return { ok: false, error: result.error ?? t('app.data_exports.msg_delete_failed', 'Failed to delete export') };
		},
		formSnippet: ExportForm,
		viewSnippet: ExportView
	});

</script>

<svelte:head>
	<title>{t('app.data_exports.page_title', 'Data Exports — {site} Admin')}</title>
</svelte:head>

<PageHeader
	title={t('app.data_exports.title', 'Data Exports')}
	description={t('app.data_exports.description', 'Export platform data in various formats')}
	actions={[
		{ label: t('app.data_exports.action_refresh', 'Refresh'), icon: RefreshCw, variant: 'ghost', onclick: () => { entityController?.reload(); loadSummary(); } }
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
						<p class="text-sm font-medium text-muted">{t('app.data_exports.total_exports', 'Total Exports')}</p>
						<p class="text-2xl font-bold text-foreground">{summaryTotal}</p>
					</div>
					<div class="rounded-lg bg-primary-100 p-3">
						<Download class="h-6 w-6 text-primary-600" />
					</div>
				</div>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-muted">{t('app.data_exports.tab_in_progress', 'In Progress')}</p>
						<p class="text-2xl font-bold text-warning-600">
							{summaryData.filter((e) => e.status === 'pending' || e.status === 'processing').length}
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
						<p class="text-sm font-medium text-muted">{t('app.data_exports.tab_completed', 'Completed')}</p>
						<p class="text-2xl font-bold text-success-600">{summaryData.filter((e) => e.status === 'completed').length}</p>
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
						<p class="text-sm font-medium text-muted">{t('app.data_exports.tab_failed', 'Failed')}</p>
						<p class="text-2xl font-bold text-danger-600">{summaryData.filter((e) => e.status === 'failed').length}</p>
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

<!-- Create form (formSnippet). Binds to page-local "form" state. -->
{#snippet ExportForm({ value, editing }: { value: any; editing: boolean })}
	<div class="space-y-4">
		<div>
			<label for="export-name" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.data_exports.export_name', 'Export Name')}</label>
			<input
				id="export-name"
				type="text"
				class="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				placeholder={t('app.data_exports.name_placeholder', 'e.g., Users Q1 2024')}
				bind:value={value.name}
			/>
		</div>
		<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
			<div>
				<label for="export-type" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.data_exports.data_type', 'Data Type')}</label>
				<Select bind:value={value.type} options={exportTypes} />
			</div>
			<div>
				<label for="export-format" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.data_exports.format', 'Format')}</label>
				<Select bind:value={value.format} options={exportFormatOptions} />
			</div>
		</div>
		<div class="flex items-center gap-2">
			<input
				id="export-metadata"
				type="checkbox"
				bind:checked={value.includeMetadata}
				class="h-4 w-4 rounded border-border text-primary-600 focus:ring-primary"
			/>
			<label for="export-metadata" class="text-sm font-medium text-foreground-alt">{t('app.data_exports.include_metadata_timestamps_ids', 'Include metadata (timestamps, IDs)')}</label>
		</div>
	</div>
{/snippet}

<!-- View modal body (viewSnippet) -->
{#snippet ExportView({ row }: { row: DataExport })}
	<div class="space-y-4">
		<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.data_exports.detail_name', 'Name')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.data_exports.detail_type', 'Type')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.type} · {row.format.toUpperCase()}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.data_exports.detail_status', 'Status')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.data_exports.detail_size', 'Size')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{formatFileSize(row.fileSize)}</p>
			</div>
			<div class="col-span-2">
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.data_exports.detail_created', 'Created')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(row.createdAt).toLocaleString()}</p>
			</div>
			{#if row.completedAt}
				<div class="col-span-2">
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.data_exports.col_completed', 'Completed')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(row.completedAt).toLocaleString()}</p>
				</div>
			{/if}
			{#if row.status === 'completed' && row.fileUrl}
				<div class="col-span-2">
					<button
						type="button"
						class="inline-flex items-center gap-1 text-sm font-medium disabled:opacity-50"
						style="color: var(--color-primary)"
						onclick={() => downloadExport(row)}
						disabled={downloading}
					>
						<FileDown class="h-3.5 w-3.5" /> {downloading ? t('app.data_exports.detail_downloading', 'Downloading…') : t('app.data_exports.detail_download', 'Download file')}
					</button>
				</div>
			{/if}
			{#if row.errorMessage}
				<div class="col-span-2">
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.data_exports.detail_error', 'Error')}</p>
					<p class="mt-1 text-sm font-medium text-danger">{row.errorMessage}</p>
				</div>
			{/if}
		</div>
	</div>
{/snippet}
