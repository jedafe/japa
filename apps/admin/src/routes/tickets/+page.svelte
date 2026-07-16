<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Badge } from "$lib/components/ui/badge";
	import { Button } from "$lib/components/ui/button";
	import Pagination from "$lib/components/ui/Pagination.svelte";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { RefreshCw, Eye, Ticket, Clock, CheckCircle, AlertCircle } from "lucide-svelte";
	import { ticketsAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	type TicketItem = {
		id: string;
		subject: string;
		type: string;
		status: string;
		priority: string;
		assignee?: string;
		created_at: string;
	};

	let tickets = $state<TicketItem[]>([]);
	let currentPage = $state(0);
	let totalItems = $state(0);
	let pageSize = $state(25);
	let loading = $state(true);
	let error = $state("");
	let searchQuery = $state("");
	let filterValues = $state<Record<string, string>>({});
	let statusFilter = $state("");

	const statusTabs = [
		{ value: "", label: t("tickets.all", "All"), icon: Ticket },
		{ value: "open", label: t("tickets.open", "Open"), icon: AlertCircle },
		{ value: "in_progress", label: t("tickets.in_progress", "In Progress"), icon: Clock },
		{ value: "resolved", label: t("tickets.resolved", "Resolved"), icon: CheckCircle }
	];

	const filterDefs = [
		{
			key: "type",
			label: t("tickets.type", "Type"),
			options: [
				{ value: "bug", label: "Bug" },
				{ value: "feature", label: "Feature" },
				{ value: "question", label: "Question" },
				{ value: "account", label: "Account" }
			]
		},
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
			key: "type",
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
			key: "assignee",
			label: t("tickets.assignee", "Assignee"),
			render: (v: string) => v || "—"
		},
		{
			key: "created_at",
			label: t("common.created", "Created"),
			sortable: true,
			render: (v: string) => new Date(v).toLocaleDateString()
		},
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-32",
			render: (_: any, row: TicketItem) => `
				<a href="/tickets/${row.id}" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100">${t("common.view", "View")}</a>`
		}
	];

	async function loadTickets() {
		loading = true;
		error = "";
		try {
			const params = new URLSearchParams({
				page: String(currentPage + 1),
				limit: String(pageSize)
			});
			if (searchQuery) params.set("q", searchQuery);
			if (statusFilter) params.set("status", statusFilter);
			if (filterValues.type) params.set("type", filterValues.type);
			if (filterValues.priority) params.set("priority", filterValues.priority);

			const result = await ticketsAdminApi.list(params.toString());
			if (result.ok) {
				const d = result.data as { tickets: TicketItem[]; total: number; page: number; total_pages: number };
				tickets = d.tickets ?? [];
				totalItems = d.total ?? 0;
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
			<DataTable {columns} data={tickets} pageSize={tickets.length} selectable />
			<Pagination bind:page={currentPage} totalPages={Math.ceil(totalItems / pageSize)} {totalItems} bind:pageSize />
		{/if}
	</CardContent>
</Card>
