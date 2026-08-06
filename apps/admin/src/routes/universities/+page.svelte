<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { SlideOver } from "$lib/components/ui/slide-over";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import type { ActionItem, BulkAction } from "$lib/components/ui/data-table";
	import { Plus, RefreshCw, Eye, Pencil, Trash2, BadgeCheck, XCircle, Upload, FileEdit } from "lucide-svelte";
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

	let showViewSlideOver = $state(false);
	let viewTarget = $state<University | null>(null);
	let showCreateSlideOver = $state(false);
	let showEditSlideOver = $state(false);
	let editTarget = $state<University | null>(null);
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<University | null>(null);
	let acting = $state(false);

	let formName = $state("");
	let formSlug = $state("");
	let formVerified = $state(false);
	let showBulkImportModal = $state(false);
	let csvFile = $state<File | null>(null);
	let showStatusModal = $state(false);
	let statusTarget = $state<University | null>(null);
	let formStatus = $state("draft");

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
		{ key: "created_at", label: t("universities.created", "Created"), sortable: true, render: (v: string) => new Date(v).toLocaleDateString() }
	];

	const rowActions: ActionItem[] = [
		{ label: t("common.view", "View"), icon: Eye, onclick: (row) => openView(row) },
		{
			label: t("universities.verify", "Verify"),
			icon: BadgeCheck,
			onclick: (row) => toggleVerify(row),
			show: (row: University) => canManage && !row.verified
		},
		{
			label: t("universities.unverify", "Unverify"),
			icon: XCircle,
			variant: "destructive",
			onclick: (row) => toggleVerify(row),
			show: (row: University) => canManage && row.verified
		},
		{
			label: t("universities.changeStatus", "Change Status"),
			icon: FileEdit,
			onclick: (row) => openStatusModal(row),
			show: () => canManage
		},
		{
			label: t("common.edit", "Edit"),
			icon: Pencil,
			onclick: (row) => openEdit(row),
			show: () => canManage
		},
		{
			label: t("common.delete", "Delete"),
			icon: Trash2,
			variant: "destructive",
			onclick: (row) => confirmDelete(row),
			show: () => canManage
		}
	];

	const bulkActions: BulkAction[] = $derived(canManage
		? [{ label: t("universities.deleteSelected", "Delete Selected"), icon: Trash2, variant: "destructive", onclick: bulkDelete }]
		: []);

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

	function openView(uni: University) { viewTarget = uni; showViewSlideOver = true; }

	function openCreate() {
		formName = ""; formSlug = ""; formVerified = false;
		showCreateSlideOver = true;
	}

	function openEdit(uni: University) {
		editTarget = uni;
		formName = uni.name; formSlug = uni.slug; formVerified = uni.status === "active";
		showEditSlideOver = true;
	}

	async function submitCreate() {
		if (!formName.trim()) return;
		acting = true;
		try {
			const result = await universitiesAdminApi.create({ name: formName.trim(), slug: formSlug.trim(), verified: formVerified });
			if (result.ok) { showCreateSlideOver = false; load(); } else { error = result.error; }
		} catch { error = t("common.error", "Failed to create"); } finally { acting = false; }
	}

	async function submitEdit() {
		if (!editTarget) return;
		acting = true;
		try {
			const result = await universitiesAdminApi.update(editTarget.id, { name: formName.trim(), slug: formSlug.trim(), verified: formVerified });
			if (result.ok) { showEditSlideOver = false; load(); } else { error = result.error; }
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

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0 || acting) return;
		acting = true;
		try {
			await Promise.all(idArr.map((id) => universitiesAdminApi.delete(id)));
			load();
		} catch { error = t("common.error", "Failed to delete"); } finally { acting = false; }
	}

	async function toggleVerify(uni: University) {
		acting = true;
		try {
			const result = uni.verified
				? await universitiesAdminApi.unverify(uni.id)
				: await universitiesAdminApi.verify(uni.id);
			if (result.ok) { load(); } else { error = result.error; }
		} catch { error = t("common.error", "Failed to update verification"); } finally { acting = false; }
	}

	function openStatusModal(uni: University) {
		statusTarget = uni;
		formStatus = uni.status;
		showStatusModal = true;
	}

	async function submitStatusChange() {
		if (!statusTarget) return;
		acting = true;
		try {
			const result = await universitiesAdminApi.updateStatus(statusTarget.id, { status: formStatus });
			if (result.ok) { showStatusModal = false; statusTarget = null; load(); } else { error = result.error; }
		} catch { error = t("common.error", "Failed to update status"); } finally { acting = false; }
	}

	function openBulkImport() {
		csvFile = null;
		showBulkImportModal = true;
	}

	function handleFileSelect(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		csvFile = input.files?.[0] ?? null;
	}

	async function submitBulkImport() {
		if (!csvFile) return;
		acting = true;
		try {
			const formData = new FormData();
			formData.append('file', csvFile);
			const result = await universitiesAdminApi.bulkImport(formData);
			if (result.ok) { showBulkImportModal = false; csvFile = null; load(); } else { error = result.error; }
		} catch { error = t("common.error", "Failed to import"); } finally { acting = false; }
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
		...(canManage ? [{ label: t("universities.bulkImport", "CSV Import"), icon: Upload, onclick: openBulkImport, variant: "outline" as const }] : []),
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost" as const, onclick: load }
	]}
/>

