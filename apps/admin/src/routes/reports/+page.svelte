<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import Pagination from "$lib/components/ui/Pagination.svelte";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { RefreshCw, Eye, Trash2, Download } from "lucide-svelte";
	import { reportsAdminApiExtended } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";

	type Report = {
		id: string;
		title: string;
		report_type: string;
		status: string;
		generated_at: string;
		file_url?: string;
		created_at: string;
	};

	let reports = $state<Report[]>([]);
	let loading = $state(true);
	let error = $state("");
	let currentPage = $state(0);
	let totalItems = $state(0);
	let pageSize = $state(25);
	let totalPages = $state(1);
	let searchQuery = $state("");
	let filterValues = $state<Record<string, string>>({});
	let selectedIds = $state<Set<string>>(new Set());

	// Modals
	let showViewModal = $state(false);
	let viewTarget = $state<Report | null>(null);
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<Report | null>(null);
	let acting = $state(false);

	const filterDefs = [
		{
			key: "status",
			label: t("reports.status", "Status"),
			options: [
				{ value: "ready", label: "Ready" },
				{ value: "generating", label: "Generating" },
				{ value: "failed", label: "Failed" }
			]
		},
		{
			key: "report_type",
			label: t("reports.type", "Type"),
			options: [
				{ value: "analytics", label: "Analytics" },
				{ value: "moderation", label: "Moderation" },
				{ value: "user_activity", label: "User Activity" },
				{ value: "financial", label: "Financial" }
			]
		}
	];

	const columns = [
		{
			key: "title",
			label: t("reports.title", "Title"),
			sortable: true
		},
		{
			key: "report_type",
			label: t("reports.type", "Type"),
			sortable: true
		},
		{
			key: "status",
			label: t("reports.status", "Status"),
			sortable: true,
			render: (v: string) => {
				const colors = v === "ready" ? "bg-success/15 text-success dark:text-success" : v === "generating" ? "bg-blue-500/15 text-info dark:text-blue-400" : "bg-danger/15 text-danger dark:text-danger";
				return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors}">${v}</span>`;
			}
		},
		{
			key: "generated_at",
			label: t("reports.generated", "Generated"),
			sortable: true,
			render: (v: string) => new Date(v).toLocaleString()
		},
		{
			key: "actions",
			label: t("common.actions", "Actions"),
			class: "w-32",
			componentSnippet: ReportActionsCell
		}
	];

	async function loadReports() {
		loading = true;
		error = "";
		try {
			const params = new URLSearchParams({
				page: String(currentPage),
				limit: String(pageSize)
			});
			if (searchQuery) params.set("q", searchQuery);
			if (filterValues.status) params.set("status", filterValues.status);
			if (filterValues.report_type) params.set("report_type", filterValues.report_type);

			const result = await reportsAdminApiExtended.list(params.toString());
			if (result.ok) {
				const d = result.data as { items: Report[]; page: number; total_pages: number } | Report[];
				if (Array.isArray(d)) {
					reports = d;
					totalItems = d.length;
					totalPages = 1;
				} else {
					reports = d.items ?? [];
					totalItems = d.items?.length ?? 0;
					totalPages = d.total_pages ?? 1;
				}
			} else {
				error = result.error ?? t("common.error", "Failed to load reports");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load reports");
		} finally {
			loading = false;
		}
	}

	function handleSearch(values: Record<string, string>) {
		searchQuery = values.search ?? "";
		filterValues = values;
		currentPage = 0;
	}

	function handleClear() {
		searchQuery = "";
		filterValues = {};
		currentPage = 0;
	}

	function openView(report: Report) {
		viewTarget = report;
		showViewModal = true;
	}

	function confirmDelete(report: Report) {
		deleteTarget = report;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		acting = true;
		try {
			const result = await reportsAdminApiExtended.delete(deleteTarget.id);
			if (result.ok) {
				showDeleteConfirm = false;
				deleteTarget = null;
				loadReports();
			} else {
				error = result.error;
			}
		} catch {
			error = t("common.error", "Failed to delete report");
		} finally {
			acting = false;
		}
	}

	async function bulkDelete() {
		const ids = Array.from(selectedIds);
		if (ids.length === 0) return;
		acting = true;
		try {
			await Promise.all(ids.map((id) => reportsAdminApiExtended.delete(id)));
			selectedIds = new Set();
			loadReports();
		} catch {
			error = t("common.error", "Failed to delete reports");
		} finally {
			acting = false;
		}
	}

	function downloadReport(report: Report) {
		if (report.file_url) {
			window.open(report.file_url, "_blank");
		}
	}

	$effect(() => {
		void currentPage;
		void pageSize;
		void searchQuery;
		void filterValues;
		loadReports();
	});
</script>

<svelte:head>
	<title>{t("reports.title", "Reports")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("reports.title", "Reports")}
	description={t("reports.description", "View and manage generated reports")}
	actions={[
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost", onclick: loadReports }
	]}
/>

{#if error}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{error}
	</div>
{/if}

{#if selectedIds.size > 0}
	<div class="mb-4 flex items-center gap-3 rounded-lg p-3" style="background: var(--color-primary); color: white">
		<span class="text-sm font-medium">{selectedIds.size} selected</span>
		<Button variant="ghost" size="sm" onclick={bulkDelete} disabled={acting}>
			<Trash2 class="mr-1 h-3.5 w-3.5" /> Delete Selected
		</Button>
	</div>
{/if}

<Card>
	<CardHeader>
		<FilterBar
			filters={filterDefs}
			onsearch={handleSearch}
			onclear={handleClear}
		/>
	</CardHeader>
	<CardContent>
		{#if loading}
			<TableSkeleton rows={8} columns={columns.length} />
		{:else}
			<div aria-live="polite">
				<DataTable
					{columns}
					data={reports}
					selectable
					bind:selectedIds
				/>
			</div>
			<Pagination
				bind:page={currentPage}
				{totalPages}
				{totalItems}
				bind:pageSize
			/>
		{/if}
	</CardContent>
</Card>

<!-- View Modal -->
<Modal bind:open={showViewModal} title={t("reports.details", "Report Details")} size="default">
	{#if viewTarget}
		<div class="space-y-4">
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('reports.title_2')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.title}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('reports.type')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.report_type}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('reports.status')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.status}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('reports.generated')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(viewTarget.generated_at).toLocaleString()}</p>
				</div>
			</div>
			{#if viewTarget.file_url}
				<div class="pt-4 border-t" style="border-color: var(--color-border)">
					<Button variant="outline" onclick={() => downloadReport(viewTarget!)}>
						<Download class="mr-2 h-4 w-4" /> Download Report
					</Button>
				</div>
			{/if}
		</div>
	{/if}
	<div class="mt-6 flex justify-end">
		<Button variant="outline" onclick={() => (showViewModal = false)}>{t('reports.close')}</Button>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t("common.confirmDelete", "Confirm Delete")} size="sm">
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			Delete report <strong>{deleteTarget.title}</strong>? This cannot be undone.
		</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteConfirm = false)}>{t('reports.cancel')}</Button>
		<Button variant="destructive" onclick={executeDelete} disabled={acting}>{acting ? "Deleting..." : "Delete"}</Button>
	</div>
</Modal>

<!-- ReportActionsCell component -->
{#snippet ReportActionsCell({ value, row }: { value: any; row: Report })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView(row)}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if row.file_url}
			<Button variant="ghost" size="sm" onclick={() => downloadReport(row)}>
				<Download class="h-3.5 w-3.5" />
			</Button>
		{/if}
		<Button variant="ghost" size="sm" onclick={() => confirmDelete(row)} style="color: var(--color-danger)">
			<Trash2 class="h-3.5 w-3.5" />
		</Button>
	</div>
{/snippet}
