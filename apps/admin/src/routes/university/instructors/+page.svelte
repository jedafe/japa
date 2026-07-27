<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { Check, X, Ban, RotateCcw, Eye, RefreshCw } from "lucide-svelte";
	import { universityAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

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
	let searchQuery = $state("");
	let filterValues = $state<Record<string, string>>({});
	let meta = $state<{ total: number; limit: number; offset: number }>({ total: 0, limit: 25, offset: 0 });
	let currentPage = $state(0);
	let pageSize = $state(25);

	let showApproveModal = $state(false);
	let showRejectModal = $state(false);
	let showSuspendModal = $state(false);
	let showReinstateModal = $state(false);
	let showViewModal = $state(false);
	let target = $state<InstructorProfile | null>(null);
	let actionLoading = $state(false);

	let approveTier = $state("standard");
	let approveCommission = $state("");

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
		{ key: "created_at", label: t("common.created", "Created"), sortable: true, render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleDateString()}</span>` },
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-40", componentSnippet: InstructorActionsCell }
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
				const data = result.data as { data: InstructorProfile[]; meta: typeof meta };
				instructors = data.data ?? [];
				meta = data.meta ?? { total: instructors.length, limit: pageSize, offset: 0 };
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
	}

	function openApprove(i: InstructorProfile) {
		target = i;
		approveTier = i.tier === "pending" ? "standard" : i.tier;
		approveCommission = i.commission_pct_override != null ? String(i.commission_pct_override) : "";
		showApproveModal = true;
	}

	function openReject(i: InstructorProfile) { target = i; showRejectModal = true; }
	function openSuspend(i: InstructorProfile) { target = i; showSuspendModal = true; }
	function openReinstate(i: InstructorProfile) { target = i; showReinstateModal = true; }
	function openView(i: InstructorProfile) { target = i; showViewModal = true; }

	async function doApprove() {
		if (!target) return;
		actionLoading = true;
		errorMessage = "";
		const body: { tier?: string; commission_pct_override?: number } = {};
		if (approveTier) body.tier = approveTier;
		if (approveCommission.trim()) body.commission_pct_override = Number(approveCommission);
		const result = await universityAdminApi.approveInstructor(target.id, body);
		if (result.ok) {
			showApproveModal = false;
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

	const totalPages = $derived(Math.max(1, Math.ceil(meta.total / pageSize)));

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t("university.instructors_title", "University Instructors")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("university.instructors_title", "University Instructors")}
	description={t("university.instructors_desc", "Review and manage instructor applications for the Japa University creator economy")}
/>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}
{#if successMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-success); background-color: color-mix(in srgb, var(--color-success) 8%, transparent); color: var(--color-success)">
		{successMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<Button variant="outline" size="sm" onclick={load}>
		<RefreshCw class="h-3.5 w-3.5" />
		{t("common.refresh", "Refresh")}
	</Button>
</div>

<FilterBar filters={statusFilters} onsearch={handleSearch} onclear={() => { searchQuery = ""; filterValues = {}; currentPage = 0; }} />

{#if loading}
	<TableSkeleton rows={5} />
{:else}
	<DataTable
		{columns}
		data={filtered}
		searchable={true}
		searchPlaceholder={t("university.search_instructors", "Search instructors...")}
		onsearch={(q: string) => { searchQuery = q; }}
		pageSize={pageSize}
		emptyMessage={t("university.no_instructors", "No instructors found")}
	/>
	<div class="mt-4 flex items-center justify-between text-xs" style="color: var(--color-muted)">
		<span>{meta.total} {t("common.total", "total")}</span>
		<div class="flex items-center gap-2">
			<Button variant="ghost" size="sm" disabled={currentPage === 0} onclick={() => { currentPage = Math.max(0, currentPage - 1); }}>
				{t("common.prev", "Prev")}
			</Button>
			<span>{currentPage + 1} / {totalPages}</span>
			<Button variant="ghost" size="sm" disabled={currentPage >= totalPages - 1} onclick={() => { currentPage = Math.min(totalPages - 1, currentPage + 1); }}>
				{t("common.next", "Next")}
			</Button>
		</div>
	</div>
{/if}

<!-- View Modal -->
<Modal open={showViewModal} onclose={() => { showViewModal = false; }} title={t("university.instructor_details", "Instructor Details")}>
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
	<div class="mt-6 flex justify-end">
		<Button variant="outline" onclick={() => { showViewModal = false; }}>{t("common.close", "Close")}</Button>
	</div>
</Modal>

<!-- Approve Modal -->
<Modal open={showApproveModal} onclose={() => { showApproveModal = false; }} title={t("university.approve_instructor", "Approve Instructor")}>
	{#if target}
		<div class="space-y-4">
			<p class="text-sm" style="color: var(--color-muted)">{t("university.approve_confirm", "Approve")} <strong>{target.display_name}</strong>?</p>
			<div>
				<label for="approve-tier" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t("university.tier", "Tier")}</label>
				<select id="approve-tier" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={approveTier}>
					<option value="standard">standard</option>
					<option value="verified">verified</option>
					<option value="partner">partner</option>
				</select>
			</div>
			<div>
				<label for="approve-commission" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t("university.commission_override", "Commission Override (%) — optional")}</label>
				<input id="approve-commission" type="number" step="0.01" min="0" max="100" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="e.g. 70" bind:value={approveCommission} />
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => { showApproveModal = false; }}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={doApprove} disabled={actionLoading}>
			{actionLoading ? t("common.processing", "Processing...") : t("common.approve", "Approve")}
		</Button>
	</div>
</Modal>

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

{#snippet StatusBadge({ value }: { value: string; row: InstructorProfile })}
	<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium
		{value === 'approved' ? 'bg-success/15 text-success' :
		 value === 'pending' ? 'bg-warning/15 text-warning' :
		 value === 'suspended' ? 'bg-danger/15 text-danger' :
		 'bg-muted/15 text-muted'}">
		{value}
	</span>
{/snippet}

{#snippet InstructorActionsCell({ row }: { value: any; row: InstructorProfile })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView(row)} aria-label={t("common.view", "View")}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canApprove && row.status === "pending"}
			<Button variant="ghost" size="sm" onclick={() => openApprove(row)} style="color: var(--color-success)" aria-label={t("common.approve", "Approve")}>
				<Check class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => openReject(row)} style="color: var(--color-danger)" aria-label={t("common.reject", "Reject")}>
				<X class="h-3.5 w-3.5" />
			</Button>
		{/if}
		{#if canManage && row.status === "approved"}
			<Button variant="ghost" size="sm" onclick={() => openSuspend(row)} style="color: var(--color-danger)" aria-label={t("common.suspend", "Suspend")}>
				<Ban class="h-3.5 w-3.5" />
			</Button>
		{/if}
		{#if canManage && row.status === "suspended"}
			<Button variant="ghost" size="sm" onclick={() => openReinstate(row)} style="color: var(--color-success)" aria-label={t("common.reinstate", "Reinstate")}>
				<RotateCcw class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
