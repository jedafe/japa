<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Badge } from "$lib/components/ui/badge";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { Select } from "$lib/components/ui/select";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { Plus, Pencil, Trash2, RefreshCw, ArrowRightLeft } from "lucide-svelte";
	import { redirectsAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	type Redirect = {
		id: string;
		from_path: string;
		to_path: string;
		status_code: number;
		is_active: boolean;
		created_at: string;
	};

	let redirects = $state<Redirect[]>([]);
	let loading = $state(true);
	let error = $state("");
	let success = $state("");

	// Modal state
	let showModal = $state(false);
	let modalMode = $state<"create" | "edit">("create");
	let editingRedirect = $state<Redirect | null>(null);
	let saving = $state(false);

	// Form
	let fromPath = $state("");
	let toPath = $state("");
	let statusCode = $state(301);
	let isActive = $state(true);

	// Delete
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<Redirect | null>(null);
	let bulkDeleteIds = $state<Set<string>>(new Set());
	let filterValues = $state<Record<string, string>>({});

	const canManageRedirects = $derived(hasPermission("redirects.manage"));

	const filterDefs = [
		{
			key: "is_active",
			label: t("common.status", "Status"),
			options: [
				{ value: "true", label: t("common.active", "Active") },
				{ value: "false", label: t("common.inactive", "Inactive") }
			]
		}
	];

	const columns = [
		{ key: "from_path", label: t("redirects.from", "From"), sortable: true },
		{ key: "to_path", label: t("redirects.to", "To"), sortable: true },
		{ key: "status_code", label: t("redirects.code", "Code"), sortable: true },
		{ key: "is_active", label: t("common.status", "Status"), sortable: true,
			render: (v: boolean) => v ? t("common.active", "Active") : t("common.inactive", "Inactive")
		},
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-32",
			componentSnippet: RedirectActionsCell
		}
	];

	async function loadRedirects() {
		loading = true;
		error = "";
		try {
			const params = new URLSearchParams();
			if (filterValues.is_active) params.set("is_active", filterValues.is_active);
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
		fromPath = "";
		toPath = "";
		statusCode = 301;
		isActive = true;
		showModal = true;
	}

	function openEdit(redirect: Redirect | null) {
		if (!redirect) return;
		modalMode = "edit";
		editingRedirect = redirect;
		fromPath = redirect.from_path;
		toPath = redirect.to_path;
		statusCode = redirect.status_code;
		isActive = redirect.is_active;
		showModal = true;
	}

	async function saveRedirect() {
		if (!fromPath.trim() || !toPath.trim()) return;
		if (!/^\/[\w\-./]*$/.test(fromPath.trim()) || !/^\/[\w\-./]*$/.test(toPath.trim())) {
			error = t("redirects.invalid_path", "Paths must start with /");
			return;
		}
		saving = true;
		error = "";
		try {
			const body = {
				from_path: fromPath.trim(),
				to_path: toPath.trim(),
				status_code: statusCode,
				is_active: isActive
			};
			const result = editingRedirect
				? await redirectsAdminApi.update(editingRedirect.id, body)
				: await redirectsAdminApi.create(body);

			if (result.ok) {
				showModal = false;
				success = editingRedirect ? t("redirects.updated", "Redirect updated") : t("redirects.created", "Redirect created");
	loadRedirects();

	function handleSearch(values: Record<string, string>) {
		filterValues = values;
		loadRedirects();
	}

	function handleClear() {
		filterValues = {};
		loadRedirects();
	}
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

	async function bulkDelete() {
		if (bulkDeleteIds.size === 0) return;
		const ids = Array.from(bulkDeleteIds);
		const result = await redirectsAdminApi.bulkDelete({ ids });
		if (result.ok) {
			success = t("redirects.bulk_deleted", "Redirects deleted");
			bulkDeleteIds = new Set();
			loadRedirects();
		} else {
			error = result.error ?? t("common.error", "Failed to delete redirects");
		}
	}

	loadRedirects();

	function handleSearch(values: Record<string, string>) {
		filterValues = values;
		loadRedirects();
	}

	function handleClear() {
		filterValues = {};
		loadRedirects();
	}
</script>

<svelte:head>
	<title>{t("redirects.title", "Redirects")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("redirects.title", "Redirects")}
	description={t("redirects.description", "Manage URL redirects")}
	actions={[
		...(canManageRedirects ? [{ label: t("redirects.create", "Create Redirect"), icon: Plus, onclick: openCreate }] : []),
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost", onclick: loadRedirects }
	]}
/>

{#if error}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{error}
	</div>
{/if}
{#if success}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)">
		{success}
	</div>
{/if}

{#if bulkDeleteIds.size > 0}
	<div class="mb-4 flex items-center justify-between rounded-lg border p-3" style="border-color: var(--color-border)">
		<span class="text-sm" style="color: var(--color-foreground)">{bulkDeleteIds.size} {t("redirects.selected", "selected")}</span>
		<Button variant="destructive" onclick={bulkDelete}>{t("redirects.bulk_delete", "Delete Selected")}</Button>
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
			<!-- wire the search box to re-fetch redirects; API ignores q today (see report) -->
			<DataTable
				{columns}
				data={redirects}
				pageSize={10}
				selectable
				bind:selectedIds={bulkDeleteIds}
				searchable
				onsearch={() => loadRedirects()}
			/>
		{/if}
	</CardContent>
</Card>

<!-- Create/Edit Modal -->
<Modal bind:open={showModal} title={modalMode === "create" ? t("redirects.create", "Create Redirect") : t("redirects.edit", "Edit Redirect")}>
	<div class="space-y-4">
		<div>
			<label for="redir-from" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("redirects.from", "From Path")}</label>
			<input id="redir-from" type="text" bind:value={fromPath} class="mt-1 w-full rounded-lg border p-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="/old-page" />
		</div>
		<div>
			<label for="redir-to" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("redirects.to", "To Path")}</label>
			<input id="redir-to" type="text" bind:value={toPath} class="mt-1 w-full rounded-lg border p-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="/new-page" />
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="redir-code" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("redirects.code", "Status Code")}</label>
				<select id="redir-code" bind:value={statusCode} class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)">
					<option value={301}>301 {t("redirects.permanent", "Permanent")}</option>
					<option value={302}>302 {t("redirects.temporary", "Temporary")}</option>
					<option value={307}>307 {t("redirects.temp_redirect", "Temporary Redirect")}</option>
					<option value={308}>308 {t("redirects.perm_redirect", "Permanent Redirect")}</option>
				</select>
			</div>
			<div class="flex items-center gap-2 pt-6">
				<input id="redir-active" type="checkbox" bind:checked={isActive} class="h-4 w-4 rounded" style="accent-color: var(--color-primary)" />
				<label for="redir-active" class="text-sm font-medium" style="color: var(--color-foreground)">{t("common.active", "Active")}</label>
			</div>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showModal = false}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={saveRedirect} disabled={saving || !fromPath.trim() || !toPath.trim()}>
			{saving ? t("common.saving", "Saving...") : t("common.save", "Save")}
		</Button>
	</div>
</Modal>

<!-- RedirectActionsCell -->
{#snippet RedirectActionsCell({ value, row }: { value: any; row: Redirect })}
	<div class="flex items-center gap-1">
		{#if canManageRedirects}
			<Button variant="ghost" size="sm" onclick={() => openEdit(row)}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => openDelete(row)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
