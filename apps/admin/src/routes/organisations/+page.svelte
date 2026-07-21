<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import Pagination from "$lib/components/ui/Pagination.svelte";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { Plus, RefreshCw, Eye, Pencil, Trash2, ShieldCheck, ShieldOff } from "lucide-svelte";
	import { organisationsAdminApi } from "$lib/api";
	import { goto } from "$app/navigation";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	type Organisation = {
		id: string;
		name: string;
		slug?: string;
		owner_user_id?: string;
		member_count: number;
		verified: boolean;
		created_at: string;
		status?: string;
	};

	let orgs = $state<Organisation[]>([]);
	let loading = $state(true);
	let error = $state("");
	let currentPage = $state(0);
	let totalItems = $state(0);
	let pageSize = $state(25);
	let totalPages = $state(1);
	let searchQuery = $state("");
	let filterValues = $state<Record<string, string>>({});
	let selectedIds = $state<Set<string>>(new Set());

	let showViewModal = $state(false);
	let viewTarget = $state<Organisation | null>(null);
	let showCreateModal = $state(false);
	let showEditModal = $state(false);
	let editTarget = $state<Organisation | null>(null);
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<Organisation | null>(null);
	let showClaimModal = $state(false);
	let claimOrg = $state<Organisation | null>(null);
	let claimNote = $state("");
	let acting = $state(false);

	let formName = $state("");
	let formSlug = $state("");
	let formDescription = $state("");

	// Backend gates organisation mutations on organisations.manage
	const canManage = $derived(hasPermission("organisations.manage"));

	const filterDefs = [
		{
			key: "verified",
			label: t("organisations.verified", "Verified"),
			options: [
				{ value: "true", label: t("common.yes", "Yes") },
				{ value: "false", label: t("common.no", "No") }
			]
		}
	];

	const columns = [
		{ key: "name", label: t("organisations.name", "Name"), sortable: true },
		{ key: "slug", label: t("organisations.slug", "Slug"), sortable: true, render: (v: string) => v || "—" },
		{ key: "owner_user_id", label: t("organisations.owner", "Owner"), render: (v: string) => v || "—" },
		{ key: "member_count", label: t("organisations.members", "Members"), sortable: true, render: (v: number) => (v ?? 0).toLocaleString() },
		{
			key: "verified",
			label: t("organisations.verified", "Verified"),
			sortable: true,
			render: (v: boolean) => {
				const cls = v ? "bg-success/15 text-success dark:text-success" : "bg-muted/15 text-muted dark:text-muted";
				return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${cls}">${v ? "Yes" : "No"}</span>`;
			}
		},
		{ key: "created_at", label: t("common.created", "Created"), sortable: true, render: (v: string) => new Date(v).toLocaleDateString() },
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-48", component: ActionsCell }
	];

	async function load() {
		loading = true;
		error = "";
		try {
			const params = new URLSearchParams({ page: String(currentPage), limit: String(pageSize) });
			if (searchQuery) params.set("q", searchQuery);
			if (filterValues.verified) params.set("verified", filterValues.verified);
			const result = await organisationsAdminApi.list(params.toString());
			if (result.ok) {
				const d = result.data as Organisation[] | { items: Organisation[]; page: number; total_pages: number };
				if (Array.isArray(d)) {
					orgs = d;
					totalItems = d.length;
					totalPages = 1;
				} else {
					orgs = d.items ?? [];
					totalItems = d.items?.length ?? 0;
					totalPages = d.total_pages ?? 1;
				}
			} else {
				error = result.error ?? t("common.error", "Failed to load");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load");
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

	function openView(org: Organisation) { viewTarget = org; showViewModal = true; }

	function openCreate() {
		formName = ""; formSlug = ""; formDescription = "";
		showCreateModal = true;
	}

	function openEdit(org: Organisation) {
		editTarget = org;
		formName = org.name; formSlug = org.slug ?? ""; formDescription = "";
		showEditModal = true;
	}

	async function submitCreate() {
		if (!formName.trim()) return;
		acting = true;
		try {
			const result = await organisationsAdminApi.create({ name: formName.trim(), slug: formSlug.trim() || undefined });
			if (result.ok) { showCreateModal = false; load(); } else { error = result.error; }
		} catch { error = t("common.error", "Failed to create"); } finally { acting = false; }
	}

	async function submitEdit() {
		if (!editTarget) return;
		acting = true;
		try {
			const result = await organisationsAdminApi.update(editTarget.id, { name: formName.trim(), slug: formSlug.trim() || undefined });
			if (result.ok) { showEditModal = false; load(); } else { error = result.error; }
		} catch { error = t("common.error", "Failed to update"); } finally { acting = false; }
	}

	function confirmDelete(org: Organisation) { deleteTarget = org; showDeleteConfirm = true; }

	async function executeDelete() {
		if (!deleteTarget) return;
		acting = true;
		try {
			const result = await organisationsAdminApi.delete(deleteTarget.id);
			if (result.ok) { showDeleteConfirm = false; deleteTarget = null; load(); } else { error = result.error; }
		} catch { error = t("common.error", "Failed to delete"); } finally { acting = false; }
	}

	async function toggleVerify(org: Organisation) {
		acting = true;
		try {
			const result = org.verified
				? await organisationsAdminApi.unverify(org.id)
				: await organisationsAdminApi.verify(org.id, { verified: true });
			if (result.ok) load(); else error = result.error;
		} catch { error = t("common.error", "Failed to update verification"); } finally { acting = false; }
	}

	function openClaim(org: Organisation) { claimOrg = org; claimNote = ""; showClaimModal = true; }

	async function submitClaim() {
		if (!claimOrg) return;
		acting = true;
		try {
			const result = await organisationsAdminApi.claim(claimOrg.id, { note: claimNote.trim() || undefined });
			if (result.ok) showClaimModal = false; else error = result.error;
		} catch { error = t("common.error", "Failed to submit claim"); } finally { acting = false; }
	}

	async function bulkDelete() {
		const ids = Array.from(selectedIds);
		if (ids.length === 0) return;
		acting = true;
		try {
			await Promise.all(ids.map((id) => organisationsAdminApi.delete(id)));
			selectedIds = new Set(); load();
		} catch { error = t("common.error", "Failed to delete"); } finally { acting = false; }
	}

	$effect(() => { void currentPage; void pageSize; void searchQuery; void filterValues; load(); });
</script>

<svelte:head>
	<title>{t("organisations.title", "Organisations")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("organisations.title", "Organisations")}
	description={t("organisations.description", "Manage organizations and verification")}
	actions={[
		...(canManage ? [{ label: t("organisations.create", "Create Organisation"), icon: Plus, onclick: openCreate }] : []),
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost", onclick: load }
	]}
/>

{#if error}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">{error}</div>
{/if}

{#if selectedIds.size > 0}
	<div class="mb-4 flex items-center gap-3 rounded-lg p-3" style="background: var(--color-primary); color: white">
		<span class="text-sm font-medium">{selectedIds.size} selected</span>
		{#if canManage}
			<Button variant="ghost" size="sm" onclick={bulkDelete} disabled={acting}>
				<Trash2 class="mr-1 h-3.5 w-3.5" /> Delete Selected
			</Button>
		{/if}
	</div>
{/if}

<Card>
	<CardHeader><FilterBar filters={filterDefs} onsearch={handleSearch} onclear={handleClear} /></CardHeader>
	<CardContent>
		{#if loading}
			<TableSkeleton rows={8} columns={columns.length} />
		{:else}
			<div aria-live="polite">
				<DataTable {columns} data={orgs} selectable={canManage} bind:selectedIds />
			</div>
			<Pagination bind:page={currentPage} {totalPages} {totalItems} bind:pageSize />
		{/if}
	</CardContent>
</Card>

<Modal bind:open={showViewModal} title={t("organisations.details", "Organisation Details")} size="default">
	{#if viewTarget}
		<div class="grid grid-cols-2 gap-4">
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("organisations.name")}</p><p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.name}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Slug</p><p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{viewTarget.slug ?? "—"}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("organisations.owner")}</p><p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.owner_user_id ?? "—"}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("organisations.members")}</p><p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{(viewTarget.member_count ?? 0).toLocaleString()}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("organisations.verified")}</p><p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.verified ? "Yes" : "No"}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("common.created")}</p><p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(viewTarget.created_at).toLocaleString()}</p></div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		{#if canManage && viewTarget}
			<Button variant="outline" onclick={() => { showViewModal = false; toggleVerify(viewTarget!); }}>
				{#if viewTarget.verified}
					<ShieldOff class="mr-1 h-3.5 w-3.5" /> Unverify
				{:else}
					<ShieldCheck class="mr-1 h-3.5 w-3.5" /> Verify
				{/if}
			</Button>
		{/if}
		<Button variant="outline" onclick={() => (showViewModal = false)}>{t("common.close", "Close")}</Button>
	</div>
</Modal>

<Modal bind:open={showCreateModal} title={t("organisations.create", "Create Organisation")} size="default">
	<div class="space-y-3">
		<div><label for="org-name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("organisations.name")}</label><input id="org-name" type="text" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={formName} /></div>
		<div><label for="org-slug" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">Slug</label><input id="org-slug" type="text" class="w-full rounded-lg border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={formSlug} /></div>
	</div>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showCreateModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={submitCreate} disabled={acting || !formName.trim()}>{acting ? t("common.creating", "Creating...") : t("common.create", "Create")}</Button>
	</div>
</Modal>

<Modal bind:open={showEditModal} title={t("organisations.edit", "Edit Organisation")} size="default">
	{#if editTarget}
		<div class="space-y-3">
			<div><label for="edit-org-name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("organisations.name")}</label><input id="edit-org-name" type="text" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={formName} /></div>
			<div><label for="edit-org-slug" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">Slug</label><input id="edit-org-slug" type="text" class="w-full rounded-lg border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={formSlug} /></div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showEditModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={submitEdit} disabled={acting}>{acting ? t("common.saving", "Saving...") : t("common.save", "Save")}</Button>
	</div>
</Modal>

<Modal bind:open={showDeleteConfirm} title={t("common.confirmDelete", "Confirm Delete")} size="sm">
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">Delete organisation <strong>{deleteTarget.name}</strong>? This cannot be undone.</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteConfirm = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={executeDelete} disabled={acting}>{acting ? "Deleting..." : "Delete"}</Button>
	</div>
</Modal>

<Modal bind:open={showClaimModal} title={t("organisations.claim_queue", "Claim Queue")} size="default">
	<p class="text-sm mb-3" style="color: var(--color-muted)">Review ownership claim for <strong>{claimOrg?.name}</strong>.</p>
	<textarea class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" rows="3" placeholder="Admin note (optional)" bind:value={claimNote}></textarea>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showClaimModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={submitClaim} disabled={acting}>{t("common.confirm", "Confirm")}</Button>
	</div>
</Modal>

{#snippet ActionsCell(org: Organisation)}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView(org)}><Eye class="h-3.5 w-3.5" /></Button>
		{#if canManage}
			<Button variant="ghost" size="sm" onclick={() => openEdit(org)}><Pencil class="h-3.5 w-3.5" /></Button>
			<Button variant="ghost" size="sm" onclick={() => toggleVerify(org)} style="color: {org.verified ? 'var(--color-warning)' : 'var(--color-success)'}">
				{#if org.verified}<ShieldOff class="h-3.5 w-3.5" />{:else}<ShieldCheck class="h-3.5 w-3.5" />{/if}
			</Button>
			<Button variant="ghost" size="sm" onclick={() => openClaim(org)} style="color: var(--color-primary)">Claim</Button>
			<Button variant="ghost" size="sm" onclick={() => confirmDelete(org)} style="color: var(--color-danger)"><Trash2 class="h-3.5 w-3.5" /></Button>
		{/if}
	</div>
{/snippet}
