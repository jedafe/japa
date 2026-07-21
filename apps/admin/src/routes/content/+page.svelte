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
	import { RefreshCw, Eye, RotateCcw, Trash2, Pencil } from "lucide-svelte";
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
		slug?: string;
		body?: string;
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

	// View modal
	let showViewModal = $state(false);
	let viewTarget = $state<ContentItem | null>(null);

	// Remove modal
	let showRemoveModal = $state(false);
	let removeTarget = $state<ContentItem | null>(null);
	let removeReason = $state("");

	// Restore modal
	let showRestoreModal = $state(false);
	let restoreTarget = $state<ContentItem | null>(null);

	// Edit modal (PATCH /admin/content/{id})
	let showEditModal = $state(false);
	let editTarget = $state<ContentItem | null>(null);
	let editTitle = $state("");
	let editSlug = $state("");
	let editStatus = $state("");
	let editBody = $state("");

	// Backend gates content mutations on content.delete (list/restore/delete)
	const canManageContent = $derived(hasPermission("content.delete"));
	// PATCH /admin/content/{id} is gated on content.edit
	const canEditContent = $derived(hasPermission("content.edit"));

	let selectedIds = $state<Set<string>>(new Set());

	function openView(item: ContentItem) {
		viewTarget = item;
		showViewModal = true;
	}

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
			component: ContentActionsCell
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

	function openEdit(item: ContentItem) {
		editTarget = item;
		editTitle = item.title ?? "";
		editSlug = item.slug ?? "";
		editStatus = item.status ?? "draft";
		editBody = item.body ?? "";
		showEditModal = true;
	}

	async function confirmEdit() {
		if (!editTarget) return;
		const body: Record<string, unknown> = {};
		if (editTitle.trim()) body.title = editTitle.trim();
		if (editSlug.trim()) body.slug = editSlug.trim();
		if (editStatus) body.status = editStatus;
		if (editBody.trim()) body.body = editBody.trim();
		const result = await contentAdminApi.update(editTarget.id, body);
		if (result.ok) {
			showEditModal = false;
			success = t("content.updated", "Content updated successfully");
			loadItems();
		} else {
			error = result.error ?? t("common.error", "Failed to update content");
		}
	}

	let bulkRemoving = $state(false);
	async function bulkRemove() {
		const ids = Array.from(selectedIds);
		if (ids.length === 0) return;
		bulkRemoving = true;
		error = "";
		try {
			await Promise.all(ids.map((id) => contentAdminApi.remove(id)));
			selectedIds = new Set();
			success = t("content.removed", "Content removed successfully");
			loadItems();
		} catch {
			error = t("common.error", "Failed to remove content");
		} finally {
			bulkRemoving = false;
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
			<DataTable {columns} data={items} pageSize={10} selectable bind:selectedIds>
				{#snippet bulkActions()}
					<Button variant="destructive" size="sm" onclick={bulkRemove} disabled={bulkRemoving}>
						<Trash2 class="mr-1 h-3.5 w-3.5" />
						{bulkRemoving ? t("common.removing", "Removing...") : `${t("content.remove_selected", "Remove Selected")} (${selectedIds.size})`}
					</Button>
				{/snippet}
			</DataTable>
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

<!-- View Modal -->
<Modal bind:open={showViewModal} title={t("content.details", "Content Details")} size="lg">
	{#if viewTarget}
		<div class="space-y-4">
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("content.title", "Title")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.title || "Untitled"}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("content.type", "Type")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.content_type}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("content.author", "Author")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.author_name}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("common.status", "Status")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.status}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("content.views", "Views")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{(viewTarget.views ?? 0).toLocaleString()}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("common.created", "Created")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(viewTarget.created_at).toLocaleString()}</p>
				</div>
				{#if viewTarget.url}
					<div class="col-span-2">
						<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">URL</p>
						<a class="mt-1 block break-all text-sm font-mono" style="color: var(--color-primary)" href={viewTarget.url} target="_blank" rel="noreferrer">{viewTarget.url}</a>
					</div>
				{/if}
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end">
		<Button variant="outline" onclick={() => (showViewModal = false)}>{t("common.close", "Close")}</Button>
	</div>
</Modal>

<!-- Edit Modal -->
<Modal bind:open={showEditModal} title={t("content.edit_title", "Edit Content")}>
	{#if editTarget}
		<div class="space-y-4">
			<div>
				<label class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)" for="edit-title">{t("content.title", "Title")}</label>
				<input id="edit-title" class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={editTitle} />
			</div>
			<div>
				<label class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)" for="edit-slug">{t("content.slug", "Slug")}</label>
				<input id="edit-slug" class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={editSlug} />
			</div>
			<div>
				<label class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)" for="edit-status">{t("common.status", "Status")}</label>
				<Select
					options={[
						{ value: "published", label: t("content.status.published", "Published") },
						{ value: "draft", label: t("content.status.draft", "Draft") },
						{ value: "removed", label: t("content.status.removed", "Removed") },
						{ value: "archived", label: t("content.status.archived", "Archived") }
					]}
					value={editStatus}
					onchange={(val: string) => (editStatus = val)}
				/>
			</div>
			<div>
				<label class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)" for="edit-body">{t("content.body", "Body")}</label>
				<textarea id="edit-body" class="mt-1 w-full rounded-lg border p-3 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" rows="6" bind:value={editBody}></textarea>
			</div>
		</div>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showEditModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="default" onclick={confirmEdit}>{t("common.save", "Save")}</Button>
	</div>
</Modal>

<!-- ContentActionsCell -->
{#snippet ContentActionsCell(row: ContentItem)}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView(row)}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canEditContent}
			<Button variant="ghost" size="sm" onclick={() => openEdit(row)}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
		{/if}
		{#if canManageContent}
			{#if row.status === "removed" || !!row.removed_at}
				<Button variant="ghost" size="sm" onclick={() => openRestore(row)} style="color: var(--color-success)">
					<RotateCcw class="h-3.5 w-3.5" />
				</Button>
			{:else}
				<Button variant="ghost" size="sm" onclick={() => openRemove(row)} style="color: var(--color-danger)">
					<Trash2 class="h-3.5 w-3.5" />
				</Button>
			{/if}
		{/if}
	</div>
{/snippet}
