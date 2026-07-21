<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { Select } from "$lib/components/ui/select";
	import Pagination from "$lib/components/ui/Pagination.svelte";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { Plus, RefreshCw, Eye, Pencil, Trash2 } from "lucide-svelte";
	import { spacesAdminApi } from "$lib/api";
	import { goto } from "$app/navigation";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	type Space = {
		id: string;
		name: string;
		owner: string;
		owner_id: string;
		member_count: number;
		status: string;
		space_type: string;
		created_at: string;
		updated_at: string;
	};

	let spaces = $state<Space[]>([]);
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
	let viewTarget = $state<Space | null>(null);
	let showEditModal = $state(false);
	let editTarget = $state<Space | null>(null);
	let editName = $state("");
	let editType = $state("");
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<Space | null>(null);
	let showSuspendModal = $state(false);
	let suspendTarget = $state<Space | null>(null);
	let suspendReason = $state("");
	let acting = $state(false);

	const canModerateSpaces = $derived(hasPermission("spaces.moderate.any"));
	const canManageSpaces = $derived(hasPermission("admin.settings"));

	const filterDefs = [
		{
			key: "status",
			label: t("spaces.status", "Status"),
			options: [
				{ value: "active", label: "Active" },
				{ value: "suspended", label: "Suspended" },
				{ value: "archived", label: "Archived" }
			]
		},
		{
			key: "space_type",
			label: t("spaces.type", "Type"),
			options: [
				{ value: "public", label: "Public" },
				{ value: "private", label: "Private" },
				{ value: "university", label: "University" }
			]
		}
	];

	const columns = [
		{
			key: "name",
			label: t("spaces.name", "Name"),
			sortable: true
		},
		{
			key: "owner",
			label: t("spaces.owner", "Owner"),
			sortable: true
		},
		{
			key: "member_count",
			label: t("spaces.members", "Members"),
			sortable: true,
			render: (v: number) => (v ?? 0).toLocaleString()
		},
		{
			key: "space_type",
			label: t("spaces.type", "Type"),
			sortable: true,
			render: (v: string) => v || "—"
		},
		{
			key: "status",
			label: t("spaces.status", "Status"),
			sortable: true,
			render: (v: string) => {
				const colors = v === "active" ? "bg-success/15 text-success dark:text-success" : v === "suspended" ? "bg-danger/15 text-danger dark:text-danger" : "bg-muted/15 text-muted dark:text-muted";
				return `<span class="inline-block rounded-full px-2 py-0.5 text-xs ${colors}">${v}</span>`;
			}
		},
		{
			key: "created_at",
			label: t("spaces.created", "Created"),
			sortable: true,
			render: (v: string) => new Date(v).toLocaleDateString()
		},
		{
			key: "actions",
			label: t("common.actions", "Actions"),
			class: "w-40",
			component: SpaceActionsCell
		}
	];

	async function loadSpaces() {
		loading = true;
		error = "";
		try {
			const params = new URLSearchParams({
				page: String(currentPage),
				limit: String(pageSize)
			});
			if (searchQuery) params.set("q", searchQuery);
			if (filterValues.status) params.set("status", filterValues.status);
			if (filterValues.space_type) params.set("space_type", filterValues.space_type);

			const result = await spacesAdminApi.list(params.toString());
			if (result.ok) {
				const d = result.data as Space[] | { items: Space[]; page: number; total_pages: number };
				if (Array.isArray(d)) {
					spaces = d;
					totalItems = d.length;
					totalPages = 1;
				} else {
					spaces = d.items ?? [];
					totalItems = d.items?.length ?? 0;
					totalPages = d.total_pages ?? 1;
				}
			} else {
				error = result.error ?? t("common.error", "Failed to load spaces");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load spaces");
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

	function openView(space: Space) {
		viewTarget = space;
		showViewModal = true;
	}

	function openEdit(space: Space) {
		editTarget = space;
		editName = space.name;
		editType = space.space_type;
		showEditModal = true;
	}

	async function saveEdit() {
		if (!editTarget) return;
		acting = true;
		try {
			const result = await spacesAdminApi.update(editTarget.id, { name: editName, space_type: editType });
			if (result.ok) {
				showEditModal = false;
				loadSpaces();
			} else {
				error = result.error;
			}
		} catch {
			error = t("common.error", "Failed to update space");
		} finally {
			acting = false;
		}
	}

	function confirmDelete(space: Space) {
		deleteTarget = space;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		acting = true;
		try {
			const result = await spacesAdminApi.delete(deleteTarget.id);
			if (result.ok) {
				showDeleteConfirm = false;
				deleteTarget = null;
				loadSpaces();
			} else {
				error = result.error;
			}
		} catch {
			error = t("common.error", "Failed to delete space");
		} finally {
			acting = false;
		}
	}

	function openSuspend(space: Space) {
		suspendTarget = space;
		suspendReason = "";
		showSuspendModal = true;
	}

	async function executeSuspend() {
		if (!suspendTarget) return;
		acting = true;
		try {
			const result = await spacesAdminApi.suspend(suspendTarget.id, {
				status: suspendTarget.status === "suspended" ? "active" : "suspended",
				reason: suspendReason.trim()
			});
			if (result.ok) {
				showSuspendModal = false;
				loadSpaces();
			} else {
				error = result.error;
			}
		} catch {
			error = t("common.error", "Failed to update space status");
		} finally {
			acting = false;
		}
	}

	async function bulkDelete() {
		const ids = Array.from(selectedIds);
		if (ids.length === 0) return;
		acting = true;
		try {
			await Promise.all(ids.map((id) => spacesAdminApi.delete(id)));
			selectedIds = new Set();
			loadSpaces();
		} catch {
			error = t("common.error", "Failed to delete spaces");
		} finally {
			acting = false;
		}
	}

	$effect(() => {
		void currentPage;
		void pageSize;
		void searchQuery;
		void filterValues;
		loadSpaces();
	});
</script>

<svelte:head>
	<title>{t("spaces.title", "Spaces")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("spaces.title", "Spaces")}
	description={t("spaces.description", "Manage platform spaces")}
	actions={[
		...(canManageSpaces ? [{ label: t("spaces.create", "Create Space"), icon: Plus, onclick: () => goto("/spaces/new") }] : []),
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost", onclick: loadSpaces }
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
		{#if canManageSpaces}
			<Button variant="ghost" size="sm" onclick={bulkDelete} disabled={acting}>
				<Trash2 class="mr-1 h-3.5 w-3.5" /> Delete Selected
			</Button>
		{/if}
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
					data={spaces}
					selectable={canManageSpaces}
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
<Modal bind:open={showViewModal} title={t("spaces.details", "Space Details")} size="default">
	{#if viewTarget}
		<div class="space-y-4">
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('spaces.name')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.name}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('spaces.owner')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.owner}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('spaces.members')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.member_count.toLocaleString()}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('spaces.type')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.space_type || "—"}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('spaces.status')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.status}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('spaces.created')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(viewTarget.created_at).toLocaleString()}</p>
				</div>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end">
		<Button variant="outline" onclick={() => (showViewModal = false)}>{t('spaces.close')}</Button>
	</div>
</Modal>

<!-- Edit Modal -->
<Modal bind:open={showEditModal} title={t("spaces.edit", "Edit Space")} size="default">
	{#if editTarget}
		<div class="space-y-4">
			<div>
				<label for="edit-name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('spaces.name')}</label>
				<input
					id="edit-name"
					type="text"
					class="w-full rounded-lg border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					bind:value={editName}
				/>
			</div>
			<div>
				<label for="edit-type" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('spaces.type')}</label>
				<select
					id="edit-type"
					class="w-full rounded-lg border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					bind:value={editType}
				>
					<option value="public">{t('spaces.public')}</option>
					<option value="private">{t('spaces.private')}</option>
					<option value="university">{t('spaces.university')}</option>
				</select>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showEditModal = false)}>{t('spaces.cancel')}</Button>
		<Button onclick={saveEdit} disabled={acting}>{acting ? "Saving..." : "Save"}</Button>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t("common.confirmDelete", "Confirm Delete")} size="sm">
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			Delete space <strong>{deleteTarget.name}</strong>? This cannot be undone.
		</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteConfirm = false)}>{t('spaces.cancel')}</Button>
		<Button variant="destructive" onclick={executeDelete} disabled={acting}>{acting ? "Deleting..." : "Delete"}</Button>
	</div>
