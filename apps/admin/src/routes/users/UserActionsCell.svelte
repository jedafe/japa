<script lang="ts">
	import { ActionCell } from "$lib/components/ui/data-table";
	import { Modal } from "$lib/components/ui/modal";
	import { Button } from "$lib/components/ui/button";
	import { Eye, Pencil, AlertTriangle, Ban, Trash2, RotateCcw, UserPlus, Trash } from "lucide-svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { t } from "$lib/stores/i18n.svelte";
	import { goto } from "$app/navigation";
	import { userManagementApi, impersonationApi } from "$lib/api";
	import type { Component } from "svelte";

	let { value, row, onchange }: { value: unknown; row: { id: string; username: string; email?: string; status?: string }; onchange?: () => void } = $props();

	let showWarnModal = $state(false);
	let warnReason = $state("");
	let showBanConfirm = $state(false);
	let showSuspendConfirm = $state(false);
	let showDeleteConfirm = $state(false);
	let showHardDeleteConfirm = $state(false);
	let showRestoreConfirm = $state(false);
	let acting = $state(false);

	const canManage = hasPermission("admin.users.manage");
	const canImpersonate = hasPermission("admin.impersonate");
	const isDeleted = $derived(row?.status === "deleted");
	const isBanned = $derived(row?.status === "banned");
	const isSuspended = $derived(row?.status === "suspended");

	const actions = $derived([
		{ label: t("common.view", "View"), icon: Eye as unknown as Component, onclick: () => goto(`/users/${row?.id}`) },
		...(canManage ? [
			{ label: t("common.edit", "Edit"), icon: Pencil as unknown as Component, onclick: () => goto(`/users/${row?.id}`) },
			...(isDeleted
				? [
					{ label: t("users.restore", "Restore"), icon: RotateCcw as unknown as Component, onclick: () => { showRestoreConfirm = true; } },
					{ label: t("users.hard_delete", "Hard Delete"), icon: Trash as unknown as Component, onclick: () => { showHardDeleteConfirm = true; }, variant: "destructive" as const }
				]
				: [
					{ label: t("users.warn", "Warn"), icon: AlertTriangle as unknown as Component, onclick: () => { warnReason = ""; showWarnModal = true; }, variant: "destructive" as const },
					isBanned
						? { label: t("users.unban", "Unban"), icon: Ban as unknown as Component, onclick: () => { showBanConfirm = true; } }
						: isSuspended
							? { label: t("users.unsuspend", "Unsuspend"), icon: RotateCcw as unknown as Component, onclick: () => { showSuspendConfirm = true; } }
							: { label: t("users.ban", "Ban"), icon: Ban as unknown as Component, onclick: () => { showBanConfirm = true; }, variant: "destructive" as const },
					...(!isBanned && !isSuspended ? [{ label: t("users.suspend", "Suspend"), icon: AlertTriangle as unknown as Component, onclick: () => { showSuspendConfirm = true; }, variant: "destructive" as const }] : []),
					{ label: t("common.delete", "Delete"), icon: Trash2 as unknown as Component, onclick: () => { showDeleteConfirm = true; }, variant: "destructive" as const },
					...(canImpersonate ? [{ label: t("users.impersonate", "Impersonate"), icon: UserPlus as unknown as Component, onclick: () => { confirmImpersonate(); } }] : []),
				]
			),
		] : [])
	]);

	async function confirmWarn() {
		if (!warnReason.trim() || !row?.id) return;
		acting = true;
		try {
			await userManagementApi.issueWarning(row.id, { reason: warnReason });
			showWarnModal = false;
			onchange?.();
		} finally {
			acting = false;
		}
	}

	async function confirmBan() {
		if (!row?.id) return;
		acting = true;
		try {
			if (isBanned) {
				await userManagementApi.unbanUser(row.id);
			} else {
				await userManagementApi.banUser(row.id);
			}
			showBanConfirm = false;
			onchange?.();
		} finally {
			acting = false;
		}
	}

	async function confirmSuspend() {
		if (!row?.id) return;
		acting = true;
		try {
			const newStatus = isSuspended ? "active" : "suspended";
			await userManagementApi.suspendUser(row.id, { status: newStatus });
			showSuspendConfirm = false;
			onchange?.();
		} finally {
			acting = false;
		}
	}

	async function confirmDelete() {
		if (!row?.id) return;
		acting = true;
		try {
			await userManagementApi.deleteUser(row.id);
			showDeleteConfirm = false;
			onchange?.();
		} finally {
			acting = false;
		}
	}

	async function confirmHardDelete() {
		if (!row?.id) return;
		acting = true;
		try {
			await userManagementApi.hardDeleteUser(row.id);
			showHardDeleteConfirm = false;
			onchange?.();
		} finally {
			acting = false;
		}
	}

	async function confirmRestore() {
		if (!row?.id) return;
		acting = true;
		try {
			await userManagementApi.suspendUser(row.id, { status: "active" });
			showRestoreConfirm = false;
			onchange?.();
		} finally {
			acting = false;
		}
	}

	async function confirmImpersonate() {
		if (!row?.id) return;
		acting = true;
		try {
			const result = await impersonationApi.start(row.id);
			if (result.ok) {
				goto("/", { invalidateAll: true });
			}
		} finally {
			acting = false;
		}
	}
