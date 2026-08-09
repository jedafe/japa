<script lang="ts">
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from "$lib/components/entity";
	import { Modal } from "$lib/components/ui/modal";
	import { Button } from "$lib/components/ui/button";
	import { userManagementApi, impersonationApi } from "$lib/api";
	import type { ModerationConfig } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { goto, invalidateAll } from "$app/navigation";
	import { onMount } from "svelte";
	import {
		Eye, Pencil, Trash2, Ban, AlertTriangle, RotateCcw, Trash, UserPlus
	} from "lucide-svelte";
	import type { ActionItem, FilterDef } from "$lib/components/ui/data-table";

	type User = {
		id: string;
		username: string;
		email: string;
		profile_type: string;
		status: string;
		follower_count: number;
		groups: string[];
		created_at: string;
		updated_at: string;
	};

	const canManageUsers = $derived(hasPermission("admin.users.manage"));
	const canImpersonate = $derived(hasPermission("admin.impersonate"));

	// ── Filter options ──────────────────────────────────────────────────────
	// Hardcoded fallback defaults, refreshed from backend on mount (status via getModerationConfig, groups via listGroups); failures keep defaults.
	let filterDefs = $state<FilterDef[]>([
		{
			key: "status",
			label: t("common.status", "Status"),
			options: [
				{ value: "active", label: t("users.status_active", "Active") },
				{ value: "pending", label: t("users.status_pending", "Pending") },
				{ value: "restricted", label: t("users.status_restricted", "Restricted") },
				{ value: "suspended", label: t("users.status_suspended", "Suspended") },
				{ value: "banned", label: t("users.status_banned", "Banned") },
				{ value: "deleted", label: t("users.status_deleted", "Deleted") }
			]
		},
		{
			key: "group",
			label: t("users.group", "Group"),
			options: [
				{ value: "user", label: t("users.role_user", "User") },
				{ value: "moderator", label: t("users.role_moderator", "Moderator") },
				{ value: "admin", label: t("users.role_admin", "Admin") },
				{ value: "super_admin", label: t("users.role_super_admin", "Super Admin") }
			]
		}
	]);

	async function loadFilterOptions() {
		try {
			const [configResult, groupsResult] = await Promise.all([
				userManagementApi.getModerationConfig(),
				userManagementApi.listGroups()
			]);

			let statusOptions: Array<{ value: string; label: string }> | undefined;
			if (configResult.ok) {
				const cfg = configResult.data as ModerationConfig;
				if (cfg?.statuses?.length) {
					statusOptions = cfg.statuses.map((s) => ({ value: s.value, label: s.label }));
				}
			}

			let groupOptions: Array<{ value: string; label: string }> | undefined;
			if (groupsResult.ok) {
				const groups = groupsResult.data.groups ?? [];
				if (groups.length) {
					groupOptions = groups.map((g) => ({ value: g.slug, label: g.name }));
				}
			}

			filterDefs = filterDefs.map((def) => {
				if (def.key === "status" && statusOptions) {
					return { ...def, options: statusOptions };
				}
				if (def.key === "group" && groupOptions) {
					return { ...def, options: groupOptions };
				}
				return def;
			});
		} catch {
			// Keep hardcoded fallback defaults on failure.
		}
	}

	onMount(() => {
		loadFilterOptions();
	});

	// ── Impersonation banner ────────────────────────────────────────────────
	let impersonating = $state(false);
	let impersonatingTargetId = $state<string | null>(null);

	async function checkImpersonation() {
		const result = await impersonationApi.status();
		if (result.ok) {
			impersonating = result.data.active;
			impersonatingTargetId = result.data.target_user_id;
		}
	}

	async function endImpersonation() {
		const result = await impersonationApi.end();
		if (result.ok) await invalidateAll();
	}

	$effect(() => {
		checkImpersonation();
	});

	// ── Bulk actions ────────────────────────────────────────────────────────
	// EntityPage auto-reloads + clears selection after bulk handlers, so no remount key is needed.
	let pageError = $state("");
	let bulkDeleting = $state(false);

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return; // in-flight guard
		bulkDeleting = true;
		pageError = "";
		try {
			await Promise.all(idArr.map((id) => userManagementApi.deleteUser(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t("common.error", "Failed to delete users");
		} finally {
			bulkDeleting = false;
		}
	}

	// Bulk hard-delete only applies to already-soft-deleted users; the disabled predicate hides it when none of the selected rows are deleted.
	async function bulkHardDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return; // in-flight guard
		bulkDeleting = true;
		pageError = "";
		try {
			await Promise.all(idArr.map((id) => userManagementApi.hardDeleteUser(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t("common.error", "Failed to hard delete users");
		} finally {
			bulkDeleting = false;
		}
	}

	// ── State-action confirmation modals ────────────────────────────────────
	// EntityPage's confirm Modal only covers default delete; state actions keep page-local Modals with richer copy, confirm calls API + controller.reload().
	let entityController = $state<EntityPageController | undefined>(undefined);
	let acting = $state(false);
	let actionTarget = $state<User | null>(null);

	let showBanConfirm = $state(false);
	let showSuspendConfirm = $state(false);
	let showWarnModal = $state(false);
	let showHardDeleteConfirm = $state(false);
	let showRestoreConfirm = $state(false);
	let showDeleteConfirm = $state(false);
	let warnReason = $state("");

	const isBanned = $derived(actionTarget?.status === "banned");
	const isSuspended = $derived(actionTarget?.status === "suspended");

	function openBan(user: User) { actionTarget = user; showBanConfirm = true; }
	function openSuspend(user: User) { actionTarget = user; showSuspendConfirm = true; }
	function openWarn(user: User) { actionTarget = user; warnReason = ""; showWarnModal = true; }
	function openHardDelete(user: User) { actionTarget = user; showHardDeleteConfirm = true; }
	function openRestore(user: User) { actionTarget = user; showRestoreConfirm = true; }
	function openDelete(user: User) { actionTarget = user; showDeleteConfirm = true; }

	async function confirmBan() {
		if (!actionTarget) return;
		acting = true;
		try {
			const result = isBanned
				? await userManagementApi.unbanUser(actionTarget.id)
				: await userManagementApi.banUser(actionTarget.id);
			if (result.ok) {
				showBanConfirm = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t("common.error", "Failed to update user status");
			}
		} catch {
			pageError = t("common.error", "Failed to update user status");
		} finally {
			acting = false;
		}
	}

	async function confirmSuspend() {
		if (!actionTarget) return;
		acting = true;
		try {
			const result = await userManagementApi.suspendUser(actionTarget.id, {
				status: isSuspended ? "active" : "suspended"
			});
			if (result.ok) {
				showSuspendConfirm = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t("common.error", "Failed to update user status");
			}
		} catch {
			pageError = t("common.error", "Failed to update user status");
		} finally {
			acting = false;
		}
	}

	async function confirmWarn() {
		if (!warnReason.trim() || !actionTarget) return;
		acting = true;
		try {
			const result = await userManagementApi.issueWarning(actionTarget.id, { reason: warnReason });
			if (result.ok) {
				showWarnModal = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t("common.error", "Failed to issue warning");
			}
		} catch {
			pageError = t("common.error", "Failed to issue warning");
		} finally {
			acting = false;
		}
	}

	async function confirmHardDelete() {
		if (!actionTarget) return;
		acting = true;
		try {
			const result = await userManagementApi.hardDeleteUser(actionTarget.id);
			if (result.ok) {
				showHardDeleteConfirm = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t("common.error", "Failed to hard delete user");
			}
		} catch {
			pageError = t("common.error", "Failed to hard delete user");
		} finally {
			acting = false;
		}
	}

	async function confirmRestore() {
		if (!actionTarget) return;
		acting = true;
		try {
			const result = await userManagementApi.suspendUser(actionTarget.id, { status: "active" });
			if (result.ok) {
				showRestoreConfirm = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t("common.error", "Failed to restore user");
			}
		} catch {
			pageError = t("common.error", "Failed to restore user");
		} finally {
			acting = false;
		}
	}

	async function confirmDelete() {
		if (!actionTarget) return;
		acting = true;
		try {
			const result = await userManagementApi.deleteUser(actionTarget.id);
			if (result.ok) {
				showDeleteConfirm = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t("common.error", "Failed to delete user");
			}
		} catch {
			pageError = t("common.error", "Failed to delete user");
		} finally {
			acting = false;
		}
	}

	async function confirmImpersonate(user: User) {
		acting = true;
		try {
			const result = await impersonationApi.start(user.id);
			if (result.ok) {
				goto("/", { invalidateAll: true });
			} else {
				pageError = result.error ?? t("common.error", "Failed to impersonate user");
			}
		} catch {
			pageError = t("common.error", "Failed to impersonate user");
		} finally {
			acting = false;
		}
	}

	// ── Row actions ─────────────────────────────────────────────────────────
	// ActionItem[] with show() predicates; custom actions replace defaults, so View/Edit/Delete are included explicitly.
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t("common.view", "View"), icon: Eye, onclick: (row: User) => goto(`/users/${row.id}`) }
		];
		if (canManageUsers) {
			acts.push({ label: t("common.edit", "Edit"), icon: Pencil, onclick: (row: User) => goto(`/users/${row.id}`) });
			acts.push({
				label: t("users.warn", "Warn"),
				icon: AlertTriangle,
				variant: "destructive",
				onclick: openWarn,
				show: (row: User) => row.status !== "deleted"
			});
			acts.push({
				label: t("users.ban", "Ban"),
				icon: Ban,
				variant: "destructive",
				onclick: openBan,
				show: (row: User) => row.status !== "deleted" && row.status !== "banned"
			});
			acts.push({
				label: t("users.unban", "Unban"),
				icon: Ban,
				onclick: openBan,
				show: (row: User) => row.status === "banned"
			});
			acts.push({
				label: t("users.suspend", "Suspend"),
				icon: AlertTriangle,
				variant: "destructive",
				onclick: openSuspend,
				show: (row: User) => row.status !== "deleted" && row.status !== "banned" && row.status !== "suspended"
			});
			acts.push({
				label: t("users.unsuspend", "Unsuspend"),
				icon: RotateCcw,
				onclick: openSuspend,
				show: (row: User) => row.status === "suspended"
			});
			acts.push({
				label: t("common.delete", "Delete"),
				icon: Trash2,
				variant: "destructive",
				onclick: openDelete,
				show: (row: User) => row.status !== "deleted"
			});
			acts.push({
				label: t("users.restore", "Restore"),
				icon: RotateCcw,
				onclick: openRestore,
				show: (row: User) => row.status === "deleted"
			});
			acts.push({
				label: t("users.hard_delete", "Hard Delete"),
				icon: Trash,
				variant: "destructive",
				onclick: openHardDelete,
				show: (row: User) => row.status === "deleted"
			});
		}
		if (canImpersonate) {
			acts.push({
				label: t("users.impersonate", "Impersonate"),
				icon: UserPlus,
				onclick: confirmImpersonate,
				show: (row: User) => row.status !== "deleted"
			});
		}
		return acts;
	});

	// ── Create form fields ──────────────────────────────────────────────────
	// Edit form lives on /users/[id]; EntityPage's create slide-over only handles creation.
	const fields: FieldDef[] = [
		{ key: "email", label: t("users.email", "Email"), type: "email", placeholder: "user@example.com", required: true },
		{ key: "username", label: t("users.username", "Username"), type: "text", placeholder: "username", required: true },
		{ key: "display_name", label: t("users.display_name", "Display Name"), type: "text", placeholder: "Display Name" },
		{ key: "password", label: t("users.password", "Password"), type: "password", placeholder: "••••••••", required: true }
	];

	// Config built as a $derived so filterDefs updates (loaded on mount) propagate.
	const config = $derived<EntityPageConfig>({
		title: t("users.title", "Users"),
		description: t("users.description", "Manage platform users"),
		createLabel: t("users.create", "Create User"),
		createPermission: "admin.users.manage",
		pageSize: 25,
		columns: [
			{ key: "username", label: t("users.username", "Username"), sortable: true },
			{ key: "email", label: t("users.email", "Email"), sortable: true },
			{ key: "profile_type", label: t("users.profile_type", "Profile Type"), sortable: true },
			{
				key: "status",
				label: t("common.status", "Status"),
				sortable: true,
				render: (v: string) => {
					const colors = v === "active" ? "bg-success/15 text-success dark:text-success" : v === "banned" ? "bg-danger/15 text-danger dark:text-danger" : v === "suspended" ? "bg-warning/15 text-warning dark:text-warning" : v === "deleted" ? "bg-muted/30 text-muted dark:text-muted" : v === "restricted" ? "bg-warning/15 text-warning dark:text-warning" : "bg-info/15 text-info dark:text-info";
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors}">${v}</span>`;
				}
			},
			{
				key: "groups",
				label: t("users.groups", "Groups"),
				sortable: true,
				render: (v: string[]) => {
					if (!v || v.length === 0) return `<span style="color: var(--color-muted)">—</span>`;
					return v.map(g => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium bg-info/15 text-info dark:text-info">${g}</span>`).join(' ');
				}
			},
			{
				key: "follower_count",
				label: t("users.followers", "Followers"),
				sortable: true,
				render: (v: number) => (v ?? 0).toLocaleString()
			},
			{
				key: "created_at",
				label: t("users.joined", "Joined"),
				sortable: true,
				render: (value: string) => new Date(value).toLocaleDateString()
			}
		],
		filters: filterDefs,
		actions: rowActions,
		bulkActions: [
			{
				label: t("users.bulk_delete", "Delete Selected"),
				icon: Trash2,
				variant: "destructive",
				onclick: bulkDelete
			},
			{
				label: t("users.bulk_hard_delete", "Hard Delete Selected"),
				icon: Trash,
				variant: "destructive",
				onclick: bulkHardDelete,
				disabled: (_ids: Set<string>, rows?: any[]) => !rows?.some((r) => r.status === "deleted")
			}
		],
		fields,
		load: async ({ page, pageSize, q, filter }) => {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(page * pageSize)
			});
			if (q) params.set("search", q);
			if (filter.status) params.set("status", filter.status);
			if (filter.group) params.set("group", filter.group);
			const result = await userManagementApi.list(params.toString());
			if (result.ok) {
				const d = result.data as unknown as { items: User[]; total: number; page: number; total_pages: number };
				return {
					data: d.items ?? [],
					total: d.total ?? 0,
					totalPages: d.total_pages ?? 1
				};
			}
			throw new Error(result.error ?? t("common.error", "Failed to load users"));
		},
		create: async (body) => {
			const res = await userManagementApi.createUser({
				email: String(body.email ?? "").trim(),
				username: String(body.username ?? "").trim(),
				password: String(body.password ?? ""),
				display_name: String(body.display_name ?? "").trim() || undefined
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await userManagementApi.deleteUser(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: UserView
	});
</script>

<svelte:head>
	<title>{t("users.title", "Users")} — JapaConnect Admin</title>
</svelte:head>

<!-- Impersonation Banner -->
{#if impersonating}
	<div
		class="mb-4 flex items-center justify-between rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-warning) 10%, transparent); color: var(--color-warning)"
	>
		<span>{t("users.impersonating", "Impersonating")} <strong>{impersonatingTargetId ?? ''}</strong></span>
		<button
			class="rounded-md px-3 py-1 text-xs font-medium transition-colors"
			style="background: var(--color-warning); color: white"
			onclick={endImpersonation}
		>
			{t("users.end_impersonation", "End Impersonation")}
		</button>
	</div>
{/if}

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- ── State-action confirmation modals ─────────────────────────────────── -->

<!-- Ban / Unban -->
<Modal bind:open={showBanConfirm} title={isBanned ? t("users.unban_user", "Unban User") : t("users.ban_user", "Ban User")}>
	{#if actionTarget}
		{#if isBanned}
			<p class="text-sm" style="color: var(--color-foreground)">
				{t("users.unban_confirm", "Are you sure you want to unban")} <strong>{actionTarget.username}</strong>?
			</p>
		{:else}
			<p class="text-sm" style="color: var(--color-foreground)">
				{t("users.ban_confirm", "Are you sure you want to ban")} <strong>{actionTarget.username}</strong>?
			</p>
			<p class="mt-2 text-sm" style="color: var(--color-danger)">
				{t("users.ban_warning", "This will prevent them from accessing the platform.")}
			</p>
		{/if}
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showBanConfirm = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button variant={isBanned ? "default" : "destructive"} onclick={confirmBan} disabled={acting}>
			{isBanned ? t("users.unban", "Unban") : t("users.ban", "Ban")}
		</Button>
	</div>
</Modal>

<!-- Suspend / Unsuspend -->
<Modal bind:open={showSuspendConfirm} title={isSuspended ? t("users.unsuspend_user", "Unsuspend User") : t("users.suspend_user", "Suspend User")}>
	{#if actionTarget}
		{#if isSuspended}
			<p class="text-sm" style="color: var(--color-foreground)">
				{t("users.unsuspend_confirm", "Are you sure you want to unsuspend")} <strong>{actionTarget.username}</strong>?
			</p>
		{:else}
			<p class="text-sm" style="color: var(--color-foreground)">
				{t("users.suspend_confirm", "Are you sure you want to suspend")} <strong>{actionTarget.username}</strong>?
			</p>
			<p class="mt-2 text-sm" style="color: var(--color-warning)">
				{t("users.suspend_warning", "The user will be unable to access the platform until unsuspended.")}
			</p>
		{/if}
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showSuspendConfirm = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button variant={isSuspended ? "default" : "destructive"} onclick={confirmSuspend} disabled={acting}>
			{isSuspended ? t("users.unsuspend", "Unsuspend") : t("users.suspend", "Suspend")}
		</Button>
	</div>
</Modal>

<!-- Warn -->
<Modal bind:open={showWarnModal} title={t("users.warn_user", "Warn User")}>
	<p class="mb-4 text-sm" style="color: var(--color-foreground)">
		{t("users.warn_confirm", "Issue a warning to")} <strong>{actionTarget?.username}</strong>
	</p>
	<textarea
		class="w-full rounded-lg border p-3 text-sm"
		style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
		rows="3"
		placeholder={t("users.warn_reason", "Reason for warning...")}
		bind:value={warnReason}
	></textarea>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showWarnModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmWarn} disabled={!warnReason.trim() || acting}>
			{t("users.issue_warning", "Issue Warning")}
		</Button>
	</div>
</Modal>

<!-- Restore -->
<Modal bind:open={showRestoreConfirm} title={t("users.restore_user", "Restore User")}>
	<p class="text-sm" style="color: var(--color-foreground)">
		{t("users.restore_confirm", "Are you sure you want to restore")} <strong>{actionTarget?.username}</strong>?
	</p>
	<p class="mt-2 text-sm" style="color: var(--color-muted)">
		{t("users.restore_notice", "Their account will be reactivated and they will be able to log in again.")}
	</p>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showRestoreConfirm = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="default" onclick={confirmRestore} disabled={acting}>{t("users.restore", "Restore")}</Button>
	</div>
</Modal>

<!-- Hard Delete -->
<Modal bind:open={showHardDeleteConfirm} title={t("users.hard_delete_user", "Hard Delete User")}>
	<p class="text-sm" style="color: var(--color-foreground)">
		{t("users.hard_delete_confirm", "Permanently erase")} <strong>{actionTarget?.username}</strong>?
	</p>
	<p class="mt-2 text-sm" style="color: var(--color-danger)">
		{t("users.hard_delete_warning", "This will permanently delete all user data including posts, comments, payments, and sessions. This action cannot be undone.")}
	</p>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showHardDeleteConfirm = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmHardDelete} disabled={acting}>
			{acting ? t("common.deleting", "Deleting...") : t("users.hard_delete", "Hard Delete")}
		</Button>
	</div>
</Modal>

<!-- Delete (soft) -->
<Modal bind:open={showDeleteConfirm} title={t("users.delete_user", "Delete User")}>
	<p class="text-sm" style="color: var(--color-foreground)">
		{t("users.delete_confirm", "Are you sure you want to delete")} <strong>{actionTarget?.username}</strong>?
	</p>
	<p class="mt-2 text-sm" style="color: var(--color-muted)">
		{t("users.delete_soft_notice", "The user will be soft-deleted and can be restored later.")}
	</p>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteConfirm = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmDelete} disabled={acting}>{t("common.delete", "Delete")}</Button>
	</div>
</Modal>

<!-- ── View snippet ──────────────────────────────────────────────────────── -->
{#snippet UserView({ row }: { row: User })}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("users.username", "Username")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.username}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("users.email", "Email")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.email}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("users.profile_type", "Profile Type")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.profile_type || "—"}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("users.status", "Status")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("users.followers", "Followers")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{(row.follower_count ?? 0).toLocaleString()}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("users.joined", "Joined")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(row.created_at).toLocaleString()}</p>
			</div>
		</div>
	</div>
{/snippet}
