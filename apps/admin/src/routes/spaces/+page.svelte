<script lang="ts">
	// /spaces — migrated to EntityPage scaffold. EntityPage owns list/pagination/
	// search/filter state and URL sync. Page keeps local Modals for Suspend
	// (reason input) and Delete confirmation, plus a page-local Edit SlideOver
	// (name + visibility). View opens via ctx.openView; Manage navigates to the
	// dedicated detail route.
	import { EntityPage, type EntityPageConfig, type EntityPageController } from "$lib/components/entity";
	import { Modal } from "$lib/components/ui/modal";
	import { Button } from "$lib/components/ui/button";
	import { SlideOver } from "$lib/components/ui/slide-over";
	import Autocomplete, { type AutocompleteItem } from "$lib/components/ui/Autocomplete.svelte";
	import { spacesAdminApi, adminCatalogApi, spaceTypesApi, type CatalogOption } from "$lib/api";
	import { goto } from "$app/navigation";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { Eye, Pencil, Plus, Trash2, SlidersHorizontal } from "lucide-svelte";
	import type { ActionItem, FilterDef } from "$lib/components/ui/data-table";

	type Space = {
		id: string;
		name: string;
		owner: string;
		owner_id: string;
		member_count: number;
		status: string;
		visibility: string;
		space_type: string;
		deleted_at?: string | null;
		created_at: string;
		updated_at: string;
	};

	const canModerateSpaces = $derived(hasPermission("spaces.moderate"));
	const canManageSpaces = $derived(hasPermission("spaces.admin"));

	let visibilityOptions = $state<{ value: string; label: string }[]>([
		{ value: "public", label: t("spaces.public", "Public") },
		{ value: "members", label: t("spaces.perm.members", "Members") },
		{ value: "private", label: t("spaces.private", "Private") }
	]);

	$effect(() => {
		void (async () => {
			const res = await adminCatalogApi.get("spaces.visibility");
			if (res.ok && res.data?.length) {
				visibilityOptions = res.data;
			}
		})();
	});

	let spaceStatusOptions = $state<CatalogOption[]>([
		{ value: "active", label: t("spaces.active", "Active") },
		{ value: "suspended", label: t("spaces.suspended", "Suspended") },
		{ value: "archived", label: t("spaces.archived", "Archived") }
	]);

	$effect(() => {
		void (async () => {
			const statusRes = await adminCatalogApi.get("spaces.status");
			if (statusRes.ok && statusRes.data?.length) spaceStatusOptions = statusRes.data;
		})();
	});

	// Space-type filter options come from GET /admin/space-types — the backend
	// is the source of truth for the set of types (Rule 16).
	let spaceTypeOptions = $state<{ value: string; label: string }[]>([]);

	$effect(() => {
		void (async () => {
			const res = await spaceTypesApi.list();
			if (res.ok && Array.isArray(res.data)) {
				spaceTypeOptions = (res.data as { id: string; name: string }[]).map((st) => ({
					value: st.id,
					label: st.name
				}));
			}
		})();
	});

	// ── Page-local state for sub-flows ──────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state("");
	let acting = $state(false);

	// Edit SlideOver (page-local — name + visibility)
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

	// ── Filters ─────────────────────────────────────────────────────────────
	// Option filters render through FilterPanel's Autocomplete path — every
	// dropdown on this surface uses the shared Autocomplete.svelte.
	const filterDefs = $derived<FilterDef[]>([
		{
			key: "status",
			label: t("spaces.status", "Status"),
			options: spaceStatusOptions
		},
		{
			key: "visibility",
			label: t("spaces.visibility", "Visibility"),
			options: visibilityOptions
		},
		{
			key: "space_type",
			label: t("spaces.space_type", "Type"),
			options: spaceTypeOptions
		},
		// Soft-deleted spaces are hidden by default — this filter surfaces
		// them so the Force Delete action has targets.
		{
			key: "deleted",
			label: t("common.filter_deleted", "Deleted"),
			options: [
				{ value: "only", label: t("common.filter_deleted_only", "Deleted only") },
				{ value: "all", label: t("common.filter_deleted_all", "Include deleted") }
			]
		}
	]);

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t("common.view", "View"), icon: Eye, onclick: (row: Space, ctx) => ctx?.openView(row) },
			{ label: t("spaces.manage", "Manage"), onclick: (row: Space) => goto(`/spaces/${row.id}`) }
		];
		if (canManageSpaces) {
			acts.push({
				label: t("common.edit", "Edit"),
				icon: Pencil,
				onclick: (row: Space) => openEdit(row),
				show: (row: Space) => !row.deleted_at
			});
		}
		if (canModerateSpaces) {
			acts.push({
				label: t("spaces.suspend", "Suspend"),
				onclick: (row: Space) => openSuspend(row),
				show: (row: Space) => !row.deleted_at && row.status !== "suspended"
			});
			acts.push({
				label: t("spaces.unsuspend", "Unsuspend"),
				onclick: (row: Space) => openSuspend(row),
				show: (row: Space) => !row.deleted_at && row.status === "suspended"
			});
		}
		if (canManageSpaces) {
			acts.push({
				label: t("common.delete", "Delete"),
				icon: Trash2,
				variant: "destructive",
				onclick: (row: Space) => confirmDelete(row),
				show: (row: Space) => !row.deleted_at
			});
			// Permanent removal — only for already-soft-deleted rows.
			acts.push({
				label: t("common.force_delete", "Force Delete"),
				icon: Trash2,
				variant: "destructive",
				onclick: (row: Space) => openForceDelete(row),
				show: (row: Space) => !!row.deleted_at
			});
		}
		return acts;
	});

	// ── Config ──────────────────────────────────────────────────────────────
	const config = $derived<EntityPageConfig>({
		title: t("spaces.title", "Spaces"),
		description: t("spaces.description", "Manage platform spaces"),
		createPermission: "spaces.admin",
		// No built-in EntityForm — spaces use the dedicated /spaces/new page
		// and a page-local edit SlideOver (type/level/parent can't be edited
		// through the generic form anyway). hideCreate suppresses the generic
		// Create button; the "Create Space" header action navigates instead.
		fields: [],
		hideCreate: true,
		pageSize: 25,
		columns: [
			{ key: "name", label: t("spaces.name", "Name"), sortable: true },
			{ key: "owner", label: t("spaces.owner", "Owner"), sortable: true, render: (v: string) => v || "—" },
			{ key: "member_count", label: t("spaces.members", "Members"), sortable: true, render: (v: number) => (v ?? 0).toLocaleString() },
			{ key: "visibility", label: t("spaces.visibility", "Visibility"), sortable: true, render: (v: string) => (v ? t(`spaces.perm.${v}`, v) : "—") },
			{
				key: "status",
				label: t("spaces.status", "Status"),
				sortable: true,
				render: (v: string, row: Space) => {
					if (row.deleted_at) {
						return `<span class="inline-block rounded-full px-2 py-0.5 text-xs bg-danger/15 text-danger dark:text-danger">${t("common.filter_deleted", "Deleted")}</span>`;
					}
					const cls = v === "active" ? "bg-success/15 text-success dark:text-success" : v === "suspended" ? "bg-danger/15 text-danger dark:text-danger" : "bg-muted/15 text-muted dark:text-muted";
					const label = t(`spaces.${v}`, v);
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs ${cls}">${label}</span>`;
				}
			},
			{ key: "space_type", label: t("spaces.space_type", "Type"), sortable: true },
			{ key: "created_at", label: t("spaces.created", "Created"), sortable: true, render: (v: string) => new Date(v).toLocaleDateString() }
		],
		filters: filterDefs,
		actions: rowActions,
		bulkActions: canManageSpaces ? [
			{
				label: t("common.force_delete", "Force Delete Selected"),
				icon: Trash2,
				variant: "destructive",
				onclick: openBulkForceDelete,
				disabled: (ids) => acting || !spaceRowsFor(ids).some((r) => !!r.deleted_at)
			}
		] : [],
		headerActions: canManageSpaces ? [
			{ label: t("spaces.create", "Create Space"), icon: Plus, onclick: () => goto("/spaces/new") },
			{ label: t("spaces.presets_title", "Permission Presets"), icon: SlidersHorizontal, onclick: () => goto("/spaces/presets"), variant: "ghost" }
		] : [],
		load: async ({ page, pageSize, q, sort, filter }) => {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(page * pageSize)
			});
			if (q) params.set("q", q);
			if (filter?.status) params.set("status", filter.status);
			if (filter?.visibility) params.set("visibility", filter.visibility);
			if (filter?.space_type) params.set("space_type_id", filter.space_type);
			if (filter?.deleted) params.set("deleted", filter.deleted);
			// EntityPage passes sort as "key:dir"; the backend whitelist
			// (spaces.sql AdminListSpaces) ignores unknown keys.
			if (sort) {
				const [key, dir] = sort.split(":");
				if (key) params.set("sort", key);
				if (dir === "asc" || dir === "desc") params.set("dir", dir);
			}
			const result = await spacesAdminApi.list(params.toString());
			if (result.ok) {
				const d = result.data as Space[] | { items: Space[]; total: number; total_pages: number };
				const pageRows = Array.isArray(d) ? d : (d.items ?? []);
				for (const r of pageRows) spaceRowMap.set(r.id, r);
				if (Array.isArray(d)) {
					const start = page * pageSize;
					const total = d.length;
					return { data: d.slice(start, start + pageSize), total, totalPages: Math.max(1, Math.ceil(total / pageSize)) };
				}
				return {
					data: d.items ?? [],
					total: d.total ?? d.items?.length ?? 0,
					totalPages: d.total_pages ?? 1
				};
			}
			throw new Error(result.error ?? t("common.error", "Failed to load spaces"));
		},
		remove: async (id) => {
			const res = await spacesAdminApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: SpaceView
	});

	// ── Sub-flow handlers ───────────────────────────────────────────────────
	function openEdit(space: Space) {
		editTarget = space;
		editName = space.name;
		editVisibility = space.visibility;
		showEditSlideOver = true;
	}

	function confirmDelete(space: Space) {
		deleteTarget = space;
		showDeleteConfirm = true;
	}

	function openSuspend(space: Space) {
		suspendTarget = space;
		suspendReason = "";
		showSuspendModal = true;
	}

	async function saveEdit() {
		if (!editTarget) return;
		acting = true;
		try {
			const result = await spacesAdminApi.update(editTarget.id, { name: editName, visibility: editVisibility });
			if (result.ok) {
				showEditSlideOver = false;
				entityController?.reload();
			} else {
				pageError = result.error;
			}
		} catch {
			pageError = t("common.error", "Failed to update space");
		} finally {
			acting = false;
		}
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		acting = true;
		try {
			const result = await spacesAdminApi.delete(deleteTarget.id);
			if (result.ok) {
				showDeleteConfirm = false;
				deleteTarget = null;
				entityController?.reload();
			} else {
				pageError = result.error;
			}
		} catch {
			pageError = t("common.error", "Failed to delete space");
		} finally {
			acting = false;
		}
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
				entityController?.reload();
			} else {
				pageError = result.error;
			}
		} catch {
			pageError = t("common.error", "Failed to update space status");
		} finally {
			acting = false;
		}
	}

	// Soft "Delete Selected" is EntityPage's built-in (confirm-gated, per-row
	// failure reporting); only the Force Delete bulk action is page-local.

	// ── Force delete (spaces.admin) — single + bulk share one confirm modal ──
	// BulkAction.onclick receives only ids — resolve them against a map of
	// every row seen this session so deleted-row filtering keeps working.
	const spaceRowMap = new Map<string, Space>();
	const spaceRowsFor = (ids: Set<string>): Space[] =>
		Array.from(ids)
			.map((id) => spaceRowMap.get(id))
			.filter((r): r is Space => !!r);

	let forceTargets = $state<Space[]>([]);
	let showForceModal = $state(false);

	function openForceDelete(row: Space) {
		forceTargets = [row];
		showForceModal = true;
	}

	function openBulkForceDelete(ids: Set<string>) {
		const rows = spaceRowsFor(ids).filter((r) => !!r.deleted_at);
		if (rows.length === 0) return;
		forceTargets = rows;
		showForceModal = true;
	}

	async function confirmForceDelete() {
		if (acting || forceTargets.length === 0) return;
		acting = true;
		pageError = "";
		try {
			const targets = forceTargets;
			let failed = 0;
			let lastErr = "";
			for (const row of targets) {
				const res = await spacesAdminApi.hardDelete(row.id);
				if (!res.ok) {
					failed++;
					lastErr = res.error ?? "";
				}
			}
			showForceModal = false;
			forceTargets = [];
			if (failed > 0) {
				pageError =
					t("common.bulkDeletePartial", "{failed} of {n} items could not be deleted")
						.replace("{failed}", String(failed))
						.replace("{n}", String(targets.length)) + (lastErr ? ` — ${lastErr}` : "");
			}
			entityController?.reload();
		} finally {
			acting = false;
		}
	}
