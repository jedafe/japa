<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Badge } from "$lib/components/ui/badge";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { Select } from "$lib/components/ui/select";
	import Pagination from "$lib/components/ui/Pagination.svelte";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { RefreshCw, Eye, Pencil, Trash2, Ticket, Clock, CheckCircle, AlertCircle } from "lucide-svelte";
	import { ticketsAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	type TicketItem = {
		id: string;
		subject: string;
		type_name: string;
		type_slug: string;
		type_id: string;
		status: string;
		priority: string;
		assigned_to?: string;
		assigned_group_id?: string;
		submitter_user_id?: string;
		submitter_email?: string;
		description: string;
		resolution_note?: string;
		created_at: string;
		updated_at: string;
	};

	let tickets = $state<TicketItem[]>([]);
	let currentPage = $state(0);
	let totalItems = $state(0);
	let totalPages = $state(1);
	let pageSize = $state(25);
	let loading = $state(true);
	let error = $state("");
	let searchQuery = $state("");
	let filterValues = $state<Record<string, string>>({});
	let statusFilter = $state("");
	let selectedIds = $state<Set<string>>(new Set());

	// View modal
	let showViewModal = $state(false);
	let viewTarget = $state<TicketItem | null>(null);
	let loadingView = $state(false);

	// Edit modal
	let showEditModal = $state(false);
	let editingTicket = $state<TicketItem | null>(null);
	let editForm = $state({ subject: "", status: "", priority: "", assigned_to: "" });
	let savingEdit = $state(false);

	// Delete modal
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<TicketItem | null>(null);

	// Backend gates ticket mutations on tickets.admin
	const canManageTickets = $derived(hasPermission("tickets.admin"));

	const statusTabs = [
		{ value: "", label: t("tickets.all", "All"), icon: Ticket },
		{ value: "open", label: t("tickets.open", "Open"), icon: AlertCircle },
		{ value: "in_progress", label: t("tickets.in_progress", "In Progress"), icon: Clock },
		{ value: "resolved", label: t("tickets.resolved", "Resolved"), icon: CheckCircle }
	];

	const filterDefs = [
		{
			key: "priority",
			label: t("tickets.priority", "Priority"),
			options: [
				{ value: "low", label: "Low" },
				{ value: "medium", label: "Medium" },
				{ value: "high", label: "High" },
				{ value: "urgent", label: "Urgent" }
			]
		}
	];

	const columns = [
		{
			key: "subject",
			label: t("tickets.subject", "Subject"),
			sortable: true
		},
		{
			key: "type_name",
			label: t("tickets.type", "Type"),
			sortable: true
		},
		{
			key: "status",
			label: t("common.status", "Status"),
			sortable: true
		},
		{
			key: "priority",
			label: t("tickets.priority", "Priority"),
			sortable: true
		},
		{
			key: "assigned_to",
			label: t("tickets.assignee", "Assignee"),
			render: (v: string) => v || "—"
		},
		{
			key: "created_at",
			label: t("common.created", "Created"),
			sortable: true,
			render: (v: string) => new Date(v).toLocaleDateString()
		},
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-32", component: TicketActionsCell }
	];

	async function openView(ticket: TicketItem) {
		viewTarget = ticket;
		showViewModal = true;
		loadingView = true;
		try {
			const result = await ticketsAdminApi.get(ticket.id);
			if (result.ok) viewTarget = (result.data as TicketItem) ?? ticket;
		} catch {
			viewTarget = ticket;
		} finally {
			loadingView = false;
		}
	}

	function openEdit(ticket: TicketItem) {
		editingTicket = ticket;
		editForm = {
			subject: ticket.subject,
			status: ticket.status,
			priority: ticket.priority,
			assigned_to: ticket.assigned_to ?? ""
		};
		showEditModal = true;
	}

	async function submitEdit() {
		if (!editingTicket) return;
		savingEdit = true;
		error = "";
		try {
			const result = await ticketsAdminApi.update(editingTicket.id, {
				subject: editForm.subject,
				status: editForm.status,
				priority: editForm.priority,
				assigned_to: editForm.assigned_to
			});
			if (result.ok) {
				showEditModal = false;
				loadTickets();
			} else {
				error = result.error ?? t("common.error", "Failed to update ticket");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to update ticket");
		} finally {
			savingEdit = false;
		}
	}

	function confirmDelete(ticket: TicketItem) {
		deleteTarget = ticket;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		error = "";
		try {
			const result = await ticketsAdminApi.delete(deleteTarget.id);
			if (result.ok) {
				showDeleteConfirm = false;
				deleteTarget = null;
				loadTickets();
			} else {
				error = result.error ?? t("common.error", "Failed to delete ticket");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to delete ticket");
		}
	}

	let bulkDeleting = $state(false);
	async function bulkDelete() {
		const ids = Array.from(selectedIds);
		if (ids.length === 0) return;
		bulkDeleting = true;
		error = "";
		try {
			await Promise.all(ids.map((id) => ticketsAdminApi.delete(id)));
			selectedIds = new Set();
			loadTickets();
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to delete tickets");
		} finally {
			bulkDeleting = false;
		}
	}

	async function loadTickets() {
		loading = true;
		error = "";
		try {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(currentPage * pageSize)
			});
			if (searchQuery) params.set("search", searchQuery);
			if (statusFilter) params.set("status", statusFilter);
			if (filterValues.priority) params.set("priority", filterValues.priority);

			const result = await ticketsAdminApi.list(params.toString());
			if (result.ok) {
				const d = result.data as { tickets: TicketItem[]; total: number; page: number; total_pages: number };
				tickets = d.tickets ?? [];
				totalItems = d.total ?? 0;
				totalPages = d.total_pages >= 0 ? d.total_pages : 1;
			} else {
				error = result.error ?? t("common.error", "Failed to load tickets");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load tickets");
		} finally {
			loading = false;
		}
	}

	function handleStatusTab(tab: string) {
		statusFilter = tab;
		currentPage = 0;
		loadTickets();
	}

	function handleSearch(values: Record<string, string>) {
		searchQuery = values.search ?? "";
		filterValues = values;
		currentPage = 0;
		loadTickets();
	}

	function handleClear() {
		searchQuery = "";
		filterValues = {};
		currentPage = 0;
		loadTickets();
	}

	$effect(() => { loadTickets(); });
</script>

<svelte:head>
	<title>{t("tickets.title", "Support Tickets")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("tickets.title", "Support Tickets")}
	description={t("tickets.description", "Manage user support requests")}
	actions={[
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost", onclick: loadTickets }
	]}
/>

{#if error}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{error}
	</div>
{/if}

<!-- Status tabs (manual, not using broken Tabs component for content panels) -->
<div class="mb-4 flex gap-1 rounded-lg p-1" style="background: var(--color-surface)">
	{#each statusTabs as tab}
		{@const Icon = tab.icon}
		<button
			class="inline-flex items-center whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition-all"
			style={statusFilter === tab.value
				? "background: var(--color-background); color: var(--color-foreground); box-shadow: 0 1px 2px rgba(0,0,0,0.05)"
				: "color: var(--color-muted)"}
			onclick={() => handleStatusTab(tab.value)}
		>
			<Icon class="mr-2 h-4 w-4" />
			{tab.label}
		</button>
	{/each}
</div>

<Card>
	<CardHeader>
		<FilterBar filters={filterDefs} onsearch={handleSearch} onclear={handleClear} />
	</CardHeader>
	<CardContent>
		{#if loading}
			<TableSkeleton rows={8} columns={columns.length} />
		{:else}
			<DataTable {columns} data={tickets} pageSize={10} selectable bind:selectedIds>
				{#snippet bulkActions()}
					<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting}>
						<Trash2 class="mr-1 h-3.5 w-3.5" />
						{bulkDeleting ? t("common.deleting", "Deleting...") : `${t("tickets.delete_selected", "Delete Selected")} (${selectedIds.size})`}
					</Button>
				{/snippet}
			</DataTable>
			<Pagination bind:page={currentPage} {totalPages} {totalItems} bind:pageSize />
		{/if}
	</CardContent>
</Card>

<!-- View Modal -->
<Modal bind:open={showViewModal} title={t("tickets.details", "Ticket Details")} size="sm">
	{#if loadingView}
		<p class="text-sm" style="color: var(--color-muted)">{t("common.loading", "Loading...")}</p>
	{:else if viewTarget}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("tickets.subject", "Subject")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.subject}</p>
			</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("tickets.type", "Type")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.type_name}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("tickets.priority", "Priority")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.priority}</p>
			</div>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("common.status", "Status")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.status}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("tickets.assignee", "Assignee")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.assigned_to || "—"}</p>
			</div>
		</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("common.created", "Created")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(viewTarget.created_at).toLocaleString()}</p>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end">
		<Button variant="outline" onclick={() => (showViewModal = false)}>{t("common.close", "Close")}</Button>
	</div>
