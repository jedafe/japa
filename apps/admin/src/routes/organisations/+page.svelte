<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { siteName } from '$lib/stores/site.svelte';
	// /organisations — migrated to the EntityPage scaffold; CRUD entity with state actions (verify/unverify) and an approve-claim sub-flow.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from "$lib/components/entity";
	import { Modal } from "$lib/components/ui/modal";
	import { safeHref } from "$lib/utils/html";
	import { Button } from "$lib/components/ui/button";
	import { organisationsAdminApi, apiRaw } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { Eye, Pencil, Trash2, ShieldCheck, ShieldOff } from "lucide-svelte";
	import type { ActionItem, FilterDef } from "$lib/components/ui/data-table";

	type Organisation = {
		id: string;
		name: string;
		slug?: string;
		org_type?: string;
		tagline?: string | null;
		description?: string | null;
		website_url?: string | null;
		country_code?: string | null;
		owner_user_id?: string;
		owner_username?: string;
		member_count: number;
		follower_count?: number;
		verified: boolean;
		verification_type?: string | null;
		created_at: string;
		status?: string;
		deleted_at?: string | null;
	};

	type PendingClaim = {
		id: string;
		organization_id: string;
		claimant_username: string;
		org_name: string;
		evidence_url?: string | null;
		notes?: string | null;
		created_at: string;
	};

	const canManage = $derived(hasPermission("admin.moderation"));

	// Pending claims loaded as a side effect of load() so the "Approve Claim" action show() predicate can react.
	let pendingClaimsByOrg = $state<Record<string, PendingClaim>>({});

	// ── Bulk delete ───── EntityPage auto-reloads + clears selection after a bulk action handler completes.
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state("");
	let bulkDeleting = $state(false);

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return; // in-flight guard
		bulkDeleting = true;
		pageError = "";
		try {
			await Promise.all(idArr.map((id) => organisationsAdminApi.delete(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t("common.error", "Failed to delete organisations");
		} finally {
			bulkDeleting = false;
		}
	}

	// ── Force delete — single + bulk share one confirm modal ──
	// BulkAction.onclick receives only ids — resolve them against a map of
	// every row seen this session so deleted-row filtering keeps working.
	const orgRowMap = new Map<string, Organisation>();
	const orgRowsFor = (ids: Set<string>): Organisation[] =>
		Array.from(ids)
			.map((id) => orgRowMap.get(id))
			.filter((r): r is Organisation => !!r);

	let forceTargets = $state<Organisation[]>([]);
	let showForceModal = $state(false);

	function openForceDelete(row: Organisation) {
		forceTargets = [row];
		showForceModal = true;
	}

	function openBulkForceDelete(ids: Set<string>) {
		const rows = orgRowsFor(ids).filter((r) => !!r.deleted_at);
		if (rows.length === 0) return;
		forceTargets = rows;
		showForceModal = true;
	}

	async function confirmForceDelete() {
		if (acting || forceTargets.length === 0) return;
		acting = true;
		pageError = "";
		try {
			const targets = forceTargets;
			let failed = 0;
			let lastErr = "";
			for (const row of targets) {
				const res = await organisationsAdminApi.hardDelete(row.id);
				if (!res.ok) {
					failed++;
					lastErr = res.error ?? "";
				}
			}
			showForceModal = false;
			forceTargets = [];
			if (failed > 0) {
				pageError =
					t("common.bulkDeletePartial", "{failed} of {n} items could not be deleted")
						.replace("{failed}", String(failed))
						.replace("{n}", String(targets.length)) + (lastErr ? ` — ${lastErr}` : "");
			}
			entityController?.reload();
		} finally {
			acting = false;
		}
	}

	// ── Approve Claim modal ───── page-local Modal with claim details + admin note; EntityPage's built-in confirm only covers default delete.
	let acting = $state(false);
	let actionTarget = $state<Organisation | null>(null);
	let showClaimModal = $state(false);
	let claimNote = $state("");

	const pendingClaim = $derived(actionTarget ? pendingClaimsByOrg[actionTarget.id] ?? null : null);

	function openClaim(org: Organisation) {
		actionTarget = org;
		claimNote = "";
		showClaimModal = true;
	}

	async function confirmClaim() {
		if (!actionTarget) return;
		acting = true;
		try {
			const result = await organisationsAdminApi.claim(actionTarget.id, { action: "approve", note: claimNote.trim() || undefined });
			if (result.ok) {
				showClaimModal = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t("common.error", "Failed to approve claim");
			}
		} catch {
			pageError = t("common.error", "Failed to approve claim");
		} finally {
			acting = false;
		}
	}

	// ── Verify / Unverify ───── direct toggle, no confirmation (mirrors legacy behaviour).
	async function toggleVerify(row: Organisation, ctx?: { reload: () => void }) {
		if (!row?.id) return;
		try {
			const result = row.verified
				? await organisationsAdminApi.unverify(row.id)
				: await organisationsAdminApi.verify(row.id, { verified: true });
			if (result.ok) {
				ctx?.reload();
			} else {
				pageError = result.error ?? t("common.error", "Failed to update verification");
			}
		} catch {
			pageError = t("common.error", "Failed to update verification");
		}
	}

	// ── Row actions ───── custom actions replace defaults, so View/Edit/Delete are included explicitly.
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t("common.view", "View"), icon: Eye, onclick: (row: Organisation, ctx?: any) => ctx?.openView(row) }
		];
		if (canManage) {
			acts.push({ label: t("common.edit", "Edit"), icon: Pencil, onclick: (row: Organisation, ctx?: any) => ctx?.openEdit(row), show: (row: Organisation) => !row.deleted_at });
			acts.push({
				label: t("organisations.verify", "Verify"),
				icon: ShieldCheck,
				onclick: toggleVerify,
				show: (row: Organisation) => !row.deleted_at && !row.verified
			});
			acts.push({
				label: t("organisations.unverify", "Unverify"),
				icon: ShieldOff,
				onclick: toggleVerify,
				show: (row: Organisation) => !row.deleted_at && row.verified
			});
			acts.push({
				label: t("organisations.approve_claim", "Approve Claim"),
				icon: ShieldCheck,
				onclick: (row: Organisation) => openClaim(row),
				show: (row: Organisation) => !row.deleted_at && !!pendingClaimsByOrg[row.id]
			});
			acts.push({
				label: t("common.delete", "Delete"),
				icon: Trash2,
				variant: "destructive",
				onclick: (row: Organisation, ctx?: any) => ctx?.openDelete(row),
				show: (row: Organisation) => !row.deleted_at
			});
			// Permanent removal — only for already-soft-deleted rows.
			acts.push({
				label: t("common.force_delete", "Force Delete"),
				icon: Trash2,
				variant: "destructive",
				onclick: (row: Organisation) => openForceDelete(row),
				show: (row: Organisation) => !!row.deleted_at
			});
		}
		return acts;
	});

	// BUG-022: Org type options fetched from the API (Rule 16).
	let orgTypeOptions = $state<{ value: string; label: string }[]>([]);
	$effect(() => {
		(async () => {
			const result = await organisationsAdminApi.meta();
			if (result.ok && result.data?.org_types) {
				orgTypeOptions = result.data.org_types;
			}
		})();
	});

	// ── Org type autocomplete helpers (filter pre-loaded options client-side) ──
	function fetchOrgTypeOptions(q: string): Promise<{ id: string; label: string }[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? orgTypeOptions
			: orgTypeOptions.filter((o) => o.label.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((o) => ({ id: o.value, label: o.label })));
	}

	function fetchOrgTypeOptionsInitial(): Promise<{ id: string; label: string }[]> {
		return Promise.resolve(orgTypeOptions.map((o) => ({ id: o.value, label: o.label })));
	}

	function getOrgTypeSelectedLabel(id: string): string {
		return orgTypeOptions.find((o) => o.value === id)?.label ?? id;
	}

	// ── Create/Edit form fields ───── standard flat form, EntityForm handles rendering + edit seeding.
	// BUG-022: org_type autocomplete is API-driven (Rule 16). Built as $derived so
	// the options populate after the meta endpoint resolves.
	const fields = $derived<FieldDef[]>([
		{ key: "name", label: t("organisations.name", "Name"), type: "text", required: true },
		{ key: "slug", label: t("common.slug", "Slug"), type: "text" },
		...(orgTypeOptions.length > 0
			? [{ key: "org_type", label: t("organisations.new.field_type", "Organization type"), type: "autocomplete" as const, placeholder: t("organisations.org_type_placeholder", "Search organization type…"), emptyMessage: t("common.no_results", "No results found"), fetcher: fetchOrgTypeOptions, initialFetcher: fetchOrgTypeOptionsInitial, getSelectedLabel: getOrgTypeSelectedLabel }]
			: [])
	]);

	const filters = $derived<FilterDef[]>([
		{
			key: "verified",
			label: t("organisations.verified", "Verified"),
			options: [
				{ value: "true", label: t("organisations.yes", "Yes") },
				{ value: "false", label: t("organisations.no", "No") }
			]
		},
		// BUG-022: Org type filter driven by the API meta endpoint (Rule 16).
		...(orgTypeOptions.length > 0
			? [{ key: "org_type", label: t("organisations.new.field_type", "Type"), options: orgTypeOptions }]
			: []),
		// Soft-deleted orgs are hidden by default — this filter surfaces them
		// so the Force Delete action has targets.
		{
			key: "deleted",
			label: t("common.filter_deleted", "Deleted"),
			options: [
				{ value: "only", label: t("common.filter_deleted_only", "Deleted only") },
				{ value: "all", label: t("common.filter_deleted_all", "Include deleted") }
			]
		}
	]);

	// Config built as $derived so pendingClaimsByOrg updates propagate to the actions show() predicates.
	const config = $derived<EntityPageConfig>({
		title: t("organisations.title", "Organisations"),
		description: t("organisations.description", "Manage organizations and verification"),
		createLabel: t("organisations.create", "Create Organisation"),
		createPermission: "admin.moderation",
		pageSize: 25,
		columns: [
			{ key: "name", label: t("organisations.name", "Name") },
			{ key: "slug", label: t("organisations.slug", "Slug"), render: (v: string) => v || "—" },
			{ key: "org_type", label: t("organisations.new.field_type", "Type"), render: (v: string) => v ? v.charAt(0).toUpperCase() + v.slice(1) : "—" },
			{ key: "owner_username", label: t("organisations.owner", "Owner"), render: (v: string) => v || "—" },
			{ key: "member_count", label: t("organisations.members", "Members"), render: (v: number) => (v ?? 0).toLocaleString() },
			{ key: "verified", label: t("organisations.verified", "Verified"), componentSnippet: VerifiedCell },
			{ key: "created_at", label: t("common.created", "Created"), render: (v: string) => new Date(v).toLocaleDateString() }
		],
		filters,
		actions: rowActions,
		bulkActions: [
			{
				label: t("common.delete_selected", "Delete Selected"),
				icon: Trash2,
				variant: "destructive",
				onclick: bulkDelete,
				// Rows already soft-deleted can't be deleted again.
				disabled: (_ids, rows) => (rows ?? []).some((r) => !!r.deleted_at)
			},
			{
				label: t("common.force_delete", "Force Delete Selected"),
				icon: Trash2,
				variant: "destructive",
				onclick: openBulkForceDelete,
				disabled: (ids) => acting || !orgRowsFor(ids).some((r) => !!r.deleted_at)
			}
		],
		fields,
		load: async ({ page, pageSize, q, filter }) => {
			const params = new URLSearchParams({ page_size: String(pageSize), page_offset: String(page * pageSize) });
			if (q) params.set("q", q);
			if (filter.verified) params.set("verified", filter.verified);
			if (filter.org_type) params.set("org_type", filter.org_type);
			if (filter.deleted) params.set("deleted", filter.deleted);
			const [orgResult, claimsResult] = await Promise.all([
				apiRaw<{ data?: Organisation[]; items?: Organisation[]; meta?: { total?: number; total_pages?: number }; total?: number; total_pages?: number }>(`/admin/organizations?${params.toString()}`),
				organisationsAdminApi.pendingClaims()
			]);
			if (!orgResult.ok) {
				throw new Error(orgResult.error ?? t("common.error", "Failed to load"));
			}
			const d = orgResult.data;
			const data = d?.data ?? d?.items ?? [];
			for (const r of data) orgRowMap.set(r.id, r);
			// BUG-019: Read pagination from meta (standard format), falling back
			// to top-level for backwards compatibility.
			const meta = d?.meta ?? {};
			const total = meta.total ?? d?.total ?? data.length;
			const totalPages = meta.total_pages ?? d?.total_pages ?? 1;
			// Side effect: update pending claims map for the "Approve Claim" action show() predicate.
			if (claimsResult.ok && Array.isArray(claimsResult.data)) {
				const map: Record<string, PendingClaim> = {};
				for (const c of claimsResult.data) {
					map[c.organization_id] = c;
				}
				pendingClaimsByOrg = map;
			}
			return { data, total, totalPages };
		},
		create: async (body) => {
			const payload: Record<string, unknown> = { name: String(body.name ?? "").trim(), slug: String(body.slug ?? "").trim() || undefined };
			if (body.org_type) payload.org_type = body.org_type;
			const res = await organisationsAdminApi.create(payload);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const payload: Record<string, unknown> = { name: String(body.name ?? "").trim(), slug: String(body.slug ?? "").trim() || undefined };
			if (body.org_type) payload.org_type = body.org_type;
			const res = await organisationsAdminApi.update(id, payload);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await organisationsAdminApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: OrgView
	});
</script>

<svelte:head>
	<title>{t("organisations.title", "Organisations")} — {siteName()} Admin</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- ── Approve Claim modal ──────────────────────────────────────────────── -->

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
					<a href={safeHref(pendingClaim.evidence_url)} target="_blank" rel="noopener noreferrer" class="text-sm break-all" style="color: var(--color-primary)">{pendingClaim.evidence_url}</a>
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
			<HintIcon text={t('organisations.claim_note_hint', 'Internal note on this claim decision.')} />
			<textarea id="claim-note" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" rows="2" placeholder={t("organisations.claim_note_placeholder", "Reason for approval...")} bind:value={claimNote}></textarea>
		</div>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showClaimModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={confirmClaim} disabled={acting}>{acting ? t("common.saving", "Saving...") : t("common.approve", "Approve")}</Button>
	</div>
</Modal>

<!-- ── Force Delete confirmation (single + bulk) — permanent erasure of soft-deleted rows ── -->
<Modal bind:open={showForceModal} title={t("common.force_delete", "Force Delete")} size="sm">
	{#if forceTargets.length > 0}
		<p class="text-sm" style="color: var(--color-foreground)">
			{#if forceTargets.length === 1}
				{t("common.force_delete_confirm", 'Permanently delete "{name}"? This cannot be undone.').replace("{name}", forceTargets[0].name)}
			{:else}
				{t("common.force_delete_confirm_bulk", "Permanently delete {count} selected items? This cannot be undone.").replace("{count}", String(forceTargets.length))}
			{/if}
		</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showForceModal = false)} disabled={acting}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmForceDelete} disabled={acting}>{acting ? t("common.deleting", "Deleting...") : t("common.force_delete", "Force Delete")}</Button>
	</div>
