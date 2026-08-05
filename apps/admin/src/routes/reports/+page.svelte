<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import Pagination from "$lib/components/ui/Pagination.svelte";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { RefreshCw, Eye, Trash2 } from "lucide-svelte";
	import { reportsAdminApiExtended } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";

	// ContentReport fields returned by GET /admin/reports
	// (internal/reports — content moderation reports, not generated analytics)
	type Report = {
		id: string;
		reporter_user_id: string;
		reporter_username?: string;
		reporter_display_name?: string;
		entity_type: string;
		entity_id: string;
		reason: string;
		notes?: string;
		evidence_urls?: string[];
		status: string;
		reviewed_by?: string;
		resolution_note?: string;
		resolved_at?: string;
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
				{ value: "pending", label: t("reports.status_pending", "Pending") },
				{ value: "reviewing", label: t("reports.status_reviewing", "Reviewing") },
				{ value: "actioned", label: t("reports.status_actioned", "Actioned") },
				{ value: "dismissed", label: t("reports.status_dismissed", "Dismissed") }
			]
		},
		{
			key: "entity_type",
			label: t("reports.type", "Type"),
			options: [
				{ value: "content", label: "Content" },
				{ value: "comment", label: "Comment" },
				{ value: "user", label: "User" },
				{ value: "space", label: "Space" }
			]
		}
	];

	function statusBadgeClass(status: string): string {
		switch (status) {
			case "pending": return "bg-warning/15 text-warning dark:text-warning";
			case "reviewing": return "bg-info/15 text-info dark:text-info";
			case "actioned": return "bg-success/15 text-success dark:text-success";
			case "dismissed": return "bg-muted/15 text-muted dark:text-muted";
			default: return "bg-muted/15 text-muted dark:text-muted";
		}
	}

	const columns = [
		{
			key: "reporter_display_name",
			label: t("reports.reporter", "Reporter"),
			sortable: true,
			render: (v: string, row: Report) => `<span class="text-sm">${v || row.reporter_username || 'anonymous'}</span>`
		},
		{
			key: "entity_type",
			label: t("reports.target", "Target"),
			sortable: true,
			render: (_: any, row: Report) =>
				`<span class="text-xs" style="color: var(--color-muted)">${row.entity_type} &middot; ${(row.entity_id ?? "").slice(0, 10)}</span>`
		},
		{
			key: "reason",
			label: t("reports.reason", "Reason"),
			render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v || "—"}</span>`
		},
		{
			key: "status",
			label: t("reports.status", "Status"),
			sortable: true,
			render: (v: string) =>
				`<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${statusBadgeClass(v)}">${v}</span>`
		},
		{
			key: "created_at",
			label: t("reports.created", "Created"),
			sortable: true,
			render: (v: string) => {
				if (!v) return `<span class="text-xs" style="color: var(--color-muted)">—</span>`;
				const d = new Date(v);
				return isNaN(d.getTime())
					? `<span class="text-xs" style="color: var(--color-muted)">—</span>`
					: `<span class="text-xs" style="color: var(--color-muted)">${d.toLocaleString()}</span>`;
			}
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
			if (filterValues.entity_type) params.set("entity_type", filterValues.entity_type);

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
				error = result.error ?? t("reports.error", "Failed to load reports");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("reports.error", "Failed to load reports");
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
			error = t("reports.error", "Failed to delete report");
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
			error = t("reports.error", "Failed to delete reports");
		} finally {
			acting = false;
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
	description={t("reports.description", "View and manage reported content")}
	actions={[
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost" as const, onclick: loadReports }
	]}
/>

{#if error}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{error}
	</div>
{/if}

{#if selectedIds.size > 0}
	<div class="mb-4 flex items-center gap-3 rounded-lg p-3" style="background: var(--color-primary); color: white">
		<span class="text-sm font-medium">{selectedIds.size} {t("reports.selected", "selected")}</span>
		<Button variant="ghost" size="sm" onclick={bulkDelete} disabled={acting}>
			<Trash2 class="mr-1 h-3.5 w-3.5" /> {t("reports.delete_selected", "Delete Selected")}
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
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("reports.reporter", "Reporter")}</p>
					<p class="mt-1 text-sm font-medium font-mono" style="color: var(--color-foreground)">{viewTarget.reporter_user_id}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("reports.target", "Target")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.entity_type} &middot; {viewTarget.entity_id}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("reports.reason", "Reason")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.reason || "—"}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("reports.status", "Status")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.status}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("reports.created", "Created")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.created_at ? new Date(viewTarget.created_at).toLocaleString() : "—"}</p>
				</div>
				{#if viewTarget.notes}
					<div>
						<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("reports.notes", "Notes")}</p>
						<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.notes}</p>
					</div>
				{/if}
				{#if viewTarget.resolution_note}
					<div>
						<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("reports.resolution", "Resolution")}</p>
						<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.resolution_note}</p>
					</div>
				{/if}
			</div>
			{#if viewTarget.evidence_urls && viewTarget.evidence_urls.length > 0}
				<div class="pt-4 border-t" style="border-color: var(--color-border)">
					<p class="text-xs font-medium uppercase tracking-wider mb-2" style="color: var(--color-muted)">{t("reports.evidence", "Evidence")}</p>
					<ul class="space-y-1">
						{#each viewTarget.evidence_urls as url}
							<li><a href={url} target="_blank" rel="noopener" class="text-sm text-primary underline-offset-4 hover:underline">{url}</a></li>
						{/each}
					</ul>
				</div>
			{/if}
		</div>
	{/if}
	<div class="mt-6 flex justify-end">
		<Button variant="outline" onclick={() => (showViewModal = false)}>{t('reports.close', 'Close')}</Button>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t("common.confirmDelete", "Confirm Delete")} size="sm">
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("reports.confirm_delete", "Delete report")} <strong>{deleteTarget.entity_type} &middot; {deleteTarget.entity_id}</strong>? {t("reports.cannot_undo", "This cannot be undone.")}
		</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteConfirm = false)}>{t('reports.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={executeDelete} disabled={acting}>{acting ? t("reports.deleting", "Deleting...") : t("reports.delete", "Delete")}</Button>
	</div>
</Modal>

<!-- ReportActionsCell component -->
{#snippet ReportActionsCell({ value, row }: { value: any; row: Report })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView(row)}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		<Button variant="ghost" size="sm" onclick={() => confirmDelete(row)} style="color: var(--color-danger)">
			<Trash2 class="h-3.5 w-3.5" />
		</Button>
	</div>
{/snippet}
