<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { Badge } from "$lib/components/ui/badge";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { Plus, Pencil, Trash2, RefreshCw, Star } from "lucide-svelte";
	import { pageBuilderAdminApiExtended } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	type PageTheme = {
		id: string;
		name: string;
		is_default: boolean;
		created_at: string;
	};

	let themes = $state<PageTheme[]>([]);
	let loading = $state(true);
	let error = $state("");
	let success = $state("");

	// Search term wired into the themes load via the DataTable search box
	let searchTerm = $state("");

	// Modal
	let showModal = $state(false);
	let modalMode = $state<"create" | "edit">("create");
	let editingTheme = $state<PageTheme | null>(null);
	let saving = $state(false);
	let themeName = $state("");

	// Set Default confirmation
	let showSetDefaultConfirm = $state(false);
	let setDefaultTarget = $state<PageTheme | null>(null);

	// Delete confirmation
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<PageTheme | null>(null);

	const canManageThemes = $derived(hasPermission("themes.manage"));

	function confirmSetDefault(theme: PageTheme) {
		setDefaultTarget = theme;
		showSetDefaultConfirm = true;
	}

	async function executeSetDefault() {
		if (!setDefaultTarget) return;
		const result = await pageBuilderAdminApiExtended.setDefaultTheme(setDefaultTarget.id);
		if (result.ok) {
			showSetDefaultConfirm = false;
			setDefaultTarget = null;
			success = t("themes.default_set", "Default theme updated");
			loadThemes();
		} else {
			error = result.error ?? t("common.error", "Failed to set default theme");
		}
	}

	const columns = [
		{ key: "name", label: t("themes.name", "Name"), sortable: true },
		{ key: "is_default", label: t("themes.default", "Default"), sortable: true,
			render: (v: boolean) => v ? t("common.yes", "Yes") : t("common.no", "No")
		},
		{ key: "created_at", label: t("themes.created", "Created"), sortable: true,
			render: (v: string) => new Date(v).toLocaleDateString()
		},
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-48",
			component: ThemeActionsCell
		}
	];

	async function loadThemes() {
		loading = true;
		error = "";
		try {
			const params = new URLSearchParams();
			if (searchTerm.trim()) params.set("q", searchTerm.trim());
			const qs = params.toString();
			const result = await pageBuilderAdminApiExtended.listThemes(qs);
			if (result.ok) {
				themes = (result.data as PageTheme[]) ?? [];
			} else {
				error = result.error ?? t("common.error", "Failed to load themes");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load themes");
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		modalMode = "create";
		editingTheme = null;
		themeName = "";
		showModal = true;
	}

	function openEdit(theme: PageTheme | null) {
		if (!theme) return;
		modalMode = "edit";
		editingTheme = theme;
		themeName = theme.name;
		showModal = true;
	}

	async function saveTheme() {
		if (!themeName.trim()) return;
		saving = true;
		error = "";
		try {
			const result = editingTheme
				? await pageBuilderAdminApiExtended.updateTheme(editingTheme.id, { name: themeName.trim() })
				: await pageBuilderAdminApiExtended.createTheme({ name: themeName.trim() });

			if (result.ok) {
				showModal = false;
				success = editingTheme ? t("themes.updated", "Theme updated") : t("themes.created", "Theme created");
				loadThemes();
			} else {
				error = result.error ?? t("common.error", "Failed to save theme");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to save theme");
		} finally {
			saving = false;
		}
	}

	function openDelete(theme: PageTheme | null) {
		if (!theme) return;
		deleteTarget = theme;
		showDeleteConfirm = true;
	}

	async function confirmDelete() {
		if (!deleteTarget) return;
		const result = await pageBuilderAdminApiExtended.deleteTheme(deleteTarget.id);
		if (result.ok) {
			showDeleteConfirm = false;
			deleteTarget = null;
			success = t("themes.deleted", "Theme deleted");
			loadThemes();
		} else {
			error = result.error ?? t("common.error", "Failed to delete theme");
		}
	}

	loadThemes();
</script>

<svelte:head>
	<title>{t("themes.title", "Page Themes")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("themes.title", "Page Themes")}
	description={t("themes.description", "Manage page builder themes")}
	actions={[
		...(canManageThemes ? [{ label: t("themes.create", "Create Theme"), icon: Plus, onclick: openCreate }] : []),
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost", onclick: loadThemes }
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
	<CardContent>
		{#if loading}
			<TableSkeleton rows={6} columns={columns.length} />
		{:else if themes.length === 0}
			<div class="py-12 text-center text-sm" style="color: var(--color-muted)">
				{t("themes.empty", "No themes found")}
			</div>
		{:else}
			<DataTable {columns} data={themes} pageSize={10} searchable onsearch={(q) => { searchTerm = q; loadThemes(); }} />
		{/if}
	</CardContent>
</Card>

<!-- Create/Edit Modal -->
<Modal bind:open={showModal} title={modalMode === "create" ? t("themes.create", "Create Theme") : t("themes.edit", "Edit Theme")}>
	<div>
		<label for="theme-name" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("themes.name", "Name")}</label>
		<input id="theme-name" type="text" bind:value={themeName} class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="e.g., Dark Theme" />
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showModal = false}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={saveTheme} disabled={saving || !themeName.trim()}>
			{saving ? t("common.saving", "Saving...") : t("common.save", "Save")}
		</Button>
	</div>
</Modal>

<!-- Set Default Confirm -->
<Modal bind:open={showSetDefaultConfirm} title={t("themes.set_default", "Set Default Theme")}>
	{#if setDefaultTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("themes.set_default_confirm", "Set")} <strong>{setDefaultTarget.name}</strong> {t("themes.as_default", "as the default theme?")}
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showSetDefaultConfirm = false}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={executeSetDefault}>{t("themes.set_default", "Set Default")}</Button>
	</div>
</Modal>

<!-- Delete Confirm -->
<Modal bind:open={showDeleteConfirm} title={t("common.confirmDelete", "Confirm Delete")}>
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("themes.delete_confirm", "Delete theme")} <strong>{deleteTarget.name}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">{t("common.cannotUndo", "This action cannot be undone.")}</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteConfirm = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmDelete}>{t("common.delete", "Delete")}</Button>
	</div>
</Modal>

<!-- ThemeActionsCell -->
{#snippet ThemeActionsCell(row: PageTheme)}
	<div class="flex items-center gap-1">
		{#if canManageThemes}
			{#if !row.is_default}
				<Button variant="ghost" size="sm" onclick={() => confirmSetDefault(row)} style="color: var(--color-primary)">
					<Star class="h-3.5 w-3.5" />
				</Button>
			{/if}
			<Button variant="ghost" size="sm" onclick={() => openEdit(row)}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			{#if !row.is_default}
				<Button variant="ghost" size="sm" onclick={() => openDelete(row)} style="color: var(--color-danger)">
					<Trash2 class="h-3.5 w-3.5" />
				</Button>
			{/if}
		{/if}
	</div>
{/snippet}