{#if error}
	<div class="mb-4 flex items-center justify-between rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		<span>{error}</span>
		<Button variant="outline" size="sm" onclick={load}>{t("common.retry", "Retry")}</Button>
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
					data={universities}
					page={currentPage}
					{pageSize}
					totalRows={totalItems}
					{totalPages}
					onpagechange={(p) => { currentPage = p; }}
					onpagesizechange={(s) => { pageSize = s; currentPage = 0; }}
					searchable
					onsearch={(q) => { searchQuery = q; currentPage = 0; applyFilters(); }}
					filters={filterDefs}
					bind:filterValue={filterValues}
					onfilterapply={() => { currentPage = 0; applyFilters(); }}
					onfilterclear={() => { currentPage = 0; applyFilters(); }}
					selectable={canManage}
					bind:selectedIds
					actions={rowActions}
					bulkActionsList={bulkActions}
				/>
			</div>
		{/if}
	</CardContent>
</Card>

<SlideOver bind:open={showViewSlideOver} title={t("universities.details", "University Details")}>
	{#if viewTarget}
		<div class="grid grid-cols-2 gap-4">
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("universities.name", "Name")}</p><p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.name}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("universities.slug", "Slug")}</p><p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{viewTarget.slug}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("universities.status", "Status")}</p><p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.status}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("universities.students", "Members")}</p><p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{(viewTarget.member_count ?? 0).toLocaleString()}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("universities.created", "Created")}</p><p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(viewTarget.created_at).toLocaleString()}</p></div>
		</div>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end"><Button variant="outline" onclick={() => (showViewSlideOver = false)}>{t("common.close", "Close")}</Button></div>
	{/snippet}
</SlideOver>

<SlideOver bind:open={showCreateSlideOver} title={t("universities.create", "Create University")}>
	<div class="space-y-3">
		<div><label for="uni-name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("universities.name", "Name")}</label><input id="uni-name" type="text" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={formName} /></div>
		<div><label for="uni-domain" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("universities.domain_slug", "Domain")}</label><input id="uni-domain" type="text" class="w-full rounded-lg border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="mit.edu" bind:value={formSlug} /></div>
		<div class="flex items-center gap-2"><input id="uni-verified" type="checkbox" bind:checked={formVerified} /><label for="uni-verified" class="text-sm">{t("universities.verified", "Verified")}</label></div>
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={() => (showCreateSlideOver = false)}>{t("common.cancel", "Cancel")}</Button>
			<Button onclick={submitCreate} disabled={acting || !formName.trim()}>{acting ? t("common.creating", "Creating...") : t("common.create", "Create")}</Button>
		</div>
	{/snippet}
</SlideOver>

<SlideOver bind:open={showEditSlideOver} title={t("universities.edit", "Edit University")}>
	{#if editTarget}
		<div class="space-y-3">
			<div><label for="edit-uni-name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("universities.name", "Name")}</label><input id="edit-uni-name" type="text" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={formName} /></div>
			<div><label for="edit-uni-domain" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("universities.domain_slug", "Domain")}</label><input id="edit-uni-domain" type="text" class="w-full rounded-lg border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={formSlug} /></div>
			<div class="flex items-center gap-2"><input id="edit-uni-verified" type="checkbox" bind:checked={formVerified} /><label for="edit-uni-verified" class="text-sm">{t("universities.verified", "Verified")}</label></div>
		</div>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={() => (showEditSlideOver = false)}>{t("common.cancel", "Cancel")}</Button>
			<Button onclick={submitEdit} disabled={acting}>{acting ? t("common.saving", "Saving...") : t("common.save", "Save")}</Button>
		</div>
	{/snippet}
</SlideOver>

<Modal bind:open={showDeleteConfirm} title={t("common.confirmDelete", "Confirm Delete")} size="sm">
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">{t("universities.deleteConfirm", "Delete university")} <strong>{deleteTarget.name}</strong>? {t("universities.deleteWarning", "This cannot be undone.")}</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteConfirm = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={executeDelete} disabled={acting}>{acting ? t("common.deleting", "Deleting...") : t("common.delete", "Delete")}</Button>
	</div>
</Modal>

<Modal bind:open={showStatusModal} title={t("universities.changeStatus", "Change Status")} size="sm">
	{#if statusTarget}
		<p class="mb-3 text-sm" style="color: var(--color-foreground)">{t("universities.statusFor", "Status for")} <strong>{statusTarget.name}</strong></p>
		<div class="space-y-2">
			{#each ["draft", "published", "archived"] as s}
				<label class="flex items-center gap-2 text-sm" style="color: var(--color-foreground)">
					<input type="radio" name="status" value={s} bind:group={formStatus} />
					{s}
				</label>
			{/each}
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showStatusModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={submitStatusChange} disabled={acting}>{acting ? t("common.saving", "Saving...") : t("common.save", "Save")}</Button>
	</div>
</Modal>

<Modal bind:open={showBulkImportModal} title={t("universities.bulkImport", "CSV Bulk Import")} size="default">
	<div class="space-y-3">
		<p class="text-sm" style="color: var(--color-muted)">{t("universities.bulkImportHelp", "Upload a CSV file with columns: name, slug, domain. Each row will create a university.")}</p>
		<div>
			<label for="csv-file" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("universities.csvFile", "CSV File")}</label>
			<input id="csv-file" type="file" accept=".csv" onchange={handleFileSelect} class="w-full text-sm" />
		</div>
	</div>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showBulkImportModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={submitBulkImport} disabled={acting || !csvFile}>{acting ? t("common.importing", "Importing...") : t("common.import", "Import")}</Button>
	</div>
</Modal>

