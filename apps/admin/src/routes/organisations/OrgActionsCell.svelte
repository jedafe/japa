<script lang="ts">
	import { ActionCell } from "$lib/components/ui/data-table";
	import { Modal } from "$lib/components/ui/modal";
	import { Button } from "$lib/components/ui/button";
	import { Eye, Pencil, Trash2, ShieldCheck, ShieldOff } from "lucide-svelte";
	import { t } from "$lib/stores/i18n.svelte";
	import { getPermissions } from "$lib/stores/permissions.svelte";
	import { organisationsAdminApi } from "$lib/api";
	import type { Component } from "svelte";

	let { value, row, onchange, pendingClaimsByOrg }: {
		value: unknown;
		row: any;
		onchange?: () => void;
		pendingClaimsByOrg?: Record<string, { id: string; claimant_username: string; evidence_url?: string; notes?: string }>;
	} = $props();

	let showViewModal = $state(false);
	let showEditModal = $state(false);
	let showDeleteConfirm = $state(false);
	let showClaimModal = $state(false);
	let formName = $state("");
	let formSlug = $state("");
	let claimNote = $state("");
	let acting = $state(false);

	const canManage = $derived(
		getPermissions().includes("organizations.manage") || getPermissions().includes("*")
	);
	const pendingClaim = $derived(pendingClaimsByOrg?.[row?.id] ?? null);

	const actions = $derived([
		{ label: t("common.view", "View"), icon: Eye as unknown as Component, onclick: () => { showViewModal = true; } },
		...(canManage ? [
			{ label: t("common.edit", "Edit"), icon: Pencil as unknown as Component, onclick: () => { formName = row?.name ?? ""; formSlug = row?.slug ?? ""; showEditModal = true; } },
			{ label: row?.verified ? t("organisations.unverify", "Unverify") : t("organisations.verify", "Verify"), icon: (row?.verified ? ShieldOff : ShieldCheck) as unknown as Component, onclick: toggleVerify, variant: "ghost" as const },
			...(pendingClaim ? [
				{ label: t("organisations.approve_claim", "Approve Claim"), icon: ShieldCheck as unknown as Component, onclick: () => { claimNote = ""; showClaimModal = true; }, variant: "ghost" as const }
			] : []),
			{ label: t("common.delete", "Delete"), icon: Trash2 as unknown as Component, onclick: () => { showDeleteConfirm = true; }, variant: "destructive" as const }
		] : [])
	]);

	async function toggleVerify() {
		if (!row?.id) return;
		acting = true;
		try {
			const result = row.verified
				? await organisationsAdminApi.unverify(row.id)
				: await organisationsAdminApi.verify(row.id, { verified: true });
			if (result.ok) onchange?.();
		} finally { acting = false; }
	}

	async function submitEdit() {
		if (!row?.id || !formName.trim()) return;
		acting = true;
		try {
			const result = await organisationsAdminApi.update(row.id, { name: formName.trim(), slug: formSlug.trim() || undefined });
			if (result.ok) { showEditModal = false; onchange?.(); }
		} finally { acting = false; }
	}

	async function executeDelete() {
		if (!row?.id) return;
		acting = true;
		try {
			const result = await organisationsAdminApi.delete(row.id);
			if (result.ok) { showDeleteConfirm = false; onchange?.(); }
		} finally { acting = false; }
	}

	async function submitClaim() {
		if (!row?.id) return;
		acting = true;
		try {
			const result = await organisationsAdminApi.claim(row.id, { note: claimNote.trim() || undefined });
			if (result.ok) { showClaimModal = false; onchange?.(); }
		} finally { acting = false; }
	}
</script>

<ActionCell {actions} />

