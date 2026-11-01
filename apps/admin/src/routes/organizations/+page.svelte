<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /organisations — migrated to the EntityPage scaffold; CRUD entity with state actions (verify/unverify) and an approve-claim sub-flow.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from "$lib/components/entity";
	import { Modal } from "$lib/components/ui/modal";
	import { safeHref } from "$lib/utils/html";
	import { Button } from "$lib/components/ui/button";
	import { Tabs } from "$lib/components/ui/tabs";
	import { organisationsAdminApi, apiRaw, fieldGroupsAdminApiExtended, geoApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { Eye, Pencil, Trash2, ShieldCheck, ShieldOff, RefreshCw, Building2, ListChecks, Plus } from "lucide-svelte";
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

	// EntityPage auto-reloads + clears selection after bulk actions; the
	// built-in confirm-gated "Delete Selected" covers bulk soft-deletes.
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state("");

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

	// Verify modal — badge type (business|embassy|notable|immigration_lawyer|
	// professional_body) fetched from the public vocabulary endpoint (Rule
	// 16). The Verify row action opens this modal instead of firing a bare
	// verify call that always writes NULL verification_type.
	let showVerifyModal = $state(false);
	let verifyTarget = $state<Organisation | null>(null);
	let verifyType = $state("");
	let verificationTypeOptions = $state<{ value: string; label: string }[]>([]);
	$effect(() => {
		(async () => {
			const res = await organisationsAdminApi.verificationTypes("organization");
			if (res.ok && res.data?.types) {
				verificationTypeOptions = [
					{ value: "", label: t("organisations.verify_type_none", "No badge type") },
					...res.data.types.map((ty: { value: string; label: string }) => ({ value: ty.value, label: ty.label }))
				];
			}
		})();
	});

	function openVerify(row: Organisation) {
		verifyTarget = row;
		verifyType = row.verification_type ?? "";
		showVerifyModal = true;
	}

	async function confirmVerify() {
		if (!verifyTarget) return;
		acting = true;
		pageError = "";
		try {
			const res = await organisationsAdminApi.verify(verifyTarget.id, {
				...(verifyType ? { verification_type: verifyType } : {})
			});
			if (res.ok) {
				showVerifyModal = false;
				entityController?.reload();
			} else {
				pageError = res.error ?? t("common.error", "Failed to update verification");
			}
		} finally {
			acting = false;
		}
	}

	const pendingClaim = $derived(actionTarget ? pendingClaimsByOrg[actionTarget.id] ?? null : null);

	function openClaim(org: Organisation) {
		actionTarget = org;
		claimNote = "";
		showClaimModal = true;
	}

	async function confirmClaim() {
		if (!actionTarget || !pendingClaim) return;
		acting = true;
		try {
			const result = await organisationsAdminApi.reviewClaim(pendingClaim.id, { status: "approved", resolution_note: claimNote.trim() || undefined });
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

	// ── Unverify — direct action (Verify opens the badge-type modal).
	async function toggleVerify(row: Organisation, ctx?: { reload: () => void }) {
		if (!row?.id) return;
		if (!row.verified) { openVerify(row); return; }
		try {
			const result = await organisationsAdminApi.unverify(row.id);
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
				label: t("common.restore", "Restore"),
				icon: RefreshCw,
				onclick: async (row: Organisation, ctx?: any) => {
					const res = await organisationsAdminApi.restore(row.id);
					if (res.ok) ctx?.reload();
					else pageError = res.error ?? t("common.error", "Failed to restore");
				},
				show: (row: Organisation) => !!row.deleted_at
			});
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

	// Countries fetched once (Rule 16) — feeds the country_code autocomplete
	// form field, the Country column label, and the country filter options.
	let countryOptions = $state<{ value: string; label: string }[]>([]);
	$effect(() => {
		(async () => {
			const res = await geoApi.listCountries();
			if (res.ok && Array.isArray(res.data)) {
				countryOptions = res.data
					.filter((c) => c.geo_code)
					.map((c) => ({ value: c.geo_code, label: c.name }))
					.sort((a, b) => a.label.localeCompare(b.label));
			}
		})();
	});

	const countryLabel = (code: string) => countryOptions.find((c) => c.value === code)?.label;

	function fetchCountryOptions(q: string): Promise<{ id: string; label: string; sublabel?: string }[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? countryOptions
			: countryOptions.filter((o) => o.label.toLowerCase().includes(query) || o.value.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((o) => ({ id: o.value, label: o.label, sublabel: o.value })));
	}

	function fetchCountryOptionsInitial(): Promise<{ id: string; label: string; sublabel?: string }[]> {
		return Promise.resolve(countryOptions.map((o) => ({ id: o.value, label: o.label, sublabel: o.value })));
	}

	function getCountrySelectedLabel(id: string): string {
		return countryLabel(id) ?? id;
	}

	// ── Org Types management ("types" tab) ──────────────────
	// Org types are the options of the org_type custom field in the
	// organization_fields group — /organizations/meta + validateOrgType both
	// read them, so editing here updates the whole platform (Rule 16).
	const canEditOrgTypes = $derived(hasPermission("custom_fields.field.edit"));
	let activeTab = $state("organizations");
	const pageTabs = $derived([
		{ value: "organizations", label: t("organisations.title", "Organisations"), icon: Building2 },
		{ value: "types", label: t("organisations.types_tab", "Org Types"), icon: ListChecks, count: orgTypeOptions.length }
	]);

	type OrgTypeField = {
		id: string; label: string; key: string; field_type: string;
		options: { value: string; label: string }[] | null;
		embed_platforms?: string[] | null; placeholder?: string | null;
		default_value?: string | null; is_required: boolean;
		show_in_frontend: boolean; filterable: boolean; sortable: boolean;
		searchable: boolean; position: number;
	};

	let orgTypeField = $state<{ groupId: string; field: OrgTypeField } | null>(null);
	let orgTypesLoading = $state(false);
	let orgTypesError = $state("");
	// True when the field lookup is unavailable (no custom_fields.group.view)
	// — the tab falls back to the read-only /organizations/meta list.
	let orgTypesReadonly = $state(false);
	let typeSaving = $state(false);
	let newTypeLabel = $state("");
	let newTypeValue = $state("");
	let editType = $state<{ value: string; label: string } | null>(null);
	let editTypeLabel = $state("");
	let deleteTypeTarget = $state<{ value: string; label: string } | null>(null);

	// Field options are authoritative once loaded; meta is the fallback.
	const orgTypesList = $derived(orgTypeField?.field.options ?? orgTypeOptions);

	async function loadOrgTypeField() {
		orgTypesLoading = true;
		orgTypesError = "";
		try {
			const res = await fieldGroupsAdminApiExtended.list("q=organization_fields&page_size=100");
			const groups = (res.ok ? (res.data?.data ?? []) : []) as { id: string; slug: string; fields?: OrgTypeField[] }[];
			const group = groups.find((g) => g.slug === "organization_fields");
			const field = group?.fields?.find((f) => f.key === "org_type");
			if (group && field) {
				orgTypeField = { groupId: group.id, field };
			} else {
				orgTypesReadonly = true;
			}
		} finally {
			orgTypesLoading = false;
		}
	}

	// Load the field lazily the first time the types tab opens.
	$effect(() => {
		if (activeTab === "types" && !orgTypeField && !orgTypesReadonly && !orgTypesLoading) {
			void loadOrgTypeField();
		}
	});

	function slugifyType(s: string): string {
		return s.toLowerCase().replace(/[^a-z0-9]+/g, "_").replace(/^_+|_+$/g, "");
	}

	// PATCHes the whole options array — the field request requires the full
	// field shape, so every unchanged property is echoed back as-is.
	async function saveTypeOptions(options: { value: string; label: string }[]): Promise<boolean> {
		if (!orgTypeField) return false;
		typeSaving = true;
		orgTypesError = "";
		const f = orgTypeField.field;
		const res = await fieldGroupsAdminApiExtended.updateField(orgTypeField.groupId, f.id, {
			label: f.label,
			field_type: f.field_type,
			options,
			embed_platforms: f.embed_platforms ?? [],
			placeholder: f.placeholder ?? null,
			default_value: f.default_value ?? null,
			is_required: f.is_required,
			show_in_frontend: f.show_in_frontend,
			filterable: f.filterable,
			sortable: f.sortable,
			searchable: f.searchable,
			position: f.position
		});
		typeSaving = false;
		if (!res.ok) {
			orgTypesError = res.error ?? t("common.error", "Failed to save org types");
			return false;
		}
		orgTypeField = { groupId: orgTypeField.groupId, field: { ...f, options } };
		// Keep the form field + filter options in sync without a meta re-fetch.
		orgTypeOptions = options.map((o) => ({ value: o.value, label: o.label }));
		return true;
	}

	async function addType() {
		const label = newTypeLabel.trim();
		const value = slugifyType(newTypeValue.trim() || label);
		if (!label || !value || orgTypesList.some((o) => o.value === value)) return;
		if (await saveTypeOptions([...orgTypesList, { value, label }])) {
			newTypeLabel = "";
			newTypeValue = "";
		}
	}

	async function confirmEditType() {
		if (!editType) return;
		const label = editTypeLabel.trim();
		if (!label) return;
		// Value is immutable on edit — orgs store the value, so renaming it
		// would strand existing rows.
		const next = orgTypesList.map((o) => (o.value === editType!.value ? { value: o.value, label } : o));
		if (await saveTypeOptions(next)) editType = null;
	}

	async function confirmDeleteType() {
		if (!deleteTypeTarget || orgTypesList.length <= 1) return;
		const next = orgTypesList.filter((o) => o.value !== deleteTypeTarget!.value);
		if (await saveTypeOptions(next)) deleteTypeTarget = null;
	}

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
		// Slug is immutable — the update query has no slug column, so the
		// field is locked on edit and only sent on create.
		{ key: "slug", label: t("common.slug", "Slug"), type: "text", disabledWhen: (editing: any) => !!editing, lockedHelp: t("organisations.slug_locked", "Slug cannot be changed after creation") },
		{ key: "tagline", label: t("organisations.tagline", "Tagline"), type: "text" },
		{ key: "description", label: t("organisations.field_description", "Description"), type: "textarea" },
		{ key: "website_url", label: t("organisations.website", "Website"), type: "url" },
		{ key: "country_code", label: t("organisations.country", "Country"), type: "autocomplete", placeholder: t("organisations.country_placeholder", "Search country…"), emptyMessage: t("common.no_results", "No results found"), fetcher: fetchCountryOptions, initialFetcher: fetchCountryOptionsInitial, getSelectedLabel: getCountrySelectedLabel },
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
		// Country filter — options come from the geo spaces country list
		// (Rule 16); the admin list endpoint accepts ?country_code= (ISO-2).
		...(countryOptions.length > 0
			? [{ key: "country_code", label: t("organisations.country", "Country"), options: countryOptions }]
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
			{ key: "name", label: t("organisations.name", "Name"), sortable: true },
			{ key: "slug", label: t("organisations.slug", "Slug"), sortable: true, render: (v: string) => v || "—" },
			{ key: "org_type", label: t("organisations.new.field_type", "Type"), sortable: true, render: (v: string) => v ? (orgTypeOptions.find((o) => o.value === v)?.label ?? v.charAt(0).toUpperCase() + v.slice(1)) : "—" },
			{ key: "country_code", label: t("organisations.country", "Country"), render: (v: string) => v ? (countryLabel(v) ?? v) : "—" },
			{ key: "owner_username", label: t("organisations.owner", "Owner"), sortable: true, sortKey: "owner", render: (v: string) => v || "—" },
			{ key: "member_count", label: t("organisations.members", "Members"), sortable: true, render: (v: number) => (v ?? 0).toLocaleString() },
			{ key: "verified", label: t("organisations.verified", "Verified"), sortable: true, componentSnippet: VerifiedCell },
			{ key: "created_at", label: t("common.created", "Created"), sortable: true, render: (v: string) => new Date(v).toLocaleDateString() }
		],
		filters,
		actions: rowActions,
		bulkActions: [
			{
				label: t("common.force_delete_selected", "Force Delete Selected"),
				icon: Trash2,
				variant: "destructive",
				onclick: openBulkForceDelete,
				disabled: (ids) => acting || !orgRowsFor(ids).some((r) => !!r.deleted_at)
			}
		],
		fields,
		load: async ({ page, pageSize, q, sort, filter }) => {
			const params = new URLSearchParams({ page_size: String(pageSize), page_offset: String(page * pageSize) });
			// EntityPage passes sort as "key:dir"; the API takes sort + sort_dir
			// (whitelisted server-side, same contract as /admin/users).
			if (sort) {
				const [key, dir] = sort.split(":");
				params.set("sort", key);
				params.set("sort_dir", dir === "desc" ? "desc" : "asc");
			}
			if (q) params.set("q", q);
			if (filter.verified) params.set("verified", filter.verified);
			if (filter.org_type) params.set("org_type", filter.org_type);
			if (filter.country_code) params.set("country_code", filter.country_code);
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
			if (body.tagline) payload.tagline = body.tagline;
			if (body.description) payload.description = body.description;
			if (body.website_url) payload.website_url = body.website_url;
			if (body.country_code) payload.country_code = String(body.country_code).trim().toUpperCase();
			const res = await organisationsAdminApi.create(payload);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			// slug intentionally omitted — immutable server-side.
			const payload: Record<string, unknown> = { name: String(body.name ?? "").trim() };
			if (body.org_type) payload.org_type = body.org_type;
			if (body.tagline !== undefined) payload.tagline = body.tagline;
			if (body.description !== undefined) payload.description = body.description;
			if (body.website_url !== undefined) payload.website_url = body.website_url;
			if (body.country_code !== undefined) payload.country_code = String(body.country_code).trim().toUpperCase();
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

<Tabs tabs={pageTabs} bind:value={activeTab} tabParam="tab" tabPreserve={[]} />

{#if activeTab === "organizations"}
	<EntityPage {config} bind:controller={entityController} />
{:else}
	<!-- ── Org Types management ───────────────────────────────────────── -->
	<section class="rounded-lg border p-4" style="border-color: var(--color-border); background: var(--color-surface)">
		<div class="mb-4 flex items-center justify-between gap-2 flex-wrap">
			<div>
				<h2 class="text-base font-semibold" style="color: var(--color-foreground)">{t("organisations.types_title", "Organization Types")}</h2>
				<p class="text-xs mt-0.5" style="color: var(--color-muted)">{t("organisations.types_desc", "Types selectable on organization profiles — stored as the org_type field options.")}</p>
			</div>
		</div>

		{#if orgTypesError}
			<div class="mb-3 rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-danger); color: var(--color-danger)">{orgTypesError}</div>
		{/if}

		{#if orgTypesLoading}
			<p class="text-sm py-4" style="color: var(--color-muted)">{t("common.loading", "Loading...")}</p>
		{:else}
			{#if orgTypesReadonly}
				<p class="mb-3 text-xs" style="color: var(--color-muted)">{t("organisations.types_readonly", "Read-only — editing requires the custom_fields.field.edit permission.")}</p>
			{/if}
			<ul class="divide-y" style="border-color: var(--color-border)">
				{#each orgTypesList as opt (opt.value)}
					<li class="flex items-center gap-3 py-2">
						<span class="flex-1 text-sm font-medium" style="color: var(--color-foreground)">{opt.label}</span>
						<span class="font-mono text-xs" style="color: var(--color-muted)">{opt.value}</span>
						{#if canEditOrgTypes && orgTypeField}
							<Button variant="ghost" size="sm" onclick={() => { editType = opt; editTypeLabel = opt.label; }} aria-label={t("common.edit", "Edit")}>
								<Pencil class="h-3.5 w-3.5" />
							</Button>
							<Button variant="ghost" size="sm" onclick={() => (deleteTypeTarget = opt)} disabled={orgTypesList.length <= 1} style="color: var(--color-danger)" aria-label={t("common.delete", "Delete")}>
								<Trash2 class="h-3.5 w-3.5" />
							</Button>
						{/if}
					</li>
				{/each}
			</ul>

			{#if canEditOrgTypes && orgTypeField}
				<form class="mt-4 flex items-end gap-2 flex-wrap border-t pt-4" style="border-color: var(--color-border)" onsubmit={(e) => { e.preventDefault(); addType(); }}>
					<div class="flex-1 min-w-40">
						<label for="new-type-label" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t("organisations.type_label", "Label")}</label>
						<input id="new-type-label" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)" bind:value={newTypeLabel} placeholder={t("organisations.type_label_placeholder", "e.g. Non-profit")} />
					</div>
					<div class="w-40">
						<label for="new-type-value" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t("organisations.type_value", "Value")}</label>
						<input id="new-type-value" class="w-full rounded-md border px-3 py-2 font-mono text-sm" style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)" bind:value={newTypeValue} placeholder={slugifyType(newTypeLabel) || "non_profit"} />
					</div>
					<Button type="submit" disabled={typeSaving || !newTypeLabel.trim()}>
						<Plus class="h-3.5 w-3.5" />
						{typeSaving ? t("common.saving", "Saving...") : t("organisations.type_add", "Add Type")}
					</Button>
				</form>
			{:else if !orgTypesReadonly}
				<p class="mt-3 text-xs" style="color: var(--color-muted)">{t("organisations.types_readonly", "Read-only — editing requires the custom_fields.field.edit permission.")}</p>
			{/if}
		{/if}
	</section>
{/if}

<!-- ── Edit Org Type modal (label only — values are stored on orgs) ──────── -->
<Modal open={!!editType} onclose={() => (editType = null)} title={t("organisations.type_edit", "Edit Type")} size="sm">
	{#if editType}
		<label for="edit-type-label" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("organisations.type_label", "Label")}</label>
		<input id="edit-type-label" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={editTypeLabel} />
		<p class="mt-2 font-mono text-xs" style="color: var(--color-muted)">{editType.value}</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (editType = null)} disabled={typeSaving}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={confirmEditType} disabled={typeSaving || !editTypeLabel.trim()}>{typeSaving ? t("common.saving", "Saving...") : t("common.save", "Save")}</Button>
	</div>
</Modal>

<!-- ── Delete Org Type confirm ─────────────────────────────────────────── -->
<Modal open={!!deleteTypeTarget} onclose={() => (deleteTypeTarget = null)} title={t("common.confirmDelete", "Confirm Delete")} size="sm">
	{#if deleteTypeTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("organisations.type_delete_confirm", 'Remove the type "{name}"? Organizations already using it keep their stored value, but it can no longer be selected.').replace("{name}", deleteTypeTarget.label)}
		</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (deleteTypeTarget = null)} disabled={typeSaving}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmDeleteType} disabled={typeSaving}>{typeSaving ? t("common.deleting", "Deleting...") : t("common.delete", "Delete")}</Button>
	</div>
</Modal>

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
			<textarea id="claim-note" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" rows="2" placeholder={t("organisations.claim_note_placeholder", "Reason for approval...")} bind:value={claimNote}></textarea>
		</div>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showClaimModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={confirmClaim} disabled={acting}>{acting ? t("common.saving", "Saving...") : t("common.approve", "Approve")}</Button>
	</div>
</Modal>

<!-- ── Force Delete confirmation (single + bulk) — permanent erasure of soft-deleted rows ── -->
<Modal bind:open={showVerifyModal} title={t("organisations.verify", "Verify Organization")} size="sm">
	{#if verifyTarget}
		<p class="text-sm mb-3" style="color: var(--color-foreground)">
			{t("organisations.verify_confirm", 'Grant a verification badge to "{name}"?').replace("{name}", verifyTarget.name)}
		</p>
		<label for="verify-type" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("organisations.verification_type", "Verification type")}</label>
		<select id="verify-type" bind:value={verifyType} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)">
			{#each verificationTypeOptions as opt (opt.value)}
				<option value={opt.value}>{opt.label}</option>
			{/each}
		</select>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showVerifyModal = false)} disabled={acting}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={confirmVerify} disabled={acting}>{acting ? t("common.saving", "Saving...") : t("organisations.verify", "Verify")}</Button>
	</div>
</Modal>

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

<!-- View modal body (mirrors the legacy view modal) -->
{#snippet OrgView({ row }: { row: Organisation })}
	<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
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
				<p class="mt-1 text-sm" style="color: var(--color-foreground)"><a href={safeHref(row.website_url)} target="_blank" rel="noopener" class="text-[var(--color-link)] hover:underline">{row.website_url}</a></p>
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
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("organisations.field_description", "Description")}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description}</p>
		</div>
	{/if}
{/snippet}
