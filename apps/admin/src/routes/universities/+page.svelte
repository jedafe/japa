<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import Pagination from "$lib/components/ui/Pagination.svelte";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { Plus, RefreshCw, Eye, Pencil, Trash2 } from "lucide-svelte";
	import { universitiesAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	type University = {
		id: string;
		name: string;
		slug: string;
		member_count: number;
		status: string;
		verified: boolean;
		created_at: string;
	};

	type UniversityRaw = Omit<University, "verified">;

	function deriveVerified(u: UniversityRaw): University {
		return { ...u, verified: u.status === "active" };
	}

	let universities = $state<University[]>([]);
	let loading = $state(true);
	let error = $state("");
	let currentPage = $state(0);
	let totalItems = $state(0);
	let pageSize = $state(25);
	let totalPages = $state(1);
	let searchQuery = $state("");
	let filterValues = $state<Record<string, string>>({});
	let selectedIds = $state<Set<string>>(new Set());

	let showViewModal = $state(false);
	let viewTarget = $state<University | null>(null);
	let showCreateModal = $state(false);
	let showEditModal = $state(false);
	let editTarget = $state<University | null>(null);
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<University | null>(null);
	let acting = $state(false);

	let formName = $state("");
	let formSlug = $state("");
	let formVerified = $state(false);

	const canManage = $derived(hasPermission("admin.settings"));

	const filterDefs = [
		{
			key: "verified",
			label: t("universities.verified", "Verified"),
			options: [
				{ value: "true", label: t("common.yes", "Yes") },
				{ value: "false", label: t("common.no", "No") }
			]
		}
	];

	const columns = [
		{ key: "name", label: t("universities.name", "Name"), sortable: true },
		{ key: "slug", label: t("universities.slug", "Slug"), sortable: true, render: (v: string) => `<span class="font-mono text-xs">${v}</span>` },
		{
			key: "status",
			label: t("universities.status", "Status"),
			sortable: true,
			render: (v: string) => {
				const cls = v === "active" ? "bg-emerald-500/15 text-emerald-700 dark:text-emerald-400" : "bg-amber-500/15 text-amber-700 dark:text-amber-400";
				return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${cls}">${v}</span>`;
			}
		},
		{ key: "member_count", label: t("universities.students", "Members"), sortable: true, render: (v: number) => (v ?? 0).toLocaleString() },
		{ key: "created_at", label: t("universities.created", "Created"), sortable: true, render: (v: string) => new Date(v).toLocaleDateString() },
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-32", componentSnippet: ActionsCell }
	];

	let allUniversities = $state<University[]>([]);

	async function fetchAll() {
		error = "";
		try {
			const result = await universitiesAdminApi.list();
			if (result.ok) {
				const d = result.data as UniversityRaw[] | { items: UniversityRaw[]; total: number; page: number; total_pages: number };
				const raw = Array.isArray(d) ? d : (d.items ?? []);
				allUniversities = raw.map(deriveVerified);
			} else {
				error = result.error ?? t("common.error", "Failed to load");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load");
		}
	}

	function applyFilters() {
		let filtered = allUniversities;
		if (searchQuery) {
			const q = searchQuery.toLowerCase();
			filtered = filtered.filter((u) => u.name.toLowerCase().includes(q) || u.slug.toLowerCase().includes(q));
		}
		if (filterValues.verified === "true") {
			filtered = filtered.filter((u) => u.status === "active");
		} else if (filterValues.verified === "false") {
			filtered = filtered.filter((u) => u.status !== "active");
		}
		totalItems = filtered.length;
		totalPages = Math.max(1, Math.ceil(totalItems / pageSize));
		if (currentPage >= totalPages) currentPage = totalPages - 1;
		const start = currentPage * pageSize;
		universities = filtered.slice(start, start + pageSize);
	}

	async function load() {
		loading = true;
		await fetchAll();
		applyFilters();
		loading = false;
	}

	function handleSearch(values: Record<string, string>) {
		searchQuery = values.search ?? "";
		filterValues = values;
		currentPage = 0;
		applyFilters();
	}

	function handleClear() {
		searchQuery = "";
		filterValues = {};
		currentPage = 0;
		applyFilters();
	}

	function openView(uni: University) { viewTarget = uni; showViewModal = true; }

	function openCreate() {
		formName = ""; formSlug = ""; formVerified = false;
		showCreateModal = true;
	}

	function openEdit(uni: University) {
		editTarget = uni;
		formName = uni.name; formSlug = uni.slug; formVerified = uni.status === "active";
		showEditModal = true;
	}

	async function submitCreate() {
		if (!formName.trim()) return;
		acting = true;
		try {
			const result = await universitiesAdminApi.create({ name: formName.trim(), slug: formSlug.trim(), verified: formVerified });
			if (result.ok) { showCreateModal = false; load(); } else { error = result.error; }
		} catch { error = t("common.error", "Failed to create"); } finally { acting = false; }
	}

	async function submitEdit() {
		if (!editTarget) return;
		acting = true;
		try {
			const result = await universitiesAdminApi.update(editTarget.id, { name: formName.trim(), slug: formSlug.trim(), verified: formVerified });
			if (result.ok) { showEditModal = false; load(); } else { error = result.error; }
		} catch { error = t("common.error", "Failed to update"); } finally { acting = false; }
	}

	function confirmDelete(uni: University) { deleteTarget = uni; showDeleteConfirm = true; }

	async function executeDelete() {
		if (!deleteTarget) return;
		acting = true;
		try {
			const result = await universitiesAdminApi.delete(deleteTarget.id);
			if (result.ok) { showDeleteConfirm = false; deleteTarget = null; load(); } else { error = result.error; }
		} catch { error = t("common.error", "Failed to delete"); } finally { acting = false; }
	}

	async function bulkDelete() {
		const ids = Array.from(selectedIds);
		if (ids.length === 0) return;
		acting = true;
		try {
			await Promise.all(ids.map((id) => universitiesAdminApi.delete(id)));
			selectedIds = new Set(); load();
		} catch { error = t("common.error", "Failed to delete"); } finally { acting = false; }
	}

	$effect(() => {
		void currentPage;
		void pageSize;
		if (allUniversities.length > 0) {
			applyFilters();
		} else {
			load();
		}
	});
</script>

<svelte:head>
	<title>{t("universities.title", "Universities")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("universities.title", "Universities")}
	description={t("universities.description", "Manage university registrations and verification")}
	actions={[
		...(canManage ? [{ label: t("universities.create", "Create University"), icon: Plus, onclick: openCreate }] : []),
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost", onclick: load }
	]}
/>

{#if error}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">{error}</div>
{/if}

{#if selectedIds.size > 0}
	<div class="mb-4 flex items-center gap-3 rounded-lg p-3" style="background: var(--color-primary); color: white">
		<span class="text-sm font-medium">{selectedIds.size} {t("common.selected", "selected")}</span>
		{#if canManage}
			<Button variant="ghost" size="sm" onclick={bulkDelete} disabled={acting}>
				<Trash2 class="mr-1 h-3.5 w-3.5" /> {t("universities.deleteSelected", "Delete Selected")}
			</Button>
		{/if}
	</div>
{/if}

<Card>
	<CardHeader><FilterBar filters={filterDefs} onsearch={handleSearch} onclear={handleClear} /></CardHeader>
	<CardContent>
		{#if loading}
			<TableSkeleton rows={8} columns={columns.length} />
		{:else}
			<div aria-live="polite">
				<DataTable {columns} data={universities} selectable={canManage} bind:selectedIds />
			</div>
			<Pagination bind:page={currentPage} {totalPages} {totalItems} bind:pageSize />
		{/if}
	</CardContent>
</Card>

<Modal bind:open={showViewModal} title={t("universities.details", "University Details")} size="default">
	{#if viewTarget}
		<div class="grid grid-cols-2 gap-4">
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("universities.name", "Name")}</p><p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.name}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("universities.slug", "Slug")}</p><p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{viewTarget.slug}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("universities.status", "Status")}</p><p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.status}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("universities.students", "Members")}</p><p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{(viewTarget.member_count ?? 0).toLocaleString()}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("universities.created", "Created")}</p><p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(viewTarget.created_at).toLocaleString()}</p></div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end"><Button variant="outline" onclick={() => (showViewModal = false)}>{t("common.close", "Close")}</Button></div>
</Modal>

