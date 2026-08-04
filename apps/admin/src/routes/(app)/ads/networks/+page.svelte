<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { Plus, Pencil, Trash2, RefreshCw } from "lucide-svelte";
	import { adNetworksApi, type AdNetwork } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	const PROVIDERS = ["adsense", "ezoic", "media_net", "custom"] as const;

	let networks = $state<AdNetwork[]>([]);
	let loading = $state(true);
	let error = $state("");
	let success = $state("");
	let selectedIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	const canManage = $derived(hasPermission("ad.network.manage"));

	// Modal state
	let showModal = $state(false);
	let modalMode = $state<"create" | "edit">("create");
	let editingNetwork = $state<AdNetwork | null>(null);
	let saving = $state(false);

	// Delete state
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<AdNetwork | null>(null);

	// Form state
	let provider = $state<string>("adsense");
	let networkName = $state("");
	let clientId = $state("");
	let apiKey = $state("");
	let priority = $state("0");
	let slotMapping = $state("");
	let config = $state("");
	let isActive = $state(true);
	let jsonError = $state("");

	const columns = $derived([
		{
			key: "name",
			label: t("networks.col_name", "Name"),
			sortable: true,
			render: (v: string) =>
				`<span class="font-medium">${v ?? "—"}</span>`
		},
		{
			key: "provider",
			label: t("networks.col_provider", "Provider"),
			sortable: true,
		render: (v: string) => {
			const colorMap: Record<string, string> = {
				adsense: "var(--color-primary)",
				ezoic: "var(--color-success)",
				media_net: "#a855f7",
				custom: "var(--color-muted)",
			};
			const c = colorMap[v] ?? "var(--color-muted)";
			return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold" style="background: color-mix(in srgb, ${c} 15%, transparent); color: ${c}; border-color: ${c}">${v}</span>`;
		}
		},
		{
			key: "client_id",
			label: t("networks.col_client_id", "Client ID"),
			render: (v: string) => {
				if (!v) return `<span class="text-muted">—</span>`;
				const truncated = v.length > 20 ? v.slice(0, 20) + "…" : v;
				return `<span class="font-mono text-xs">${truncated}</span>`;
			}
		},
		{
			key: "priority",
			label: t("networks.col_priority", "Priority"),
			sortable: true,
			render: (v: number) => String(v)
		},
		{
			key: "is_active",
			label: t("common.status", "Status"),
			sortable: true,
			render: (v: boolean) =>
				v
					? `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold bg-success/15 text-success border-success">${t("common.active", "Active")}</span>`
					: `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold bg-surface text-foreground border-border">${t("common.inactive", "Inactive")}</span>`
		},
		{
			key: "actions",
			label: t("common.actions", "Actions"),
			class: "w-24",
			componentSnippet: NetworkActionsCell
		}
	]);

	function parseJsonField(raw: string, field: string): Record<string, unknown> | undefined {
		const trimmed = raw.trim();
		if (!trimmed) return undefined;
		try {
			const parsed = JSON.parse(trimmed);
			if (typeof parsed !== "object" || Array.isArray(parsed)) {
				jsonError = `${field}: ${t("networks.json_object_required", "Must be a JSON object")}`;
				return undefined;
			}
			return parsed as Record<string, unknown>;
		} catch {
			jsonError = `${field}: ${t("networks.json_invalid", "Invalid JSON")}`;
			return undefined;
		}
	}

	async function loadNetworks() {
		loading = true;
		error = "";
		try {
			const result = await adNetworksApi.list("page_size=100");
			if (result.ok) {
				const resp = result.data as Record<string, unknown> | undefined;
				networks = (Array.isArray(resp) ? resp : (resp?.data as AdNetwork[]) ?? []) as AdNetwork[];
			} else {
				error = result.error ?? t("networks.load_error", "Failed to load ad networks");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("networks.load_error", "Failed to load ad networks");
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		modalMode = "create";
		editingNetwork = null;
		provider = "adsense";
		networkName = "";
		clientId = "";
		apiKey = "";
		priority = "0";
		slotMapping = "";
		config = "";
		isActive = true;
		jsonError = "";
		showModal = true;
	}

	function openEdit(network: AdNetwork) {
		modalMode = "edit";
		editingNetwork = network;
		provider = network.provider;
		networkName = network.name;
		clientId = network.client_id ?? "";
		// Ref: security — never pre-fill the API key on edit. The backend stores it
		// and we only send a new value if the admin explicitly enters one.
		apiKey = "";
		priority = String(network.priority);
		slotMapping = network.slot_mapping ? JSON.stringify(network.slot_mapping, null, 2) : "";
		config = network.config ? JSON.stringify(network.config, null, 2) : "";
		isActive = network.is_active;
		jsonError = "";
		showModal = true;
	}

	async function saveNetwork() {
		if (!networkName.trim()) return;
		saving = true;
		error = "";
		jsonError = "";

		const slotMappingParsed = parseJsonField(slotMapping, t("networks.slot_mapping", "Slot Mapping"));
		if (jsonError && slotMapping.trim()) { saving = false; return; }

		const configParsed = parseJsonField(config, t("networks.config", "Config"));
		if (jsonError && config.trim()) { saving = false; return; }

		try {
			const body: Record<string, unknown> = {
				provider,
				name: networkName.trim(),
				priority: Number(priority) || 0,
				is_active: isActive,
			};
			if (clientId.trim()) body.client_id = clientId.trim();
			if (apiKey.trim()) body.api_key = apiKey.trim();
			if (slotMappingParsed) body.slot_mapping = slotMappingParsed;
			if (configParsed) body.config = configParsed;

			const result = editingNetwork
				? await adNetworksApi.update(editingNetwork.id, body)
				: await adNetworksApi.create(body);

			if (result.ok) {
				showModal = false;
				success = editingNetwork
					? t("networks.updated", "Network updated")
					: t("networks.created", "Network created");
				loadNetworks();
			} else {
				error = result.error ?? t("common.error", "Failed to save network");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to save network");
		} finally {
			saving = false;
		}
	}

	function openDelete(network: AdNetwork) {
		deleteTarget = network;
		showDeleteConfirm = true;
	}

	async function confirmDelete() {
		if (!deleteTarget) return;
		try {
			const result = await adNetworksApi.delete(deleteTarget.id);
			if (result.ok) {
				showDeleteConfirm = false;
				deleteTarget = null;
				success = t("networks.deleted", "Network deleted");
				loadNetworks();
			} else {
				error = result.error ?? t("common.error", "Failed to delete network");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to delete network");
		}
	}

	async function bulkDelete() {
		if (selectedIds.size === 0) return;
		bulkDeleting = true;
		error = "";
		try {
			let successCount = 0;
			let failCount = 0;
			for (const id of selectedIds) {
				const result = await adNetworksApi.delete(id);
				if (result.ok) {
					successCount++;
				} else {
					failCount++;
				}
			}
			selectedIds = new Set();
			if (failCount > 0 && successCount > 0) {
				error = t("networks.bulk_partial_failure", "{failCount} of {totalCount} deletions failed")
					.replace("{failCount}", String(failCount))
					.replace("{totalCount}", String(successCount + failCount));
			} else if (failCount > 0) {
				error = t("common.error", "Failed to delete selected networks");
			} else {
				success = t("networks.bulk_deleted", "Selected networks deleted");
			}
			loadNetworks();
		} catch {
			error = t("common.error", "Failed to delete selected networks");
		} finally {
			bulkDeleting = false;
		}
	}

	$effect(() => {
		loadNetworks();
	});
</script>

<svelte:head>
	<title>{t("networks.title", "Ad Networks")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("networks.title", "Ad Networks")}
	description={t("networks.description", "Manage ad network integrations — configure providers, API keys, slot mappings, and priority")}
	actions={[
		...(canManage ? [{ label: t("networks.create", "Create Network"), icon: Plus, onclick: openCreate }] : []),
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost", onclick: loadNetworks }
	]}
/>

{#if error}
	<div role="alert" class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{error}
	</div>
{/if}
{#if success}
	<div role="alert" class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)">
		{success}
	</div>
{/if}

<Card>
	<CardContent>
		{#if loading}
			<TableSkeleton rows={8} columns={columns.length} />
		{:else}
			<DataTable {columns} data={networks} pageSize={20} selectable bind:selectedIds searchable>
				{#snippet bulkActions()}
					{#if canManage}
						<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting || selectedIds.size === 0}>
							<Trash2 class="h-3.5 w-3.5 mr-1" />
							{bulkDeleting ? t("common.deleting", "Deleting...") : t("common.deleteSelected", "Delete Selected")} ({selectedIds.size})
						</Button>
					{/if}
				{/snippet}
			</DataTable>
		{/if}
	</CardContent>
</Card>

<!-- Create/Edit Modal -->
<Modal bind:open={showModal} title={modalMode === "create" ? t("networks.create", "Create Network") : t("networks.edit", "Edit Network")}>
	<div class="space-y-4">
		<div>
			<label for="network-provider" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("networks.provider", "Provider")} *</label>
			<select
				id="network-provider"
				bind:value={provider}
				class="mt-1 w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
			>
				{#each PROVIDERS as p}
					<option value={p}>{p}</option>
				{/each}
			</select>
		</div>
		<div>
			<label for="network-name" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("networks.name", "Name")} *</label>
			<input
				id="network-name"
				type="text"
				bind:value={networkName}
				class="mt-1 w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				placeholder={t("networks.name_placeholder", "e.g. Production AdSense")}
			/>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="network-client-id" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("networks.client_id", "Client ID")}</label>
				<input
					id="network-client-id"
					type="text"
					bind:value={clientId}
					class="mt-1 w-full rounded-lg border p-2 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					placeholder={t("networks.client_id_placeholder", "Optional")}
				/>
			</div>
			<div>
				<label for="network-api-key" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("networks.api_key", "API Key")}</label>
				<input
					id="network-api-key"
					type="password"
					bind:value={apiKey}
					class="mt-1 w-full rounded-lg border p-2 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					placeholder={modalMode === "edit"
						? t("networks.api_key_keep", "Leave empty to keep current key")
						: t("networks.api_key_placeholder", "Optional")}
				/>
			</div>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="network-priority" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("networks.priority", "Priority")}</label>
				<input
					id="network-priority"
					type="number"
					min="0"
					bind:value={priority}
					class="mt-1 w-full rounded-lg border p-2 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				/>
			</div>
			<div class="flex items-center gap-2 pt-6">
				<input id="network-active" type="checkbox" bind:checked={isActive} class="h-4 w-4 rounded" style="accent-color: var(--color-primary)" />
				<label for="network-active" class="text-sm font-medium" style="color: var(--color-foreground)">{t("networks.active", "Active")}</label>
			</div>
		</div>
		<div>
			<label for="network-slot-mapping" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("networks.slot_mapping", "Slot Mapping")}</label>
			<textarea
				id="network-slot-mapping"
				rows="3"
				bind:value={slotMapping}
				class="mt-1 w-full rounded-lg border p-2 text-sm font-mono"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				placeholder={'{"leaderboard": "ca-pub-123/456"}'}
			></textarea>
		</div>
		<div>
			<label for="network-config" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("networks.config", "Config")}</label>
			<textarea
				id="network-config"
				rows="3"
				bind:value={config}
				class="mt-1 w-full rounded-lg border p-2 text-sm font-mono"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				placeholder={'{"auto_ads": true, "ads_per_page": 3}'}
			></textarea>
		</div>
		{#if jsonError}
			<p class="text-xs" style="color: var(--color-danger)">{jsonError}</p>
		{/if}
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showModal = false}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={saveNetwork} disabled={saving || !networkName.trim()}>{saving ? t("common.saving", "Saving...") : t("common.save", "Save")}</Button>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t("common.confirmDelete", "Confirm Delete")}>
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("networks.delete_confirm", "Delete network")} <strong class="font-mono">{deleteTarget.name}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">{t("common.cannotUndo", "This action cannot be undone.")}</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showDeleteConfirm = false}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmDelete}>{t("common.delete", "Delete")}</Button>
	</div>
</Modal>

<!-- NetworkActionsCell snippet -->
{#snippet NetworkActionsCell({ value, row }: { value: any; row: AdNetwork })}
	<div class="flex items-center gap-1">
		{#if canManage}
			<Button variant="ghost" size="sm" onclick={() => openEdit(row)} aria-label={t("networks.edit", "Edit")}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => openDelete(row)} style="color: var(--color-danger)" aria-label={t("networks.delete", "Delete")}>
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
