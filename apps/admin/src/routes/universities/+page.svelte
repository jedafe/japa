<script lang="ts">
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from "$lib/components/entity";
	import { Modal } from "$lib/components/ui/modal";
	import { Button } from "$lib/components/ui/button";
	import { SlideOver } from "$lib/components/ui/slide-over";
	import { universitiesAdminApi, geoApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import {
		Eye, Pencil, Trash2, BadgeCheck, XCircle, Upload, FileEdit
	} from "lucide-svelte";
	import type { ActionItem, FilterDef } from "$lib/components/ui/data-table";

	type University = {
		id: string;
		name: string;
		slug: string;
		initials?: string;
		geo_code?: string;
		member_count: number;
		status: string;
		verified: boolean;
		created_at: string;
	};

	type UniversityRaw = Omit<University, "verified">;

	function deriveVerified(u: UniversityRaw): University {
		return { ...u, verified: u.status === "active" };
	}

	const canManage = $derived(hasPermission("admin.settings"));

	// ── Country filter options (loaded from /geo/countries — Rule 16) ───────
	// The backend ListUniversitiesAdmin accepts ?country= (ISO 3166-1 alpha-2).
	// The country list is fetched from the geo API so it stays in sync with the
	// seeded Primary Spaces and is never hardcoded.
	let countryOptions = $state<{ value: string; label: string }[]>([]);
	$effect(() => {
		geoApi.listCountries().then((res: any) => {
			const rows: Array<{ geo_code?: string; name?: string; slug?: string }> =
				res?.ok ? (res.data ?? []) : [];
			countryOptions = rows
				.filter((c) => c.geo_code)
				.map((c) => ({ value: c.geo_code!, label: c.name ?? c.geo_code! }));
		}).catch(() => { /* empty filter on failure — list still works */ });
	});

	// ── Filter options ──────────────────────────────────────────────────────
	// Section 39 spec filters: country (geo_code). The "verified" filter was
	// removed (audit U-4) - never wired to backend, not in spec.
	const filterDefs = $derived<FilterDef[]>([
		{
			key: "country",
			label: t("universities.country", "Country"),
			options: countryOptions
		}
	]);

	// ── Page-local state for sub-flows ──────────────────────────────────────
	// EntityPage owns list/pagination/search state; the page only owns
	// state-action modals + the create/edit form fields.
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state("");
	let acting = $state(false);

	let showEditSlideOver = $state(false);
	let editTarget = $state<University | null>(null);
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<University | null>(null);
	let showStatusModal = $state(false);
	let statusTarget = $state<University | null>(null);
	let formStatus = $state("pending");
	let showBulkImportModal = $state(false);
	let csvFile = $state<File | null>(null);

	let formName = $state("");
	let formSlug = $state("");
	let formInitials = $state("");
	let formCountry = $state("");
	let formLogoUrl = $state("");
	let formVerified = $state(false);

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t("common.view", "View"), icon: Eye, onclick: (row: University, ctx) => ctx?.openView(row) }
		];
		if (canManage) {
			acts.push({
				label: t("universities.verify", "Verify"),
				icon: BadgeCheck,
				onclick: (row: University) => toggleVerify(row),
				show: (row: University) => !row.verified
			});
			acts.push({
				label: t("universities.unverify", "Unverify"),
				icon: XCircle,
				variant: "destructive",
				onclick: (row: University) => toggleVerify(row),
				show: (row: University) => row.verified
			});
			acts.push({
				label: t("universities.changeStatus", "Change Status"),
				icon: FileEdit,
				onclick: (row: University) => openStatusModal(row)
			});
			acts.push({
				label: t("common.edit", "Edit"),
				icon: Pencil,
				onclick: (row: University) => openEdit(row)
			});
			acts.push({
				label: t("common.delete", "Delete"),
				icon: Trash2,
				variant: "destructive",
				onclick: (row: University) => openDelete(row)
			});
		}
		return acts;
	});

	// ── Country autocomplete helpers (filter pre-loaded options client-side) ──
	function fetchCountryOptions(q: string): Promise<{ id: string; label: string }[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? countryOptions
			: countryOptions.filter((o) => o.label.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((o) => ({ id: o.value, label: o.label })));
	}

	function fetchCountryOptionsInitial(): Promise<{ id: string; label: string }[]> {
		return Promise.resolve(countryOptions.map((o) => ({ id: o.value, label: o.label })));
	}

	function getCountrySelectedLabel(id: string): string {
		return countryOptions.find((o) => o.value === id)?.label ?? id;
	}

	// ── Create/Edit form fields ─────────────────────────────────────────────
	// logo_url is accepted by the backend (POST/PATCH /admin/universities) and
	// enqueues university:fetch_logo to download + store it (Section 39).
	// country is stored as geo_code (ISO 3166-1 alpha-2) (audit U-1/U-2).
	const fields = $derived<FieldDef[]>([
		{ key: "name", label: t("universities.name", "Name"), type: "text", required: true },
		{ key: "slug", label: t("universities.domain_slug", "Domain"), type: "text", placeholder: "mit.edu" },
		{ key: "initials", label: t("universities.initials", "Initials"), type: "text", placeholder: "UNILAG" },
		{ key: "country", label: t("universities.country", "Country"), type: "autocomplete", placeholder: t("universities.country_placeholder", "Select country"), emptyMessage: t("common.no_results", "No results found"), fetcher: fetchCountryOptions, initialFetcher: fetchCountryOptionsInitial, getSelectedLabel: getCountrySelectedLabel },
		{ key: "logo_url", label: t("universities.logo_url", "Logo URL"), type: "url", placeholder: "https://…" },
		{ key: "verified", label: t("universities.verified", "Verified"), type: "checkbox" }
	]);

	// ── Config ──────────────────────────────────────────────────────────────
	// Built as $derived so canManage/rowActions updates propagate.
	const config = $derived<EntityPageConfig>({
		title: t("universities.title", "Universities"),
		description: t("universities.description", "Manage university registrations and verification"),
		createLabel: t("universities.create", "Create University"),
		createPermission: "admin.settings",
		pageSize: 25,
		columns: [
			{ key: "name", label: t("universities.name", "Name"), sortable: true },
			{ key: "slug", label: t("universities.slug", "Slug"), sortable: true, render: (v: string) => `<span class="font-mono text-xs">${v}</span>` },
			{ key: "initials", label: t("universities.initials", "Initials"), sortable: true, render: (v: string) => v ? `<span class="font-mono text-xs font-semibold">${v}</span>` : '' },
			{
				key: "status",
				label: t("universities.status", "Status"),
				sortable: true,
				render: (v: string) => {
					const cls = v === "active" ? "bg-emerald-500/15 text-emerald-700 dark:text-emerald-400" : "bg-amber-500/15 text-amber-700 dark:text-amber-400";
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${cls}">${v}</span>`;
				}
			},
			{ key: "member_count", label: t("universities.students", "Members"), sortable: true, render: (v: number) => (v ?? 0).toLocaleString() },
			{ key: "created_at", label: t("universities.created", "Created"), sortable: true, render: (v: string) => new Date(v).toLocaleDateString() }
		],
		filters: filterDefs,
		actions: rowActions,
		bulkActions: canManage ? [
			{
				label: t("universities.deleteSelected", "Delete Selected"),
				icon: Trash2,
				variant: "destructive",
				onclick: bulkDelete
			}
		] : [],
		fields,
		headerActions: canManage ? [
			{ label: t("universities.bulkImport", "CSV Import"), icon: Upload, onclick: openBulkImport, variant: "outline" }
		] : [],
		load: async ({ page, pageSize, q, filter }) => {
			const params = new URLSearchParams({
				page: String(page + 1),
				limit: String(pageSize)
			});
			if (q) params.set("q", q);
			// Section 39: admin list filterable by country (geo_code).
			if (filter?.country) params.set("country", filter.country);
			const result = await universitiesAdminApi.list(params.toString());
			if (result.ok) {
				const body = result.data as any;
				const raw: UniversityRaw[] = body?.data ?? [];
				const meta = body?.meta ?? { total: raw.length, total_pages: 1 };
				return {
					data: raw.map(deriveVerified),
					total: meta.total ?? raw.length,
					totalPages: meta.total_pages ?? 1
				};
			}
			throw new Error(result.error ?? t("common.error", "Failed to load"));
		},
		create: async (body) => {
			const res = await universitiesAdminApi.create({
				name: String(body.name ?? "").trim(),
				slug: String(body.slug ?? "").trim(),
				initials: String(body.initials ?? "").trim() || undefined as any,
				country: String(body.country ?? "").trim() || undefined as any,
				logo_url: String(body.logo_url ?? "").trim() || undefined as any,
				verified: !!body.verified
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await universitiesAdminApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: UniversityView
	});

	// ── Sub-flow handlers ───────────────────────────────────────────────────
	function openEdit(uni: University) {
		editTarget = uni;
		formName = uni.name; formSlug = uni.slug; formInitials = uni.initials ?? ""; formCountry = uni.geo_code ?? ""; formLogoUrl = ""; formVerified = uni.status === "active";
		showEditSlideOver = true;
	}

	function openDelete(uni: University) { deleteTarget = uni; showDeleteConfirm = true; }

	function openStatusModal(uni: University) {
		statusTarget = uni;
		formStatus = uni.status;
		showStatusModal = true;
	}

	function openBulkImport() {
		csvFile = null;
		showBulkImportModal = true;
	}

	async function submitEdit() {
		if (!editTarget) return;
		acting = true;
		try {
			const result = await universitiesAdminApi.update(editTarget.id, {
				name: formName.trim(),
				slug: formSlug.trim(),
				initials: formInitials.trim() || undefined,
				country: formCountry.trim() || undefined,
				logo_url: formLogoUrl.trim() || undefined,
				verified: formVerified
			});
			if (result.ok) { showEditSlideOver = false; entityController?.reload(); }
			else { pageError = result.error; }
		} catch { pageError = t("common.error", "Failed to update"); } finally { acting = false; }
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		acting = true;
		try {
			const result = await universitiesAdminApi.delete(deleteTarget.id);
			if (result.ok) { showDeleteConfirm = false; deleteTarget = null; entityController?.reload(); }
			else { pageError = result.error; }
		} catch { pageError = t("common.error", "Failed to delete"); } finally { acting = false; }
	}

	let bulkDeleting = $state(false);
	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0 || bulkDeleting) return;
		bulkDeleting = true;
		pageError = "";
		try {
			await Promise.all(idArr.map((id) => universitiesAdminApi.delete(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t("common.error", "Failed to delete");
		} finally {
			bulkDeleting = false;
		}
	}

	async function toggleVerify(uni: University) {
		acting = true;
		try {
			const result = uni.verified
				? await universitiesAdminApi.unverify(uni.id)
				: await universitiesAdminApi.verify(uni.id);
			if (result.ok) { entityController?.reload(); } else { pageError = result.error; }
		} catch { pageError = t("common.error", "Failed to update verification"); } finally { acting = false; }
	}

	async function submitStatusChange() {
		if (!statusTarget) return;
		acting = true;
		try {
			const result = await universitiesAdminApi.updateStatus(statusTarget.id, { status: formStatus });
			if (result.ok) { showStatusModal = false; statusTarget = null; entityController?.reload(); }
			else { pageError = result.error; }
		} catch { pageError = t("common.error", "Failed to update status"); } finally { acting = false; }
	}

	function handleFileSelect(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		csvFile = input.files?.[0] ?? null;
	}

	async function submitBulkImport() {
		if (!csvFile) return;
		acting = true;
		try {
			const formData = new FormData();
			formData.append('file', csvFile);
			const result = await universitiesAdminApi.bulkImport(formData);
			if (result.ok) { showBulkImportModal = false; csvFile = null; entityController?.reload(); }
			else { pageError = result.error; }
		} catch { pageError = t("common.error", "Failed to import"); } finally { acting = false; }
	}

	// ── View snippet is declared in the template as {#snippet UniversityView(...)}
</script>

<svelte:head>
	<title>{t("universities.title", "Universities")} — JapaConnect Admin</title>
</svelte:head>

{#if pageError}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- ── Edit slide-over (page-local because update isn't in EntityPage config) ── -->
<SlideOver bind:open={showEditSlideOver} title={t("universities.edit", "Edit University")}>
	{#if editTarget}
		<div class="space-y-3">
			<div><label for="edit-uni-name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("universities.name", "Name")}</label><input id="edit-uni-name" type="text" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={formName} /></div>
			<div><label for="edit-uni-domain" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("universities.domain_slug", "Domain")}</label><input id="edit-uni-domain" type="text" class="w-full rounded-lg border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={formSlug} /></div>
			<div><label for="edit-uni-initials" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("universities.initials", "Initials")}</label><input id="edit-uni-initials" type="text" placeholder="UNILAG" class="w-full rounded-lg border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={formInitials} /></div>
			<div>
				<label for="edit-uni-country" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("universities.country", "Country")}</label>
				<select id="edit-uni-country" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={formCountry}>
					<option value="">{t("universities.country_placeholder", "Select country")}</option>
					{#each countryOptions as opt (opt.value)}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>
			<div><label for="edit-uni-logo" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("universities.logo_url", "Logo URL")}</label><input id="edit-uni-logo" type="url" placeholder="https://…" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={formLogoUrl} /></div>
			<div class="flex items-center gap-2"><input id="edit-uni-verified" type="checkbox" bind:checked={formVerified} /><label for="edit-uni-verified" class="text-sm">{t("universities.verified", "Verified")}</label></div>
		</div>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={() => (showEditSlideOver = false)}>{t("common.cancel", "Cancel")}</Button>
			<Button onclick={submitEdit} disabled={acting}>{acting ? t("common.saving", "Saving...") : t("common.save", "Save")}</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- ── Delete confirmation ── -->
<Modal bind:open={showDeleteConfirm} title={t("common.confirmDelete", "Confirm Delete")} size="sm">
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">{t("universities.deleteConfirm", "Delete university")} <strong>{deleteTarget.name}</strong>? {t("universities.deleteWarning", "This cannot be undone.")}</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteConfirm = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={executeDelete} disabled={acting}>{acting ? t("common.deleting", "Deleting...") : t("common.delete", "Delete")}</Button>
	</div>
</Modal>

<!-- ── Status change modal ── -->
<Modal bind:open={showStatusModal} title={t("universities.changeStatus", "Change Status")} size="sm">
	{#if statusTarget}
		<p class="mb-3 text-sm" style="color: var(--color-foreground)">{t("universities.statusFor", "Status for")} <strong>{statusTarget.name}</strong></p>
		<div class="space-y-2">
			{#each ["active", "pending", "suspended"] as s (s)}
				<label class="flex items-center gap-2 text-sm" style="color: var(--color-foreground)">
					<input type="radio" name="status" value={s} bind:group={formStatus} />
					{s}
				</label>
			{/each}
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showStatusModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={submitStatusChange} disabled={acting}>{acting ? t("common.saving", "Saving...") : t("common.save", "Save")}</Button>
	</div>
</Modal>

<!-- ── CSV bulk import modal ── -->
<Modal bind:open={showBulkImportModal} title={t("universities.bulkImport", "CSV Bulk Import")} size="default">
	<div class="space-y-3">
		<p class="text-sm" style="color: var(--color-muted)">{t("universities.bulkImportHelp", "Upload a CSV file with columns: name, slug, initials, country, logo_url. The initials and country columns are optional. Each row will create a university. Duplicate slugs are skipped.")}</p>
		<div>
			<label for="csv-file" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t("universities.csvFile", "CSV File")}</label>
			<input id="csv-file" type="file" accept=".csv" onchange={handleFileSelect} class="w-full text-sm" />
		</div>
	</div>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showBulkImportModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={submitBulkImport} disabled={acting || !csvFile}>{acting ? t("common.importing", "Importing...") : t("common.import", "Import")}</Button>
	</div>
</Modal>

<!-- ── View snippet ──────────────────────────────────────────────────────── -->
{#snippet UniversityView({ row }: { row: University })}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("universities.name", "Name")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("universities.slug", "Slug")}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("universities.initials", "Initials")}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.initials ?? '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("universities.status", "Status")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("universities.students", "Members")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{(row.member_count ?? 0).toLocaleString()}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("universities.created", "Created")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(row.created_at).toLocaleString()}</p>
			</div>
		</div>
	</div>
{/snippet}
