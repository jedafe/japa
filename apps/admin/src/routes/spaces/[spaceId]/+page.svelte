<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { siteName } from '$lib/stores/site.svelte';
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import type { ActionItem, BulkAction } from "$lib/components/ui/data-table";
	import { Plus, Trash2, RefreshCw, ArrowLeft, Shield } from "lucide-svelte";
	import Autocomplete, { type AutocompleteItem } from "$lib/components/ui/Autocomplete.svelte";
	import { apiFetch } from "$lib/api";
	import { page } from "$app/state";
	import { goto } from "$app/navigation";
	import { spacesAdminApi, spacesModeratorsAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	const spaceId = $derived(page.params.spaceId ?? "");

	type Moderator = {
		id: string;
		user_id: string;
		space_id: string;
		assigned_by: string;
		assigned_by_user_id: string;
		user_username: string;
		user_display_name?: string;
		created_at: string;
	};

	// GET /admin/spaces/{id} returns { space: <spaces row>, rules: [] } —
	// the row has no owner username or space_type name (those are only
	// joined on the admin LIST query), so show the real columns.
	type SpaceDetail = {
		id: string;
		name: string;
		slug: string;
		level: string;
		visibility: string;
		member_count: number;
		status: string;
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

	// Add moderator modal — user picked via the shared Autocomplete and
	// submitted by user_id (username resolves server-side as fallback).
	let showAddModal = $state(false);
	let newModeratorId = $state("");
	let newModeratorLabel = $state("");
	let adding = $state(false);

	// Remove moderator
	let showRemoveConfirm = $state(false);
	let removeTarget = $state<Moderator | null>(null);

	// POST/DELETE /admin/spaces/{id}/moderators are gated by
	// spaces.moderator.assign — spaces.manage is not a seeded node.
	const canManageModerators = $derived(hasPermission("spaces.moderator.assign"));

	async function userFetcher(q: string): Promise<AutocompleteItem[]> {
		const res = await apiFetch<{ items: { id: string; username: string; display_name?: string }[] }>(
			`/admin/users?page_size=10&search=${encodeURIComponent(q)}`
		);
		if (res.error || !res.data?.items) return [];
		return res.data.items.map((u) => ({ id: u.id, label: u.username, sublabel: u.display_name }));
	}

	function filteredModerators(): Moderator[] {
		const q = searchQuery.toLowerCase().trim();
		if (!q) return moderators;
		return moderators.filter((m) =>
			m.user_username?.toLowerCase().includes(q) ||
			(m.user_display_name ?? "").toLowerCase().includes(q)
		);
	}

	const columns = [
		{ key: "user_username", label: t("spaces.moderator", "Moderator"), sortable: true,
			render: (v: string, row: Moderator) => row.user_display_name || v || "—"
		},
		{ key: "created_at", label: t("spaces.added_at", "Added"), sortable: true,
			render: (v: string) => {
				if (!v) return "—";
				const d = new Date(v);
				return isNaN(d.getTime()) ? "—" : d.toLocaleDateString();
			}
		}
	];

	const rowActions: ActionItem[] = [
		{
			label: t("common.remove", "Remove"),
			icon: Trash2,
			variant: "destructive",
			onclick: (row: Moderator) => { removeTarget = row; showRemoveConfirm = true; },
			show: () => canManageModerators
		}
	];

	const bulkActions: BulkAction[] = $derived(canManageModerators
		? [{ label: t("common.delete_selected", "Remove Selected"), icon: Trash2, variant: "destructive", onclick: bulkDelete }]
		: []);

	async function loadSpace() {
		loading = true;
		error = "";
		try {
			const result = await spacesAdminApi.get(spaceId);
			if (result.ok) {
				const d = result.data as any;
				// Handler returns { space: {...}, rules: [...] } or just the space object
				space = d?.space ?? d ?? null;
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
			const result = await spacesAdminApi.getModerators(spaceId);
			if (result.ok) {
				moderators = (result.data as Moderator[]) ?? [];
			} else {
				error = result.error ?? t("common.error", "Failed to load moderators");
			}
		} catch {
			error = t("common.error", "Failed to load moderators");
		} finally {
			loadingModerators = false;
		}
	}

	async function addModerator() {
		if (!newModeratorId) return;
		adding = true;
		error = "";
		try {
			const result = await spacesModeratorsAdminApi.add(spaceId, { user_id: newModeratorId });
			if (result.ok) {
				showAddModal = false;
				newModeratorId = "";
				newModeratorLabel = "";
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

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0 || bulkDeleting) return;
		bulkDeleting = true;
		error = "";
		success = "";
		try {
			// Map moderator record IDs to user IDs
			const userIds = idArr.map((modId) => {
				const mod = moderators.find((m) => m.id === modId);
				return mod ? mod.user_id : modId;
			});
			const results = await Promise.all(userIds.map((userId) => spacesModeratorsAdminApi.remove(spaceId, userId)));
			const failed = results.filter((r) => !r.ok).length;
			if (failed === 0) {
				success = t("spaces.moderators_removed", "Moderators removed");
			} else {
				error = t("common.bulkDeletePartial", "{failed} of {n} items could not be deleted")
					.replace("{failed}", String(failed))
					.replace("{n}", String(userIds.length));
			}
			await loadModerators();
		} catch {
			error = t("common.error", "Failed to remove moderators");
		} finally {
			bulkDeleting = false;
		}
	}

	$effect(() => {
		const id = spaceId;
		if (id) {
			loadSpace();
			loadModerators();
		}
	});
</script>

<svelte:head>
	<title>{space?.name ?? t("spaces.title", "Space")} — {siteName()} Admin</title>
</svelte:head>

<PageHeader
	title={space?.name ?? t("spaces.title", "Space")}
	description={t("spaces.moderators_desc", "Manage space moderators")}
	actions={[
		{ label: t("common.back", "Back"), icon: ArrowLeft, variant: "ghost", onclick: () => goto("/spaces") },
		...(canManageModerators ? [{ label: t("spaces.add_moderator", "Add Moderator"), icon: Plus, onclick: () => showAddModal = true }] : []),
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
			<div class="grid grid-cols-2 gap-4 md:grid-cols-5">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("spaces.slug", "Slug")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{space.slug}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("spaces.level", "Level")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{space.level}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("spaces.visibility", "Visibility")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{space.visibility}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("spaces.members", "Members")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{space.member_count.toLocaleString()}</p>
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
			<DataTable {columns} data={filteredModerators()} pageSize={10} searchable onsearch={(q) => { searchQuery = q; }} selectable bind:selectedIds actions={rowActions} bulkActionsList={bulkActions} />
		{/if}
	</CardContent>
</Card>

<!-- Add Moderator Modal -->
<Modal bind:open={showAddModal} title={t("spaces.add_moderator", "Add Moderator")}>
	<div>
		<label for="mod-user" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("spaces.user", "User")}</label>
		<HintIcon text={t('spaces.mod_user_hint', 'The user to grant moderator rights.')} />
		<div class="mt-1">
			<Autocomplete
				id="mod-user"
				placeholder={t("spaces.search_users", "Search users...")}
				emptyMessage={t("common.no_data", "No matching users")}
				fetcher={userFetcher}
				selectedId={newModeratorId || null}
				selectedLabel={newModeratorLabel}
				onSelect={(item: AutocompleteItem) => {
					newModeratorId = item.id;
					newModeratorLabel = item.label;
				}}
				onClear={() => {
					newModeratorId = "";
					newModeratorLabel = "";
				}}
			/>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showAddModal = false}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={addModerator} disabled={adding || !newModeratorId}>
			{adding ? t("common.adding", "Adding...") : t("common.add", "Add")}
		</Button>
	</div>
</Modal>

<!-- Remove Confirm -->
<Modal bind:open={showRemoveConfirm} title={t("common.confirmDelete", "Confirm Remove")}>
	{#if removeTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("spaces.remove_moderator_confirm", "Remove")} <strong>{removeTarget.user_display_name || removeTarget.user_username}</strong> {t("spaces.as_moderator", "as moderator?")}
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showRemoveConfirm = false}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmRemove}>{t("common.remove", "Remove")}</Button>
	</div>
</Modal>

