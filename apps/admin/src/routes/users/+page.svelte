<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import Pagination from "$lib/components/ui/Pagination.svelte";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { RefreshCw, Trash2, Plus, Trash } from "lucide-svelte";
	import { userManagementApi, impersonationApi } from "$lib/api";
	import type { UserDetail } from "$lib/api";
	import UserActionsCell from "./UserActionsCell.svelte";
	import { t } from "$lib/stores/i18n.svelte";
	import { goto, invalidateAll } from "$app/navigation";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	// Usernames are user-controlled, so they must be escaped before being
	// interpolated into the {@html} confirmation copy.
	function escapeHtml(value: string): string {
		return value
			.replace(/&/g, "&amp;")
			.replace(/</g, "&lt;")
			.replace(/>/g, "&gt;")
			.replace(/"/g, "&quot;")
			.replace(/'/g, "&#39;");
	}

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

	let users = $state<User[]>([]);
	let currentPage = $state(0);
	let totalItems = $state(0);
	let pageSize = $state(25);
	let totalPages = $state(1);
	let loading = $state(true);
	let error = $state("");
	let searchQuery = $state("");
	let filterValues = $state<Record<string, string>>({});
	let selectedIds = $state<Set<string>>(new Set());

	// Modals
	let showViewModal = $state(false);
	let viewTarget = $state<User | null>(null);
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<User | null>(null);
	let showBulkHardDeleteConfirm = $state(false);
	let acting = $state(false);

	// Create user modal
	let showCreateModal = $state(false);
	let createForm = $state({ email: "", username: "", password: "", display_name: "" });
	let creating = $state(false);
	let createError = $state("");

	// Impersonation state
	let impersonating = $state(false);
	let impersonatingUsername = $state("");
	let impersonateError = $state("");

	const canManageUsers = $derived(hasPermission("admin.users.manage"));
	const canImpersonate = $derived(hasPermission("admin.impersonate"));
	const hasDeletedSelected = $derived(
		Array.from(selectedIds).some(id => users.find(u => u.id === id)?.status === "deleted")
	);

	const filterDefs = [
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
	];

	const columns = [
		{
			key: "username",
			label: t("users.username", "Username"),
			sortable: true
		},
		{
			key: "email",
			label: t("users.email", "Email"),
			sortable: true
		},
		{
			key: "profile_type",
			label: t("users.profile_type", "Profile Type"),
			sortable: true
		},
		{
			key: "status",
			label: t("common.status", "Status"),
			sortable: true,
			render: (v: string) => {
				const colors = v === "active" ? "bg-success/15 text-success dark:text-success" : v === "banned" ? "bg-danger/15 text-danger dark:text-danger" : v === "suspended" ? "bg-warning/15 text-warning dark:text-warning" : v === "deleted" ? "bg-muted/30 text-muted dark:text-muted" : v === "restricted" ? "bg-orange-500/15 text-orange-700 dark:text-orange-400" : "bg-info/15 text-info dark:text-info";
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
		},
		{
			key: "actions",
			label: t("common.actions", "Actions"),
			class: "w-48",
			component: UserActionsCell,
			componentProps: { onchange: loadUsers }
		}
	];

	async function loadUsers() {
		loading = true;
		error = "";
		try {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(currentPage * pageSize)
			});
			if (searchQuery) params.set("search", searchQuery);
			if (filterValues.status) params.set("status", filterValues.status);
			if (filterValues.group) params.set("group", filterValues.group);

			const result = await userManagementApi.list(params.toString());
			if (result.ok) {
				const d = result.data as unknown as { items: User[]; total: number; page: number; total_pages: number };
				users = d.items ?? [];
				totalItems = d.total ?? 0;
				totalPages = d.total_pages ?? 1;
			} else {
				error = result.error ?? t("common.error", "Failed to load users");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load users");
		} finally {
			loading = false;
		}
	}

	function handleSearch(values: Record<string, string>) {
		searchQuery = values.search ?? "";
		filterValues = values;
		currentPage = 0;
	}

	function handleClear() {
		searchQuery = "";
		filterValues = {};
		currentPage = 0;
	}

	function openView(user: User) {
		viewTarget = user;
		showViewModal = true;
	}

	function confirmDelete(user: User) {
		deleteTarget = user;
		showDeleteConfirm = true;
	}

	async function executeCreate() {
		creating = true;
		createError = "";
		try {
			const result = await userManagementApi.createUser({
				email: createForm.email,
				username: createForm.username,
				password: createForm.password,
				display_name: createForm.display_name || undefined
			});
			if (result.ok) {
				showCreateModal = false;
				createForm = { email: "", username: "", password: "", display_name: "" };
				loadUsers();
			} else {
				createError = result.error ?? t("common.error", "Failed to create user");
			}
		} catch (e) {
			createError = e instanceof Error ? e.message : t("common.error", "Failed to create user");
		} finally {
			creating = false;
		}
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		acting = true;
		try {
			const result = await userManagementApi.deleteUser(deleteTarget.id);
			if (result.ok) {
				showDeleteConfirm = false;
				deleteTarget = null;
				loadUsers();
			} else {
				error = result.error;
			}
		} catch {
			error = t("common.error", "Failed to delete user");
		} finally {
			acting = false;
		}
	}

	async function handleBan(user: User) {
		acting = true;
		try {
			const result = user.status === "banned"
				? await userManagementApi.unbanUser(user.id)
				: await userManagementApi.banUser(user.id);
			if (result.ok) {
				loadUsers();
			} else {
				error = result.error;
			}
		} catch {
			error = t("common.error", "Failed to update user status");
		} finally {
			acting = false;
		}
	}

	async function handleSuspend(user: User) {
		acting = true;
		try {
			const result = await userManagementApi.suspendUser(user.id, {
				status: user.status === "suspended" ? "active" : "suspended"
			});
			if (result.ok) {
				loadUsers();
			} else {
				error = result.error;
			}
		} catch {
			error = t("common.error", "Failed to update user status");
		} finally {
			acting = false;
		}
	}

	async function bulkDelete() {
		const ids = Array.from(selectedIds);
		if (ids.length === 0) return;
		acting = true;
		try {
			await Promise.all(ids.map((id) => userManagementApi.deleteUser(id)));
			selectedIds = new Set();
			loadUsers();
		} catch {
			error = t("common.error", "Failed to delete users");
		} finally {
			acting = false;
		}
	}

	async function bulkHardDelete() {
		const ids = Array.from(selectedIds).filter(id => users.find(u => u.id === id)?.status === "deleted");
		if (ids.length === 0) return;
		acting = true;
		try {
			await Promise.all(ids.map((id) => userManagementApi.hardDeleteUser(id)));
			selectedIds = new Set();
			loadUsers();
		} catch {
			error = t("common.error", "Failed to hard delete users");
		} finally {
			acting = false;
		}
	}

	async function checkImpersonation() {
		const result = await impersonationApi.status();
		if (result.ok) {
			const d = result.data as unknown as { impersonating: boolean; username?: string; user_id?: string };
			impersonating = d.impersonating;
			impersonatingUsername = d.username ?? "";
		}
	}

	async function endImpersonation() {
		const result = await impersonationApi.end();
		if (result.ok) await invalidateAll();
	}

	$effect(() => {
		void currentPage;
		void pageSize;
		void searchQuery;
		void filterValues;
		loadUsers();
	});

	$effect(() => {
		checkImpersonation();
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
		<span>{t("users.impersonating", "Impersonating")} <strong>{impersonatingUsername}</strong></span>
		<button
			class="rounded-md px-3 py-1 text-xs font-medium transition-colors"
			style="background: var(--color-warning); color: white"
			onclick={endImpersonation}
		>
			{t("users.end_impersonation", "End Impersonation")}
		</button>
	</div>
{/if}

<PageHeader
	title={t("users.title", "Users")}
	description={t("users.description", "Manage platform users")}
	actions={[
		...(canManageUsers ? [{ label: t("users.create", "Create User"), icon: Plus, variant: "default" as const, onclick: () => { createForm = { email: "", username: "", password: "", display_name: "" }; showCreateModal = true; } }] : []),
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost" as const, onclick: loadUsers }
	]}
/>

{#if error}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{error}
	</div>
{/if}

{#if impersonateError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{impersonateError}
	</div>
{/if}

{#if selectedIds.size > 0}
	<div class="mb-4 flex items-center gap-3 rounded-lg p-3" style="background: var(--color-primary); color: white">
		<span class="text-sm font-medium">{selectedIds.size} {t('app.users.selected_count', 'selected')}</span>
		{#if canManageUsers}
			<Button variant="ghost" size="sm" onclick={bulkDelete} disabled={acting}>
				<Trash2 class="mr-1 h-3.5 w-3.5" /> {t('users.bulk_delete', 'Delete Selected')}
			</Button>
			{#if hasDeletedSelected}
				<Button variant="ghost" size="sm" onclick={() => { showBulkHardDeleteConfirm = true; }} disabled={acting}>
					<Trash class="mr-1 h-3.5 w-3.5" /> {t('users.bulk_hard_delete', 'Hard Delete Selected')}
				</Button>
			{/if}
		{/if}
	</div>
{/if}

<Card>
	<CardHeader>
		<FilterBar
			filters={filterDefs}
			onsearch={handleSearch}
			onclear={handleClear}
		/>
	</CardHeader>
	<CardContent>
		{#if loading}
			<TableSkeleton rows={8} columns={columns.length} />
		{:else}
			<div aria-live="polite">
				<DataTable
					{columns}
					data={users}
					selectable={canManageUsers}
					bind:selectedIds
				/>
			</div>
			<Pagination
				bind:page={currentPage}
				{totalPages}
				{totalItems}
				bind:pageSize
			/>
		{/if}
	</CardContent>
</Card>

<!-- View Modal -->
<Modal bind:open={showViewModal} title={t("users.details", "User Details")} size="default">
	{#if viewTarget}
		<div class="space-y-4">
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.username')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.username}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.email')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.email}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.profile_type')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.profile_type || "—"}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.status')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.status}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.followers')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{(viewTarget.follower_count ?? 0).toLocaleString()}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.joined')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(viewTarget.created_at).toLocaleString()}</p>
				</div>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => goto(`/users/${viewTarget?.id}`)}>{t('users.view_full_profile')}</Button>
		<Button variant="outline" onclick={() => (showViewModal = false)}>{t('users.close')}</Button>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t("common.confirmDelete", "Confirm Delete")} size="sm">
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{@html t('app.users.delete_confirm', `Delete user <strong>{username}</strong>? This cannot be undone.`).replace('{username}', escapeHtml(deleteTarget.username))}
		</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteConfirm = false)}>{t('users.cancel')}</Button>
		<Button variant="destructive" onclick={executeDelete} disabled={acting}>{acting ? t('app.users.deleting', 'Deleting...') : t('app.users.delete_action', 'Delete')}</Button>
	</div>
</Modal>

<!-- Bulk Hard Delete Confirmation -->
<Modal bind:open={showBulkHardDeleteConfirm} title={t("users.bulk_hard_delete_title", "Hard Delete Selected Users")} size="sm">
	<p class="text-sm" style="color: var(--color-foreground)">
		{t("users.bulk_hard_delete_confirm", "Permanently erase all selected deleted users?")}
	</p>
	<p class="mt-2 text-sm" style="color: var(--color-danger)">
		{t("users.bulk_hard_delete_warning", "This will permanently delete all data for the selected users including posts, comments, payments, and sessions. This action cannot be undone.")}
	</p>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showBulkHardDeleteConfirm = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={async () => { showBulkHardDeleteConfirm = false; await bulkHardDelete(); }} disabled={acting}>
			{acting ? t("common.deleting", "Deleting...") : t("users.bulk_hard_delete", "Hard Delete Selected")}
		</Button>
	</div>
</Modal>

<!-- Create User Modal -->
<Modal bind:open={showCreateModal} title={t("users.create", "Create User")} size="default">
	{#if createError}
		<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
			{createError}
		</div>
	{/if}
	<form onsubmit={(e) => { e.preventDefault(); executeCreate(); }} class="space-y-4">
		<div>
			<label for="create-email" class="mb-1 block text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("users.email", "Email")} *</label>
			<input id="create-email" type="email" required bind:value={createForm.email}
				class="w-full rounded-lg border p-2 text-sm outline-none focus:ring-2"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground); --tw-ring-color: var(--color-primary)" />
		</div>
		<div>
			<label for="create-username" class="mb-1 block text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("users.username", "Username")} *</label>
			<input id="create-username" type="text" required bind:value={createForm.username}
				class="w-full rounded-lg border p-2 text-sm outline-none focus:ring-2"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground); --tw-ring-color: var(--color-primary)" />
		</div>
		<div>
			<label for="create-display-name" class="mb-1 block text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("users.display_name", "Display Name")}</label>
			<input id="create-display-name" type="text" bind:value={createForm.display_name}
				class="w-full rounded-lg border p-2 text-sm outline-none focus:ring-2"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground); --tw-ring-color: var(--color-primary)" />
		</div>
		<div>
			<label for="create-password" class="mb-1 block text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("users.password", "Password")} *</label>
			<input id="create-password" type="password" required bind:value={createForm.password}
				class="w-full rounded-lg border p-2 text-sm outline-none focus:ring-2"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground); --tw-ring-color: var(--color-primary)" />
		</div>
	</form>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showCreateModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="default" onclick={executeCreate} disabled={creating}>{creating ? t("common.creating", "Creating...") : t("users.create", "Create User")}</Button>
	</div>
</Modal>


