<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import Pagination from "$lib/components/ui/Pagination.svelte";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { RefreshCw, Check, X, Eye, AlertTriangle, Trash2 } from "lucide-svelte";
	import { moderationAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	type ModItem = {
		id: string;
		type: "user_report" | "ai_flag";
		content_type: string;
		content_id: string;
		reporter_name?: string;
		reason: string;
		status: string;
		created_at: string;
		content_preview?: string;
	};

	let items = $state<ModItem[]>([]);
	let currentPage = $state(0);
	let totalItems = $state(0);
	let pageSize = $state(25);
	let totalPages = $state(1);
	let loading = $state(true);
	let error = $state("");
	let searchQuery = $state("");
	let filterValues = $state<Record<string, string>>({});

	// Modals
	let showViewModal = $state(false);
	let viewTarget = $state<ModItem | null>(null);
	let showActionModal = $state(false);
	let actionItem = $state<ModItem | null>(null);
	let actionType = $state<"approve" | "reject" | "delete">("approve");
	let actionReason = $state("");
	let acting = $state(false);

	// Bulk selection
	let selectedIds = $state<Set<string>>(new Set());

	const canModerate = $derived(hasPermission("admin.moderation"));

	const filterDefs = [
		{
			key: "type",
			label: t("moderation.source", "Source"),
			options: [
				{ value: "user_report", label: "User Report" },
				{ value: "ai_flag", label: "AI Flag" }
			]
		},
		{
			key: "content_type",
			label: t("moderation.content_type", "Content Type"),
			options: [
				{ value: "post", label: "Post" },
				{ value: "comment", label: "Comment" },
				{ value: "blog", label: "Blog" },
				{ value: "forum", label: "Forum" },
				{ value: "message", label: "Message" }
			]
		},
		{
			key: "status",
			label: t("common.status", "Status"),
			options: [
				{ value: "pending", label: "Pending" },
				{ value: "reviewed", label: "Reviewed" },
				{ value: "resolved", label: "Resolved" },
				{ value: "dismissed", label: "Dismissed" }
			]
		}
	];

	const columns = [
		{
			key: "type",
			label: t("moderation.source", "Source"),
			sortable: true,
			render: (v: string) => {
				const colors = v === "ai_flag" ? "bg-purple-500/15 text-purple-700 dark:text-purple-400" : "bg-blue-500/15 text-info dark:text-blue-400";
				return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors}">${v === "ai_flag" ? "AI Flag" : "User Report"}</span>`;
			}
		},
		{
			key: "content_type",
			label: t("moderation.content_type", "Content Type"),
			sortable: true
		},
		{
			key: "reason",
			label: t("moderation.reason", "Reason"),
			sortable: true
		},
		{
			key: "status",
			label: t("common.status", "Status"),
			sortable: true,
			render: (v: string) => {
				const colors = v === "pending" ? "bg-warning/15 text-warning dark:text-warning" : v === "resolved" ? "bg-success/15 text-success dark:text-success" : v === "dismissed" ? "bg-muted/15 text-muted dark:text-muted" : "bg-blue-500/15 text-info dark:text-blue-400";
				return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors}">${v}</span>`;
			}
		},
		{
			key: "reporter_name",
			label: t("moderation.reporter", "Reporter"),
			render: (v: string) => v || "—"
		},
		{
			key: "created_at",
			label: t("common.created", "Created"),
			sortable: true,
			render: (v: string) => new Date(v).toLocaleDateString()
		},
		{
			key: "actions",
			label: t("common.actions", "Actions"),
			class: "w-40",
			component: ModActionsCell
		}
	];

	async function loadItems() {
		loading = true;
		error = "";
		try {
			const params = new URLSearchParams({
				page: String(currentPage),
				limit: String(pageSize)
			});
			if (searchQuery) params.set("q", searchQuery);
			if (filterValues.type) params.set("type", filterValues.type);
			if (filterValues.content_type) params.set("content_type", filterValues.content_type);
			if (filterValues.status) params.set("status", filterValues.status);

			const result = await moderationAdminApi.getQueue(params.toString());
			if (result.ok) {
				const d = result.data as { items: ModItem[]; total: number; page: number; total_pages: number };
				items = d.items ?? [];
				totalItems = d.total ?? items.length;
				totalPages = d.total_pages ?? 1;
			} else {
				error = result.error ?? t("common.error", "Failed to load moderation queue");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load moderation queue");
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

	function openView(item: ModItem) {
		viewTarget = item;
		showViewModal = true;
	}

	function openAction(item: ModItem, action: "approve" | "reject" | "delete") {
		actionItem = item;
		actionType = action;
		actionReason = "";
		showActionModal = true;
	}

	async function executeAction() {
		if (!actionItem) return;
		acting = true;
		try {
			const body: Record<string, unknown> = { action: actionType };
			if (actionReason.trim()) body.reason = actionReason.trim();
			const result = await moderationAdminApi.updateItem(actionItem.id, body);
			if (result.ok) {
				showActionModal = false;
				loadItems();
			} else {
				error = result.error ?? t("common.error", "Action failed");
			}
		} catch {
			error = t("common.error", "Action failed");
		} finally {
			acting = false;
		}
	}

	async function bulkAction(action: "approve" | "reject") {
		const ids = Array.from(selectedIds);
		if (ids.length === 0) return;
		acting = true;
		try {
			await moderationAdminApi.bulkAction({ ids, action });
			selectedIds = new Set();
			loadItems();
		} catch {
			error = t("common.error", "Bulk action failed");
		} finally {
			acting = false;
		}
	}

	$effect(() => {
		void currentPage;
		void pageSize;
		void searchQuery;
		void filterValues;
		loadItems();
	});
</script>

<svelte:head>
	<title>{t("moderation.title", "Moderation")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("moderation.title", "Moderation")}
	description={t("moderation.description", "Review and moderate platform content")}
	actions={[
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost", onclick: loadItems }
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
		<Button variant="ghost" size="sm" onclick={() => bulkAction("approve")} disabled={acting}>
			<Check class="mr-1 h-3.5 w-3.5" /> {t("moderation.approve", "Approve")}
		</Button>
		<Button variant="ghost" size="sm" onclick={() => bulkAction("reject")} disabled={acting}>
			<X class="mr-1 h-3.5 w-3.5" /> {t("moderation.reject", "Reject")}
		</Button>
	</div>
{/if}

<Card>
	<CardHeader>
		<FilterBar filters={filterDefs} onsearch={handleSearch} onclear={handleClear} />
	</CardHeader>
	<CardContent>
		{#if loading}
			<TableSkeleton rows={8} columns={columns.length} />
		{:else}
			<div aria-live="polite">
				<DataTable
					{columns}
					data={items}
					selectable={canModerate}
					bind:selectedIds
				/>
			</div>
			<Pagination bind:page={currentPage} {totalPages} {totalItems} bind:pageSize />
		{/if}
	</CardContent>
</Card>

<!-- View Modal -->
<Modal bind:open={showViewModal} title={t("moderation.details", "Moderation Details")} size="default">
	{#if viewTarget}
		<div class="space-y-4">
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('moderation.source')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.type === "ai_flag" ? "AI Flag" : "User Report"}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('moderation.content_type')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.content_type}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('moderation.status')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.status}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('moderation.reporter')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.reporter_name || "—"}</p>
				</div>
				<div class="col-span-2">
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('moderation.reason')}</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">{viewTarget.reason}</p>
				</div>
				{#if viewTarget.content_preview}
					<div class="col-span-2">
						<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('moderation.content_preview')}</p>
						<p class="mt-1 text-sm rounded-lg p-3" style="background: var(--color-surface); color: var(--color-foreground)">{viewTarget.content_preview}</p>
					</div>
				{/if}
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('moderation.created')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(viewTarget.created_at).toLocaleString()}</p>
				</div>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		{#if viewTarget && canModerate}
			<Button variant="outline" onclick={() => { showViewModal = false; openAction(viewTarget!, "approve"); }}>
				<Check class="mr-1 h-3.5 w-3.5" /> Approve
			</Button>
			<Button variant="destructive" onclick={() => { showViewModal = false; openAction(viewTarget!, "reject"); }}>
				<X class="mr-1 h-3.5 w-3.5" /> Reject
			</Button>
		{/if}
		<Button variant="outline" onclick={() => (showViewModal = false)}>{t('moderation.close')}</Button>
	</div>
</Modal>

<!-- Action Modal -->
<Modal bind:open={showActionModal} title={actionType === "approve" ? t("moderation.approve_title", "Approve Content") : actionType === "reject" ? t("moderation.reject_title", "Reject Content") : t("moderation.delete_title", "Delete Content")}>
	{#if actionItem}
		<div class="mb-3 rounded-lg p-3 text-sm" style="background: var(--color-surface); border: 1px solid var(--color-border)">
			<p class="font-medium" style="color: var(--color-foreground)">{actionItem.content_type}</p>
			<p style="color: var(--color-muted)">{actionItem.reason}</p>
		</div>
	{/if}
	{#if actionType !== "approve"}
		<textarea
			class="w-full rounded-lg border p-3 text-sm"
			style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
			rows="3"
			placeholder={t("moderation.action_reason", "Reason (optional)...")}
			bind:value={actionReason}
		></textarea>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showActionModal = false}>{t("common.cancel", "Cancel")}</Button>
		<Button variant={actionType === "approve" ? "default" : "destructive"} onclick={executeAction} disabled={acting}>
			{acting ? "Processing..." : actionType === "approve" ? t("moderation.approve", "Approve") : actionType === "reject" ? t("moderation.reject", "Reject") : t("common.delete", "Delete")}
		</Button>
	</div>
</Modal>

<!-- ModActionsCell component -->
{#snippet ModActionsCell(item: ModItem)}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView(item)}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canModerate}
			{#if item.status === "pending"}
				<Button variant="ghost" size="sm" onclick={() => openAction(item, "approve")} style="color: var(--color-success)">
					<Check class="h-3.5 w-3.5" />
				</Button>
				<Button variant="ghost" size="sm" onclick={() => openAction(item, "reject")} style="color: var(--color-danger)">
					<X class="h-3.5 w-3.5" />
				</Button>
			{/if}
			<Button variant="ghost" size="sm" onclick={() => openAction(item, "delete")} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
