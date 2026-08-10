<script lang="ts">
	// /spaces — migrated to shared components (ActionMenu, FilterPanel,
	// SlideOver, bulkActionsList). Server-side paginated list; state actions
	// (suspend/unsuspend) via ActionItem with show() predicates. Keeps
	// page-local Modals for suspend (reason input) and delete confirmation.
	// View/Edit open in SlideOvers.
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { SlideOver } from "$lib/components/ui/slide-over";
	import { Select } from "$lib/components/ui/select";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import type { ActionItem, BulkAction } from "$lib/components/ui/data-table";
	import { Plus, RefreshCw, Eye, Pencil, Trash2, SlidersHorizontal } from "lucide-svelte";
	import { spacesAdminApi } from "$lib/api";
	import { goto } from "$app/navigation";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { pageFromUrl, pageSizeFromUrl, searchFromUrl, filterFromUrl, syncListStateToUrl } from "$lib/utils/url-sync.svelte";

	type Space = {
		id: string;
		name: string;
		owner: string;
		owner_id: string;
		member_count: number;
		status: string;
		visibility: string;
		space_type: string;
		created_at: string;
		updated_at: string;
	};

	let spaces = $state<Space[]>([]);
	let loading = $state(true);
	let error = $state("");
	let currentPage = $state(pageFromUrl());
	let totalItems = $state(0);
	let pageSize = $state(pageSizeFromUrl(25));
	let totalPages = $state(1);
	let searchQuery = $state(searchFromUrl());
	const _initFilterValue: Record<string, string> = {};
	{
		const status = filterFromUrl("status");
		if (status) _initFilterValue.status = status;
		const visibility = filterFromUrl("visibility");
		if (visibility) _initFilterValue.visibility = visibility;
	}
	let filterValue = $state<Record<string, string>>(_initFilterValue);
	let selectedIds = $state<Set<string>>(new Set());

	// View/Edit SlideOvers
	let showViewSlideOver = $state(false);
	let viewTarget = $state<Space | null>(null);
	let showEditSlideOver = $state(false);
	let editTarget = $state<Space | null>(null);
	let editName = $state("");
	let editVisibility = $state("");

	// Delete confirmation Modal
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<Space | null>(null);

	// Suspend/Unsuspend Modal (page-local — reason input)
	let showSuspendModal = $state(false);
	let suspendTarget = $state<Space | null>(null);
	let suspendReason = $state("");
	let acting = $state(false);

	const canModerateSpaces = $derived(hasPermission("spaces.moderate"));
	const canManageSpaces = $derived(hasPermission("spaces.admin"));

	const visibilitySelectOptions = [
		{ value: "public", label: t("spaces.public", "Public") },
		{ value: "members", label: t("spaces.perm.members", "Members") },
		{ value: "private", label: t("spaces.private", "Private") }
	];

	const filterDefs = [
		{
			key: "status",
			label: t("spaces.status", "Status"),
			options: [
				{ value: "active", label: t("spaces.active", "Active") },
				{ value: "suspended", label: t("spaces.suspended", "Suspended") },
				{ value: "archived", label: t("spaces.archived", "Archived") }
			]
		},
		{
			key: "visibility",
			label: t("spaces.visibility", "Visibility"),
			options: [
				{ value: "public", label: t("spaces.public", "Public") },
				{ value: "members", label: t("spaces.perm.members", "Members") },
				{ value: "private", label: t("spaces.private", "Private") }
			]
		}
	];

	const columns = [
		{ key: "name", label: t("spaces.name", "Name"), sortable: true },
		{ key: "owner", label: t("spaces.owner", "Owner"), sortable: true },
		{ key: "member_count", label: t("spaces.members", "Members"), sortable: true, render: (v: number) => (v ?? 0).toLocaleString() },
		{ key: "visibility", label: t("spaces.visibility", "Visibility"), sortable: true, render: (v: string) => v ? t(`spaces.perm.${v}`, v) : "—" },
		{
			key: "status",
			label: t("spaces.status", "Status"),
			sortable: true,
			render: (v: string) => {
				const cls = v === "active" ? "bg-success/15 text-success dark:text-success" : v === "suspended" ? "bg-danger/15 text-danger dark:text-danger" : "bg-muted/15 text-muted dark:text-muted";
				const label = t(`spaces.${v}`, v);
				return `<span class="inline-block rounded-full px-2 py-0.5 text-xs ${cls}">${label}</span>`;
			}
		},
		{ key: "created_at", label: t("spaces.created", "Created"), sortable: true, render: (v: string) => new Date(v).toLocaleDateString() }
	];

	const rowActions: ActionItem[] = [
		{ label: t("common.view", "View"), icon: Eye, onclick: (row) => openView(row) },
		{ label: t("spaces.manage", "Manage"), onclick: (row: Space) => goto(`/spaces/${row.id}`) },
		{
			label: t("common.edit", "Edit"),
			icon: Pencil,
			onclick: (row) => openEdit(row),
			show: () => canManageSpaces
		},
		{
			label: t("spaces.suspend", "Suspend"),
			onclick: (row) => openSuspend(row),
			show: (row: Space) => canModerateSpaces && row.status !== "suspended"
		},
		{
			label: t("spaces.unsuspend", "Unsuspend"),
			onclick: (row) => openSuspend(row),
			show: (row: Space) => canModerateSpaces && row.status === "suspended"
		},
		{
			label: t("common.delete", "Delete"),
			icon: Trash2,
			variant: "destructive",
			onclick: (row) => confirmDelete(row),
			show: () => canManageSpaces
		}
	];

	const bulkActions: BulkAction[] = $derived(canManageSpaces
		? [{ label: t("common.delete_selected", "Delete Selected"), icon: Trash2, variant: "destructive", onclick: bulkDelete }]
		: []);

	async function loadSpaces() {
		loading = true;
		error = "";
		try {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(currentPage * pageSize)
			});
			if (searchQuery) params.set("q", searchQuery);
			if (filterValue.status) params.set("status", filterValue.status);
			if (filterValue.visibility) params.set("visibility", filterValue.visibility);

			const result = await spacesAdminApi.list(params.toString());
			if (result.ok) {
				const d = result.data as Space[] | { items: Space[]; total: number; total_pages: number };
				if (Array.isArray(d)) {
					spaces = d;
					totalItems = d.length;
					totalPages = 1;
				} else {
					spaces = d.items ?? [];
					totalItems = d.total ?? d.items?.length ?? 0;
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

	function openView(space: Space) {
		viewTarget = space;
		showViewSlideOver = true;
	}

	function openEdit(space: Space) {
		editTarget = space;
		editName = space.name;
		editVisibility = space.visibility;
		showEditSlideOver = true;
	}

	async function saveEdit() {
		if (!editTarget) return;
		acting = true;
		try {
			const result = await spacesAdminApi.update(editTarget.id, { name: editName, visibility: editVisibility });
			if (result.ok) {
				showEditSlideOver = false;
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

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0 || acting) return;
		acting = true;
		try {
			await Promise.all(idArr.map((id) => spacesAdminApi.delete(id)));
			selectedIds = new Set();
			await loadSpaces();
		} catch {
			error = t("common.error", "Failed to delete spaces");
		} finally {
			acting = false;
		}
	}

	function handlePageChange(p: number) {
		currentPage = p;
	}

	function handlePageSizeChange(s: number) {
		pageSize = s;
		currentPage = 0;
	}

	// Sync list state (page, search, filters) to URL for back/forward survival.
	$effect(() => {
		void currentPage;
		void pageSize;
		void searchQuery;
		void filterValue;
		syncListStateToUrl({
			page: currentPage,
			pageSize,
			defaultPageSize: 25,
			search: searchQuery,
			filters: filterValue
		});
	});

	$effect(() => {
		void currentPage;
		void pageSize;
		void searchQuery;
		void filterValue;
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
		...(canManageSpaces ? [{ label: t("spaces.presets_title", "Permission Presets"), icon: SlidersHorizontal, variant: "ghost" as const, onclick: () => goto("/spaces/presets") }] : []),
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost" as const, onclick: loadSpaces }
	]}
/>

{#if error}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{error}
	</div>
{/if}

<Card>
	<CardContent>
		{#if loading}
			<TableSkeleton rows={8} columns={columns.length} />
		{:else}
			<div aria-live="polite">
				<DataTable
					{columns}
					data={spaces}
					serverSide
					page={currentPage}
					{pageSize}
					totalRows={totalItems}
					{totalPages}
					onpagechange={handlePageChange}
					onpagesizechange={handlePageSizeChange}
					searchable
					onsearch={(q) => { searchQuery = q; currentPage = 0; }}
					filters={filterDefs}
					bind:filterValue
					onfilterapply={() => { currentPage = 0; }}
					selectable={canManageSpaces}
					bind:selectedIds
					actions={rowActions}
					bulkActionsList={bulkActions}
				/>
			</div>
		{/if}
	</CardContent>
</Card>

<!-- View SlideOver -->
<SlideOver bind:open={showViewSlideOver} title={t("spaces.details", "Space Details")}>
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
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('spaces.visibility')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.visibility ? t(`spaces.perm.${viewTarget.visibility}`, viewTarget.visibility) : "—"}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('spaces.status')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{t(`spaces.${viewTarget.status}`, viewTarget.status)}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('spaces.created')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(viewTarget.created_at).toLocaleString()}</p>
				</div>
			</div>
		</div>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end">
			<Button variant="outline" onclick={() => (showViewSlideOver = false)}>{t('spaces.close')}</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- Edit SlideOver -->
<SlideOver bind:open={showEditSlideOver} title={t("spaces.edit", "Edit Space")}>
	{#if editTarget}
		<div class="space-y-4">
			<div>
				<label for="edit-name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('spaces.name')}</label>
				<input id="edit-name" type="text" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={editName} />
			</div>
			<div>
				<label for="edit-type" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('spaces.visibility')}</label>
				<Select options={visibilitySelectOptions} bind:value={editVisibility} />
			</div>
		</div>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={() => (showEditSlideOver = false)}>{t('spaces.cancel')}</Button>
			<Button onclick={saveEdit} disabled={acting}>{acting ? t("common.saving", "Saving...") : t("common.save", "Save")}</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- Delete Confirmation Modal -->
<Modal bind:open={showDeleteConfirm} title={t("common.confirmDelete", "Confirm Delete")} size="sm">
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("spaces.delete_confirm", "Delete space")} <strong>{deleteTarget.name}</strong>? {t("common.cannotUndo", "This action cannot be undone.")}
		</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showDeleteConfirm = false)}>{t('spaces.cancel')}</Button>
		<Button variant="destructive" onclick={executeDelete} disabled={acting}>{acting ? t("common.deleting", "Deleting...") : t("common.delete", "Delete")}</Button>
	</div>