</Modal>

<!-- Suspend/Unsuspend Modal -->
<Modal bind:open={showSuspendModal} title={suspendTarget?.status === "suspended" ? "Unsuspend Space" : "Suspend Space"} size="sm">
	{#if suspendTarget}
		<p class="text-sm mb-3" style="color: var(--color-foreground)">
			{#if suspendTarget.status === "suspended"}
				Unsuspend <strong>{suspendTarget.name}</strong>? It will become accessible again.
			{:else}
				Suspend <strong>{suspendTarget.name}</strong>? New content will be blocked.
			{/if}
		</p>
		{#if suspendTarget.status !== "suspended"}
			<textarea
				class="w-full rounded-lg border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				rows="3"
				placeholder="Reason (optional)"
				bind:value={suspendReason}
			></textarea>
		{/if}
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showSuspendModal = false)}>{t('spaces.cancel')}</Button>
		<Button variant={suspendTarget?.status === "suspended" ? "default" : "destructive"} onclick={executeSuspend} disabled={acting}>
			{acting ? "Processing..." : suspendTarget?.status === "suspended" ? "Unsuspend" : "Suspend"}
		</Button>
	</div>
</Modal>

<!-- SpaceActionsCell component -->
{#snippet SpaceActionsCell(space: Space)}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView(space)}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		<Button variant="ghost" size="sm" onclick={() => goto(`/spaces/${space.id}`)} style="color: var(--color-primary)">
			{t("spaces.manage", "Manage")}
		</Button>
		{#if canManageSpaces}
			<Button variant="ghost" size="sm" onclick={() => openEdit(space)}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
		{/if}
		{#if canModerateSpaces}
			<Button variant="ghost" size="sm" onclick={() => openSuspend(space)} style="color: var(--color-warning)">
				{space.status === "suspended" ? "Unsuspend" : "Suspend"}
			</Button>
		{/if}
		{#if canManageSpaces}
			<Button variant="ghost" size="sm" onclick={() => confirmDelete(space)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
