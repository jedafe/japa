<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { Badge } from "$lib/components/ui/badge";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { Plus, Pencil, Trash2, RefreshCw, ArrowUp, ArrowDown, Menu as MenuIcon } from "lucide-svelte";
	import { Select } from '$lib/components/ui/select';
	import { menusAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	type MenuItem = {
		id: string;
		label: string;
		url: string;
		parent_id?: string | null;
		sort_order: number;
		is_visible: boolean;
	};

	type Menu = {
		id: string;
		name: string;
		slug: string;
		location: string;
		items?: MenuItem[];
		created_at: string;
	};

	let menus = $state<Menu[]>([]);
	let loading = $state(true);
	let error = $state("");
	let success = $state("");

	// Menu modal state
	let showMenuModal = $state(false);
	let menuModalMode = $state<"create" | "edit">("create");
	let editingMenu = $state<Menu | null>(null);
	let savingMenu = $state(false);
	let menuName = $state("");
	let menuSlug = $state("");
	let menuLocation = $state("");

	// Item modal state
	let showItemModal = $state(false);
	let editingItem = $state<MenuItem | null>(null);
	let savingItem = $state(false);
	let itemLabel = $state("");
	let itemUrl = $state("");
	let itemParent = $state("");
	let itemVisible = $state(true);
	let activeMenuForItems = $state<Menu | null>(null);

	// Selected menu for viewing items
	let selectedMenu = $state<Menu | null>(null);

	let selectedMenuIds = $state<Set<string>>(new Set());
	let selectedItemIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	const canManageMenus = $derived(hasPermission("menus.manage"));

	function viewMenuItems(menu: Menu) {
		selectedMenu = menu;
	}

	const menuColumns = [
		{ key: "name", label: t("menus.name", "Name"), sortable: true },
		{ key: "slug", label: t("menus.slug", "Slug"), sortable: true },
		{ key: "location", label: t("menus.location", "Location"), sortable: true },
		{
			key: "items_count",
			label: t("menus.items", "Items"),
			sortable: true,
			render: (_: any, row: Menu) => String(row.items?.length ?? 0)
		},
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-40",
			componentSnippet: MenuActionsCell
		}
	];

	const itemColumns = [
		{ key: "label", label: t("menus.item_label", "Label"), sortable: true },
		{ key: "url", label: t("menus.item_url", "URL"), sortable: true },
		{ key: "is_visible", label: t("common.visible", "Visible"), sortable: true,
			render: (v: boolean) => v ? t("common.yes", "Yes") : t("common.no", "No")
		},
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-48",
			componentSnippet: ItemActionsCell
		}
	];

	async function loadMenus() {
		loading = true;
		error = "";
		try {
			const result = await menusAdminApi.list();
			if (result.ok) {
				menus = ((result.data as { items: Menu[] })?.items) ?? [];
			} else {
				error = result.error ?? t("common.error", "Failed to load menus");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load menus");
		} finally {
			loading = false;
		}
	}

	// Menu CRUD
	function openCreateMenu() {
		menuModalMode = "create";
		editingMenu = null;
		menuName = "";
		menuSlug = "";
		menuLocation = "";
		showMenuModal = true;
	}

	function openEditMenu(menu: Menu | null) {
		if (!menu) return;
		menuModalMode = "edit";
		editingMenu = menu;
		menuName = menu.name;
		menuSlug = menu.slug;
		menuLocation = menu.location;
		showMenuModal = true;
	}

	async function saveMenu() {
		if (!menuName.trim() || !menuSlug.trim()) return;
		savingMenu = true;
		error = "";
		try {
			const body = {
				name: menuName.trim(),
				slug: menuSlug.trim().toLowerCase().replace(/\s+/g, "-"),
				location: menuLocation.trim()
			};
			const result = editingMenu
				? await menusAdminApi.update(editingMenu.id, body)
				: await menusAdminApi.create(body);

			if (result.ok) {
				showMenuModal = false;
				success = editingMenu ? t("menus.updated", "Menu updated") : t("menus.created", "Menu created");
				loadMenus();
			} else {
				error = result.error ?? t("common.error", "Failed to save menu");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to save menu");
		} finally {
			savingMenu = false;
		}
	}

	let deleteMenuTarget = $state<Menu | null>(null);
	let showDeleteMenuConfirm = $state(false);
	function openDeleteMenu(menu: Menu | null) {
		if (!menu) return;
		deleteMenuTarget = menu;
		showDeleteMenuConfirm = true;
	}
	async function confirmDeleteMenu() {
		if (!deleteMenuTarget) return;
		const deletedId = deleteMenuTarget.id;
		const result = await menusAdminApi.delete(deletedId);
		if (result.ok) {
			showDeleteMenuConfirm = false;
			deleteMenuTarget = null;
			success = t("menus.deleted", "Menu deleted");
			if (selectedMenu?.id === deletedId) selectedMenu = null;
			loadMenus();
		} else {
			error = result.error ?? t("common.error", "Failed to delete menu");
		}
	}

	// Item CRUD
	function openCreateItem() {
		if (!selectedMenu) return;
		activeMenuForItems = selectedMenu;
		editingItem = null;
		itemLabel = "";
		itemUrl = "";
		itemParent = "";
		itemVisible = true;
		showItemModal = true;
	}

	async function openEditItem(itemId: string) {
		if (!selectedMenu) return;
		const item = (selectedMenu.items ?? []).find((i) => i.id === itemId);
		if (!item) return;
		activeMenuForItems = selectedMenu;
		editingItem = item;
		itemLabel = item.label;
		itemUrl = item.url;
		itemParent = item.parent_id ?? "";
		itemVisible = item.is_visible;
		showItemModal = true;
	}

	async function saveItem() {
		if (!itemLabel.trim() || !itemUrl.trim() || !activeMenuForItems) return;
		savingItem = true;
		error = "";
		try {
			const body = {
				label: itemLabel.trim(),
				url: itemUrl.trim(),
				parent_id: itemParent || null,
				is_visible: itemVisible
			};
			const result = editingItem
				? await menusAdminApi.updateItem(activeMenuForItems.id, editingItem.id, body)
				: await menusAdminApi.addItem(activeMenuForItems.id, body);

			if (result.ok) {
				showItemModal = false;
				success = editingItem ? t("menus.item_updated", "Item updated") : t("menus.item_created", "Item created");
				await loadMenus();
				// Refresh selected menu
				const refreshed = menus.find((m) => m.id === activeMenuForItems?.id);
				if (refreshed) selectedMenu = refreshed;
			} else {
				error = result.error ?? t("common.error", "Failed to save item");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to save item");
		} finally {
			savingItem = false;
		}
	}

	let deleteItemTarget = $state<string | null>(null);
	let showDeleteItemConfirm = $state(false);
	function openDeleteItem(itemId: string) {
		deleteItemTarget = itemId;
		showDeleteItemConfirm = true;
	}
	async function confirmDeleteItem() {
		if (!deleteItemTarget || !selectedMenu) return;
		const result = await menusAdminApi.deleteItem(selectedMenu.id, deleteItemTarget);
		if (result.ok) {
			showDeleteItemConfirm = false;
			deleteItemTarget = null;
			success = t("menus.item_deleted", "Item deleted");
			await loadMenus();
			const refreshed = menus.find((m) => m.id === selectedMenu?.id);
			if (refreshed) selectedMenu = refreshed;
		} else {
			error = result.error ?? t("common.error", "Failed to delete item");
		}
	}

	async function moveItem(itemId: string, direction: number) {
		if (!selectedMenu) return;
		const items = [...(selectedMenu.items ?? [])].sort((a, b) => a.sort_order - b.sort_order);
		const idx = items.findIndex((i) => i.id === itemId);
		const swapIdx = idx + direction;
		if (idx === -1 || swapIdx < 0 || swapIdx >= items.length) return;

		const reordered = [...items];
		[reordered[idx], reordered[swapIdx]] = [reordered[swapIdx], reordered[idx]];
		const itemIds = reordered.map((i) => i.id);

		const result = await menusAdminApi.reorderItems(selectedMenu.id, { item_ids: itemIds });
		if (result.ok) {
			await loadMenus();
			const refreshed = menus.find((m) => m.id === selectedMenu?.id);
			if (refreshed) selectedMenu = refreshed;
		} else {
			error = result.error ?? t("common.error", "Failed to reorder items");
		}
	}

	function backToMenus() {
		selectedMenu = null;
		selectedItemIds = new Set();
	}

	async function bulkDeleteMenus() {
		if (selectedMenuIds.size === 0) return;
		bulkDeleting = true;
		error = "";
		try {
			for (const id of selectedMenuIds) {
				await menusAdminApi.delete(id);
			}
			selectedMenuIds = new Set();
			loadMenus();
		} catch {
			error = t("common.error", "Failed to delete menus");
		}
		bulkDeleting = false;
	}

	async function bulkDeleteItems() {
		if (selectedItemIds.size === 0 || !selectedMenu) return;
		bulkDeleting = true;
		error = "";
		try {
			for (const itemId of selectedItemIds) {
				await menusAdminApi.deleteItem(selectedMenu.id, itemId);
			}
			selectedItemIds = new Set();
			await loadMenus();
			const refreshed = menus.find((m) => m.id === selectedMenu?.id);
			if (refreshed) selectedMenu = refreshed;
		} catch {
			error = t("common.error", "Failed to delete items");
		}
		bulkDeleting = false;
	}

	loadMenus();
</script>

<svelte:head>
	<title>{t("menus.title", "Menus")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("menus.title", "Menus")}
	description={t("menus.description", "Manage navigation menus and their items")}
	actions={[
		...(selectedMenu ? [{ label: t("menus.back", "Back to Menus"), icon: MenuIcon, variant: "ghost" as const, onclick: backToMenus }] : []),
		...(canManageMenus ? [{ label: selectedMenu ? t("menus.add_item", "Add Item") : t("menus.create", "Create Menu"), icon: Plus, onclick: selectedMenu ? openCreateItem : openCreateMenu }] : []),
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost" as const, onclick: loadMenus }
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

{#if !selectedMenu}
	<Card>
		<CardContent>
			{#if loading}
				<TableSkeleton rows={6} columns={menuColumns.length} />
			{:else if menus.length === 0}
				<div class="py-12 text-center text-sm" style="color: var(--color-muted)">
					{t("menus.empty", "No menus found")}
				</div>
			{:else}
				<!-- wire the search box to re-fetch menus; API ignores q today (see report) -->
				<DataTable columns={menuColumns} data={menus} pageSize={10} searchable onsearch={() => loadMenus()} selectable bind:selectedIds={selectedMenuIds}>
					{#snippet bulkActions()}
						<Button variant="destructive" size="sm" onclick={bulkDeleteMenus} disabled={bulkDeleting || selectedMenuIds.size === 0}>
							<Trash2 class="h-3.5 w-3.5 mr-1" />
							{bulkDeleting ? t("common.deleting", "Deleting...") : t("common.deleteSelected", "Delete Selected")} ({selectedMenuIds.size})
						</Button>
					{/snippet}
				</DataTable>
			{/if}
		</CardContent>
	</Card>
{:else}
	<Card>
		<CardHeader>
			<h3 class="text-lg font-medium" style="color: var(--color-foreground)">
				{selectedMenu.name} — {t("menus.items", "Items")}
			</h3>
		</CardHeader>
		<CardContent>
			{#if loading}
				<TableSkeleton rows={6} columns={itemColumns.length} />
			{:else if (selectedMenu.items ?? []).length === 0}
				<div class="py-12 text-center text-sm" style="color: var(--color-muted)">
					{t("menus.no_items", "No items in this menu")}
				</div>
			{:else}
				<DataTable columns={itemColumns} data={[...(selectedMenu.items ?? [])].sort((a, b) => a.sort_order - b.sort_order)} pageSize={(selectedMenu.items ?? []).length} selectable bind:selectedIds={selectedItemIds}>
					{#snippet bulkActions()}
						<Button variant="destructive" size="sm" onclick={bulkDeleteItems} disabled={bulkDeleting || selectedItemIds.size === 0}>
							<Trash2 class="h-3.5 w-3.5 mr-1" />
							{bulkDeleting ? t("common.deleting", "Deleting...") : t("common.deleteSelected", "Delete Selected")} ({selectedItemIds.size})
						</Button>
					{/snippet}
				</DataTable>
			{/if}
		</CardContent>
	</Card>
{/if}

<!-- Menu Create/Edit Modal -->
<Modal bind:open={showMenuModal} title={menuModalMode === "create" ? t("menus.create", "Create Menu") : t("menus.edit", "Edit Menu")}>
	<div class="space-y-4">
		<div>
			<label for="menu-name" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("menus.name", "Name")}</label>
			<input id="menu-name" type="text" bind:value={menuName} class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="e.g., Primary Nav" />
		</div>
		<div>
			<label for="menu-slug" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("menus.slug", "Slug")}</label>
			<input id="menu-slug" type="text" bind:value={menuSlug} class="mt-1 w-full rounded-lg border p-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="primary-nav" />
		</div>
		<div>
			<label for="menu-location" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("menus.location", "Location")}</label>
			<input id="menu-location" type="text" bind:value={menuLocation} class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="header" />
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showMenuModal = false}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={saveMenu} disabled={savingMenu || !menuName.trim() || !menuSlug.trim()}>
			{savingMenu ? t("common.saving", "Saving...") : t("common.save", "Save")}
		</Button>
	</div>
</Modal>

<!-- Item Create/Edit Modal -->
<Modal bind:open={showItemModal} title={editingItem ? t("menus.edit_item", "Edit Item") : t("menus.add_item", "Add Item")}>
	<div class="space-y-4">
		<div>
			<label for="item-label" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("menus.item_label", "Label")}</label>
			<input id="item-label" type="text" bind:value={itemLabel} class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="Home" />
		</div>
		<div>
			<label for="item-url" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("menus.item_url", "URL")}</label>
			<input id="item-url" type="text" bind:value={itemUrl} class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="/" />
		</div>
		{#if selectedMenu && (selectedMenu.items ?? []).length > 0}
			<div>
				<label for="item-parent" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("menus.parent", "Parent")}</label>
				<Select
					options={[
						{ value: "", label: t("menus.no_parent", "No parent") },
						...(selectedMenu?.items ?? []).filter((i) => !editingItem || i.id !== editingItem.id).map((item) => ({
							value: item.id,
							label: item.label
						}))
					]}
					value={itemParent}
					onchange={(val) => (itemParent = val)}
				/>
			</div>
		{/if}
		<div class="flex items-center gap-2">
			<input id="item-visible" type="checkbox" bind:checked={itemVisible} class="h-4 w-4 rounded" style="accent-color: var(--color-primary)" />
			<label for="item-visible" class="text-sm font-medium" style="color: var(--color-foreground)">{t("common.visible", "Visible")}</label>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showItemModal = false}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={saveItem} disabled={savingItem || !itemLabel.trim() || !itemUrl.trim()}>
			{savingItem ? t("common.saving", "Saving...") : t("common.save", "Save")}
		</Button>
	</div>
</Modal>

<!-- Delete Menu Confirm -->
<Modal bind:open={showDeleteMenuConfirm} title={t("common.confirmDelete", "Confirm Delete")}>
	{#if deleteMenuTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("menus.delete_confirm", "Delete menu")} <strong>{deleteMenuTarget.name}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">{t("common.cannotUndo", "This action cannot be undone.")}</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showDeleteMenuConfirm = false}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmDeleteMenu}>{t("common.delete", "Delete")}</Button>
	</div>
</Modal>

<!-- Delete Item Confirm -->
<Modal bind:open={showDeleteItemConfirm} title={t("common.confirmDelete", "Confirm Delete")}>
	<p class="text-sm" style="color: var(--color-foreground)">{t("menus.delete_item_confirm", "Delete this menu item?")}</p>
	<p class="mt-2 text-sm" style="color: var(--color-danger)">{t("common.cannotUndo", "This action cannot be undone.")}</p>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showDeleteItemConfirm = false}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmDeleteItem}>{t("common.delete", "Delete")}</Button>
	</div>
</Modal>

<!-- MenuActionsCell -->
{#snippet MenuActionsCell({ value, row }: { value: any; row: Menu })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => viewMenuItems(row)} style="color: var(--color-primary)">
			{t("menus.items", "Items")}
		</Button>
		{#if canManageMenus}
			<Button variant="ghost" size="sm" onclick={() => openEditMenu(row)}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => openDeleteMenu(row)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}

<!-- ItemActionsCell -->
{#snippet ItemActionsCell({ value, row }: { value: any; row: MenuItem })}
	<div class="flex items-center gap-1">
		{#if canManageMenus}
			<Button variant="ghost" size="sm" onclick={() => moveItem(row.id, -1)}>
				<ArrowUp class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => moveItem(row.id, 1)}>
				<ArrowDown class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => openEditItem(row.id)}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => openDeleteItem(row.id)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
