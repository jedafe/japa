<script lang="ts">
	import { ActionCell } from "$lib/components/ui/data-table";
	import { Modal } from "$lib/components/ui/modal";
	import { Button } from "$lib/components/ui/button";
	import { Eye, AlertTriangle, Ban, Trash2 } from "lucide-svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { t } from "$lib/stores/i18n.svelte";
	import { goto } from "$app/navigation";
	import { userManagementApi } from "$lib/api";
	import type { Component } from "svelte";

	let { value, row }: { value: unknown; row: { id: string; username: string } } = $props();

	let showWarnModal = $state(false);
	let warnReason = $state("");
	let showBanConfirm = $state(false);
	let showDeleteConfirm = $state(false);

	const canManage = hasPermission("users.manage");

	const actions = $derived([
		{ label: t("common.view", "View"), icon: Eye as unknown as Component, onclick: () => goto(`/users/${row.id}`) },
		...(canManage ? [
			{ label: t("users.warn", "Warn"), icon: AlertTriangle as unknown as Component, onclick: () => { warnReason = ""; showWarnModal = true; }, variant: "destructive" as const },
			{ label: t("users.ban", "Ban"), icon: Ban as unknown as Component, onclick: () => { showBanConfirm = true; }, variant: "destructive" as const },
			{ label: t("common.delete", "Delete"), icon: Trash2 as unknown as Component, onclick: () => { showDeleteConfirm = true; }, variant: "destructive" as const },
		] : [])
	]);

	async function confirmWarn() {
		if (!warnReason.trim()) return;
		await userManagementApi.issueWarning(row.id, { reason: warnReason });
		showWarnModal = false;
	}

	async function confirmBan() {
		await userManagementApi.banUser(row.id);
		showBanConfirm = false;
	}

	async function confirmDelete() {
		await userManagementApi.deleteUser(row.id);
		showDeleteConfirm = false;
	}
</script>

<ActionCell {actions} />

<Modal bind:open={showWarnModal} title={t("users.warn_user", "Warn User")}>
	<p class="mb-4 text-sm" style="color: var(--color-foreground)">
		{t("users.warn_confirm", "Issue a warning to")} <strong>{row.username}</strong>
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
		<Button variant="destructive" onclick={confirmWarn} disabled={!warnReason.trim()}>
			{t("users.issue_warning", "Issue Warning")}
		</Button>
	</div>
</Modal>

<Modal bind:open={showBanConfirm} title={t("users.ban_user", "Ban User")}>
	<p class="text-sm" style="color: var(--color-foreground)">
		{t("users.ban_confirm", "Are you sure you want to ban")} <strong>{row.username}</strong>?
	</p>
	<p class="mt-2 text-sm" style="color: var(--color-danger)">
		{t("users.ban_warning", "This will prevent them from accessing the platform.")}
	</p>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showBanConfirm = false}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmBan}>{t("users.ban", "Ban")}</Button>
	</div>
</Modal>

<Modal bind:open={showDeleteConfirm} title={t("users.delete_user", "Delete User")}>
	<p class="text-sm" style="color: var(--color-foreground)">
		{t("users.delete_confirm", "Are you sure you want to delete")} <strong>{row.username}</strong>?
	</p>
	<p class="mt-2 text-sm" style="color: var(--color-danger)">
		{t("common.cannotUndo", "This action cannot be undone.")}
	</p>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showDeleteConfirm = false}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmDelete}>{t("common.delete", "Delete")}</Button>
	</div>
</Modal>