</Modal>

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

{#snippet VerifiedCell({ value, row }: { value: any; row: Organisation })}
	{#if row.deleted_at}
		<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium bg-danger/15 text-danger dark:text-danger">
			{t("common.filter_deleted", "Deleted")}
		</span>
	{:else}
		<span class={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${value ? 'bg-success/15 text-success dark:text-success' : 'bg-muted/15 text-muted dark:text-muted'}`}>
			{value ? t("organisations.yes", "Yes") : t("organisations.no", "No")}
		</span>
	{/if}
{/snippet}

<!-- View slide-over body (mirrors the legacy view modal) -->
{#snippet OrgView({ row }: { row: Organisation })}
	<div class="grid grid-cols-2 gap-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("organisations.name", "Name")}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("common.slug", "Slug")}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug ?? "—"}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("organisations.new.field_type", "Type")}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.org_type ? row.org_type.charAt(0).toUpperCase() + row.org_type.slice(1) : "—"}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("organisations.tagline", "Tagline")}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.tagline ?? "—"}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("organisations.owner", "Owner")}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.owner_username ?? row.owner_user_id ?? "—"}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("organisations.members", "Members")}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{(row.member_count ?? 0).toLocaleString()}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("organisations.followers", "Followers")}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{(row.follower_count ?? 0).toLocaleString()}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("organisations.verified", "Verified")}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.verified ? t("organisations.yes", "Yes") : t("organisations.no", "No")}</p>
		</div>
		{#if row.verification_type}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("organisations.verification_type", "Verification Type")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.verification_type}</p>
			</div>
		{/if}
		{#if row.website_url}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("organisations.website", "Website")}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)"><a href={row.website_url} target="_blank" rel="noopener" class="text-[var(--color-link)] hover:underline">{row.website_url}</a></p>
			</div>
		{/if}
		{#if row.country_code}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("organisations.country", "Country")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.country_code}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("common.created", "Created")}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : "—"}</p>
		</div>
	</div>
	{#if row.description}
		<div class="mt-4">
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("organisations.description", "Description")}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description}</p>
		</div>
	{/if}
{/snippet}