</script>

<ActionCell {actions} />

<Modal bind:open={showWarnModal} title={t("users.warn_user", "Warn User")}>
	<p class="mb-4 text-sm" style="color: var(--color-foreground)">
		{t("users.warn_confirm", "Issue a warning to")} <strong>{row?.username}</strong>
	</p>
	<textarea
		class="w-full rounded-lg border p-3 text-sm"
		style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
		rows="3"
		placeholder={t("users.warn_reason", "Reason for warning...")}
		bind:value={warnReason}
	></textarea>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showWarnModal = false}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmWarn} disabled={!warnReason.trim() || acting}>
			{t("users.issue_warning", "Issue Warning")}
		</Button>
	</div>
</Modal>

<Modal bind:open={showBanConfirm} title={isBanned ? t("users.unban_user", "Unban User") : t("users.ban_user", "Ban User")}>
	{#if isBanned}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("users.unban_confirm", "Are you sure you want to unban")} <strong>{row?.username}</strong>?
		</p>
	{:else}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("users.ban_confirm", "Are you sure you want to ban")} <strong>{row?.username}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">
			{t("users.ban_warning", "This will prevent them from accessing the platform.")}
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showBanConfirm = false}>{t("common.cancel", "Cancel")}</Button>
		<Button variant={isBanned ? "default" : "destructive"} onclick={confirmBan} disabled={acting}>
			{isBanned ? t("users.unban", "Unban") : t("users.ban", "Ban")}
		</Button>
	</div>
</Modal>

<Modal bind:open={showDeleteConfirm} title={t("users.delete_user", "Delete User")}>
	<p class="text-sm" style="color: var(--color-foreground)">
		{t("users.delete_confirm", "Are you sure you want to delete")} <strong>{row?.username}</strong>?
	</p>
	<p class="mt-2 text-sm" style="color: var(--color-muted)">
		{t("users.delete_soft_notice", "The user will be soft-deleted and can be restored later.")}
	</p>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showDeleteConfirm = false}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmDelete} disabled={acting}>{t("common.delete", "Delete")}</Button>
	</div>
</Modal>

<Modal bind:open={showSuspendConfirm} title={isSuspended ? t("users.unsuspend_user", "Unsuspend User") : t("users.suspend_user", "Suspend User")}>
	{#if isSuspended}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("users.unsuspend_confirm", "Are you sure you want to unsuspend")} <strong>{row?.username}</strong>?
		</p>
	{:else}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("users.suspend_confirm", "Are you sure you want to suspend")} <strong>{row?.username}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-warning)">
			{t("users.suspend_warning", "The user will be unable to access the platform until unsuspended.")}
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showSuspendConfirm = false}>{t("common.cancel", "Cancel")}</Button>
		<Button variant={isSuspended ? "default" : "destructive"} onclick={confirmSuspend} disabled={acting}>
			{isSuspended ? t("users.unsuspend", "Unsuspend") : t("users.suspend", "Suspend")}
		</Button>
	</div>
</Modal>

<Modal bind:open={showRestoreConfirm} title={t("users.restore_user", "Restore User")}>
	<p class="text-sm" style="color: var(--color-foreground)">
		{t("users.restore_confirm", "Are you sure you want to restore")} <strong>{row?.username}</strong>?
	</p>
	<p class="mt-2 text-sm" style="color: var(--color-muted)">
		{t("users.restore_notice", "Their account will be reactivated and they will be able to log in again.")}
	</p>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showRestoreConfirm = false}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="default" onclick={confirmRestore} disabled={acting}>{t("users.restore", "Restore")}</Button>
	</div>
</Modal>

<Modal bind:open={showHardDeleteConfirm} title={t("users.hard_delete_user", "Hard Delete User")}>
	<p class="text-sm" style="color: var(--color-foreground)">
		{t("users.hard_delete_confirm", "Permanently erase")} <strong>{row?.username}</strong>?
	</p>
	<p class="mt-2 text-sm" style="color: var(--color-danger)">
		{t("users.hard_delete_warning", "This will permanently delete all user data including posts, comments, payments, and sessions. This action cannot be undone.")}
	</p>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showHardDeleteConfirm = false}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmHardDelete} disabled={acting}>{acting ? t("common.deleting", "Deleting...") : t("users.hard_delete", "Hard Delete")}</Button>
	</div>
</Modal>
