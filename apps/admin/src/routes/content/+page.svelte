<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Badge } from "$lib/components/ui/badge";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import Pagination from "$lib/components/ui/Pagination.svelte";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { RefreshCw, Eye, RotateCcw, Trash2 } from "lucide-svelte";
	import { contentAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	type ContentItem = {
		id: string;
		title: string;
		content_type: string;
		status: string;
		author_name: string;
		space_name?: string;
		created_at: string;
		removed_at?: string;
		views?: number;
		url?: string;
	};

	let items = $state<ContentItem[]>([]);
	let currentPage = $state(0);
	let totalItems = $state(0);
	let pageSize = $state(25);
	let loading = $state(true);
	let error = $state("");
	let success = $state("");
	let searchQuery = $state("");
	let filterValues = $state<Record<string, string>>({});

	// Remove modal
	let showRemoveModal = $state(false);
	let removeTarget = $state<ContentItem | null>(null);
	let removeReason = $state("");

	// Restore modal
	let showRestoreModal = $state(false);
	let restoreTarget = $state<ContentItem | null>(null);

	const filterDefs = [
		{
			key: "content_type",
			label: t("content.type", "Type"),
			options: [
				{ value: "post", label: t("content.type.post", "Post") },
				{ value: "comment", label: t("content.type.comment", "Comment") },
				{ value: "blog", label: t("content.type.blog", "Blog") },
				{ value: "forum", label: t("content.type.forum", "Forum") }
			]
		},
		{
			key: "status",
			label: t("common.status", "Status"),
			options: [
				{ value: "published", label: t("content.status.published", "Published") },
				{ value: "draft", label: t("content.status.draft", "Draft") },
				{ value: "removed", label: t("content.status.removed", "Removed") },
				{ value: "archived", label: t("content.status.archived", "Archived") }
			]
		}
	];

	const columns = [
		{
			key: "title",
			label: t("content.title", "Title"),
			sortable: true
		},
		{
			key: "content_type",
			label: t("content.type", "Type"),
			sortable: true
		},
		{
			key: "status",
			label: t("common.status", "Status"),
			sortable: true
		},
		{
			key: "author_name",
			label: t("content.author", "Author"),
			sortable: true
		},
		{
			key: "space_name",
			label: t("content.space", "Space"),
			render: (v: string) => v || "—"
		},
		{
			key: "views",
			label: t("content.views", "Views"),
			sortable: true,
			render: (v: number) => (v ?? 0).toLocaleString()
		},
		{
			key: "created_at",
			label: t("common.created", "Created"),
			sortable: true,
			render: (v: string) => new Date(v).toLocaleDateString()
		},
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-32",
			render: (_: any, row: ContentItem) => {
				const isRemoved = row.status === "removed" || !!row.removed_at;
				if (isRemoved) {
					return `<button onclick="window.__contentRestore('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-success-700 hover:bg-success-50">${t("content.restore", "Restore")}</button>`;
				}
				return `<div class="flex items-center gap-1">
					<button onclick="window.__contentRemove('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-danger-700 hover:bg-danger-50">${t("content.remove", "Remove")}</button>
				</div>`;
			}
		}
	];

	async function loadItems() {
		loading = true;
		error = "";
		try {
			const params = new URLSearchParams({
				page: String(currentPage + 1),
				limit: String(pageSize)
			});
			if (searchQuery) params.set("q", searchQuery);
			if (filterValues.content_type) params.set("content_type", filterValues.content_type);
			if (filterValues.status) params.set("status", filterValues.status);

			const result = await contentAdminApi.list(params.toString());
			if (result.ok) {
				const d = result.data as { items: ContentItem[]; total: number };
				items = d.items ?? (Array.isArray(result.data) ? result.data : []);
				totalItems = d.total ?? items.length;
			} else {
				error = result.error ?? t("common.error", "Failed to load content");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load content");
		} finally {
			loading = false;
		}
	}

	function handleSearch(values: Record<string, string>) {
		searchQuery = values.search ?? "";
		filterValues = values;
		currentPage = 0;
		loadItems();
	}

	function handleClear() {
		searchQuery = "";
		filterValues = {};
		currentPage = 0;
		loadItems();
	}

	function openRemove(item: ContentItem) {
		removeTarget = item;
		removeReason = "";
		showRemoveModal = true;
	}

	async function confirmRemove() {
		if (!removeTarget) return;
		const result = await contentAdminApi.remove(removeTarget.id, { reason: removeReason || undefined });
		if (result.ok) {
			showRemoveModal = false;
			success = t("content.removed", "Content removed successfully");
			loadItems();
		} else {
			error = result.error ?? t("common.error", "Failed to remove content");
		}
	}

	function openRestore(item: ContentItem) {
		restoreTarget = item;
		showRestoreModal = true;
	}

	async function confirmRestore() {
		if (!restoreTarget) return;
		const result = await contentAdminApi.restore(restoreTarget.id);
		if (result.ok) {
			showRestoreModal = false;
			success = t("content.restored", "Content restored successfully");
			loadItems();
		} else {
			error = result.error ?? t("common.error", "Failed to restore content");
		}
	}

</script>

<svelte:head>
	<title>{t("content.title", "Content")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("content.title", "Content")}
	description={t("content.description", "Manage all platform content")}
	actions={[
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost", onclick: loadItems }
	]}
/>

{#if error}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{error}
	</div>
{/if}
{#if success}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)">
		{success}
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
			<DataTable {columns} data={items} pageSize={items.length} selectable />
			<Pagination bind:page={currentPage} totalPages={Math.ceil(totalItems / pageSize)} {totalItems} bind:pageSize />
		{/if}
	</CardContent>
</Card>

<!-- Remove Modal -->
<Modal bind:open={showRemoveModal} title={t("content.remove_title", "Remove Content")}>
	{#if removeTarget}
		<p class="mb-2 text-sm" style="color: var(--color-foreground)">
			{t("content.remove_confirm", "Remove this content?")} <strong>{removeTarget.title || "Untitled"}</strong>
		</p>
		<textarea
			class="w-full rounded-lg border p-3 text-sm"
			style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
			rows="3"
			placeholder={t("content.remove_reason", "Reason (optional)...")}
			bind:value={removeReason}
		></textarea>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showRemoveModal = false}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmRemove}>{t("content.remove", "Remove")}</Button>
	</div>
</Modal>

<!-- Restore Modal -->
<Modal bind:open={showRestoreModal} title={t("content.restore_title", "Restore Content")}>
	{#if restoreTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("content.restore_confirm", "Restore this content?")} <strong>{restoreTarget.title || "Untitled"}</strong>
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showRestoreModal = false}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={confirmRestore}>{t("content.restore", "Restore")}</Button>
	</div>
</Modal>
