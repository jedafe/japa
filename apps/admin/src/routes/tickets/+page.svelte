<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import type { ActionItem, FilterDef } from "$lib/components/ui/data-table";
	import { SlideOver } from "$lib/components/ui/slide-over";
	import { Badge } from "$lib/components/ui/badge";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { Select } from "$lib/components/ui/select";
	import Pagination from "$lib/components/ui/Pagination.svelte";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { RefreshCw, Eye, Pencil, Trash2, Ticket, Clock, CheckCircle, AlertCircle, MessageSquare, StickyNote } from "lucide-svelte";
	import { ticketsAdminApi, adminCatalogApi, type CatalogOption } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { pageFromUrl, pageSizeFromUrl, searchFromUrl, filterFromUrl, syncListStateToUrl } from "$lib/utils/url-sync.svelte";

	type TicketItem = {
		id: string;
		ticket_number?: number;
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
	let currentPage = $state(pageFromUrl());
	let totalItems = $state(0);
	let totalPages = $state(1);
	let pageSize = $state(pageSizeFromUrl(25));
	let loading = $state(true);
	let error = $state("");
	let searchQuery = $state(searchFromUrl());
	const _initFilterValues: Record<string, string> = {};
	{
		const priority = filterFromUrl("priority");
		if (priority) _initFilterValues.priority = priority;
	}
	let filterValues = $state<Record<string, string>>(_initFilterValues);
	let statusFilter = $state(filterFromUrl("status"));
	let selectedIds = $state<Set<string>>(new Set());

	// View SlideOver
	let showViewSlideOver = $state(false);
	let viewTarget = $state<TicketItem | null>(null);
	let loadingView = $state(false);
	let ticketEvents = $state<any[]>([]);
	let newComment = $state("");
	let newNote = $state("");
	let addingComment = $state(false);
	let addingNote = $state(false);

	// Edit SlideOver
	let showEditSlideOver = $state(false);
	let editingTicket = $state<TicketItem | null>(null);
	let editForm = $state({ subject: "", status: "", priority: "", assigned_to: "" });
	let savingEdit = $state(false);

	// Delete modal
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<TicketItem | null>(null);

	// Per-action RBAC gates matching the backend route middleware.
	// Ref: japaconnect-engineering/sections/38-ticket-system.md
	const canViewAllTickets = $derived(hasPermission("tickets.view_all"));
	const canManageTickets = $derived(hasPermission("tickets.manage"));
	const canDeleteTickets = $derived(hasPermission("tickets.admin"));
	const canCommentInternal = $derived(hasPermission("tickets.comment.internal"));

	let ticketPriorityOptions = $state<CatalogOption[]>([
		{ value: "low", label: t("tickets.low", "Low") },
		{ value: "normal", label: t("tickets.normal", "Normal") },
		{ value: "high", label: t("tickets.high", "High") },
		{ value: "urgent", label: t("tickets.urgent", "Urgent") }
	]);

	let ticketStatusOptions = $state<CatalogOption[]>([
		{ value: "open", label: t("tickets.open", "Open") },
		{ value: "in_progress", label: t("tickets.in_progress", "In Progress") },
		{ value: "waiting_on_user", label: t("tickets.waiting_on_user", "Waiting on User") },
		{ value: "resolved", label: t("tickets.resolved", "Resolved") },
		{ value: "closed", label: t("tickets.closed", "Closed") },
		{ value: "spam", label: t("tickets.spam", "Spam") }
	]);

	$effect(() => {
		void (async () => {
			const [priorityRes, statusRes] = await Promise.all([
				adminCatalogApi.get("tickets.priority"),
				adminCatalogApi.get("tickets.status")
			]);
			if (priorityRes.ok && priorityRes.data?.length) ticketPriorityOptions = priorityRes.data;
			if (statusRes.ok && statusRes.data?.length) ticketStatusOptions = statusRes.data;
		})();
	});

	function getStatusIcon(value: string) {
		switch (value) {
			case 'open': return AlertCircle;
			case 'in_progress': return Clock;
			case 'waiting_on_user': return MessageSquare;
			case 'resolved': return CheckCircle;
			case 'closed': return CheckCircle;
			case 'spam': return AlertCircle;
			default: return Ticket;
		}
	}

	const statusTabs = $derived([
		{ value: "", label: t("tickets.all", "All"), icon: Ticket },
		...ticketStatusOptions.map((o) => ({ ...o, icon: getStatusIcon(o.value) }))
	]);

	const filterDefs = $derived<FilterDef[]>([
		{
			key: "priority",
			label: t("tickets.priority", "Priority"),
			options: ticketPriorityOptions
		}
	]);

	const rowActions: ActionItem[] = [
		{ label: t("common.view", "View"), icon: Eye, onclick: (row: TicketItem) => openView(row) },
		{ label: t("common.edit", "Edit"), icon: Pencil, onclick: (row: TicketItem) => openEdit(row), show: () => canManageTickets },
		{ label: t("common.delete", "Delete"), icon: Trash2, variant: "destructive", onclick: (row: TicketItem) => confirmDelete(row), show: () => canDeleteTickets }
	];

	const columns = [
		{
			key: "ticket_number",
			label: t("tickets.ref", "Ref"),
			render: (v: number, row: TicketItem) => (v ? `JC-${v}` : `#${row.id.slice(0, 8)}`)
		},
		{
			key: "subject",
			label: t("tickets.subject", "Subject"),

		},
		{
			key: "type_name",
			label: t("tickets.type", "Type"),
			
		},
		{
			key: "status",
			label: t("common.status", "Status"),
			
		},
		{
			key: "priority",
			label: t("tickets.priority", "Priority"),
			
		},
		{
			key: "assigned_to",
			label: t("tickets.assignee", "Assignee"),
			render: (v: string) => v || "—"
		},
		{
			key: "created_at",
			label: t("common.created", "Created"),
			render: (v: string) => new Date(v).toLocaleDateString()
		}
	];

	async function openView(ticket: TicketItem) {
		viewTarget = ticket;
		showViewSlideOver = true;
		loadingView = true;
		ticketEvents = [];
		newComment = "";
		newNote = "";
		try {
			const [ticketRes, eventsRes] = await Promise.all([
				ticketsAdminApi.get(ticket.id),
				ticketsAdminApi.getEvents(ticket.id),
			]);
			if (ticketRes.ok) {
				const detail = ticketRes.data as { ticket: TicketItem; comments: any[]; attachments: any[]; events: any[] };
				if (detail?.ticket) viewTarget = detail.ticket;
			}
			if (eventsRes.ok) {
				const ed = eventsRes.data as any;
				ticketEvents = ed?.events ?? ed?.data ?? (Array.isArray(ed) ? ed : []) ?? [];
			}
		} catch {
			viewTarget = ticket;
		} finally {
			loadingView = false;
		}
	}

	async function submitComment() {
		if (!viewTarget || !newComment.trim()) return;
		addingComment = true;
		try {
			const result = await ticketsAdminApi.addComment(viewTarget.id, { body: newComment.trim(), is_internal: false });
			if (result.ok) {
				newComment = "";
				const eventsResult = await ticketsAdminApi.getEvents(viewTarget.id);
				if (eventsResult.ok) {
					const ed = eventsResult.data as any;
					ticketEvents = ed?.events ?? ed?.data ?? (Array.isArray(ed) ? ed : []) ?? [];
				}
			}
		} catch { /* ignore */ } finally {
			addingComment = false;
		}
	}

	async function submitNote() {
		if (!viewTarget || !newNote.trim()) return;
		addingNote = true;
		try {
			const result = await ticketsAdminApi.addNote(viewTarget.id, { body: newNote.trim() });
			if (result.ok) {
				newNote = "";
				const eventsResult = await ticketsAdminApi.getEvents(viewTarget.id);
				if (eventsResult.ok) {
					const ed = eventsResult.data as any;
					ticketEvents = ed?.events ?? ed?.data ?? (Array.isArray(ed) ? ed : []) ?? [];
				}
			}
		} catch { /* ignore */ } finally {
			addingNote = false;
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
		showEditSlideOver = true;
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
				showEditSlideOver = false;
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

	// Sync list state (page, search, filters) to URL for back/forward survival.
	$effect(() => {
		void currentPage;
		void pageSize;
		void searchQuery;
		void statusFilter;
		void filterValues;
		syncListStateToUrl({
			page: currentPage,
			pageSize,
			defaultPageSize: 25,
			search: searchQuery,
			searchParam: "search",
			filters: { ...filterValues, status: statusFilter }
		});
	});

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
	{#each statusTabs as tab (tab)}
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
		<!-- FilterBar replaced by DataTable filters below -->
	</CardHeader>
	<CardContent>
		{#if loading}
			<TableSkeleton rows={8} columns={columns.length} />
		{:else}
			<DataTable {columns} data={tickets} pageSize={10} selectable bind:selectedIds filters={filterDefs} bind:filterValue={filterValues} onfilterapply={() => { currentPage = 0; }} onfilterclear={() => { filterValues = {}; currentPage = 0; }} actions={rowActions}>
				{#snippet bulkActions()}
					<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting}>
						<Trash2 class="mr-1 h-3.5 w-3.5" />
						{bulkDeleting ? t("common.deleting", "Deleting...") : `${t("tickets.delete_selected", "Delete Selected")} (${selectedIds.size})`}
					</Button>
				{/snippet}
			</DataTable>
			<Pagination bind:page={currentPage} {totalPages} {totalItems} bind:pageSize
			pageSizeOptions={[10, 25, 50, 100]} />
		{/if}
	</CardContent>
</Card>

<!-- View SlideOver -->
<SlideOver bind:open={showViewSlideOver} title={t("tickets.details", "Ticket Details")}>
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
			{#if viewTarget.description}
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("tickets.description", "Description")}</p>
					<p class="mt-1 text-sm whitespace-pre-wrap" style="color: var(--color-foreground)">{viewTarget.description}</p>
				</div>
			{/if}

			<!-- Events timeline (comments + notes) -->
			{#if ticketEvents.length > 0}
				<div>
					<p class="mb-2 text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("tickets.activity", "Activity")}</p>
					<div class="max-h-48 space-y-2 overflow-y-auto rounded-lg border p-3" style="border-color: var(--color-border); background: var(--color-surface)">
						{#each ticketEvents as ev (ev)}
							<div class="flex gap-2 text-xs">
								{#if ev.event_type === 'comment_added'}
									<MessageSquare class="h-3.5 w-3.5 shrink-0 mt-0.5" style="color: var(--color-primary)" />
								{:else if ev.event_type === 'sla_breached'}
									<AlertCircle class="h-3.5 w-3.5 shrink-0 mt-0.5" style="color: var(--color-danger)" />
								{:else if ev.event_type === 'status_changed'}
									<Clock class="h-3.5 w-3.5 shrink-0 mt-0.5" style="color: var(--color-warning)" />
								{:else}
									<StickyNote class="h-3.5 w-3.5 shrink-0 mt-0.5" style="color: var(--color-muted)" />
								{/if}
								<div class="flex-1">
									<span class="font-medium" style="color: var(--color-foreground)">{ev.event_type}</span>
									{#if ev.old_value || ev.new_value}<span style="color: var(--color-muted)"> — {ev.old_value ?? ''} → {ev.new_value ?? ''}</span>{/if}
									<span class="block" style="color: var(--color-muted)">{ev.created_at ? new Date(ev.created_at).toLocaleString() : ''}</span>
								</div>
							</div>
						{/each}
					</div>
				</div>
			{/if}

			<!-- Add comment -->
			{#if canCommentInternal}
				<div>
					<label for="tk-comment" class="mb-1 block text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("tickets.add_comment", "Add Comment")}</label>
					<div class="flex gap-2">
						<input id="tk-comment" type="text" bind:value={newComment} class="flex-1 rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder={t("tickets.comment_placeholder", "Type a comment...")} onkeydown={(e) => { if (e.key === 'Enter') submitComment(); }} />
						<Button size="sm" onclick={submitComment} disabled={!newComment.trim() || addingComment}>{addingComment ? t("common.saving", "Saving...") : t("common.post", "Post")}</Button>
					</div>
				</div>
				<div>
					<label for="tk-note" class="mb-1 block text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("tickets.add_note", "Add Internal Note")}</label>
					<div class="flex gap-2">
						<input id="tk-note" type="text" bind:value={newNote} class="flex-1 rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder={t("tickets.note_placeholder", "Internal note (not visible to user)...")} onkeydown={(e) => { if (e.key === 'Enter') submitNote(); }} />
						<Button size="sm" variant="outline" onclick={submitNote} disabled={!newNote.trim() || addingNote}>{addingNote ? t("common.saving", "Saving...") : t("common.post", "Post")}</Button>
					</div>
				</div>
			{/if}
		</div>
	{/if}
	{#snippet footer()}
		<div class="mt-6 flex justify-end">
			<Button variant="outline" onclick={() => (showViewSlideOver = false)}>{t("common.close", "Close")}</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- Edit SlideOver -->
<SlideOver bind:open={showEditSlideOver} title={t("tickets.edit", "Edit Ticket")}>
	<div class="space-y-4">
		<div>
			<label for="tk-subject" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("tickets.subject", "Subject")}</label>
			<input id="tk-subject" type="text" bind:value={editForm.subject} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" />
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="tk-status" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("common.status", "Status")}</label>
				<Select
					options={ticketStatusOptions}
					value={editForm.status}
					onchange={(val) => (editForm.status = val)}
				/>
			</div>
			<div>
				<label for="tk-priority" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("tickets.priority", "Priority")}</label>
				<Select
					options={ticketPriorityOptions}
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
	{#snippet footer()}
		<div class="mt-4 flex justify-end gap-2">
			<Button variant="ghost" onclick={() => (showEditSlideOver = false)}>{t("common.cancel", "Cancel")}</Button>
			<Button onclick={submitEdit} disabled={savingEdit}>{savingEdit ? t("common.saving", "Saving...") : t("common.save", "Save")}</Button>
		</div>
	{/snippet}
</SlideOver>

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
