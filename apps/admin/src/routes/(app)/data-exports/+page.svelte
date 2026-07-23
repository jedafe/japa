<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { api } from "$lib/api";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent } from "$lib/components/ui/card";
	import { Tabs } from "$lib/components/ui/tabs";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Badge } from "$lib/components/ui/badge";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { Select } from "$lib/components/ui/select";
	import { Download, Plus, RefreshCw, Clock, CheckCircle, XCircle, FileDown, Eye, Trash2 } from "lucide-svelte";

	type DataExport = {
		id: string;
		name: string;
		type: string;
		format: string;
		status: "pending" | "processing" | "completed" | "failed";
		fileSize: number | null;
		fileUrl: string | null;
		createdAt: string;
		completedAt: string | null;
		errorMessage: string | null;
	};

	let exports = $state<DataExport[]>([]);
	let loading = $state(true);
	let activeTab = $state("all");
	let filters = $state<Record<string, string>>({});
	let createModalOpen = $state(false);
	let deleteConfirmOpen = $state(false);
	let deletingId = $state<string | null>(null);
	let saving = $state(false);
	let errorMessage = $state("");
	let successMessage = $state("");

	let showViewModal = $state(false);
	let viewTarget = $state<DataExport | null>(null);
	let selectedIds = $state<Set<string>>(new Set());

	const canManageExports = $derived(hasPermission("data-exports.manage"));

	let form = $state({
		name: "",
		type: "users",
		format: "csv",
		includeMetadata: true
	});

	const exportTypes = $derived([
		{ value: "users", label: t('app.data_exports.type_users', 'Users') },
		{ value: "posts", label: t('app.data_exports.type_posts', 'Posts') },
		{ value: "comments", label: t('app.data_exports.type_comments', 'Comments') },
		{ value: "orders", label: t('app.data_exports.type_orders', 'Orders') },
		{ value: "transactions", label: t('app.data_exports.type_transactions', 'Transactions') },
		{ value: "analytics", label: t('app.data_exports.type_analytics', 'Analytics') },
		{ value: "moderation", label: t('app.data_exports.type_moderation', 'Moderation Logs') }
	]);

	const formatOptions = [
		{ value: "csv", label: t('app.data_exports.format_csv', 'CSV') },
		{ value: "json", label: t('app.data_exports.format_json', 'JSON') },
		{ value: "xlsx", label: t('app.data_exports.format_xlsx', 'Excel (XLSX)') }
	];

	let tabs = $derived([
		{ value: "all", label: t('app.data_exports.tab_all', 'All Exports'), icon: Download, count: exports.length },
		{ value: "active", label: t('app.data_exports.tab_in_progress', 'In Progress'), icon: Clock, count: exports.filter((e) => e.status === "pending" || e.status === "processing").length },
		{ value: "completed", label: t('app.data_exports.tab_completed', 'Completed'), icon: CheckCircle, count: exports.filter((e) => e.status === "completed").length },
		{ value: "failed", label: t('app.data_exports.tab_failed', 'Failed'), icon: XCircle, count: exports.filter((e) => e.status === "failed").length }
	]);

	let filteredExports = $derived.by(() => {
		let result = [...exports];
		if (activeTab === "active") result = result.filter((e) => e.status === "pending" || e.status === "processing");
		if (activeTab === "completed") result = result.filter((e) => e.status === "completed");
		if (activeTab === "failed") result = result.filter((e) => e.status === "failed");
		if (filters.type) result = result.filter((e) => e.type === filters.type);
		return result;
	});

	function formatFileSize(bytes: number | null): string {
		if (!bytes) return "—";
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}

	function statusVariant(status: string) {
		switch (status) {
			case "completed": return "success";
			case "failed": return "destructive";
			case "processing": return "warning";
			default: return "secondary";
		}
	}

	const exportColumns = [
		{
			key: "name",
			label: t('app.data_exports.col_name', 'Name'),
			sortable: true,
			render: (_: any, row: any) => `
				<div>
					<p class="font-medium text-foreground">${row.name}</p>
					<p class="text-xs text-muted">${row.type} · ${row.format.toUpperCase()}</p>
				</div>
			`
		},
		{
			key: "status",
			label: t('app.data_exports.col_status', 'Status'),
			sortable: true,
			render: (value: string) => {
				const variant = statusVariant(value);
				const cls =
					variant === "success"
						? "bg-success/15 text-success border-success"
						: variant === "destructive"
							? "bg-danger/15 text-danger border-danger"
							: variant === "warning"
								? "bg-warning/15 text-warning border-warning"
								: "bg-surface text-foreground border";
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${value.charAt(0).toUpperCase() + value.slice(1)}</span>`;
			}
		},
		{
			key: "fileSize",
			label: t('app.data_exports.col_size', 'Size'),
			sortable: true,
			render: (value: number | null) => `<span class="text-sm text-foreground-alt">${formatFileSize(value)}</span>`
		},
		{
			key: "createdAt",
			label: t('app.data_exports.col_created', 'Created'),
			sortable: true,
			render: (value: string) => `<span class="text-sm text-foreground-alt">${new Date(value).toLocaleString()}</span>`
		},
		{
			key: "completedAt",
			label: t('app.data_exports.col_completed', 'Completed'),
			sortable: true,
			render: (value: string | null) => value ? `<span class="text-sm text-foreground-alt">${new Date(value).toLocaleString()}</span>` : `<span class="text-sm text-muted">—</span>`
		},
		{
			key: "actions",
			label: "",
			class: "w-24",
			component: ExportActionsCell
		}
	];

	async function loadExports() {
		loading = true;
		const result = await api<DataExport[]>("/data-exports");
		if (result.ok) {
			exports = Array.isArray(result.data) ? result.data : [];
		}
		loading = false;
	}

	async function submitExport() {
		if (!form.name.trim()) return;
		saving = true;
		errorMessage = "";
		const result = await api("/data-exports", {
			method: "POST",
			body: {
				name: form.name.trim(),
				type: form.type,
				format: form.format,
				includeMetadata: form.includeMetadata
			}
		});
		if (result.ok) {
			createModalOpen = false;
			resetForm();
			successMessage = t('app.data_exports.msg_queued', 'Export job queued successfully');
			loadExports();
		} else {
			errorMessage = result.error ?? t('app.data_exports.msg_create_failed', 'Failed to create export');
		}
		saving = false;
	}

	function openView(ex: DataExport) {
		viewTarget = ex;
		showViewModal = true;
	}

	function openDeleteConfirm(id: string) {
		deletingId = id;
		deleteConfirmOpen = true;
	}

	async function confirmDelete() {
		if (!deletingId) return;
		saving = true;
		const result = await api(`/data-exports/${deletingId}`, { method: "DELETE" });
		if (result.ok) {
			successMessage = t('app.data_exports.msg_deleted', 'Export deleted');
			loadExports();
		} else {
			errorMessage = result.error ?? t('app.data_exports.msg_delete_failed', 'Failed to delete export');
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
			await api(`/data-exports/${id}`, { method: "DELETE" });
		}
		selectedIds = new Set();
		successMessage = t('app.data_exports.msg_bulk_deleted', `${ids.length} export(s) deleted`);
		loadExports();
		saving = false;
	}

	function resetForm() {
		form = { name: "", type: "users", format: "csv", includeMetadata: true };
	}

	$effect(() => {
		loadExports();
	});
</script>

<svelte:head>
	<title>{t('app.data_exports.page_title', 'Data Exports — JapaConnect Admin')}</title>
</svelte:head>

<PageHeader
	title={t('app.data_exports.title', 'Data Exports')}
	description={t('app.data_exports.description', 'Export platform data in various formats')}
	actions={[
		{ label: t('app.data_exports.action_new', 'New Export'), icon: Plus, onclick: () => (createModalOpen = true) },
		{ label: t('app.data_exports.action_refresh', 'Refresh'), icon: RefreshCw, variant: "ghost", onclick: loadExports }
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
						<p class="text-sm font-medium text-muted">{t('app.data-exports.total_exports')}</p>
						<p class="text-2xl font-bold text-foreground">{exports.length}</p>
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
						<p class="text-sm font-medium text-muted">{t('app.data-exports.in_progress')}</p>
						<p class="text-2xl font-bold text-warning-600">
							{exports.filter((e) => e.status === "pending" || e.status === "processing").length}
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
						<p class="text-sm font-medium text-muted">{t('app.data-exports.completed')}</p>
						<p class="text-2xl font-bold text-success-600">{exports.filter((e) => e.status === "completed").length}</p>
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
						<p class="text-sm font-medium text-muted">{t('app.data-exports.failed')}</p>
						<p class="text-2xl font-bold text-danger-600">{exports.filter((e) => e.status === "failed").length}</p>
					</div>
					<div class="rounded-lg bg-danger/15 p-3">
						<XCircle class="h-6 w-6 text-danger-600" />
					</div>
				</div>
			</CardContent>
		</Card>
	</div>

	<Tabs {tabs} bind:value={activeTab} />

	<FilterBar
		bind:value={filters}
		filters={[
			{
				key: "type",
				label: t('app.data_exports.filter_export_type', 'Export Type'),
				options: exportTypes
			}
		]}
	/>

	<Card>
		<CardContent class="p-0">
			{#if loading}
				<p class="py-8 text-center text-sm text-muted">{t('app.data-exports.loading')}</p>
			{:else}
				<DataTable
					columns={exportColumns}
					data={filteredExports}
					pageSize={20}
					emptyMessage={t('app.data_exports.empty', 'No data exports found')}
					selectable
					bind:selectedIds
				>
					{#snippet bulkActions()}
						{#if canManageExports}
						<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={saving}>
							<Trash2 class="h-3.5 w-3.5 mr-1" />
							{t('app.data_exports.action_delete_selected', 'Delete Selected')} ({selectedIds.size})
						</Button>
						{/if}
					{/snippet}
				</DataTable>
			{/if}
		</CardContent>
	</Card>
</div>

<!-- Create Export Modal -->
<Modal bind:open={createModalOpen} title={t('app.data_exports.modal_new_title', 'New Data Export')} description={t('app.data_exports.modal_new_description', 'Queue a new data export job')}>
	<div class="space-y-4">
		<div>
			<label for="export-name" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.data-exports.export_name')}</label>
			<input
				id="export-name"
				type="text"
				class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				placeholder={t('app.data_exports.name_placeholder', 'e.g., Users Q1 2024')}
				bind:value={form.name}
			/>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="export-type" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.data-exports.data_type')}</label>
				<Select bind:value={form.type} options={exportTypes} />
			</div>
			<div>
				<label for="export-format" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.data-exports.format')}</label>
				<Select bind:value={form.format} options={formatOptions} />
			</div>
		</div>
		<div class="flex items-center gap-2">
			<input
				id="export-metadata"
				type="checkbox"
				bind:checked={form.includeMetadata}
				class="h-4 w-4 rounded border text-primary-600 focus:ring-primary"
			/>
			<label for="export-metadata" class="text-sm font-medium text-foreground-alt">{t('app.data-exports.include_metadata_timestamps_ids')}</label>
		</div>
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => { createModalOpen = false; resetForm(); }}>{t('app.data-exports.cancel')}</Button>
			<Button onclick={submitExport} disabled={saving || !form.name.trim()}>
				{saving ? t('app.data_exports.btn_queuing', 'Queuing...') : t('app.data_exports.btn_start', 'Start Export')}
			</Button>
		</div>
	</div>
</Modal>

<!-- View Modal -->
<Modal bind:open={showViewModal} title={t('app.data_exports.modal_details_title', 'Export Details')}>
	{#if viewTarget}
		<div class="space-y-4">
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.data_exports.detail_name', 'Name')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.name}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.data_exports.detail_type', 'Type')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.type} · {viewTarget.format.toUpperCase()}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.data_exports.detail_status', 'Status')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.status}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.data_exports.detail_size', 'Size')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{formatFileSize(viewTarget.fileSize)}</p>
				</div>
				<div class="col-span-2">
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.data_exports.detail_created', 'Created')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(viewTarget.createdAt).toLocaleString()}</p>
				</div>
				{#if viewTarget.status === "completed" && viewTarget.fileUrl}
					<div class="col-span-2">
						<a href={viewTarget.fileUrl} class="inline-flex items-center gap-1 text-sm font-medium" style="color: var(--color-primary)" download>
							<Download class="h-3.5 w-3.5" /> {t('app.data_exports.detail_download', 'Download file')}
						</a>
					</div>
				{/if}
			</div>
		</div>
		<div class="mt-6 flex justify-end">
			<Button variant="outline" onclick={() => (showViewModal = false)}>{t('app.data_exports.btn_close', 'Close')}</Button>
		</div>
	{/if}
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={deleteConfirmOpen} title={t('app.data_exports.modal_delete_title', 'Confirm Delete')} description={t('app.data_exports.modal_delete_description', 'This action cannot be undone')}>
	<div class="space-y-4">
		<p class="text-sm text-foreground-alt">
			{t('app.data_exports.modal_delete_body', 'Are you sure you want to delete this export? The downloaded file will no longer be available.')}
		</p>
		<div class="flex justify-end gap-3">
			<Button variant="outline" onclick={() => (deleteConfirmOpen = false)}>{t('app.data-exports.cancel')}</Button>
			<Button variant="destructive" onclick={confirmDelete} disabled={saving}>
				{saving ? t('app.data_exports.btn_deleting', 'Deleting...') : t('app.data_exports.btn_delete_export', 'Delete Export')}
			</Button>
		</div>
	</div>
</Modal>

{#snippet ExportActionsCell({ value, row }: { value: any; row: DataExport })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView(row)}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canManageExports}
			<Button variant="ghost" size="sm" onclick={() => openDeleteConfirm(row.id)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