<Modal bind:open={showViewModal} title={t("organisations.details", "Organisation Details")} size="default">
	<div class="grid grid-cols-2 gap-4">
		<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("organisations.name", "Name")}</p><p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row?.name}</p></div>
		<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("common.slug", "Slug")}</p><p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row?.slug ?? "—"}</p></div>
		<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("organisations.owner", "Owner")}</p><p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row?.owner_user_id ?? "—"}</p></div>
		<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("organisations.members", "Members")}</p><p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{(row?.member_count ?? 0).toLocaleString()}</p></div>
		<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("organisations.verified", "Verified")}</p><p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row?.verified ? t("organisations.yes", "Yes") : t("organisations.no", "No")}</p></div>
		<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("common.created", "Created")}</p><p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row?.created_at ? new Date(row.created_at).toLocaleString() : "—"}</p></div>
	</div>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showViewModal = false)}>{t("common.close", "Close")}</Button>
	</div>
</Modal>

<Modal bind:open={showEditModal} title={t("organisations.edit", "Edit Organisation")} size="default">
	<div class="space-y-3">
		<div><label for="edit-org-name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("organisations.name")}</label><input id="edit-org-name" type="text" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={formName} /></div>
		<div><label for="edit-org-slug" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("common.slug", "Slug")}</label><input id="edit-org-slug" type="text" class="w-full rounded-lg border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={formSlug} /></div>
	</div>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showEditModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={submitEdit} disabled={acting}>{acting ? t("common.saving", "Saving...") : t("common.save", "Save")}</Button>
	</div>
</Modal>

<Modal bind:open={showDeleteConfirm} title={t("common.confirmDelete", "Confirm Delete")} size="sm">
	<p class="text-sm" style="color: var(--color-foreground)">{t("organisations.delete_confirm", "Delete organisation {name}? This cannot be undone.").replace("{name}", row?.name ?? "")}</p>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteConfirm = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={executeDelete} disabled={acting}>{acting ? t("common.deleting", "Deleting...") : t("common.delete", "Delete")}</Button>
	</div>
</Modal>

<Modal bind:open={showClaimModal} title={t("organisations.approve_claim", "Approve Ownership Claim")} size="default">
	{#if pendingClaim}
		<div class="space-y-3 mb-4">
			<div class="rounded-lg p-3" style="background: var(--color-surface); border: 1px solid var(--color-border)">
				<p class="text-xs font-medium uppercase tracking-wider mb-1" style="color: var(--color-muted)">{t("organisations.claimant", "Claimant")}</p>
				<p class="text-sm font-medium" style="color: var(--color-foreground)">{pendingClaim.claimant_username}</p>
			</div>
			{#if pendingClaim.evidence_url}
				<div class="rounded-lg p-3" style="background: var(--color-surface); border: 1px solid var(--color-border)">
					<p class="text-xs font-medium uppercase tracking-wider mb-1" style="color: var(--color-muted)">{t("organisations.evidence", "Evidence URL")}</p>
					<a href={pendingClaim.evidence_url} target="_blank" rel="noopener noreferrer" class="text-sm break-all" style="color: var(--color-primary)">{pendingClaim.evidence_url}</a>
				</div>
			{/if}
			{#if pendingClaim.notes}
				<div class="rounded-lg p-3" style="background: var(--color-surface); border: 1px solid var(--color-border)">
					<p class="text-xs font-medium uppercase tracking-wider mb-1" style="color: var(--color-muted)">{t("organisations.claim_notes", "Claimant Notes")}</p>
					<p class="text-sm" style="color: var(--color-foreground)">{pendingClaim.notes}</p>
				</div>
			{/if}
		</div>
		<div class="border-t pt-3" style="border-color: var(--color-border)">
			<label for="claim-note" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("organisations.admin_note", "Admin Note (optional)")}</label>
			<textarea id="claim-note" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" rows="2" placeholder={t("organisations.claim_note_placeholder", "Reason for approval...")} bind:value={claimNote}></textarea>
		</div>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showClaimModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={submitClaim} disabled={acting}>{acting ? t("common.saving", "Saving...") : t("common.approve", "Approve")}</Button>
	</div>
</Modal>