</Modal>

<!-- Edit Modal -->
<Modal bind:open={showEditModal} title={t("tickets.edit", "Edit Ticket")}>
	<div class="space-y-4">
		<div>
			<label for="tk-subject" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("tickets.subject", "Subject")}</label>
			<input id="tk-subject" type="text" bind:value={editForm.subject} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" />
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="tk-status" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("common.status", "Status")}</label>
				<Select
					options={[
						{ value: "open", label: t("tickets.open", "Open") },
						{ value: "in_progress", label: t("tickets.in_progress", "In Progress") },
						{ value: "resolved", label: t("tickets.resolved", "Resolved") },
						{ value: "closed", label: t("tickets.closed", "Closed") }
					]}
					value={editForm.status}
					onchange={(val) => (editForm.status = val)}
				/>
			</div>
			<div>
				<label for="tk-priority" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("tickets.priority", "Priority")}</label>
				<Select
					options={[
						{ value: "low", label: t("tickets.low", "Low") },
						{ value: "medium", label: t("tickets.medium", "Medium") },
						{ value: "high", label: t("tickets.high", "High") },
						{ value: "urgent", label: t("tickets.urgent", "Urgent") }
					]}
					value={editForm.priority}
					onchange={(val) => (editForm.priority = val)}
				/>
			</div>
		</div>
		<div>
			<label for="tk-assignee" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("tickets.assignee", "Assignee")}</label>
			<input id="tk-assignee" type="text" bind:value={editForm.assigned_to} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" />
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showEditModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={submitEdit} disabled={savingEdit}>{savingEdit ? t("common.saving", "Saving...") : t("common.save", "Save")}</Button>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t("common.confirmDelete", "Confirm Delete")}>
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("tickets.delete_confirm", "Delete ticket")} <strong>{deleteTarget.subject}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">{t("common.cannotUndo", "This action cannot be undone.")}</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteConfirm = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={executeDelete}>{t("common.delete", "Delete")}</Button>
	</div>
</Modal>

<!-- TicketActionsCell -->
{#snippet TicketActionsCell({ value, row }: { value: any; row: TicketItem })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView(row)}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canManageTickets}
			<Button variant="ghost" size="sm" onclick={() => openEdit(row)}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => confirmDelete(row)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
