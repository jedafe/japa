<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { Plus, Pencil, Trash2, RefreshCw } from "lucide-svelte";
	import { redirectsAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	// Ref: japaconnect-engineering/adr/ADR-056-url-redirects.md
	// Field names mirror the backend gen.Redirect JSON contract:
	// source_path, target_url, http_status, is_active, notes, hit_count, last_hit_at.
	type Redirect = {
		id: string;
		tenant_id: string;
		source_path: string;
		target_url: string;
		http_status: number;
		is_active: boolean;
		notes: string | null;
		hit_count: number;
		last_hit_at: string | null;
		created_at: string;
		updated_at: string;
	};

	let redirects = $state<Redirect[]>([]);
	let loading = $state(true);
	let error = $state("");
	let success = $state("");
	let searchQuery = $state("");

	// Modal state
	let showModal = $state(false);
	let modalMode = $state<"create" | "edit">("create");
	let editingRedirect = $state<Redirect | null>(null);
	let saving = $state(false);

	// Form — names match the backend create/update payload (ADR-056).
	let sourcePath = $state("");
	let targetUrl = $state("");
	let httpStatus = $state(301);
	let isActive = $state(true);
	let notes = $state("");

	// Delete
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<Redirect | null>(null);
	let bulkDeleteIds = $state<Set<string>>(new Set());
	let filterValues = $state<Record<string, string>>({});

	const canManageRedirects = $derived(hasPermission("redirects.manage"));

	// Backend list route expects ?status=active|inactive (ADR-056), not is_active.
	const filterDefs = [
		{
			key: "status",
			label: t("common.status", "Status"),
			options: [
				{ value: "active", label: t("common.active", "Active") },
				{ value: "inactive", label: t("common.inactive", "Inactive") }
			]
		}
	];

	const columns = [
		{ key: "source_path", label: t("redirects.from", "From"), sortable: true },
		{ key: "target_url", label: t("redirects.to", "To"), sortable: true },
		{ key: "http_status", label: t("redirects.code", "Code"), sortable: true },
		{
			key: "is_active",
			label: t("common.status", "Status"),
			sortable: true,
			render: (v: boolean) => (v ? t("common.active", "Active") : t("common.inactive", "Inactive"))
		},
		{ key: "hit_count", label: t("redirects.hits", "Hits"), sortable: true },
		{
			key: "actions",
			label: t("common.actions", "Actions"),
			class: "w-32",
			componentSnippet: RedirectActionsCell
		}
	];

	async function loadRedirects() {
		loading = true;
		error = "";
		try {
			const params = new URLSearchParams();
			if (filterValues.status) params.set("status", filterValues.status);
			if (searchQuery.trim()) params.set("q", searchQuery.trim());
			const result = await redirectsAdminApi.list(params.toString() || undefined);
			if (result.ok) {
				redirects = ((result.data as { items: Redirect[] })?.items) ?? [];
			} else {
				error = result.error ?? t("common.error", "Failed to load redirects");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load redirects");
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		modalMode = "create";
		editingRedirect = null;
		sourcePath = "";
		targetUrl = "";
		httpStatus = 301;
		isActive = true;
		notes = "";
		showModal = true;
	}

	function openEdit(redirect: Redirect | null) {
		if (!redirect) return;
		modalMode = "edit";
		editingRedirect = redirect;
		sourcePath = redirect.source_path;
		targetUrl = redirect.target_url;
		httpStatus = redirect.http_status;
		isActive = redirect.is_active;
		notes = redirect.notes ?? "";
		showModal = true;
	}

	// ADR-056: source_path must start with "/"; target_url must be a root-relative
	// path or an absolute http/https URL (protocol-relative "//" and other schemes
	// are rejected by the backend validateTargetURL to prevent open-redirect).
	function isValidSourcePath(p: string): boolean {
		return p.startsWith("/") && !p.startsWith("//");
	}

	function isValidTargetUrl(u: string): boolean {
		if (u.startsWith("/")) return !u.startsWith("//"); // root-relative
		try {
			const parsed = new URL(u);
			return (parsed.protocol === "http:" || parsed.protocol === "https:") && parsed.host !== "";
		} catch {
			return false;
		}
	}

	async function saveRedirect() {
		const src = sourcePath.trim();
		const tgt = targetUrl.trim();
		if (!src || !tgt) return;
		if (!isValidSourcePath(src)) {
			error = t("redirects.invalid_source", "From path must start with /");
			return;
		}
		if (!isValidTargetUrl(tgt)) {
			error = t("redirects.invalid_target", "To URL must be a root-relative path or an absolute http/https URL");
			return;
		}
		saving = true;
		error = "";
		try {
			// Create payload omits is_active (DB defaults to true); update includes it.
			const body: Record<string, unknown> = {
				source_path: src,
				target_url: tgt,
				http_status: httpStatus,
				notes: notes.trim() || null
			};
			if (editingRedirect) body.is_active = isActive;

			const result = editingRedirect
				? await redirectsAdminApi.update(editingRedirect.id, body)
				: await redirectsAdminApi.create(body);

			if (result.ok) {
				showModal = false;
				success = editingRedirect
					? t("redirects.updated", "Redirect updated")
					: t("redirects.created", "Redirect created");
				loadRedirects();
			} else {
				error = result.error ?? t("common.error", "Failed to save redirect");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to save redirect");
		} finally {
			saving = false;
		}
	}

	function openDelete(redirect: Redirect | null) {
		if (!redirect) return;
		deleteTarget = redirect;
		showDeleteConfirm = true;
	}

	async function confirmDelete() {
		if (!deleteTarget) return;
		const result = await redirectsAdminApi.delete(deleteTarget.id);
		if (result.ok) {
			showDeleteConfirm = false;
			deleteTarget = null;
			success = t("redirects.deleted", "Redirect deleted");
			loadRedirects();
		} else {
			error = result.error ?? t("common.error", "Failed to delete redirect");
		}
	}

	// ADR-056 bulk route: { action: "activate"|"deactivate"|"delete", ids: [...] }.
	async function bulkDelete() {
		if (bulkDeleteIds.size === 0) return;
		const ids = Array.from(bulkDeleteIds);
		const result = await redirectsAdminApi.bulkDelete({ action: "delete", ids });
		if (result.ok) {
			success = t("redirects.bulk_deleted", "Redirects deleted");
			bulkDeleteIds = new Set();
			loadRedirects();
		} else {
			error = result.error ?? t("common.error", "Failed to delete redirects");
		}
	}

	function handleSearch(values: Record<string, string>) {
		filterValues = values;
		loadRedirects();
	}

	function handleClear() {
		filterValues = {};
		loadRedirects();
	}

	function handleTableSearch(q: string) {
		searchQuery = q;
		loadRedirects();
	}

	$effect(() => {
		loadRedirects();
	});
</script>

<svelte:head>
	<title>{t("redirects.title", "Redirects")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("redirects.title", "Redirects")}
	description={t("redirects.description", "Manage URL redirects")}
	actions={[
		...(canManageRedirects
			? [{ label: t("redirects.create", "Create Redirect"), icon: Plus, onclick: openCreate }]
			: []),
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost", onclick: loadRedirects }
	]}
/>

{#if error}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{error}
	</div>
{/if}
{#if success}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)"
	>
		{success}
	</div>
{/if}

{#if bulkDeleteIds.size > 0}
	<div
		class="mb-4 flex items-center justify-between rounded-lg border p-3"
		style="border-color: var(--color-border)"
	>
		<span class="text-sm" style="color: var(--color-foreground)"
			>{bulkDeleteIds.size} {t("redirects.selected", "selected")}</span
		>
		<Button variant="destructive" onclick={bulkDelete}
			>{t("redirects.bulk_delete", "Delete Selected")}</Button
		>
	</div>
{/if}

<Card>
	<CardHeader>
		<FilterBar filters={filterDefs} onsearch={handleSearch} onclear={handleClear} />
	</CardHeader>
	<CardContent>
		{#if loading}
			<TableSkeleton rows={8} columns={columns.length} />
		{:else if redirects.length === 0}
			<div class="py-12 text-center text-sm" style="color: var(--color-muted)">
				{t("redirects.empty", "No redirects found")}
			</div>
		{:else}
			<DataTable
				{columns}
				data={redirects}
				pageSize={10}
				selectable
				bind:selectedIds={bulkDeleteIds}
				searchable
				onsearch={handleTableSearch}
			/>
		{/if}
	</CardContent>
</Card>

<!-- Create/Edit Modal -->
<Modal
	bind:open={showModal}
	title={modalMode === "create" ? t("redirects.create", "Create Redirect") : t("redirects.edit", "Edit Redirect")}
>
	<div class="space-y-4">
		<div>
			<label for="redir-from" class="block text-sm font-medium" style="color: var(--color-foreground)"
				>{t("redirects.from", "From Path")}</label
			>
			<input
				id="redir-from"
				type="text"
				bind:value={sourcePath}
				class="mt-1 w-full rounded-lg border p-2 text-sm font-mono"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				placeholder="/old-page"
			/>
		</div>
		<div>
			<label for="redir-to" class="block text-sm font-medium" style="color: var(--color-foreground)"
				>{t("redirects.to", "To URL")}</label
			>
			<input
				id="redir-to"
				type="text"
				bind:value={targetUrl}
				class="mt-1 w-full rounded-lg border p-2 text-sm font-mono"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				placeholder="/new-page or https://example.com/path"
			/>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="redir-code" class="block text-sm font-medium" style="color: var(--color-foreground)"
					>{t("redirects.code", "Status Code")}</label
				>
				<select
					id="redir-code"
					bind:value={httpStatus}
					class="mt-1 w-full rounded-lg border p-2 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				>
					<option value={301}>301 {t("redirects.permanent", "Permanent")}</option>
					<option value={302}>302 {t("redirects.temporary", "Temporary")}</option>
					<option value={307}>307 {t("redirects.temp_redirect", "Temporary Redirect")}</option>
				</select>
			</div>
			<div class="flex items-center gap-2 pt-6">
				<input
					id="redir-active"
					type="checkbox"
					bind:checked={isActive}
					class="h-4 w-4 rounded"
					style="accent-color: var(--color-primary)"
				/>
				<label for="redir-active" class="text-sm font-medium" style="color: var(--color-foreground)"
					>{t("common.active", "Active")}</label
				>
			</div>
		</div>
		<div>
			<label for="redir-notes" class="block text-sm font-medium" style="color: var(--color-foreground)"
				>{t("redirects.notes", "Notes")}</label
			>
			<input
				id="redir-notes"
				type="text"
				bind:value={notes}
				class="mt-1 w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				placeholder={t("redirects.notes_placeholder", "Optional internal note")}
			/>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={saveRedirect} disabled={saving || !sourcePath.trim() || !targetUrl.trim()}>
			{saving ? t("common.saving", "Saving...") : t("common.save", "Save")}
		</Button>
	</div>
</Modal>

<!-- Delete Confirmation -->
{#if showDeleteConfirm && deleteTarget}
	<Modal bind:open={showDeleteConfirm} title={t("redirects.delete_title", "Delete Redirect")}>
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("redirects.delete_confirm", "Delete redirect from")}
			<code class="font-mono">{deleteTarget.source_path}</code>
			{t("redirects.delete_to", "to")}
			<code class="font-mono">{deleteTarget.target_url}</code>?
		</p>
		<div class="mt-4 flex justify-end gap-2">
			<Button variant="ghost" onclick={() => (showDeleteConfirm = false)}
				>{t("common.cancel", "Cancel")}</Button
			>
			<Button variant="destructive" onclick={confirmDelete}
				>{t("common.delete", "Delete")}</Button
			>
		</div>
	</Modal>
{/if}

<!-- RedirectActionsCell -->
{#snippet RedirectActionsCell({ row }: { value: any; row: Redirect })}
	<div class="flex items-center gap-1">
		{#if canManageRedirects}
			<Button variant="ghost" size="sm" onclick={() => openEdit(row)}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button
				variant="ghost"
				size="sm"
				onclick={() => openDelete(row)}
				style="color: var(--color-danger)"
			>
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