</Modal>

<!-- Suspend/Unsuspend Modal (page-local — reason input) -->
<Modal bind:open={showSuspendModal} title={suspendTarget?.status === "suspended" ? t("spaces.unsuspend_space", "Unsuspend Space") : t("spaces.suspend_space", "Suspend Space")} size="sm">
	{#if suspendTarget}
		<p class="text-sm mb-3" style="color: var(--color-foreground)">
			{#if suspendTarget.status === "suspended"}
				{t("spaces.unsuspend_confirm", "Unsuspend")} <strong>{suspendTarget.name}</strong>? {t("spaces.unsuspend_desc", "It will become accessible again.")}
			{:else}
				{t("spaces.suspend_confirm", "Suspend")} <strong>{suspendTarget.name}</strong>? {t("spaces.suspend_desc", "New content will be blocked.")}
			{/if}
		</p>
		{#if suspendTarget.status !== "suspended"}
			<textarea
				class="w-full rounded-lg border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				rows="3"
				placeholder={t("spaces.reason_placeholder", "Reason (optional)")}
				bind:value={suspendReason}
			></textarea>
		{/if}
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showSuspendModal = false)}>{t('spaces.cancel')}</Button>
		<Button variant={suspendTarget?.status === "suspended" ? "default" : "destructive"} onclick={executeSuspend} disabled={acting}>
			{acting ? t("spaces.processing", "Processing...") : suspendTarget?.status === "suspended" ? t("spaces.unsuspend", "Unsuspend") : t("spaces.suspend", "Suspend")}
		</Button>
	</div>
</Modal>
