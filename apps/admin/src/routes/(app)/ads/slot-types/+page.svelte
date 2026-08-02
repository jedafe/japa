<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { Plus, Pencil, Trash2, RefreshCw } from "lucide-svelte";
	import { adSlotTypesApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	type SlotType = {
		id: string;
		slot_name: string;
		description?: string | null;
		is_active?: boolean;
		max_display_per_page?: number;
		created_at: string;
		updated_at: string;
	};

	let slotTypes = $state<SlotType[]>([]);
	let loading = $state(true);
	let error = $state("");
	let success = $state("");
	let selectedIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	const canManage = $derived(hasPermission("ad.campaign.manage"));

	// Modal state
	let showModal = $state(false);
	let modalMode = $state<"create" | "edit">("create");
	let editingSlotType = $state<SlotType | null>(null);
	let saving = $state(false);

	// Delete state
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<SlotType | null>(null);

	// Form state
	let slotName = $state("");
	let description = $state("");
	let isActive = $state(true);
	let maxDisplay = $state("");

	const columns = $derived([
		{
			key: "slot_name",
			label: t("slot_types.col_name", "Name"),
			sortable: true,
			render: (v: string) =>
				`<span class="font-medium">${v ?? "—"}</span>`
		},
		{
			key: "description",
			label: t("slot_types.col_description", "Description"),
			render: (v: string) =>
				`<span class="text-xs text-muted">${v ?? "—"}</span>`
		},
		{
			key: "max_display_per_page",
			label: t("slot_types.col_max_display", "Max Display"),
			sortable: true,
			render: (v: number) => (v != null ? String(v) : "—")
		},
		{
			key: "is_active",
			label: t("common.status", "Status"),
			sortable: true,
			render: (v: boolean) =>
				v
					? `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold bg-success/15 text-success border-success">${t("common.active", "Active")}</span>`
					: `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold bg-surface text-foreground border-border">${t("common.inactive", "Inactive")}</span>`
		},
		{
			key: "actions",
			label: t("common.actions", "Actions"),
			class: "w-24",
			componentSnippet: SlotTypeActionsCell
		}
	]);

	async function loadSlotTypes() {
		loading = true;
		error = "";
		try {
			const result = await adSlotTypesApi.list('page_size=100');
			if (result.ok) {
				const resp = result.data as Record<string, unknown> | undefined;
				slotTypes = (Array.isArray(resp) ? resp : (resp?.data as SlotType[]) ?? []) as SlotType[];
			} else {
				error = result.error ?? t("slot_types.load_error", "Failed to load slot types");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("slot_types.load_error", "Failed to load slot types");
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		modalMode = "create";
		editingSlotType = null;
		slotName = "";
		description = "";
		isActive = true;
		maxDisplay = "";
		showModal = true;
	}

	function openEdit(slotType: SlotType) {
		modalMode = "edit";
		editingSlotType = slotType;
		slotName = slotType.slot_name;
		description = slotType.description ?? "";
		isActive = slotType.is_active ?? true;
		maxDisplay = slotType.max_display_per_page != null ? String(slotType.max_display_per_page) : "";
		showModal = true;
	}

	async function saveSlotType() {
		if (!slotName.trim()) return;
		saving = true;
		error = "";
		try {
			const body: Record<string, unknown> = {
				name: slotName.trim()
			};
			if (description.trim()) body.description = description.trim();
			if (maxDisplay) body.max_display_per_page = Number(maxDisplay);
			body.is_active = isActive;

			const result = editingSlotType
				? await adSlotTypesApi.update(editingSlotType.id, body)
				: await adSlotTypesApi.create(body as { name: string });

			if (result.ok) {
				showModal = false;
				success = editingSlotType
					? t("slot_types.updated", "Slot type updated")
					: t("slot_types.created", "Slot type created");
				loadSlotTypes();
			} else {
				error = result.error ?? t("common.error", "Failed to save slot type");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to save slot type");
		} finally {
			saving = false;
		}
	}

	function openDelete(slotType: SlotType) {
		deleteTarget = slotType;
		showDeleteConfirm = true;
	}

	async function confirmDelete() {
		if (!deleteTarget) return;
		try {
			const result = await adSlotTypesApi.delete(deleteTarget.id);
			if (result.ok) {
				showDeleteConfirm = false;
				deleteTarget = null;
				success = t("slot_types.deleted", "Slot type deleted");
				loadSlotTypes();
			} else {
				error = result.error ?? t("common.error", "Failed to delete slot type");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to delete slot type");
		} finally {
			// loading state is managed by the calling context
		}
	}

	async function bulkDelete() {
		if (selectedIds.size === 0) return;
		bulkDeleting = true;
		error = "";
		try {
			let successCount = 0;
			let failCount = 0;
			for (const id of selectedIds) {
				const result = await adSlotTypesApi.delete(id);
				if (result.ok) {
					successCount++;
				} else {
					failCount++;
				}
			}
			selectedIds = new Set();
			if (failCount > 0 && successCount > 0) {
				error = t("slot_types.bulk_partial_failure", "{failCount} of {totalCount} deletions failed")
					.replace("{failCount}", String(failCount))
					.replace("{totalCount}", String(successCount + failCount));
			} else if (failCount > 0) {
				error = t("common.error", "Failed to delete selected slot types");
			} else {
				success = t("slot_types.bulk_deleted", "Selected slot types deleted");
			}
			loadSlotTypes();
		} catch {
			error = t("common.error", "Failed to delete selected slot types");
		} finally {
			bulkDeleting = false;
		}
	}

	$effect(() => {
		loadSlotTypes();
	});
</script>

<svelte:head>
	<title>{t("slot_types.title", "Slot Types")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("slot_types.title", "Slot Types")}
	description={t("slot_types.description", "Manage ad placement slot types — define dimensions, display limits, and availability")}
	actions={[
		...(canManage ? [{ label: t("slot_types.create", "Create Slot Type"), icon: Plus, onclick: openCreate }] : []),
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost", onclick: loadSlotTypes }
	]}
/>

{#if error}
	<div role="alert" class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{error}
	</div>
{/if}
{#if success}
	<div role="alert" class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)">
		{success}
	</div>
{/if}

<Card>
	<CardContent>
		{#if loading}
			<TableSkeleton rows={8} columns={columns.length} />
		{:else}
			<DataTable {columns} data={slotTypes} pageSize={20} selectable bind:selectedIds searchable>
				{#snippet bulkActions()}
					{#if canManage}
						<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting || selectedIds.size === 0}>
							<Trash2 class="h-3.5 w-3.5 mr-1" />
							{bulkDeleting ? t("common.deleting", "Deleting...") : t("common.deleteSelected", "Delete Selected")} ({selectedIds.size})
						</Button>
					{/if}
				{/snippet}
			</DataTable>
		{/if}
	</CardContent>
</Card>

<!-- Create/Edit Modal -->
<Modal bind:open={showModal} title={modalMode === "create" ? t("slot_types.create", "Create Slot Type") : t("slot_types.edit", "Edit Slot Type")}>
	<div class="space-y-4">
		<div>
			<label for="slot-name" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("slot_types.name", "Name")} *</label>
			<input
				id="slot-name"
				type="text"
				bind:value={slotName}
				class="mt-1 w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				placeholder={t("slot_types.name_placeholder", "e.g. Leaderboard Banner")}
			/>
		</div>
		<div>
			<label for="slot-desc" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("slot_types.description", "Description")}</label>
			<textarea
				id="slot-desc"
				rows="2"
				bind:value={description}
				class="mt-1 w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				placeholder={t("slot_types.desc_placeholder", "Optional description for this slot type")}
			></textarea>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="slot-max-display" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("slot_types.max_display", "Max Display Per Page")}</label>
				<input
					id="slot-max-display"
					type="number"
					min="0"
					bind:value={maxDisplay}
					class="mt-1 w-full rounded-lg border p-2 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					placeholder={t("slot_types.max_display_placeholder", "Unlimited")}
				/>
			</div>
			<div class="flex items-center gap-2 pt-6">
				<input id="slot-active" type="checkbox" bind:checked={isActive} class="h-4 w-4 rounded" style="accent-color: var(--color-primary)" />
				<label for="slot-active" class="text-sm font-medium" style="color: var(--color-foreground)">{t("slot_types.active", "Active")}</label>
			</div>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showModal = false}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={saveSlotType} disabled={saving || !slotName.trim()}>{saving ? t("common.saving", "Saving...") : t("common.save", "Save")}</Button>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t("common.confirmDelete", "Confirm Delete")}>
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("slot_types.delete_confirm", "Delete slot type")} <strong class="font-mono">{deleteTarget.slot_name}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">{t("common.cannotUndo", "This action cannot be undone.")}</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showDeleteConfirm = false}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmDelete}>{t("common.delete", "Delete")}</Button>
	</div>
</Modal>

<!-- SlotTypeActionsCell snippet -->
{#snippet SlotTypeActionsCell({ value, row }: { value: any; row: SlotType })}
	<div class="flex items-center gap-1">
		{#if canManage}
			<Button variant="ghost" size="sm" onclick={() => openEdit(row)} aria-label={t("slot_types.edit", "Edit")}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => openDelete(row)} style="color: var(--color-danger)" aria-label={t("slot_types.delete", "Delete")}>
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
