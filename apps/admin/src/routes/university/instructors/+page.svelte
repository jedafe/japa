<script lang="ts">
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from "$lib/components/entity";
	import { Modal } from "$lib/components/ui/modal";
	import { Button } from "$lib/components/ui/button";
	import { SlideOver } from "$lib/components/ui/slide-over";
	import { universityAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { Check, X, Ban, RotateCcw, Eye, Trash2, Pencil } from "lucide-svelte";
	import type { ActionItem, FilterDef } from "$lib/components/ui/data-table";

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

	const canApprove = $derived(hasPermission("university.instructor.approve"));
	const canManage = $derived(hasPermission("university.instructor.manage"));

	// ── Filter options ──────────────────────────────────────────────────────
	const filterDefs = $derived<FilterDef[]>([
		{
			key: "status",
			label: t("university.status", "Status"),
			options: [
				{ value: "pending", label: t("university.pending", "Pending") },
				{ value: "approved", label: t("university.approved", "Approved") },
				{ value: "rejected", label: t("university.rejected", "Rejected") },
				{ value: "suspended", label: t("university.suspended", "Suspended") }
			]
		}
	]);

	// ── Page-local state for sub-flows ──────────────────────────────────────
	// EntityPage owns list/pagination/search state; the page only owns
	// state-action modals + the approve/edit-tier form fields.
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state("");
	let acting = $state(false);

	let showApproveSlideOver = $state(false);
	let showRejectModal = $state(false);
	let showSuspendModal = $state(false);
	let showReinstateModal = $state(false);
	let showDeleteModal = $state(false);
	let showEditTierSlideOver = $state(false);
	let target = $state<InstructorProfile | null>(null);

	let approveTier = $state("standard");
	let approveCommission = $state("");
	let editTier = $state("standard");
	let editCommission = $state("");

	const tierOptions = [
		{ value: "standard", label: t("university.tier_standard", "Standard") },
		{ value: "verified", label: t("university.tier_verified", "Verified") },
		{ value: "partner", label: t("university.tier_partner", "Partner") }
	];

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t("common.view", "View"), icon: Eye, onclick: (row: InstructorProfile, ctx) => ctx?.openView(row) },
			{
				label: t("common.approve", "Approve"),
				icon: Check,
				onclick: (row: InstructorProfile) => openApprove(row),
				show: (row: InstructorProfile) => canApprove && row.status === "pending"
			},
			{
				label: t("common.reject", "Reject"),
				icon: X,
				variant: "destructive",
				onclick: (row: InstructorProfile) => openReject(row),
				show: (row: InstructorProfile) => canApprove && row.status === "pending"
			},
			{
				label: t("university.edit_tier", "Edit Tier"),
				icon: Pencil,
				onclick: (row: InstructorProfile) => openEditTier(row),
				show: (row: InstructorProfile) => canManage && row.status === "approved"
			},
			{
				label: t("common.suspend", "Suspend"),
				icon: Ban,
				variant: "destructive",
				onclick: (row: InstructorProfile) => openSuspend(row),
				show: (row: InstructorProfile) => canManage && row.status === "approved"
			},
			{
				label: t("common.reinstate", "Reinstate"),
				icon: RotateCcw,
				onclick: (row: InstructorProfile) => openReinstate(row),
				show: (row: InstructorProfile) => canManage && row.status === "suspended"
			},
			{
				label: t("common.delete", "Delete"),
				icon: Trash2,
				variant: "destructive",
				onclick: (row: InstructorProfile) => openDelete(row),
				show: () => canManage
			}
		];
		return acts;
	});

	// ── Fields (readonly entity — no create; fields required by EntityPage) ──
	const fields = $derived<FieldDef[]>([
		{ key: "display_name", label: t("university.instructor", "Instructor"), type: "text", required: true },
		{ key: "tier", label: t("university.tier", "Tier"), type: "select", options: tierOptions },
		{ key: "commission_pct_override", label: t("university.commission_override", "Commission Override"), type: "number" }
	]);

	// ── Config ──────────────────────────────────────────────────────────────
	// Built as $derived so canApprove/canManage/rowActions updates propagate.
	const config = $derived<EntityPageConfig>({
		title: t("university.instructors_title", "University Instructors"),
		description: t("university.instructors_desc", "Review and manage instructor applications for the Japa University creator economy"),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: "display_name", label: t("university.instructor", "Instructor"), render: (v: string) => `<span class="font-medium">${v}</span>` },
			{ key: "headline", label: t("university.headline", "Headline"), render: (v: string | null) => `<span class="text-xs" style="color: var(--color-muted)">${v ?? "—"}</span>` },
			{
				key: "tier",
				label: t("university.tier", "Tier"),
				render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${v === "partner" ? "bg-primary/15 text-primary" : v === "verified" ? "bg-info/15 text-info" : "bg-muted/15 text-muted"}">${v}</span>`
			},
			{
				key: "status",
				label: t("university.status", "Status"),
				componentSnippet: StatusBadge
			},
			{
				key: "kyc_status",
				label: t("university.kyc", "KYC"),
				render: (v: string) => `<span class="text-xs ${v === "verified" ? "text-success" : v === "pending" ? "text-warning" : "text-muted"}">${v}</span>`
			},
			{ key: "created_at", label: t("common.created", "Created"), render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleDateString()}</span>` }
		],
		filters: filterDefs,
		actions: rowActions,
		fields,
		load: async ({ page, pageSize, q, filter }) => {
			const params = new URLSearchParams({
				limit: String(pageSize),
				offset: String(page * pageSize)
			});
			if (q) params.set("q", q);
			if (filter?.status) params.set("status", filter.status);
			const result = await universityAdminApi.listInstructors(params.toString());
			if (result.ok) {
				const body = result.data as { data?: InstructorProfile[]; meta?: { total: number; limit: number; offset: number } } | null | undefined;
				const data = body?.data ?? [];
				const meta = body?.meta ?? { total: data.length, limit: pageSize, offset: 0 };
				return {
					data,
					total: meta.total ?? data.length,
					totalPages: Math.max(1, Math.ceil((meta.total ?? data.length) / pageSize))
				};
			}
			throw new Error(result.error ?? t("common.error", "Failed to load instructors"));
		},
		viewSnippet: InstructorView
	});

	// ── Sub-flow handlers ───────────────────────────────────────────────────
	function openApprove(i: InstructorProfile) {
		target = i;
		approveTier = i.tier === "pending" ? "standard" : i.tier;
		approveCommission = i.commission_pct_override != null ? String(i.commission_pct_override) : "";
		showApproveSlideOver = true;
	}

	function openReject(i: InstructorProfile) { target = i; showRejectModal = true; }
	function openSuspend(i: InstructorProfile) { target = i; showSuspendModal = true; }
	function openReinstate(i: InstructorProfile) { target = i; showReinstateModal = true; }
	function openDelete(i: InstructorProfile) { target = i; showDeleteModal = true; }

	function openEditTier(i: InstructorProfile) {
		target = i;
		editTier = i.tier;
		editCommission = i.commission_pct_override != null ? String(i.commission_pct_override) : "";
		showEditTierSlideOver = true;
	}

	async function doApprove() {
		if (!target) return;
		acting = true;
		pageError = "";
		try {
			const body: { tier?: string; commission_pct_override?: number } = {};
			if (approveTier) body.tier = approveTier;
			if (approveCommission.trim()) body.commission_pct_override = Number(approveCommission);
			const result = await universityAdminApi.approveInstructor(target.id, body);
			if (result.ok) { showApproveSlideOver = false; entityController?.reload(); }
			else { pageError = result.error; }
		} catch { pageError = t("common.error", "Failed to approve instructor"); } finally { acting = false; }
	}

	async function doReject() {
		if (!target) return;
		acting = true;
		pageError = "";
		try {
			const result = await universityAdminApi.rejectInstructor(target.id);
			if (result.ok) { showRejectModal = false; entityController?.reload(); }
			else { pageError = result.error; }
		} catch { pageError = t("common.error", "Failed to reject instructor"); } finally { acting = false; }
	}

	async function doSuspend() {
		if (!target) return;
		acting = true;
		pageError = "";
		try {
			const result = await universityAdminApi.suspendInstructor(target.id);
			if (result.ok) { showSuspendModal = false; entityController?.reload(); }
			else { pageError = result.error; }
		} catch { pageError = t("common.error", "Failed to suspend instructor"); } finally { acting = false; }
	}

	async function doReinstate() {
		if (!target) return;
		acting = true;
		pageError = "";
		try {
			const result = await universityAdminApi.reinstateInstructor(target.id);
			if (result.ok) { showReinstateModal = false; entityController?.reload(); }
			else { pageError = result.error; }
		} catch { pageError = t("common.error", "Failed to reinstate instructor"); } finally { acting = false; }
	}

	async function doDelete() {
		if (!target) return;
		acting = true;
		pageError = "";
		try {
			const result = await universityAdminApi.deleteInstructor(target.id);
			if (result.ok) { showDeleteModal = false; entityController?.reload(); }
			else { pageError = result.error; }
		} catch { pageError = t("common.error", "Failed to delete instructor"); } finally { acting = false; }
	}

	async function doEditTier() {
		if (!target) return;
		acting = true;
		pageError = "";
		try {
			const body: { tier?: string; commission_pct_override?: number } = {};
			if (editTier) body.tier = editTier;
			if (editCommission.trim()) body.commission_pct_override = Number(editCommission);
			const result = await universityAdminApi.updateInstructor(target.id, body);
			if (result.ok) { showEditTierSlideOver = false; entityController?.reload(); }
			else { pageError = result.error; }
		} catch { pageError = t("common.error", "Failed to update instructor"); } finally { acting = false; }
	}

	// ── View snippet is declared in the template as {#snippet InstructorView(...)}
</script>

<svelte:head>
	<title>{t("university.instructors_title", "University Instructors")} — JapaConnect Admin</title>
</svelte:head>

{#if pageError}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- ── Approve slide-over (page-local because approve isn't in EntityPage config) ── -->
<SlideOver bind:open={showApproveSlideOver} title={t("university.approve_instructor", "Approve Instructor")}>
	{#if target}
		<div class="space-y-4">
			<p class="text-sm" style="color: var(--color-muted)">{t("university.approve_confirm", "Approve")} <strong>{target.display_name}</strong>?</p>
			<div>
				<label for="approve-tier" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t("university.tier", "Tier")}</label>
				<select id="approve-tier" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={approveTier}>
					{#each tierOptions as opt (opt.value)}
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
			<Button onclick={doApprove} disabled={acting}>
				{acting ? t("common.processing", "Processing...") : t("common.approve", "Approve")}
			</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- ── Reject modal ── -->
<Modal bind:open={showRejectModal} title={t("university.reject_instructor", "Reject Instructor")}>
	{#if target}
		<p class="text-sm" style="color: var(--color-muted)">{t("university.reject_confirm", "Reject instructor application from")} <strong>{target.display_name}</strong>?</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => { showRejectModal = false; }}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={doReject} disabled={acting}>
			{acting ? t("common.processing", "Processing...") : t("common.reject", "Reject")}
		</Button>
	</div>
</Modal>

<!-- ── Suspend modal ── -->
<Modal bind:open={showSuspendModal} title={t("university.suspend_instructor", "Suspend Instructor")}>
	{#if target}
		<p class="text-sm" style="color: var(--color-muted)">{t("university.suspend_confirm", "Suspend instructor")} <strong>{target.display_name}</strong>? {t("university.suspend_warning", "They will lose publishing access immediately.")}</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => { showSuspendModal = false; }}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={doSuspend} disabled={acting}>
			{acting ? t("common.processing", "Processing...") : t("common.suspend", "Suspend")}
		</Button>
	</div>
</Modal>

<!-- ── Reinstate modal ── -->
<Modal bind:open={showReinstateModal} title={t("university.reinstate_instructor", "Reinstate Instructor")}>
	{#if target}
		<p class="text-sm" style="color: var(--color-muted)">{t("university.reinstate_confirm", "Reinstate instructor")} <strong>{target.display_name}</strong>? {t("university.reinstate_warning", "They will regain publishing access.")}</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => { showReinstateModal = false; }}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={doReinstate} disabled={acting}>
			{acting ? t("common.processing", "Processing...") : t("common.reinstate", "Reinstate")}
		</Button>
	</div>
</Modal>

<!-- ── Delete confirmation ── -->
<Modal bind:open={showDeleteModal} title={t("common.confirmDelete", "Confirm Delete")} size="sm">
	{#if target}
		<p class="text-sm" style="color: var(--color-foreground)">{t("university.delete_instructor_confirm", "Delete instructor profile for")} <strong>{target.display_name}</strong>? {t("universities.deleteWarning", "This cannot be undone.")}</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => { showDeleteModal = false; }}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={doDelete} disabled={acting}>{acting ? t("common.deleting", "Deleting...") : t("common.delete", "Delete")}</Button>
	</div>
</Modal>

<!-- ── Edit Tier & Commission slide-over (page-local) ── -->
<SlideOver bind:open={showEditTierSlideOver} title={t("university.edit_tier", "Edit Tier & Commission")}>
	{#if target}
		<div class="space-y-4">
			<p class="text-sm" style="color: var(--color-muted)">{t("university.update_tier_for", "Update tier for")} <strong>{target.display_name}</strong></p>
			<div>
				<label for="edit-tier" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t("university.tier", "Tier")}</label>
				<select id="edit-tier" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={editTier}>
					{#each tierOptions as opt (opt.value)}
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
			<Button onclick={doEditTier} disabled={acting}>{acting ? t("common.saving", "Saving...") : t("common.save", "Save")}</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- ── View snippet ──────────────────────────────────────────────────────── -->
{#snippet InstructorView({ row }: { row: InstructorProfile })}
	<div class="space-y-3">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.instructor", "Instructor")}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.display_name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.headline", "Headline")}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.headline ?? "—"}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.bio", "Bio")}</p>
			<p class="mt-1 text-sm whitespace-pre-wrap" style="color: var(--color-foreground)">{row.bio ?? "—"}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.tier", "Tier")}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.tier}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.status", "Status")}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.kyc", "KYC")}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.kyc_status}</p>
		</div>
		{#if row.commission_pct_override != null}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.commission_override", "Commission Override")}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.commission_pct_override}%</p>
			</div>
		{/if}
		{#if row.application_note}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.application_note", "Application Note")}</p>
				<p class="mt-1 text-sm whitespace-pre-wrap" style="color: var(--color-foreground)">{row.application_note}</p>
			</div>
		{/if}
		{#if row.reviewed_at}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("university.reviewed_at", "Reviewed At")}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{new Date(row.reviewed_at).toLocaleString()}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("common.created", "Created")}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{new Date(row.created_at).toLocaleString()}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("common.updated", "Updated")}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{new Date(row.updated_at).toLocaleString()}</p>
		</div>
	</div>
{/snippet}

{#snippet StatusBadge({ value }: { value: string; row: InstructorProfile })}
	<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium
		{value === 'approved' ? 'bg-success/15 text-success' :
		 value === 'pending' ? 'bg-warning/15 text-warning' :
		 value === 'suspended' ? 'bg-danger/15 text-danger' :
		 'bg-muted/15 text-muted'}">
		{value}
	</span>
{/snippet}
