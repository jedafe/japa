<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { Plus, Trash2, RefreshCw, ArrowLeft, Shield } from "lucide-svelte";
	import { page } from "$app/state";
	import { goto } from "$app/navigation";
	import { spacesAdminApi, spacesModeratorsAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	const spaceId = $derived(page.params.spaceId ?? "");

	type Moderator = {
		user_id: string;
		username: string;
		added_at: string;
	};

	type SpaceDetail = {
		id: string;
		name: string;
		owner: string;
		member_count: number;
		status: string;
		space_type: string;
		description?: string;
	};

	let space = $state<SpaceDetail | null>(null);
	let moderators = $state<Moderator[]>([]);
	let loading = $state(true);
	let loadingModerators = $state(true);
	let error = $state("");
	let success = $state("");
	let searchQuery = $state("");
	let selectedIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	// Add moderator modal
	let showAddModal = $state(false);
	let newModeratorUsername = $state("");
	let adding = $state(false);

	// Remove moderator
	let showRemoveConfirm = $state(false);
	let removeTarget = $state<Moderator | null>(null);

	const canManageModerators = $derived(hasPermission("spaces.manage"));

	const columns = [
		{ key: "username", label: t("spaces.moderator", "Moderator"), sortable: true },
		{ key: "added_at", label: t("spaces.added_at", "Added"), sortable: true,
			render: (v: string) => new Date(v).toLocaleDateString()
		},
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-32",
			component: ModeratorActionsCell
		}
	];

	async function loadSpace() {
		loading = true;
		error = "";
		try {
			const result = await spacesAdminApi.get(spaceId);
			if (result.ok) {
				space = (result.data as SpaceDetail) ?? null;
			} else {
				error = result.error ?? t("common.error", "Failed to load space");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load space");
		} finally {
			loading = false;
		}
	}

	async function loadModerators() {
		loadingModerators = true;
		try {
			const result = await spacesAdminApi.get(`${spaceId}/moderators`);
			if (result.ok) {
				let items = (result.data as Moderator[]) ?? [];
				if (searchQuery) {
					const q = searchQuery.toLowerCase();
					items = items.filter((m: any) => m.username?.toLowerCase().includes(q) || m.role?.toLowerCase().includes(q));
				}
				moderators = items;
			}
		} catch {
			// moderators endpoint may not return list directly; handled in add/remove
		} finally {
			loadingModerators = false;
		}
	}

	async function addModerator() {
		if (!newModeratorUsername.trim()) return;
		adding = true;
		error = "";
		try {
			const result = await spacesAdminApi.assignModerator(spaceId, { username: newModeratorUsername.trim() });
			if (result.ok) {
				showAddModal = false;
				newModeratorUsername = "";
				success = t("spaces.moderator_added", "Moderator added");
				await loadModerators();
			} else {
				error = result.error ?? t("common.error", "Failed to add moderator");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to add moderator");
		} finally {
			adding = false;
		}
	}

	async function confirmRemove() {
		if (!removeTarget) return;
		const result = await spacesModeratorsAdminApi.remove(spaceId, removeTarget.user_id);
		if (result.ok) {
			showRemoveConfirm = false;
			removeTarget = null;
			success = t("spaces.moderator_removed", "Moderator removed");
			await loadModerators();
		} else {
			error = result.error ?? t("common.error", "Failed to remove moderator");
		}
	}

	async function bulkDelete() {
		if (selectedIds.size === 0) return;
		bulkDeleting = true;
		error = "";
		try {
			for (const userId of selectedIds) {
				await spacesModeratorsAdminApi.remove(spaceId, userId);
			}
			selectedIds = new Set();
			success = t("spaces.moderators_removed", "Moderators removed");
			await loadModerators();
		} catch {
			error = t("common.error", "Failed to remove moderators");
		}
		bulkDeleting = false;
	}

	loadSpace();
	loadModerators();
</script>

<svelte:head>
	<title>{space?.name ?? t("spaces.title", "Space")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={space?.name ?? t("spaces.title", "Space")}
	description={t("spaces.moderators_desc", "Manage space moderators")}
	actions={[
		{ label: t("common.back", "Back"), icon: ArrowLeft, variant: "ghost", onclick: () => goto("/spaces") },
		{ label: t("spaces.add_moderator", "Add Moderator"), icon: Plus, onclick: () => showAddModal = true },
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost", onclick: loadModerators }
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

{#if loading}
	<TableSkeleton rows={4} columns={2} />
{:else if space}
	<Card class="mb-6">
		<CardContent>
			<div class="grid grid-cols-2 gap-4 md:grid-cols-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("spaces.owner", "Owner")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{space.owner}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("spaces.members", "Members")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{space.member_count.toLocaleString()}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("spaces.type", "Type")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{space.space_type || "—"}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("spaces.status", "Status")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{space.status}</p>
				</div>
			</div>
		</CardContent>
	</Card>
{/if}

<Card>
	<CardHeader>
		<div class="flex items-center gap-2">
			<Shield class="h-5 w-5" style="color: var(--color-primary)" />
			<h3 class="text-lg font-medium" style="color: var(--color-foreground)">{t("spaces.moderators", "Moderators")}</h3>
		</div>
	</CardHeader>
	<CardContent>
		{#if loadingModerators}
			<TableSkeleton rows={5} columns={columns.length} />
		{:else if moderators.length === 0}
			<div class="py-12 text-center text-sm" style="color: var(--color-muted)">
				{t("spaces.no_moderators", "No moderators assigned")}
			</div>
		{:else}
			<DataTable {columns} data={moderators} pageSize={10} searchable onsearch={(q) => { searchQuery = q; loadModerators(); }} selectable bind:selectedIds>
				{#snippet bulkActions()}
					<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting || selectedIds.size === 0}>
						<Trash2 class="h-3.5 w-3.5 mr-1" />
						{bulkDeleting ? t("common.deleting", "Removing...") : t("common.deleteSelected", "Remove Selected")} ({selectedIds.size})
					</Button>
				{/snippet}
			</DataTable>
		{/if}
	</CardContent>
</Card>

<!-- Add Moderator Modal -->
<Modal bind:open={showAddModal} title={t("spaces.add_moderator", "Add Moderator")}>
	<div>
		<label for="mod-username" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("spaces.moderator_username", "Username")}</label>
		<input id="mod-username" type="text" bind:value={newModeratorUsername} class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="username" />
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showAddModal = false}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={addModerator} disabled={adding || !newModeratorUsername.trim()}>
			{adding ? t("common.adding", "Adding...") : t("common.add", "Add")}
		</Button>
	</div>
</Modal>

<!-- Remove Confirm -->
<Modal bind:open={showRemoveConfirm} title={t("common.confirmDelete", "Confirm Remove")}>
	{#if removeTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("spaces.remove_moderator_confirm", "Remove")} <strong>{removeTarget.username}</strong> {t("spaces.as_moderator", "as moderator?")}
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showRemoveConfirm = false}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmRemove}>{t("common.remove", "Remove")}</Button>
	</div>
</Modal>

{#snippet ModeratorActionsCell(row: Moderator)}
	<div class="flex items-center gap-1">
		{#if canManageModerators}
			<Button variant="ghost" size="sm" onclick={() => { removeTarget = row; showRemoveConfirm = true; }} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
