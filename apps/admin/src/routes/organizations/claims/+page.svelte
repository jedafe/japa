<script lang="ts">
	import { siteName } from '#lib/stores/site.svelte.js';
	// /organizations/claims — migrated to the EntityPage scaffold. Read-only
	// review queue: the endpoint only supports ?status= (no pagination), so
	// search/sort/page are applied client-side inside load().
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from "#lib/components/entity/index.js";
	import { Modal } from "#lib/components/ui/modal/index.js";
	import { safeHref } from "#lib/utils/html.js";
	import { Button } from "#lib/components/ui/button/index.js";
	import { Check, X, Eye } from "lucide-svelte";
	import { organisationsAdminApi } from "#lib/api.js";
	import { t } from "#lib/stores/i18n.svelte.js";
	import { hasPermission } from "#lib/stores/permissions.svelte.js";
	import type { ActionItem, FilterDef } from "#lib/components/ui/data-table/index.js";

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

	const canReview = $derived(hasPermission("admin.moderation"));

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state("");
	let successMessage = $state("");

	// BUG-022/023: Claim status options fetched from the API (Rule 16).
	let claimStatusOptions = $state<{ value: string; label: string }[]>([]);

	// View modal — page-local because it lazy-loads the claimant's form
	// submission and carries pending-only Approve/Reject footer actions.
	let showViewModal = $state(false);
	let showApproveModal = $state(false);
	let showRejectModal = $state(false);
	let target = $state<PendingClaim | null>(null);
	let claimSubmission = $state<ClaimSubmission | null>(null);
	let submissionLoading = $state(false);
	let actionLoading = $state(false);

	let showBulkApproveModal = $state(false);
	let showBulkRejectModal = $state(false);
	let bulkActionLoading = $state(false);
	let bulkTargets = $state<PendingClaim[]>([]);
	const claimRowMap = new Map<string, PendingClaim>();

	const filters = $derived<FilterDef[]>([
		{
			key: "status",
			label: t("organisations.status", "Status"),
			// BUG-022/023: API-driven claim status options with fallback. Empty
			// selection = pending (the actionable review queue).
			placeholder: t("organisations.pending", "Pending"),
			options: [
				{ value: "all", label: t("organisations.all", "All") },
				...(claimStatusOptions.length > 0
					? claimStatusOptions
					: [
							{ value: "pending", label: t("organisations.pending", "Pending") },
							{ value: "approved", label: t("organisations.approved", "Approved") },
							{ value: "rejected", label: t("organisations.rejected", "Rejected") },
							{ value: "withdrawn", label: t("organisations.withdrawn", "Withdrawn") }
						])
			]
		}
	]);

	const columns = $derived([
		{
			key: "org_name",
			sortable: true,
			label: t("organisations.name", "Organization"),
			render: (v: string, row: PendingClaim) =>
				`<span class="text-sm font-medium">${v}</span>` +
				(row.org_slug
					? `<span class="ms-2 font-mono text-xs" style="color: var(--color-muted)">${row.org_slug}</span>`
					: "")
		},
		{
			key: "claimant_username",
			sortable: true,
			label: t("organisations.claimant", "Claimant"),
			render: (v: string) => `<span class="text-sm font-medium">${v}</span>`
		},
		{
			key: "claimant_email",
			sortable: true,
			label: t("organisations.claimant_email", "Email"),
			render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v}</span>`
		},
		{
			key: "status",
			label: t("organisations.status", "Status"),
			sortable: true,
			componentSnippet: ClaimStatusBadge
		},
		{
			key: "created_at",
			sortable: true,
			label: t("organisations.submitted_at", "Submitted"),
			render: (v: string) =>
				`<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleDateString()}</span>`
		}
	]);

	const rowActions = $derived<ActionItem[]>([
		{ label: t("common.view", "View"), icon: Eye, onclick: (row: PendingClaim) => openView(row) },
		{
			label: t("organisations.approve", "Approve"),
			icon: Check,
			onclick: (row: PendingClaim) => { target = row; showApproveModal = true; },
			show: (row: PendingClaim) => canReview && row.status === "pending"
		},
		{
			label: t("organisations.reject", "Reject"),
			icon: X,
			variant: "destructive",
			onclick: (row: PendingClaim) => { target = row; showRejectModal = true; },
			show: (row: PendingClaim) => canReview && row.status === "pending"
		}
	]);

	const bulkActionsList = $derived(
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
						variant: "destructive" as const,
						onclick: (ids: Set<string>) => openBulk(ids, "reject"),
						disabled: (_ids: Set<string>, rows?: any[]) =>
							!rows?.some((r: PendingClaim) => r.status === "pending")
					}
				]
			: []
	);

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

	// PATCH /claim-ownership/{id} by claim id — deterministic (the org-scoped
	// POST resolves the NEWEST pending claim, which misfires when several
	// users hold pending claims on the same org) and transactional (approve
	// transfers ownership + adds the claimant as owner member).
	async function doApprove() {
		if (!target) return;
		actionLoading = true;
		pageError = "";
		const result = await organisationsAdminApi.reviewClaim(target.id, { status: "approved" });
		if (result.ok) {
			showApproveModal = false;
			successMessage = t("organisations.claim_approved", "Claim approved");
			entityController?.reload();
		} else {
			pageError = result.error ?? t("common.error", "Failed to approve claim");
		}
		actionLoading = false;
	}

	async function doReject() {
		if (!target) return;
		actionLoading = true;
		pageError = "";
		const result = await organisationsAdminApi.reviewClaim(target.id, { status: "rejected" });
		if (result.ok) {
			showRejectModal = false;
			successMessage = t("organisations.claim_rejected", "Claim rejected");
			entityController?.reload();
		} else {
			pageError = result.error ?? t("common.error", "Failed to reject claim");
		}
		actionLoading = false;
	}

	function openBulk(ids: Set<string>, action: "approve" | "reject") {
		bulkTargets = Array.from(ids)
			.map((id) => claimRowMap.get(id))
			.filter((c): c is PendingClaim => !!c && c.status === "pending");
		if (bulkTargets.length === 0) return;
		if (action === "approve") showBulkApproveModal = true;
		else showBulkRejectModal = true;
	}

	async function doBulkReview(status: "approved" | "rejected") {
		if (bulkTargets.length === 0) return;
		bulkActionLoading = true;
		pageError = "";
		let ok = 0;
		let fail = 0;
		for (const c of bulkTargets) {
			const r = await organisationsAdminApi.reviewClaim(c.id, { status });
			if (r.ok) ok++;
			else fail++;
		}
		bulkActionLoading = false;
		showBulkApproveModal = false;
		showBulkRejectModal = false;
		const key = status === "approved" ? "organisations.bulk_approve_success" : "organisations.bulk_reject_success";
		const fallback = status === "approved" ? "{n} claim(s) approved" : "{n} claim(s) rejected";
		if (fail === 0) {
			successMessage = t(key, fallback).replace("{n}", String(ok));
		} else {
			pageError = t("organisations.bulk_partial", "{ok} approved, {fail} failed").replace("{ok}", String(ok)).replace("{fail}", String(fail));
		}
		entityController?.reload();
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

	// `fields` is required by EntityPageConfig but unused on readonly pages.
	const fields: FieldDef[] = [];

	const config = $derived<EntityPageConfig>({
		title: t("organisations.claims_title", "Organization Claims"),
		description: t("organisations.claims_desc", "Review pending organization ownership claims"),
		readonly: true,
		pageSize: 25,
		columns,
		filters,
		actions: rowActions,
		bulkActions: bulkActionsList,
		bulkDelete: false,
		fields,
		load: async ({ page, pageSize, q, sort, filter }) => {
			// Server-side pagination/search/sort — the endpoint accepts
			// page/limit/q/sort/dir and returns {data, meta}.
			const params = new URLSearchParams();
			params.set("status", filter.status || "pending");
			params.set("page", String(page + 1));
			params.set("limit", String(pageSize));
			if (q) params.set("q", q);
			if (sort) {
				const [key, dir] = sort.split(":");
				params.set("sort", key);
				params.set("dir", dir === "asc" ? "asc" : "desc");
			}
			const result = await organisationsAdminApi.pendingClaimsMeta(params.toString());
			if (!result.ok) {
				throw new Error(result.error ?? t("organisations.claims_load_error", "Failed to load claims"));
			}
			const rows = (result.data?.data ?? []) as unknown as PendingClaim[];
			for (const c of rows) claimRowMap.set(c.id, c);
			return {
				data: rows,
				total: result.data?.meta?.total ?? rows.length,
				totalPages: Math.max(1, result.data?.meta?.total_pages ?? 1)
			};
		}
	});
</script>

<svelte:head>
	<title>{t("organisations.claims_title", "Organization Claims")} — {siteName()} Admin</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 flex items-center justify-between gap-2 flex-wrap rounded-md border px-4 py-3 text-sm"
		style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
	>
		<span>{pageError}</span>
		<Button variant="outline" size="sm" onclick={() => entityController?.reload()}>{t("common.retry", "Retry")}</Button>
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

<EntityPage {config} bind:controller={entityController} />

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
			<Button variant="destructive" onclick={() => { showViewModal = false; target = claim; showRejectModal = true; }}>
				<X class="h-3.5 w-3.5" />
				{t("organisations.reject", "Reject")}
			</Button>
			<Button onclick={() => { showViewModal = false; target = claim; showApproveModal = true; }}>
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
	onclose={() => {
		showBulkApproveModal = false;
	}}
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
		<Button onclick={() => doBulkReview("approved")} disabled={bulkActionLoading}>
			<Check class="h-3.5 w-3.5" />
			{bulkActionLoading ? t("common.processing", "Processing...") : t("organisations.approve", "Approve")}
		</Button>
	</div>
</Modal>

<!-- Bulk Reject Modal -->
<Modal
	open={showBulkRejectModal}
	onclose={() => {
		showBulkRejectModal = false;
	}}
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
		<Button variant="destructive" onclick={() => doBulkReview("rejected")} disabled={bulkActionLoading}>
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