<Modal bind:open={showCreateModal} title={t("universities.create", "Create University")} size="default">
	<div class="space-y-3">
		<div><label for="uni-name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("universities.name", "Name")}</label><input id="uni-name" type="text" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={formName} /></div>
		<div><label for="uni-domain" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("universities.domain_slug", "Domain")}</label><input id="uni-domain" type="text" class="w-full rounded-lg border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="mit.edu" bind:value={formSlug} /></div>
		<div class="flex items-center gap-2"><input id="uni-verified" type="checkbox" bind:checked={formVerified} /><label for="uni-verified" class="text-sm">{t("universities.verified", "Verified")}</label></div>
	</div>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showCreateModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={submitCreate} disabled={acting || !formName.trim()}>{acting ? t("common.creating", "Creating...") : t("common.create", "Create")}</Button>
	</div>
</Modal>

<Modal bind:open={showEditModal} title={t("universities.edit", "Edit University")} size="default">
	{#if editTarget}
		<div class="space-y-3">
			<div><label for="edit-uni-name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("universities.name", "Name")}</label><input id="edit-uni-name" type="text" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={formName} /></div>
			<div><label for="edit-uni-domain" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("universities.domain_slug", "Domain")}</label><input id="edit-uni-domain" type="text" class="w-full rounded-lg border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={formSlug} /></div>
			<div class="flex items-center gap-2"><input id="edit-uni-verified" type="checkbox" bind:checked={formVerified} /><label for="edit-uni-verified" class="text-sm">{t("universities.verified", "Verified")}</label></div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showEditModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={submitEdit} disabled={acting}>{acting ? t("common.saving", "Saving...") : t("common.save", "Save")}</Button>
	</div>
</Modal>

<Modal bind:open={showDeleteConfirm} title={t("common.confirmDelete", "Confirm Delete")} size="sm">
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">{t("universities.deleteConfirm", "Delete university")} <strong>{deleteTarget.name}</strong>? {t("universities.deleteWarning", "This cannot be undone.")}</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteConfirm = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={executeDelete} disabled={acting}>{acting ? t("common.deleting", "Deleting...") : t("common.delete", "Delete")}</Button>
	</div>
</Modal>

{#snippet ActionsCell({ value, row }: { value: any; row: University })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView(row)}><Eye class="h-3.5 w-3.5" /></Button>
		{#if canManage}
			<Button variant="ghost" size="sm" onclick={() => openEdit(row)}><Pencil class="h-3.5 w-3.5" /></Button>
			<Button variant="ghost" size="sm" onclick={() => confirmDelete(row)} style="color: var(--color-danger)"><Trash2 class="h-3.5 w-3.5" /></Button>
		{/if}
	</div>
{/snippet}
