<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { DataTable } from "$lib/components/ui/data-table";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { SlideOver } from "$lib/components/ui/slide-over";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import type { ActionItem } from "$lib/components/ui/data-table";
	import { Plus, Pencil, Trash2, RefreshCw } from "lucide-svelte";
	import { universityAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { onMount } from "svelte";

	type Category = {
		id: string;
		slug: string;
		name: string;
		sort_order: number;
		is_active: boolean;
		created_at: string;
		updated_at: string;
	};

	let categories = $state<Category[]>([]);
	let loading = $state(true);
	let errorMessage = $state("");
	let successMessage = $state("");
	let searchQuery = $state("");
	let filterValues = $state<Record<string, string>>({});

	let showSlideOver = $state(false);
	let editingId = $state<string | null>(null);
	let saving = $state(false);
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<Category | null>(null);
	let deleting = $state(false);

	let form = $state({
		name: "",
		slug: "",
		sort_order: 0,
		is_active: true
	});

	const canManage = $derived(hasPermission("university.category.manage"));

	const statusFilters = [
		{
			key: "active",
			label: t("university.status", "Status"),
			options: [
				{ value: "true", label: t("university.active", "Active") },
				{ value: "false", label: t("university.inactive", "Inactive") }
			]
		}
	];

	const columns = $derived([
		{ key: "name", label: t("university.category_name", "Category"), render: (v: string) => `<span class="font-medium">${v}</span>` },
		{ key: "slug", label: t("university.slug", "Slug"), render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>` },
		{ key: "sort_order", label: t("university.sort_order", "Sort Order"), render: (v: number) => `<span class="text-xs">${v}</span>` },
		{ key: "is_active", label: t("university.status", "Status"), render: (v: boolean) => v
			? `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium" style="background: color-mix(in srgb, var(--color-success) 15%, transparent); color: var(--color-success)">${t("university.active", "Active")}</span>`
			: `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium" style="background: color-mix(in srgb, var(--color-muted) 15%, transparent); color: var(--color-muted)">${t("university.inactive", "Inactive")}</span>`
		}
	]);

	const rowActions: ActionItem[] = [
		{
			label: t("common.edit", "Edit"),
			icon: Pencil,
			onclick: (row: Category) => openEdit(row),
			show: () => canManage
		},
		{
			label: t("common.delete", "Delete"),
			icon: Trash2,
			variant: "destructive",
			onclick: (row: Category) => confirmDelete(row),
			show: () => canManage
		}
	];

	function slugify(text: string): string {
		return text
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, "-")
			.replace(/^-|-$/g, "");
	}

	async function load() {
		loading = true;
		errorMessage = "";
		try {
			const params = new URLSearchParams();
			if (filterValues.active) params.set("active", filterValues.active);
			if (searchQuery) params.set("search", searchQuery);
			const result = await universityAdminApi.listCategories(params.toString() || undefined);
			if (result.ok) {
				categories = (result.data as Category[] | null | undefined) ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t("common.error", "Failed to load categories");
		} finally {
			loading = false;
		}
	}

	function handleSearch(values: Record<string, string>) {
		searchQuery = values.search ?? "";
		filterValues = values;
		load();
	}

	function handleClear() {
		searchQuery = "";
		filterValues = {};
		load();
	}

	function openCreate() {
		editingId = null;
		form = { name: "", slug: "", sort_order: 0, is_active: true };
		showSlideOver = true;
	}

	function openEdit(cat: Category) {
		editingId = cat.id;
		form = { name: cat.name, slug: cat.slug, sort_order: cat.sort_order, is_active: cat.is_active };
		showSlideOver = true;
	}

	async function submit() {
		if (!form.name.trim()) return;
		saving = true;
		errorMessage = "";
		successMessage = "";
		const slug = form.slug.trim() || slugify(form.name);
		const body = {
			name: form.name.trim(),
			slug,
			sort_order: form.sort_order,
			is_active: form.is_active
		};
		const result = editingId
			? await universityAdminApi.updateCategory(editingId, body)
			: await universityAdminApi.createCategory(body);
		if (result.ok) {
			showSlideOver = false;
			successMessage = editingId
				? t("university.category_updated", "Category updated")
				: t("university.category_created", "Category created");
			load();
		} else {
			errorMessage = result.error;
		}
		saving = false;
	}

	function confirmDelete(cat: Category) {
		deleteTarget = cat;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		deleting = true;
		errorMessage = "";
		successMessage = "";
		const result = await universityAdminApi.deleteCategory(deleteTarget.id);
		if (result.ok) {
			showDeleteConfirm = false;
			successMessage = t("university.category_deleted", "Category deleted");
			deleteTarget = null;
			load();
		} else {
			errorMessage = result.error;
		}
		deleting = false;
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t("university.categories_title", "University Track Categories")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("university.categories_title", "University Track Categories")}
	description={t("university.categories_desc", "Manage the canonical category list for university marketplace tracks")}
	actions={[
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "outline" as const, onclick: load },
		...(canManage ? [{ label: t("university.add_category", "Add Category"), icon: Plus, onclick: openCreate }] : [])
	]}