</script>

<svelte:head>
	<title>{t("spaces.title", "Spaces")} — JapaConnect Admin</title>
</svelte:head>

{#if pageError}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- Edit SlideOver (page-local — name + visibility) -->
<SlideOver bind:open={showEditSlideOver} title={t("spaces.edit", "Edit Space")}>
	{#if editTarget}
		<div class="space-y-4">
			<div>
				<label for="edit-name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('spaces.name')}</label>
				<input id="edit-name" type="text" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={editName} />
			</div>
			<div>
				<label for="edit-type" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('spaces.visibility')}</label>
				<Autocomplete
					id="edit-type"
					placeholder={t("spaces.visibility", "Visibility")}
					emptyMessage={t("common.no_data", "No options")}
					fetcher={async (q: string) => visibilityOptions.filter((o) => o.label.toLowerCase().includes(q.toLowerCase()) || o.value.toLowerCase().includes(q.toLowerCase())).map((o) => ({ id: o.value, label: o.label }))}
					initialFetcher={async () => visibilityOptions.map((o) => ({ id: o.value, label: o.label }))}
					selectedId={editVisibility || null}
					selectedLabel={visibilityOptions.find((o) => o.value === editVisibility)?.label ?? editVisibility}
					onSelect={(item: AutocompleteItem) => (editVisibility = item.id)}
					onClear={() => (editVisibility = "")}
				/>
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

<!-- Force Delete confirmation (single + bulk) — permanent erasure of soft-deleted rows -->
<Modal bind:open={showForceModal} title={t("common.force_delete", "Force Delete")} size="sm">
	{#if forceTargets.length > 0}
		<p class="text-sm" style="color: var(--color-foreground)">
			{#if forceTargets.length === 1}
				{t("common.force_delete_confirm", 'Permanently delete "{name}"? This cannot be undone.').replace("{name}", forceTargets[0].name)}
			{:else}
				{t("common.force_delete_confirm_bulk", "Permanently delete {count} selected items? This cannot be undone.").replace("{count}", String(forceTargets.length))}
			{/if}
		</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showForceModal = false)} disabled={acting}>{t('spaces.cancel')}</Button>
		<Button variant="destructive" onclick={confirmForceDelete} disabled={acting}>{acting ? t("common.deleting", "Deleting...") : t("common.force_delete", "Force Delete")}</Button>
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

<!-- ── View snippet ──────────────────────────────────────────────────────── -->
{#snippet SpaceView({ row }: { row: Space })}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('spaces.name')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('spaces.owner')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.owner || "—"}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('spaces.members')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.member_count.toLocaleString()}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('spaces.visibility')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.visibility ? t(`spaces.perm.${row.visibility}`, row.visibility) : "—"}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('spaces.status')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{t(`spaces.${row.status}`, row.status)}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('spaces.space_type', "Type")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.space_type ?? "—"}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('spaces.created')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(row.created_at).toLocaleString()}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('spaces.updated', "Updated")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(row.updated_at).toLocaleString()}</p>
			</div>
		</div>
	</div>
{/snippet}
