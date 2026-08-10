<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { DataTable } from "$lib/components/ui/data-table";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { SlideOver } from "$lib/components/ui/slide-over";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import type { ActionItem } from "$lib/components/ui/data-table";
	import { Check, X, Ban, RotateCcw, Eye, RefreshCw, Trash2, Pencil } from "lucide-svelte";
	import { universityAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { onMount } from "svelte";
	import { pageFromUrl, pageSizeFromUrl, searchFromUrl, filterFromUrl, syncListStateToUrl } from "$lib/utils/url-sync.svelte";

	type InstructorProfile = {
		id: string;
		user_id: string;
		display_name: string;
		headline?: string | null;
		bio?: string | null;
		tier: string;
		status: string;
		commission_pct_override?: number | null;
		kyc_status: string;
		application_note?: string | null;
		reviewed_at?: string | null;
		created_at: string;
		updated_at: string;
	};

	let instructors = $state<InstructorProfile[]>([]);
	let loading = $state(true);
	let errorMessage = $state("");
	let successMessage = $state("");
	let searchQuery = $state(searchFromUrl());
	const _initFilterValues: Record<string, string> = {};
	{
		const status = filterFromUrl("status");
		if (status) _initFilterValues.status = status;
	}
	let filterValues = $state<Record<string, string>>(_initFilterValues);
	let meta = $state<{ total: number; limit: number; offset: number }>({ total: 0, limit: 25, offset: 0 });
	let currentPage = $state(pageFromUrl());
	let pageSize = $state(pageSizeFromUrl(25));

	let showApproveSlideOver = $state(false);
	let showRejectModal = $state(false);
	let showSuspendModal = $state(false);
	let showReinstateModal = $state(false);
	let showViewSlideOver = $state(false);
	let showDeleteModal = $state(false);
	let showEditTierSlideOver = $state(false);
	let target = $state<InstructorProfile | null>(null);
	let actionLoading = $state(false);

	let approveTier = $state("standard");
	let approveCommission = $state("");
	let editTier = $state("standard");
	let editCommission = $state("");

	const tierOptions = [
		{ value: "standard", label: t("university.tier_standard", "Standard") },
		{ value: "verified", label: t("university.tier_verified", "Verified") },
		{ value: "partner", label: t("university.tier_partner", "Partner") }
	];

	const canApprove = $derived(hasPermission("university.instructor.approve"));
	const canManage = $derived(hasPermission("university.instructor.manage"));

	const statusFilters = [
		{ key: "status", label: t("university.status", "Status"), options: [
			{ value: "pending", label: t("university.pending", "Pending") },
			{ value: "approved", label: t("university.approved", "Approved") },
			{ value: "rejected", label: t("university.rejected", "Rejected") },
			{ value: "suspended", label: t("university.suspended", "Suspended") }
		]}
	];

	const columns = [
		{ key: "display_name", label: t("university.instructor", "Instructor"), sortable: true, render: (v: string) => `<span class="font-medium">${v}</span>` },
		{ key: "headline", label: t("university.headline", "Headline"), render: (v: string | null) => `<span class="text-xs" style="color: var(--color-muted)">${v ?? "—"}</span>` },
		{ key: "tier", label: t("university.tier", "Tier"), sortable: true, render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${v === "partner" ? "bg-primary/15 text-primary" : v === "verified" ? "bg-info/15 text-info" : "bg-muted/15 text-muted"}">${v}</span>` },
		{ key: "status", label: t("university.status", "Status"), sortable: true, componentSnippet: StatusBadge },
		{ key: "kyc_status", label: t("university.kyc", "KYC"), render: (v: string) => `<span class="text-xs ${v === "verified" ? "text-success" : v === "pending" ? "text-warning" : "text-muted"}">${v}</span>` },
		{ key: "created_at", label: t("common.created", "Created"), sortable: true, render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleDateString()}</span>` }
	];

	const rowActions: ActionItem[] = [
		{ label: t("common.view", "View"), icon: Eye, onclick: (row) => openView(row) },
		{
			label: t("common.approve", "Approve"),
			icon: Check,
			onclick: (row) => openApprove(row),
			show: (row: InstructorProfile) => canApprove && row.status === "pending"
		},
		{
			label: t("common.reject", "Reject"),
			icon: X,
			variant: "destructive",
			onclick: (row) => openReject(row),
			show: (row: InstructorProfile) => canApprove && row.status === "pending"
		},
		{
			label: t("university.edit_tier", "Edit Tier"),
			icon: Pencil,
			onclick: (row) => openEditTier(row),
			show: (row: InstructorProfile) => canManage && row.status === "approved"
		},
		{
			label: t("common.suspend", "Suspend"),
			icon: Ban,
			variant: "destructive",
			onclick: (row) => openSuspend(row),
			show: (row: InstructorProfile) => canManage && row.status === "approved"
		},
		{
			label: t("common.reinstate", "Reinstate"),
			icon: RotateCcw,
			onclick: (row) => openReinstate(row),
			show: (row: InstructorProfile) => canManage && row.status === "suspended"
		},
		{
			label: t("common.delete", "Delete"),
			icon: Trash2,
			variant: "destructive",
			onclick: (row) => openDelete(row),
			show: () => canManage
		}
	];

	const filtered = $derived(
		searchQuery
			? instructors.filter((i) => i.display_name.toLowerCase().includes(searchQuery.toLowerCase()) || (i.headline ?? "").toLowerCase().includes(searchQuery.toLowerCase()))
			: instructors
	);

	async function load() {
		loading = true;
		errorMessage = "";
		try {
			const params = new URLSearchParams({
				limit: String(pageSize),
				offset: String(currentPage * pageSize)
			});
			if (filterValues.status) params.set("status", filterValues.status);
			const result = await universityAdminApi.listInstructors(params.toString());
			if (result.ok) {
				const data = result.data as { data?: InstructorProfile[]; meta?: typeof meta } | null | undefined;
				instructors = data?.data ?? [];
				meta = data?.meta ?? { total: instructors.length, limit: pageSize, offset: 0 };
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t("common.error", "Failed to load instructors");
		} finally {
			loading = false;
		}
	}

	function handleSearch(values: Record<string, string>) {
		searchQuery = values.search ?? "";
		filterValues = values;
		currentPage = 0;
		load();
	}

	function openApprove(i: InstructorProfile) {
		target = i;
		approveTier = i.tier === "pending" ? "standard" : i.tier;
		approveCommission = i.commission_pct_override != null ? String(i.commission_pct_override) : "";
		showApproveSlideOver = true;
	}

	function openReject(i: InstructorProfile) { target = i; showRejectModal = true; }
	function openSuspend(i: InstructorProfile) { target = i; showSuspendModal = true; }
	function openReinstate(i: InstructorProfile) { target = i; showReinstateModal = true; }
	function openView(i: InstructorProfile) { target = i; showViewSlideOver = true; }

	async function doApprove() {
		if (!target) return;
		actionLoading = true;
		errorMessage = "";
		const body: { tier?: string; commission_pct_override?: number } = {};
		if (approveTier) body.tier = approveTier;
		if (approveCommission.trim()) body.commission_pct_override = Number(approveCommission);
		const result = await universityAdminApi.approveInstructor(target.id, body);
		if (result.ok) {
			showApproveSlideOver = false;
			successMessage = t("university.approved_success", "Instructor approved");
			load();
		} else {
			errorMessage = result.error;
		}
		actionLoading = false;
	}

	async function doReject() {
		if (!target) return;
		actionLoading = true;
		errorMessage = "";
		const result = await universityAdminApi.rejectInstructor(target.id);
		if (result.ok) {
			showRejectModal = false;
			successMessage = t("university.rejected_success", "Instructor rejected");
			load();
		} else {
			errorMessage = result.error;
		}
		actionLoading = false;
	}

	async function doSuspend() {
		if (!target) return;
		actionLoading = true;
		errorMessage = "";
		const result = await universityAdminApi.suspendInstructor(target.id);
		if (result.ok) {
			showSuspendModal = false;
			successMessage = t("university.suspended_success", "Instructor suspended");
			load();
		} else {
			errorMessage = result.error;
		}
		actionLoading = false;
	}

	async function doReinstate() {
		if (!target) return;
		actionLoading = true;
		errorMessage = "";
		const result = await universityAdminApi.reinstateInstructor(target.id);
		if (result.ok) {
			showReinstateModal = false;
			successMessage = t("university.reinstated_success", "Instructor reinstated");
			load();
		} else {
			errorMessage = result.error;
		}
		actionLoading = false;
	}

	function openDelete(i: InstructorProfile) { target = i; showDeleteModal = true; }

	async function doDelete() {
		if (!target) return;
		actionLoading = true;
		errorMessage = "";
		const result = await universityAdminApi.deleteInstructor(target.id);
		if (result.ok) {
			showDeleteModal = false;
			successMessage = t("university.instructor_deleted", "Instructor deleted");
			load();
		} else {
			errorMessage = result.error;
		}
		actionLoading = false;
	}

	function openEditTier(i: InstructorProfile) {
		target = i;
		editTier = i.tier;
		editCommission = i.commission_pct_override != null ? String(i.commission_pct_override) : "";
		showEditTierSlideOver = true;
	}

	async function doEditTier() {
		if (!target) return;
		actionLoading = true;
		errorMessage = "";
		const body: { tier?: string; commission_pct_override?: number } = {};
		if (editTier) body.tier = editTier;
		if (editCommission.trim()) body.commission_pct_override = Number(editCommission);
		const result = await universityAdminApi.updateInstructor(target.id, body);
		if (result.ok) {
			showEditTierSlideOver = false;
			successMessage = t("university.instructor_updated", "Instructor updated");
			load();
		} else {
			errorMessage = result.error;
		}
		actionLoading = false;
	}

	const totalPages = $derived(Math.max(1, Math.ceil(meta.total / pageSize)));

	// Sync list state (page, search, filters) to URL for back/forward survival.
	$effect(() => {
		void currentPage;
		void pageSize;
		void searchQuery;
		void filterValues;
		syncListStateToUrl({
			page: currentPage,
			pageSize,
			defaultPageSize: 25,
			search: searchQuery,
			filters: filterValues
		});
	});

	onMount(load);
</script>

<svelte:head>
	<title>{t("university.instructors_title", "University Instructors")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("university.instructors_title", "University Instructors")}
	description={t("university.instructors_desc", "Review and manage instructor applications for the Japa University creator economy")}
	actions={[
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "outline" as const, onclick: load }
	]}
/>

{#if errorMessage}
	<div class="mb-4 flex items-center justify-between rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		<span>{errorMessage}</span>
		<Button variant="outline" size="sm" onclick={load}>{t("common.retry", "Retry")}</Button>
	</div>
{/if}
{#if successMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-success); background-color: color-mix(in srgb, var(--color-success) 8%, transparent); color: var(--color-success)">
		{successMessage}
	</div>
{/if}

<div class="mt-4">
{#if loading}
	<TableSkeleton rows={5} />
{:else}
	<DataTable
		{columns}
		data={filtered}
		page={currentPage}
		{pageSize}
		totalRows={meta.total}
		{totalPages}
		onpagechange={(p) => { currentPage = p; load(); }}
		searchable
		onsearch={(q) => { searchQuery = q; currentPage = 0; load(); }}
		filters={statusFilters}
		bind:filterValue={filterValues}
		onfilterapply={() => { currentPage = 0; load(); }}
		onfilterclear={() => { currentPage = 0; load(); }}
		actions={rowActions}
		emptyMessage={t("university.no_instructors", "No instructors found")}
	/>
{/if}
</div>

<!-- View SlideOver -->
<SlideOver bind:open={showViewSlideOver} title={t("university.instructor_details", "Instructor Details")}>
	{#if target}
		<div class="space-y-3">
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.instructor", "Instructor")}</p><p class="mt-1 text-sm font-medium">{target.display_name}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.headline", "Headline")}</p><p class="mt-1 text-sm">{target.headline ?? "—"}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.bio", "Bio")}</p><p class="mt-1 text-sm whitespace-pre-wrap">{target.bio ?? "—"}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.tier", "Tier")}</p><p class="mt-1 text-sm font-medium">{target.tier}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.status", "Status")}</p><p class="mt-1 text-sm font-medium">{target.status}</p></div>
			<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.kyc", "KYC")}</p><p class="mt-1 text-sm">{target.kyc_status}</p></div>
			{#if target.commission_pct_override != null}
				<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.commission_override", "Commission Override")}</p><p class="mt-1 text-sm">{target.commission_pct_override}%</p></div>
			{/if}
			{#if target.application_note}
				<div><p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.application_note", "Application Note")}</p><p class="mt-1 text-sm whitespace-pre-wrap">{target.application_note}</p></div>
			{/if}
		</div>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end">
			<Button variant="outline" onclick={() => { showViewSlideOver = false; }}>{t("common.close", "Close")}</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- Approve SlideOver -->
<SlideOver bind:open={showApproveSlideOver} title={t("university.approve_instructor", "Approve Instructor")}>
	{#if target}
		<div class="space-y-4">
			<p class="text-sm" style="color: var(--color-muted)">{t("university.approve_confirm", "Approve")} <strong>{target.display_name}</strong>?</p>
			<div>
				<label for="approve-tier" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t("university.tier", "Tier")}</label>
				<select id="approve-tier" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={approveTier}>
					{#each tierOptions as opt (opt)}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>
			<div>
				<label for="approve-commission" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t("university.commission_override", "Commission Override (%) — optional")}</label>
				<input id="approve-commission" type="number" step="0.01" min="0" max="100" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="e.g. 70" bind:value={approveCommission} />
			</div>
		</div>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={() => { showApproveSlideOver = false; }}>{t("common.cancel", "Cancel")}</Button>
			<Button onclick={doApprove} disabled={actionLoading}>
				{actionLoading ? t("common.processing", "Processing...") : t("common.approve", "Approve")}
			</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- Reject Modal -->
<Modal open={showRejectModal} onclose={() => { showRejectModal = false; }} title={t("university.reject_instructor", "Reject Instructor")}>
	{#if target}
		<p class="text-sm" style="color: var(--color-muted)">{t("university.reject_confirm", "Reject instructor application from")} <strong>{target.display_name}</strong>?</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => { showRejectModal = false; }}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={doReject} disabled={actionLoading}>
			{actionLoading ? t("common.processing", "Processing...") : t("common.reject", "Reject")}
		</Button>
	</div>
</Modal>

<!-- Suspend Modal -->
<Modal open={showSuspendModal} onclose={() => { showSuspendModal = false; }} title={t("university.suspend_instructor", "Suspend Instructor")}>
	{#if target}
		<p class="text-sm" style="color: var(--color-muted)">{t("university.suspend_confirm", "Suspend instructor")} <strong>{target.display_name}</strong>? {t("university.suspend_warning", "They will lose publishing access immediately.")}</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => { showSuspendModal = false; }}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={doSuspend} disabled={actionLoading}>
			{actionLoading ? t("common.processing", "Processing...") : t("common.suspend", "Suspend")}
		</Button>
	</div>
</Modal>

<!-- Reinstate Modal -->
<Modal open={showReinstateModal} onclose={() => { showReinstateModal = false; }} title={t("university.reinstate_instructor", "Reinstate Instructor")}>
	{#if target}
		<p class="text-sm" style="color: var(--color-muted)">{t("university.reinstate_confirm", "Reinstate instructor")} <strong>{target.display_name}</strong>? {t("university.reinstate_warning", "They will regain publishing access.")}</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => { showReinstateModal = false; }}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={doReinstate} disabled={actionLoading}>
			{actionLoading ? t("common.processing", "Processing...") : t("common.reinstate", "Reinstate")}
		</Button>
	</div>
</Modal>

<!-- Delete Modal -->
<Modal open={showDeleteModal} onclose={() => { showDeleteModal = false; }} title={t("common.confirmDelete", "Confirm Delete")} size="sm">
	{#if target}
		<p class="text-sm" style="color: var(--color-foreground)">{t("university.delete_instructor_confirm", "Delete instructor profile for")} <strong>{target.display_name}</strong>? {t("universities.deleteWarning", "This cannot be undone.")}</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => { showDeleteModal = false; }}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={doDelete} disabled={actionLoading}>{actionLoading ? t("common.deleting", "Deleting...") : t("common.delete", "Delete")}</Button>
	</div>
</Modal>

<!-- Edit Tier SlideOver -->
<SlideOver bind:open={showEditTierSlideOver} title={t("university.edit_tier", "Edit Tier & Commission")}>
	{#if target}
		<div class="space-y-4">
			<p class="text-sm" style="color: var(--color-muted)">{t("university.update_tier_for", "Update tier for")} <strong>{target.display_name}</strong></p>
			<div>
				<label for="edit-tier" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t("university.tier", "Tier")}</label>
				<select id="edit-tier" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={editTier}>
					{#each tierOptions as opt (opt)}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>
			<div>
				<label for="edit-commission" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t("university.commission_override", "Commission Override (%) — optional")}</label>
				<input id="edit-commission" type="number" step="0.01" min="0" max="100" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="e.g. 70" bind:value={editCommission} />
			</div>
		</div>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={() => { showEditTierSlideOver = false; }}>{t("common.cancel", "Cancel")}</Button>
			<Button onclick={doEditTier} disabled={actionLoading}>{actionLoading ? t("common.saving", "Saving...") : t("common.save", "Save")}</Button>
		</div>
	{/snippet}
</SlideOver>

{#snippet StatusBadge({ value }: { value: string; row: InstructorProfile })}
	<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium
		{value === 'approved' ? 'bg-success/15 text-success' :
		 value === 'pending' ? 'bg-warning/15 text-warning' :
		 value === 'suspended' ? 'bg-danger/15 text-danger' :
		 'bg-muted/15 text-muted'}">
		{value}
	</span>
{/snippet}
