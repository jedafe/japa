<script lang="ts">
	// /spaces — migrated to EntityPage scaffold. EntityPage owns list/pagination/
	// search/filter state and URL sync. Page keeps local Modals for Suspend
	// (reason input) and Delete confirmation, plus a page-local Edit SlideOver
	// (name + visibility). View opens via ctx.openView; Manage navigates to the
	// dedicated detail route.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from "$lib/components/entity";
	import { Modal } from "$lib/components/ui/modal";
	import { Button } from "$lib/components/ui/button";
	import { SlideOver } from "$lib/components/ui/slide-over";
	import { Select } from "$lib/components/ui/select";
	import { spacesAdminApi, contentAdminApi, adminCatalogApi, type CatalogOption } from "$lib/api";
	import { goto } from "$app/navigation";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { Eye, Pencil, Trash2, SlidersHorizontal } from "lucide-svelte";
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
			const res = await contentAdminApi.getVisibilityOptions();
			if (res.ok && res.data) {
				visibilityOptions = res.data;
			}
		})();
	});

	let levelOptions = $state<CatalogOption[]>([
		{ value: "1", label: "1" },
		{ value: "2", label: "2" },
		{ value: "3", label: "3" }
	]);

	let spaceStatusOptions = $state<CatalogOption[]>([
		{ value: "active", label: t("spaces.active", "Active") },
		{ value: "suspended", label: t("spaces.suspended", "Suspended") },
		{ value: "archived", label: t("spaces.archived", "Archived") }
	]);

	$effect(() => {
		void (async () => {
			const [levelRes, statusRes] = await Promise.all([
				adminCatalogApi.get("spaces.level"),
				adminCatalogApi.get("spaces.status")
			]);
			if (levelRes.ok && levelRes.data?.length) levelOptions = levelRes.data;
			if (statusRes.ok && statusRes.data?.length) spaceStatusOptions = statusRes.data;
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

	// ── Create form fields ──────────────────────────────────────────────────
	const fields = $derived<FieldDef[]>([
		{ key: "name", label: t("spaces.name", "Name"), type: "text", required: true },
		{ key: "slug", label: t("spaces.slug", "Slug"), type: "text", placeholder: "my-space" },
		{ key: "space_type_id", label: t("spaces.space_type", "Space Type ID"), type: "text", placeholder: "e.g. standard" },
		{ key: "level", label: t("spaces.level", "Level"), type: "select", options: levelOptions },
		{ key: "visibility", label: t("spaces.visibility", "Visibility"), type: "select", options: visibilityOptions },
		{ key: "description", label: t("spaces.description", "Description"), type: "textarea" }
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
		createLabel: t("spaces.create", "Create Space"),
		createPermission: "spaces.admin",
		pageSize: 25,
		columns: [
			{ key: "name", label: t("spaces.name", "Name"), sortable: true },
			{ key: "owner", label: t("spaces.owner", "Owner") },
			{ key: "member_count", label: t("spaces.members", "Members"), render: (v: number) => (v ?? 0).toLocaleString() },
			{ key: "visibility", label: t("spaces.visibility", "Visibility"), render: (v: string) => (v ? t(`spaces.perm.${v}`, v) : "—") },
			{
				key: "status",
				label: t("spaces.status", "Status"),
				render: (v: string, row: Space) => {
					if (row.deleted_at) {
						return `<span class="inline-block rounded-full px-2 py-0.5 text-xs bg-danger/15 text-danger dark:text-danger">${t("common.filter_deleted", "Deleted")}</span>`;
					}
					const cls = v === "active" ? "bg-success/15 text-success dark:text-success" : v === "suspended" ? "bg-danger/15 text-danger dark:text-danger" : "bg-muted/15 text-muted dark:text-muted";
					const label = t(`spaces.${v}`, v);
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs ${cls}">${label}</span>`;
				}
			},
			{ key: "space_type", label: t("spaces.space_type", "Type") },
			{ key: "created_at", label: t("spaces.created", "Created"), render: (v: string) => new Date(v).toLocaleDateString() }
		],
		filters: filterDefs,
		actions: rowActions,
		bulkActions: canManageSpaces ? [
			{
				label: t("common.delete_selected", "Delete Selected"),
				icon: Trash2,
				variant: "destructive",
				onclick: bulkDelete,
				// Rows already soft-deleted can't be deleted again.
				disabled: (_ids, rows) => (rows ?? []).some((r) => !!r.deleted_at)
			},
			{
				label: t("common.force_delete", "Force Delete Selected"),
				icon: Trash2,
				variant: "destructive",
				onclick: openBulkForceDelete,
				disabled: (ids) => acting || !spaceRowsFor(ids).some((r) => !!r.deleted_at)
			}
		] : [],
		fields,
		headerActions: canManageSpaces ? [
			{ label: t("spaces.presets_title", "Permission Presets"), icon: SlidersHorizontal, onclick: () => goto("/spaces/presets"), variant: "ghost" }
		] : [],
		load: async ({ page, pageSize, q, filter }) => {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(page * pageSize)
			});
			if (q) params.set("q", q);
			if (filter?.status) params.set("status", filter.status);
			if (filter?.visibility) params.set("visibility", filter.visibility);
			if (filter?.deleted) params.set("deleted", filter.deleted);
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
		create: async (body) => {
			const res = await spacesAdminApi.create({
				name: String(body.name ?? "").trim(),
				slug: String(body.slug ?? "").trim(),
				space_type_id: String(body.space_type_id ?? "").trim(),
				level: String(body.level ?? "1"),
				visibility: String(body.visibility ?? "public"),
				description: String(body.description ?? "").trim() || undefined
			} as any);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await spacesAdminApi.update(id, {
				name: String(body.name ?? "").trim() || undefined,
				visibility: String(body.visibility ?? "").trim() || undefined
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
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

	let bulkDeleting = $state(false);
	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0 || bulkDeleting) return;
		bulkDeleting = true;
		pageError = "";
		try {
			await Promise.all(idArr.map((id) => spacesAdminApi.delete(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t("common.error", "Failed to delete spaces");
		} finally {
			bulkDeleting = false;
		}
	}

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
				<Select options={visibilityOptions} bind:value={editVisibility} />
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
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.owner}</p>
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
