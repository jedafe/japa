<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { DataTable } from "$lib/components/ui/data-table";
	import { safeHref } from "$lib/utils/html";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { Check, X, Eye, RefreshCw } from "lucide-svelte";
	import { organisationsAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import type { BulkAction } from "$lib/components/ui/data-table";

	type PendingClaim = {
		id: string;
		organization_id: string;
		claimant_user_id: string;
		claimant_username: string;
		claimant_email: string;
		org_name: string;
		org_slug: string;
		status: string; // 'pending' | 'approved' | 'rejected' | 'withdrawn'
		evidence_url?: string | null;
		notes?: string | null;
		form_submission_id?: string | null;
		resolution_note?: string | null;
		reviewed_by_username?: string | null;
		reviewed_at?: string | null;
		created_at: string;
	};

	type ClaimSubmission = {
		id: string;
		form_id: string;
		field_values: Record<string, unknown> | null;
		status: string;
		created_at: string;
		claim_id: string;
	};

	let claims = $state<PendingClaim[]>([]);
	let pageSize = $state(25);
	let loading = $state(true);
	let errorMessage = $state("");
	let successMessage = $state("");
	let searchQuery = $state("");
	// Default to "pending" — the actionable review queue. The status filter is
	// server-side: selecting another value re-fetches from the API. "all" shows
	// the full history (approved/rejected/withdrawn) for audit.
	let filterValues = $state<Record<string, string>>({ status: "pending" });

	// BUG-022/023: Claim status options fetched from the API (Rule 16).
	let claimStatusOptions = $state<{ value: string; label: string }[]>([]);

	let showViewModal = $state(false);
	let showApproveModal = $state(false);
	let showRejectModal = $state(false);
	let target = $state<PendingClaim | null>(null);
	let claimSubmission = $state<ClaimSubmission | null>(null);
	let submissionLoading = $state(false);
	let actionLoading = $state(false);

	// Bulk selection
	let selectedIds = $state<Set<string>>(new Set());
	let showBulkApproveModal = $state(false);
	let showBulkRejectModal = $state(false);
	let bulkActionLoading = $state(false);
	let bulkTargets = $state<PendingClaim[]>([]);

	const canReview = $derived(hasPermission("admin.moderation"));

	const statusFilters = $derived([
		{
			key: "status",
			label: t("organisations.status", "Status"),
			// BUG-022/023: Use API-driven claim status options with hardcoded fallback.
			options: claimStatusOptions.length > 0
				? [{ value: "all", label: t("organisations.all", "All") }, ...claimStatusOptions]
				: [
						{ value: "all", label: t("organisations.all", "All") },
						{ value: "pending", label: t("organisations.pending", "Pending") },
						{ value: "approved", label: t("organisations.approved", "Approved") },
						{ value: "rejected", label: t("organisations.rejected", "Rejected") },
						{ value: "withdrawn", label: t("organisations.withdrawn", "Withdrawn") }
					]
		}
	]);

	const columns = $derived([
		{
			key: "org_name",
			label: t("organisations.name", "Organization"),
			render: (v: string, row: PendingClaim) =>
				`<span class="text-sm font-medium">${v}</span>` +
				(row.org_slug
					? `<span class="ms-2 font-mono text-xs" style="color: var(--color-muted)">${row.org_slug}</span>`
					: "")
		},
		{
			key: "claimant_username",
			label: t("organisations.claimant", "Claimant"),
			render: (v: string) => `<span class="text-sm font-medium">${v}</span>`
		},
		{
			key: "claimant_email",
			label: t("organisations.claimant_email", "Email"),
			render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v}</span>`
		},
		{
			key: "status",
			label: t("organisations.status", "Status"),
			componentSnippet: ClaimStatusBadge
		},
		{
			key: "created_at",
			label: t("organisations.submitted_at", "Submitted"),
			render: (v: string) =>
				`<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleDateString()}</span>`
		},
		{
			key: "actions",
			label: t("common.actions", "Actions"),
			class: "w-40",
			componentSnippet: ClaimActionsCell
		}
	]);

	// Search is client-side; status is server-side (re-fetches on change).
	const filtered = $derived.by(() => {
		let out = claims;
		const q = searchQuery.trim().toLowerCase();
		if (q) {
			out = out.filter(
				(c) =>
					c.org_name.toLowerCase().includes(q) ||
					c.claimant_username.toLowerCase().includes(q) ||
					c.claimant_email.toLowerCase().includes(q)
			);
		}
		return out;
	});

	const bulkActionsList = $derived<BulkAction[]>(
		canReview
			? [
					{
						label: t("organisations.approve_selected", "Approve Selected"),
						icon: Check,
						onclick: (ids: Set<string>) => openBulk(ids, "approve"),
						disabled: (_ids: Set<string>, rows?: any[]) =>
							!rows?.some((r: PendingClaim) => r.status === "pending")
					},
					{
						label: t("organisations.reject_selected", "Reject Selected"),
						icon: X,
						variant: "destructive",
						onclick: (ids: Set<string>) => openBulk(ids, "reject"),
						disabled: (_ids: Set<string>, rows?: any[]) =>
							!rows?.some((r: PendingClaim) => r.status === "pending")
					}
				]
			: []
	);

	async function load() {
		loading = true;
		errorMessage = "";
		selectedIds = new Set();
		try {
			// filterValues.status is read here (before the first await) so the $effect
			// below tracks it and re-fetches when the admin changes status.
			// "all" is passed explicitly so the backend returns every claim
			// (history view); an empty param would default to "pending".
			const status = filterValues.status || "pending";
			const params = `status=${status}`;
			const result = await organisationsAdminApi.pendingClaims(params);
			if (result.ok) {
				claims = result.data as unknown as PendingClaim[];
			} else {
				errorMessage = result.error ?? t("organisations.claims_load_error", "Failed to load claims");
			}
		} catch {
			errorMessage = t("organisations.claims_load_error", "Failed to load claims");
		} finally {
			loading = false;
		}
	}

	function handleSearch(q: string) {
		searchQuery = q;
	}

	function openView(c: PendingClaim) {
		target = c;
		claimSubmission = null;
		submissionLoading = false;
		showViewModal = true;
		// Fetch the claimant's form submission data if the claim has one linked.
		if (c.form_submission_id) {
			submissionLoading = true;
			organisationsAdminApi.claimSubmission(c.id).then((res) => {
				if (res.ok && res.data) {
					claimSubmission = res.data;
				}
				submissionLoading = false;
			}).catch(() => {
				submissionLoading = false;
			});
		}
	}
	function openApprove(c: PendingClaim) {
		target = c;
		showApproveModal = true;
	}
	function openReject(c: PendingClaim) {
		target = c;
		showRejectModal = true;
	}

	async function doApprove() {
		if (!target) return;
		actionLoading = true;
		errorMessage = "";
		const result = await organisationsAdminApi.claim(target.organization_id, { action: "approve" });
		if (result.ok) {
			showApproveModal = false;
			successMessage = t("organisations.claim_approved", "Claim approved");
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
		const result = await organisationsAdminApi.claim(target.organization_id, { action: "reject" });
		if (result.ok) {
			showRejectModal = false;
			successMessage = t("organisations.claim_rejected", "Claim rejected");
			load();
		} else {
			errorMessage = result.error;
		}
		actionLoading = false;
	}

	function openBulk(ids: Set<string>, action: "approve" | "reject") {
		bulkTargets = claims.filter((c) => ids.has(c.id) && c.status === "pending");
		if (bulkTargets.length === 0) return;
		if (action === "approve") showBulkApproveModal = true;
		else showBulkRejectModal = true;
	}

	async function doBulkApprove() {
		if (bulkTargets.length === 0) return;
		bulkActionLoading = true;
		errorMessage = "";
		let ok = 0;
		let fail = 0;
		for (const c of bulkTargets) {
			const r = await organisationsAdminApi.claim(c.organization_id, { action: "approve" });
			if (r.ok) ok++;
			else fail++;
		}
		bulkActionLoading = false;
		showBulkApproveModal = false;
		if (fail === 0) {
			successMessage = t("organisations.bulk_approve_success", "{n} claim(s) approved").replace("{n}", String(ok));
		} else {
			errorMessage = t("organisations.bulk_partial", "{ok} approved, {fail} failed").replace("{ok}", String(ok)).replace("{fail}", String(fail));
		}
		load();
	}

	async function doBulkReject() {
		if (bulkTargets.length === 0) return;
		bulkActionLoading = true;
		errorMessage = "";
		let ok = 0;
		let fail = 0;
		for (const c of bulkTargets) {
			const r = await organisationsAdminApi.claim(c.organization_id, { action: "reject" });
			if (r.ok) ok++;
			else fail++;
		}
		bulkActionLoading = false;
		showBulkRejectModal = false;
		if (fail === 0) {
			successMessage = t("organisations.bulk_reject_success", "{n} claim(s) rejected").replace("{n}", String(ok));
		} else {
			errorMessage = t("organisations.bulk_partial", "{ok} rejected, {fail} failed").replace("{ok}", String(ok)).replace("{fail}", String(fail));
		}
		load();
	}

	// BUG-022/023: Fetch claim status options from the API on mount (Rule 16).
	$effect(() => {
		(async () => {
			const result = await organisationsAdminApi.meta();
			if (result.ok && result.data?.claim_statuses) {
				claimStatusOptions = result.data.claim_statuses;
			}
		})();
	});

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t("organisations.claims_title", "Organization Claims")} — {siteName()} Admin</title>
</svelte:head>

<PageHeader
	title={t("organisations.claims_title", "Organization Claims")}
	description={t("organisations.claims_desc", "Review pending organization ownership claims")}
/>

{#if errorMessage}
	<div
		class="mb-4 flex items-center justify-between gap-2 flex-wrap rounded-md border px-4 py-3 text-sm"
		style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
	>
		<span>{errorMessage}</span>
		<Button variant="outline" size="sm" onclick={load}>{t("common.retry", "Retry")}</Button>
	</div>
{/if}
{#if successMessage}
	<div
		class="mb-4 rounded-md border px-4 py-3 text-sm"
		style="border-color: var(--color-success); background-color: color-mix(in srgb, var(--color-success) 8%, transparent); color: var(--color-success)"
	>
		{successMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<Button variant="outline" size="sm" onclick={load}>
		<RefreshCw class="h-3.5 w-3.5" />
		{t("common.refresh", "Refresh")}
	</Button>
</div>

{#if loading}
	<TableSkeleton rows={5} />
{:else}
	<DataTable
		{columns}
		data={filtered}
		bind:pageSize
		pageSizeOptions={[10, 25, 50, 100]}
		selectable={canReview}
		bind:selectedIds
		bulkActionsList={bulkActionsList}
		emptyMessage={t("organisations.no_claims", "No claims found")}
		searchable
		onsearch={handleSearch}
		filters={statusFilters}
		bind:filterValue={filterValues}
	/>
{/if}

<!-- View Modal -->
<Modal
	open={showViewModal}
	onclose={() => {
		showViewModal = false;
	}}
	title={t("organisations.claim_details", "Claim Details")}
>
	{#if target}
		<div class="space-y-4">
			<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t("organisations.name", "Organization")}
					</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
						{target.org_name}
					</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t("organisations.slug", "Slug")}
					</p>
					<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">
						{target.org_slug || "—"}
					</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t("organisations.claimant", "Claimant")}
					</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
						{target.claimant_username}
					</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t("organisations.claimant_email", "Email")}
					</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">{target.claimant_email}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t("organisations.status", "Status")}
					</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{target.status}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t("organisations.submitted_at", "Submitted")}
					</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">
						{new Date(target.created_at).toLocaleString()}
					</p>
				</div>
			</div>
			{#if target.evidence_url}
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t("organisations.evidence", "Evidence")}
					</p>
					<div class="mt-1 rounded-lg border p-3 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)">
						{#if target.evidence_url.startsWith("http")}
							<a href={safeHref(target.evidence_url)} target="_blank" rel="noopener noreferrer" class="text-sm underline" style="color: var(--color-primary)">{target.evidence_url}</a>
						{:else}
							{target.evidence_url}
						{/if}
					</div>
				</div>
			{/if}
			{#if target.notes}
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t("organisations.notes", "Notes")}
					</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">{target.notes}</p>
				</div>
			{/if}
			<!-- Claimant submitted form data -->
			{#if target.form_submission_id}
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t("organisations.submitted_form", "Submitted Form Data")}
					</p>
					{#if submissionLoading}
						<p class="mt-1 text-sm" style="color: var(--color-muted)">{t("common.loading", "Loading...")}</p>
					{:else if claimSubmission}
						<div class="mt-1 rounded-lg border p-3 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)">
							{#if claimSubmission.field_values && Object.keys(claimSubmission.field_values).length > 0}
								<dl class="space-y-2">
									{#each Object.entries(claimSubmission.field_values) as [key, value]}
										<div class="flex gap-2">
											<dt class="font-medium min-w-[120px]" style="color: var(--color-muted)">{key}:</dt>
											<dd class="flex-1">{typeof value === 'object' ? JSON.stringify(value) : String(value)}</dd>
										</div>
									{/each}
								</dl>
							{:else}
								<p style="color: var(--color-muted)">{t("organisations.no_form_data", "No form data submitted")}</p>
							{/if}
							<p class="mt-3 text-xs" style="color: var(--color-muted)">
								{t("organisations.submission_status", "Submission status")}: {claimSubmission.status}
								· {new Date(claimSubmission.created_at).toLocaleString()}
							</p>
						</div>
					{:else}
						<p class="mt-1 text-sm" style="color: var(--color-muted)">{t("organisations.submission_not_found", "Form submission not found")}</p>
					{/if}
				</div>
			{/if}
			<!-- Review history for approved/rejected claims -->
			{#if target.status !== "pending" && (target.reviewed_by_username || target.reviewed_at || target.resolution_note)}
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t("organisations.review_history", "Review History")}
					</p>
					<div class="mt-1 rounded-lg border p-3 text-sm space-y-1" style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)">
						{#if target.reviewed_by_username}
							<p><span style="color: var(--color-muted)">{t("organisations.reviewed_by", "Reviewed by")}:</span> {target.reviewed_by_username}</p>
						{/if}
						{#if target.reviewed_at}
							<p><span style="color: var(--color-muted)">{t("organisations.reviewed_at", "Reviewed at")}:</span> {new Date(target.reviewed_at).toLocaleString()}</p>
						{/if}
						{#if target.resolution_note}
							<p><span style="color: var(--color-muted)">{t("organisations.resolution_note", "Resolution note")}:</span> {target.resolution_note}</p>
						{/if}
					</div>
				</div>
			{/if}
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		{#if target && canReview && target.status === "pending"}
			{@const claim = target}
			<Button variant="destructive" onclick={() => { showViewModal = false; openReject(claim); }}>
				<X class="h-3.5 w-3.5" />
				{t("organisations.reject", "Reject")}
			</Button>
			<Button onclick={() => { showViewModal = false; openApprove(claim); }}>
				<Check class="h-3.5 w-3.5" />
				{t("organisations.approve", "Approve")}
			</Button>
		{/if}
		<Button variant="outline" onclick={() => { showViewModal = false; }}>{t("common.close", "Close")}</Button>
	</div>
</Modal>

<!-- Approve Modal -->
<Modal
	open={showApproveModal}
	onclose={() => {
		showApproveModal = false;
	}}
	title={t("organisations.approve_claim", "Approve Claim")}
>
	{#if target}
		<div class="space-y-3">
			<p class="text-sm" style="color: var(--color-muted)">
				{t("organisations.approve_confirm", "Approve this ownership claim?")}
			</p>
			<div class="rounded-lg border p-4 space-y-2" style="border-color: var(--color-border); background-color: var(--color-surface)">
				<div class="flex justify-between gap-2 flex-wrap text-sm">
					<span style="color: var(--color-muted)">{t("organisations.name", "Organization")}</span>
					<span class="font-medium">{target.org_name}</span>
				</div>
				<div class="flex justify-between gap-2 flex-wrap text-sm">
					<span style="color: var(--color-muted)">{t("organisations.claimant", "Claimant")}</span>
					<span class="font-medium">{target.claimant_username}</span>
				</div>
				<div class="flex justify-between gap-2 flex-wrap text-sm">
					<span style="color: var(--color-muted)">{t("organisations.claimant_email", "Email")}</span>
					<span>{target.claimant_email}</span>
				</div>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => { showApproveModal = false; }}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={doApprove} disabled={actionLoading}>
			<Check class="h-3.5 w-3.5" />
			{actionLoading ? t("common.processing", "Processing...") : t("organisations.approve", "Approve")}
		</Button>
	</div>
</Modal>

<!-- Reject Modal -->
<Modal
	open={showRejectModal}
	onclose={() => {
		showRejectModal = false;
	}}
	title={t("organisations.reject_claim", "Reject Claim")}
>
	{#if target}
		<div class="space-y-3">
			<p class="text-sm" style="color: var(--color-muted)">
				{t("organisations.reject_confirm", "Reject this ownership claim?")}
			</p>
			<div class="rounded-lg border p-4 space-y-2" style="border-color: var(--color-border); background-color: var(--color-surface)">
				<div class="flex justify-between gap-2 flex-wrap text-sm">
					<span style="color: var(--color-muted)">{t("organisations.name", "Organization")}</span>
					<span class="font-medium">{target.org_name}</span>
				</div>
				<div class="flex justify-between gap-2 flex-wrap text-sm">
					<span style="color: var(--color-muted)">{t("organisations.claimant", "Claimant")}</span>
					<span class="font-medium">{target.claimant_username}</span>
				</div>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => { showRejectModal = false; }}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={doReject} disabled={actionLoading}>
			<X class="h-3.5 w-3.5" />
			{actionLoading ? t("common.processing", "Processing...") : t("organisations.reject", "Reject")}
		</Button>
	</div>
</Modal>

<!-- Bulk Approve Modal -->
<Modal
	open={showBulkApproveModal}
	onclose={() => { showBulkApproveModal = false; }}
	title={t("organisations.approve_claims", "Approve Selected Claims")}
>
	<p class="text-sm" style="color: var(--color-muted)">
		{t("organisations.bulk_approve_confirm", "Approve {n} pending ownership claim(s)?").replace("{n}", String(bulkTargets.length))}
	</p>
	<div class="mt-4 max-h-48 overflow-y-auto rounded-lg border p-3 space-y-1" style="border-color: var(--color-border); background-color: var(--color-surface)">
		{#each bulkTargets as c (c.id)}
			<div class="flex justify-between gap-2 flex-wrap text-sm">
				<span class="font-medium">{c.org_name}</span>
				<span style="color: var(--color-muted)">{c.claimant_username}</span>
			</div>
		{/each}
	</div>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => { showBulkApproveModal = false; }}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={doBulkApprove} disabled={bulkActionLoading}>
			<Check class="h-3.5 w-3.5" />
			{bulkActionLoading ? t("common.processing", "Processing...") : t("organisations.approve", "Approve")}
		</Button>
	</div>
</Modal>

<!-- Bulk Reject Modal -->
<Modal
	open={showBulkRejectModal}
	onclose={() => { showBulkRejectModal = false; }}
	title={t("organisations.reject_claims", "Reject Selected Claims")}
>
	<p class="text-sm" style="color: var(--color-muted)">
		{t("organisations.bulk_reject_confirm", "Reject {n} pending ownership claim(s)?").replace("{n}", String(bulkTargets.length))}
	</p>
	<div class="mt-4 max-h-48 overflow-y-auto rounded-lg border p-3 space-y-1" style="border-color: var(--color-border); background-color: var(--color-surface)">
		{#each bulkTargets as c (c.id)}
			<div class="flex justify-between gap-2 flex-wrap text-sm">
				<span class="font-medium">{c.org_name}</span>
				<span style="color: var(--color-muted)">{c.claimant_username}</span>
			</div>
		{/each}
	</div>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => { showBulkRejectModal = false; }}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={doBulkReject} disabled={bulkActionLoading}>
			<X class="h-3.5 w-3.5" />
			{bulkActionLoading ? t("common.processing", "Processing...") : t("organisations.reject", "Reject")}
		</Button>
	</div>
</Modal>

{#snippet ClaimStatusBadge({ value }: { value: string; row: PendingClaim })}
	<span
		class="inline-block rounded-full px-2 py-0.5 text-xs font-medium"
		style={value === 'approved'
			? 'background-color: color-mix(in srgb, var(--color-success) 15%, transparent); color: var(--color-success)'
			: value === 'rejected'
				? 'background-color: color-mix(in srgb, var(--color-danger) 15%, transparent); color: var(--color-danger)'
				: value === 'withdrawn'
					? 'background-color: color-mix(in srgb, var(--color-muted) 15%, transparent); color: var(--color-muted)'
					: 'background-color: color-mix(in srgb, var(--color-warning) 15%, transparent); color: var(--color-warning)'}
	>
		{value}
	</span>
{/snippet}

{#snippet ClaimActionsCell({ row }: { value: any; row: PendingClaim })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView(row)} aria-label={t("common.view", "View")}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canReview && row.status === "pending"}
			<Button variant="ghost" size="sm" onclick={() => openApprove(row)} style="color: var(--color-success)" aria-label={t("organisations.approve", "Approve")}>
				<Check class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => openReject(row)} style="color: var(--color-danger)" aria-label={t("organisations.reject", "Reject")}>
				<X class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