/>

{#if errorMessage}
	<div class="mb-4 flex items-center justify-between rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		<span>{errorMessage}</span>
		<Button variant="outline" size="sm" onclick={load}>{t("common.retry", "Retry")}</Button>
	</div>
{/if}
{#if successMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-success); background-color: color-mix(in srgb, var(--color-success) 8%, transparent); color: var(--color-success)">
		{successMessage}
	</div>
{/if}

<div class="mt-4">
{#if loading}
	<TableSkeleton rows={5} />
{:else}
	<DataTable
		{columns}
		data={categories}
		pageSize={25}
		emptyMessage={t("university.no_categories", "No categories found")}
		searchable
		searchPlaceholder={t("university.search_categories", "Search categories...")}
		onsearch={(q) => { searchQuery = q; load(); }}
		filters={statusFilters}
		bind:filterValue={filterValues}
		onfilterapply={() => { load(); }}
		onfilterclear={() => { load(); }}
		actions={rowActions}
	/>
{/if}
</div>

<!-- Create/Edit SlideOver -->
<SlideOver bind:open={showSlideOver} title={editingId ? t("university.edit_category", "Edit Category") : t("university.add_category", "Add Category")}>
	<div class="space-y-4">
		<div>
			<label for="cat-name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("university.category_name", "Category Name")}</label>
			<input
				id="cat-name"
				type="text"
				class="w-full rounded-lg border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				placeholder={t('university.categories.ph_eg_software_engineering', 'e.g. Software Engineering')}
				bind:value={form.name}
				oninput={() => { if (!editingId) form.slug = slugify(form.name); }}
			/>
		</div>
		<div>
			<label for="cat-slug" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("university.slug", "Slug")}</label>
			<input
				id="cat-slug"
				type="text"
				class="w-full rounded-lg border px-3 py-2 text-sm font-mono"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				placeholder="software-engineering"
				bind:value={form.slug}
			/>
		</div>
		<div>
			<label for="cat-sort" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("university.sort_order", "Sort Order")}</label>
			<input
				id="cat-sort"
				type="number"
				class="w-full rounded-lg border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={form.sort_order}
			/>
		</div>
		<div class="flex items-center gap-2">
			<input id="cat-active" type="checkbox" bind:checked={form.is_active} />
			<label for="cat-active" class="text-sm" style="color: var(--color-foreground)">{t("university.active", "Active")}</label>
		</div>
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={() => { showSlideOver = false; }}>{t("common.cancel", "Cancel")}</Button>
			<Button onclick={submit} disabled={saving || !form.name.trim()}>
				{saving ? t("common.processing", "Saving...") : (editingId ? t("common.update", "Update") : t("common.create", "Create"))}
			</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t("university.confirm_delete_category", "Delete Category")} onclose={() => { showDeleteConfirm = false; }}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-muted)">
			{t("university.delete_category_confirm", "Are you sure you want to delete")}
			<strong>{deleteTarget?.name}</strong>?
		</p>
		<p class="text-xs" style="color: var(--color-muted)">
			{t("university.delete_category_warning", "This will fail if any tracks still reference this category.")}
		</p>
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={() => { showDeleteConfirm = false; }}>{t("common.cancel", "Cancel")}</Button>
			<Button variant="destructive" onclick={executeDelete} disabled={deleting}>
				{deleting ? t("common.processing", "Deleting...") : t("common.delete", "Delete")}
			</Button>
		</div>
	</div>
</Modal>

