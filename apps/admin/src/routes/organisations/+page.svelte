<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import Pagination from "$lib/components/ui/Pagination.svelte";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { Plus, RefreshCw, Trash2 } from "lucide-svelte";
	import { organisationsAdminApi } from "$lib/api";
	import { goto } from "$app/navigation";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission, getPermissions } from "$lib/stores/permissions.svelte";
	import OrgActionsCell from "./OrgActionsCell.svelte";
	import VerifiedBadge from "./VerifiedBadge.svelte";

	type Organisation = {
		id: string;
		name: string;
		slug?: string;
		owner_user_id?: string;
		owner_username?: string;
		member_count: number;
		verified: boolean;
		created_at: string;
		status?: string;
	};

	type PendingClaim = {
		id: string;
		organization_id: string;
		claimant_username: string;
		org_name: string;
		evidence_url?: string;
		notes?: string;
		created_at: string;
	};

	let orgs = $state<Organisation[]>([]);
	let loading = $state(true);
	let error = $state("");
	let currentPage = $state(0);
	let totalItems = $state(0);
	let pageSize = $state(25);
	let totalPages = $state(1);
	let searchQuery = $state("");
	let filterVerified = $state("");
	let selectedIds = $state<Set<string>>(new Set());

	let showCreateModal = $state(false);
	let formName = $state("");
	let formSlug = $state("");
	let acting = $state(false);
	let pendingClaimsByOrg = $state<Record<string, PendingClaim>>({});

	const canManage = $derived(
		getPermissions().includes("admin.moderation") || getPermissions().includes("*")
	);

	const filterDefs = [
		{
			key: "verified",
			label: t("organisations.verified", "Verified"),
			options: [
				{ value: "true", label: t("organisations.yes", "Yes") },
				{ value: "false", label: t("organisations.no", "No") }
			]
		}
	];

	const columns = $derived([
		{ key: "name", label: t("organisations.name", "Name"), sortable: true },
		{ key: "slug", label: t("organisations.slug", "Slug"), sortable: true, render: (v: string) => v || "—" },
		{ key: "owner_username", label: t("organisations.owner", "Owner"), sortable: true, render: (v: string) => v || "—" },
		{ key: "member_count", label: t("organisations.members", "Members"), sortable: true, render: (v: number) => (v ?? 0).toLocaleString() },
		{
			key: "verified",
			label: t("organisations.verified", "Verified"),
			sortable: true,
			component: VerifiedBadge
		},
		{ key: "created_at", label: t("common.created", "Created"), sortable: true, render: (v: string) => new Date(v).toLocaleDateString() },
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-48", component: OrgActionsCell, componentProps: { pendingClaimsByOrg, onchange: load } }
	]);

	async function load() {
		loading = true;
		error = "";
		try {
			const params = new URLSearchParams({ page: String(currentPage), limit: String(pageSize) });
			if (searchQuery) params.set("q", searchQuery);
			if (filterVerified) params.set("verified", filterVerified);
			const [orgResult, claimsResult] = await Promise.all([
				organisationsAdminApi.list(params.toString()),
				organisationsAdminApi.pendingClaims()
			]);
			if (orgResult.ok) {
				const d = orgResult.data as Organisation[] | { items: Organisation[]; total: number; total_pages: number; page: number };
				if (Array.isArray(d)) {
					orgs = d;
					totalItems = d.length;
					totalPages = 1;
				} else {
					orgs = d.items ?? [];
					totalItems = d.total ?? d.items?.length ?? 0;
					totalPages = d.total_pages ?? 1;
				}
			} else {
				error = orgResult.error ?? t("common.error", "Failed to load");
			}
			if (claimsResult.ok && Array.isArray(claimsResult.data)) {
				const map: Record<string, PendingClaim> = {};
				for (const c of claimsResult.data) {
					map[c.organization_id] = c;
				}
				pendingClaimsByOrg = map;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load");
		} finally {
			loading = false;
		}
	}

	function handleSearch(values: Record<string, string>) {
		searchQuery = values.search ?? "";
		filterVerified = values.verified ?? "";
		currentPage = 0;
	}

	function handleClear() {
		searchQuery = "";
		filterVerified = "";
		currentPage = 0;
	}

	function openCreate() {
		formName = ""; formSlug = "";
		showCreateModal = true;
	}

	async function submitCreate() {
		if (!formName.trim()) return;
		acting = true;
		try {
			const result = await organisationsAdminApi.create({ name: formName.trim(), slug: formSlug.trim() || undefined });
			if (result.ok) { showCreateModal = false; load(); } else { error = result.error; }
		} catch { error = t("common.error", "Failed to create"); } finally { acting = false; }
	}

	async function bulkDelete() {
		const ids = Array.from(selectedIds);
		if (ids.length === 0) return;
		if (!confirm(t("organisations.bulk_delete_confirm", "Delete {count} selected organisations? This cannot be undone.").replace("{count}", String(ids.length)))) return;
		acting = true;
		try {
			await Promise.all(ids.map((id) => organisationsAdminApi.delete(id)));
			selectedIds = new Set(); load();
		} catch { error = t("common.error", "Failed to delete"); } finally { acting = false; }
	}

	$effect(() => {
		const page = currentPage;
		const size = pageSize;
		const q = searchQuery;
		const verified = filterVerified;
		void page; void size; void q; void verified;
		load();
	});
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
		<span class="text-sm font-medium">{t("common.selected", "{count} selected").replace("{count}", String(selectedIds.size))}</span>
		{#if canManage}
			<Button variant="ghost" size="sm" onclick={bulkDelete} disabled={acting}>
				<Trash2 class="mr-1 h-3.5 w-3.5" /> {t("common.delete_selected", "Delete Selected")}
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

<Modal bind:open={showCreateModal} title={t("organisations.create", "Create Organisation")} size="default">
	<div class="space-y-3">
		<div><label for="org-name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("organisations.name")}</label><input id="org-name" type="text" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={formName} /></div>
		<div><label for="org-slug" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("common.slug", "Slug")}</label><input id="org-slug" type="text" class="w-full rounded-lg border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={formSlug} /></div>
	</div>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showCreateModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={submitCreate} disabled={acting || !formName.trim()}>{acting ? t("common.creating", "Creating...") : t("common.create", "Create")}</Button>
	</div>
</Modal>
