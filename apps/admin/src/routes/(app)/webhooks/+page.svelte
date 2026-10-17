<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import type { ActionItem } from "$lib/components/ui/data-table";
	import { SlideOver } from "$lib/components/ui/slide-over";
	import { Badge } from "$lib/components/ui/badge";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { Webhook, Plus, RefreshCw, Send, Trash2, Pencil, Play, ShieldCheck, ScrollText } from "lucide-svelte";
	import { webhooksAdminApiExtended, api } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	type WebhookEndpoint = {
		id: string;
		name: string;
		url: string;
		event_types: string[];
		status: string;
		is_active: boolean;
		verified_at: string | null;
		created_by: string;
		created_at: string;
		updated_at: string;
	};

	type WebhookEventDef = {
		key: string;
		label: string;
		description: string;
	};

	type WebhookDelivery = {
		id: string;
		endpoint_id: string;
		event_id: string;
		event_type: string;
		payload: number[];
		status: "pending" | "delivered" | "failed" | "retrying";
		attempts: number;
		last_error: string | null;
		next_attempt_at: string | null;
		last_response_code: number | null;
		last_response_body: string | null;
		delivered_at: string | null;
		created_at: string;
	};

	let endpoints = $state<WebhookEndpoint[]>([]);
	let loading = $state(true);
	let error = $state("");
	let success = $state("");
	let searchQuery = $state("");
	let filterValues = $state<Record<string, string>>({});
	let selectedIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	const canManageWebhooks = $derived(hasPermission("integrations.webhooks.manage"));

	// Modal state
	let showSlideOver = $state(false);
	let modalMode = $state<"create" | "edit">("create");
	let editingEndpoint = $state<WebhookEndpoint | null>(null);
	let saving = $state(false);

	// Delete state
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<WebhookEndpoint | null>(null);

	// Test state
	let showTestModal = $state(false);
	let testTarget = $state<WebhookEndpoint | null>(null);
	let testing = $state(false);

	// Delivery log state
	let showDeliveries = $state(false);
	let deliveriesTarget = $state<WebhookEndpoint | null>(null);
	let deliveries = $state<WebhookDelivery[]>([]);
	let deliveriesLoading = $state(false);
	let deliveriesError = $state("");
	let retryingDeliveryId = $state<string | null>(null);

	let newSecret = $state("");

	// Form
	let formName = $state("");
	let formUrl = $state("");
	let formEvents = $state<string[]>([]);
	let formActive = $state(true);
	let formSecret = $state("");

	// Event types fetched from API
	let availableEvents = $state<WebhookEventDef[]>([]);

	const statusFilters = $derived([
		{
			key: 'is_active',
			label: t('common.status', 'Status'),
			options: [
				{ value: 'true', label: t('common.active', 'Active') },
				{ value: 'false', label: t('common.inactive', 'Inactive') }
			]
		}
	]);

	const filtered = $derived.by(() => {
		let out = endpoints;
		const q = searchQuery.trim().toLowerCase();
		if (q) {
			out = out.filter((ep) =>
				ep.name.toLowerCase().includes(q) ||
				ep.url.toLowerCase().includes(q) ||
				ep.event_types.some((et) => et.toLowerCase().includes(q))
			);
		}
		if (filterValues.is_active) {
			const active = filterValues.is_active === 'true';
			out = out.filter((ep) => ep.is_active === active);
		}
		return out;
	});

	const columns = [
		{ key: "name", label: t("webhooks.name", "Name"), sortable: true },
		{ key: "url", label: t("webhooks.url", "URL"), sortable: true },
		{
			key: "event_types",
			label: t("webhooks.events", "Events"),
			render: (v: string[]) => (v ?? []).slice(0, 3).join(", ") + ((v ?? []).length > 3 ? ` +${v.length - 3}` : "")
		},
		{ key: "is_active", label: t("common.status", "Status"), sortable: true },
		{
			key: "verified_at",
			label: t("webhooks.verified", "Verified"),
			sortable: true,
			render: (v: string | null) => v ? new Date(v).toLocaleDateString() : t("webhooks.not_verified", "Not verified")
		}
	];

	const rowActions: ActionItem[] = [
		{
			label: t("webhooks.test", "Test"),
			icon: Play,
			onclick: (row: WebhookEndpoint) => openTest(row),
			show: () => canManageWebhooks
		},
		{
			label: t("webhooks.verify", "Verify"),
			icon: ShieldCheck,
			onclick: (row: WebhookEndpoint) => verifyEndpoint(row),
			show: (row: WebhookEndpoint) => canManageWebhooks && !row.is_active
		},
		{
			label: t("webhooks.deliveries", "Delivery Log"),
			icon: ScrollText,
			onclick: (row: WebhookEndpoint) => openDeliveries(row),
			show: () => canManageWebhooks
		},
		{
			label: t("common.edit", "Edit"),
			icon: Pencil,
			onclick: (row: WebhookEndpoint) => openEdit(row),
			show: () => canManageWebhooks
		},
		{
			label: t("common.delete", "Delete"),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row: WebhookEndpoint) => openDelete(row),
			show: () => canManageWebhooks
		}
	];

	async function loadEndpoints() {
		loading = true;
		error = "";
		try {
			const result = await webhooksAdminApiExtended.list();
			if (result.ok) {
				endpoints = (result.data as WebhookEndpoint[]) ?? [];
			} else {
				error = result.error ?? t("common.error", "Failed to load webhooks");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load webhooks");
		} finally {
			loading = false;
		}
	}

	async function loadEvents() {
		try {
			const result = await api<WebhookEventDef[]>('/webhook-events');
			if (result.ok && Array.isArray(result.data)) {
				availableEvents = result.data;
			}
		} catch {
		}
	}

	function openCreate() {
		modalMode = "create";
		editingEndpoint = null;
		formName = "";
		formUrl = "";
		formEvents = [];
		formActive = true;
		formSecret = "";
		showSlideOver = true;
	}

	function openEdit(ep: WebhookEndpoint) {
		modalMode = "edit";
		editingEndpoint = ep;
		formName = ep.name;
		formUrl = ep.url;
		formEvents = [...(ep.event_types ?? [])];
		formActive = ep.is_active;
		formSecret = "";
		showSlideOver = true;
	}

	function toggleEvent(event: string) {
		if (formEvents.includes(event)) {
			formEvents = formEvents.filter((e) => e !== event);
		} else {
			formEvents = [...formEvents, event];
		}
	}

	async function saveEndpoint() {
		if (!formName.trim() || !formUrl.trim()) return;
		saving = true;
		error = "";
		try {
			if (editingEndpoint) {
				const body: Record<string, unknown> = {
					name: formName.trim(),
					url: formUrl.trim(),
					event_types: formEvents,
					is_active: formActive
				};
				const result = await webhooksAdminApiExtended.update(editingEndpoint.id, body);
				if (result.ok) {
					showSlideOver = false;
					success = t("webhooks.updated", "Webhook updated");
					loadEndpoints();
				} else {
					error = result.error ?? t("common.error", "Failed to save webhook");
				}
			} else {
				const body = { name: formName.trim(), url: formUrl.trim(), event_types: formEvents };
				const result = await webhooksAdminApiExtended.create(body);
				if (result.ok) {
					showSlideOver = false;
					success = t("webhooks.created", "Webhook created");
					const created = result.data as { secret?: string } | undefined;
					if (created?.secret) {
						newSecret = created.secret;
					}
					loadEndpoints();
				} else {
					error = result.error ?? t("common.error", "Failed to save webhook");
				}
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to save webhook");
		} finally {
			saving = false;
		}
	}

	async function verifyEndpoint(ep: WebhookEndpoint) {
		error = "";
		const result = await webhooksAdminApiExtended.verify(ep.id);
		if (result.ok) {
			success = t("webhooks.verify_success", "Webhook verified successfully");
			loadEndpoints();
		} else {
			error = result.error ?? t("webhooks.verify_error", "Failed to verify webhook");
		}
	}

	function openTest(ep: WebhookEndpoint) {
		testTarget = ep;
		showTestModal = true;
	}

	async function runTest() {
		if (!testTarget) return;
		testing = true;
		const result = await webhooksAdminApiExtended.test(testTarget.id);
		if (result.ok) {
			success = t("webhooks.test_sent", "Test webhook sent");
		} else {
			error = result.error ?? t("common.error", "Test failed");
		}
		showTestModal = false;
		testTarget = null;
		testing = false;
	}

	function openDeliveries(ep: WebhookEndpoint) {
		deliveriesTarget = ep;
		showDeliveries = true;
		loadDeliveries(ep.id);
	}

	async function loadDeliveries(endpointId: string) {
		deliveriesLoading = true;
		deliveriesError = "";
		try {
			const result = await webhooksAdminApiExtended.listDeliveries(endpointId);
			if (result.ok) {
				deliveries = ((result.data as { data?: WebhookDelivery[] })?.data ?? (result.data as WebhookDelivery[]) ?? []) as WebhookDelivery[];
			} else {
				deliveriesError = result.error ?? t("webhooks.deliveries_load_error", "Failed to load deliveries");
			}
		} catch (e) {
			deliveriesError = e instanceof Error ? e.message : t("webhooks.deliveries_load_error", "Failed to load deliveries");
		} finally {
			deliveriesLoading = false;
		}
	}

	async function retryDelivery(delivery: WebhookDelivery) {
		if (!deliveriesTarget) return;
		retryingDeliveryId = delivery.id;
		const result = await webhooksAdminApiExtended.retryDelivery(deliveriesTarget.id, delivery.id);
		if (result.ok) {
			success = t("webhooks.delivery_retry_success", "Delivery queued for retry");
			loadDeliveries(deliveriesTarget.id);
		} else {
			error = result.error ?? t("webhooks.delivery_retry_error", "Failed to retry delivery");
		}
		retryingDeliveryId = null;
	}

	function deliveryStatusVariant(status: WebhookDelivery["status"]): "warning" | "success" | "destructive" | "info" {
		switch (status) {
			case "pending":
				return "warning";
			case "delivered":
				return "success";
			case "failed":
				return "destructive";
			case "retrying":
				return "info";
		}
	}

	function deliveryStatusLabel(status: WebhookDelivery["status"]): string {
		switch (status) {
			case "pending":
				return t("webhooks.delivery_status_pending", "Pending");
			case "delivered":
				return t("webhooks.delivery_status_delivered", "Delivered");
			case "failed":
				return t("webhooks.delivery_status_failed", "Failed");
			case "retrying":
				return t("webhooks.delivery_status_retrying", "Retrying");
		}
	}

	function openDelete(ep: WebhookEndpoint) {
		deleteTarget = ep;
		showDeleteConfirm = true;
	}

	async function confirmDelete() {
		if (!deleteTarget) return;
		const result = await webhooksAdminApiExtended.delete(deleteTarget.id);
		if (result.ok) {
			showDeleteConfirm = false;
			deleteTarget = null;
			success = t("webhooks.deleted", "Webhook deleted");
			loadEndpoints();
		} else {
			error = result.error ?? t("common.error", "Failed to delete webhook");
		}
	}

	async function bulkDelete() {
		if (selectedIds.size === 0) return;
		bulkDeleting = true;
		error = "";
		try {
			for (const id of selectedIds) {
				await webhooksAdminApiExtended.delete(id);
			}
			selectedIds = new Set();
			success = t("webhooks.bulk_deleted", "Selected webhooks deleted");
			loadEndpoints();
		} catch {
			error = t("common.error", "Failed to delete selected webhooks");
		}
		bulkDeleting = false;
	}

	let initialized = false;
	$effect(() => {
		if (!initialized) {
			initialized = true;
			loadEndpoints();
			loadEvents();
		}
	});

	$effect(() => {
		if (success) {
			const msg = success;
			setTimeout(() => { if (success === msg) success = ''; }, 4000);
		}
		if (error) {
			const msg = error;
			setTimeout(() => { if (error === msg) error = ''; }, 6000);
		}
	});

</script>

<svelte:head>
	<title>{t("webhooks.title", "Webhooks")} — {siteName()} Admin</title>
</svelte:head>

<PageHeader
	title={t("webhooks.title", "Webhooks")}
	description={t("webhooks.description", "Manage webhook endpoints and delivery logs")}
	actions={[
		...(canManageWebhooks ? [{ label: t("webhooks.add", "Add Endpoint"), icon: Plus, onclick: openCreate }] : []),
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost", onclick: loadEndpoints }
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
{#if newSecret}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-warning) 10%, transparent); color: var(--color-warning)">
		<div class="flex items-start justify-between gap-3">
			<div class="space-y-1">
				<p class="font-medium">{t("webhooks.secret_generated", "Webhook secret generated")}</p>
				<p class="font-mono break-all">{newSecret}</p>
				<p>{t("webhooks.secret_warning", "Save this secret now — it will not be shown again.")}</p>
			</div>
			<button type="button" onclick={() => newSecret = ''} class="shrink-0 font-medium underline">{t("common.dismiss", "Dismiss")}</button>
		</div>
	</div>
{/if}

<Card>
	<CardContent>
		{#if loading}
			<TableSkeleton rows={8} columns={columns.length} />
		{:else}
			<DataTable {columns} data={filtered} actions={rowActions} pageSize={10} selectable bind:selectedIds searchable searchPlaceholder={t('webhooks.search', 'Search webhooks...')} onsearch={(q) => { searchQuery = q; }} filters={statusFilters} bind:filterValue={filterValues} onfilterapply={() => {}} onfilterclear={() => {}}>
				{#snippet bulkActions()}
					<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting || selectedIds.size === 0}>
						<Trash2 class="h-3.5 w-3.5 mr-1" />
						{bulkDeleting ? t("common.deleting", "Deleting...") : t("common.deleteSelected", "Delete Selected")} ({selectedIds.size})
					</Button>
				{/snippet}
			</DataTable>
		{/if}
	</CardContent>
</Card>

<!-- Create/Edit SlideOver -->
<SlideOver bind:open={showSlideOver} title={modalMode === "create" ? t("webhooks.add", "Add Endpoint") : t("webhooks.edit", "Edit Endpoint")}>
	<div class="space-y-4">
		<div>
			<label for="wh-name" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("webhooks.name", "Name")}</label>
			<input id="wh-name" type="text" bind:value={formName} class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder={t('app.webhooks.ph_eg_slack_notifications', 'e.g., Slack Notifications')} />
		</div>
		<div>
			<label for="wh-url" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("webhooks.url", "URL")}</label>
			<input id="wh-url" type="url" bind:value={formUrl} class="mt-1 w-full rounded-lg border p-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="https://example.com/webhook" />
		</div>
		<div>
			<span class="block text-sm font-medium" style="color: var(--color-foreground)">{t("webhooks.events", "Events")}</span>
			<div class="mt-2 flex flex-wrap gap-2">
				{#each availableEvents as event (event.key)}
					<button type="button" title={event.description} class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium transition-colors" style={formEvents.includes(event.key) ? "border-color: var(--color-primary); background: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)" : "border-color: var(--color-border); color: var(--color-muted)"} onclick={() => toggleEvent(event.key)}>
						{event.label}
					</button>
				{/each}
			</div>
		</div>
		{#if modalMode === "edit"}
			<div class="rounded-lg border p-3 text-xs" style="border-color: var(--color-border); color: var(--color-muted)">
				{t("webhooks.secret_hint", "The signing secret is set on creation and shown once. To rotate, delete and recreate the webhook.")}
			</div>
		{/if}
		<div class="flex items-center gap-2">
			<input id="wh-active" type="checkbox" bind:checked={formActive} class="h-4 w-4 rounded" style="accent-color: var(--color-primary)" />
			<label for="wh-active" class="text-sm font-medium" style="color: var(--color-foreground)">{t("webhooks.active", "Active")}</label>
		</div>
	</div>
	{#snippet footer()}
		<div class="mt-4 flex justify-end gap-2">
			<Button variant="ghost" onclick={() => showSlideOver = false}>{t("common.cancel", "Cancel")}</Button>
			<Button onclick={saveEndpoint} disabled={saving || !formName.trim() || !formUrl.trim()}>
				{saving ? t("common.saving", "Saving...") : t("common.save", "Save")}
			</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- Test Modal -->
<Modal bind:open={showTestModal} title={t("webhooks.test_title", "Test Webhook")}>
	{#if testTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("webhooks.test_confirm", "Send a test event to")} <strong>{testTarget.name}</strong>?
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showTestModal = false}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={runTest} disabled={testing}>{testing ? t("common.sending", "Sending...") : t("webhooks.send_test", "Send Test")}</Button>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t("common.confirmDelete", "Confirm Delete")}>
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("webhooks.delete_confirm", "Delete webhook endpoint")} <strong>{deleteTarget.name}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">{t("common.cannotUndo", "This action cannot be undone.")}</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showDeleteConfirm = false}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmDelete}>{t("common.delete", "Delete")}</Button>
	</div>
</Modal>

<!-- Delivery Log SlideOver -->
<SlideOver bind:open={showDeliveries} title={t("webhooks.deliveries", "Delivery Log")}>
	{#if deliveriesTarget}
		<p class="mb-4 text-sm" style="color: var(--color-muted)">
			{t("webhooks.deliveries_title", "Deliveries for {name}").replace("{name}", deliveriesTarget.name)}
		</p>
	{/if}
	{#if deliveriesError}
		<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
			{deliveriesError}
		</div>
	{/if}
	{#if deliveriesLoading}
		<div class="py-8 text-center text-sm" style="color: var(--color-muted)">{t("common.loading", "Loading...")}</div>
	{:else if deliveries.length === 0}
		<div class="py-8 text-center text-sm" style="color: var(--color-muted)">{t("webhooks.deliveries_empty", "No deliveries yet")}</div>
	{:else}
		<div class="overflow-x-auto">
			<table class="w-full text-sm">
				<thead>
					<tr class="border-b" style="border-color: var(--color-border)">
						<th class="px-2 py-2 text-left font-medium" style="color: var(--color-muted)">{t("webhooks.events", "Events")}</th>
						<th class="px-2 py-2 text-left font-medium" style="color: var(--color-muted)">{t("common.status", "Status")}</th>
						<th class="px-2 py-2 text-left font-medium" style="color: var(--color-muted)">{t("webhooks.delivery_attempts", "Attempts")}</th>
						<th class="px-2 py-2 text-left font-medium" style="color: var(--color-muted)">{t("webhooks.delivery_last_error", "Last Error")}</th>
						<th class="px-2 py-2 text-left font-medium" style="color: var(--color-muted)">{t("webhooks.delivery_response_code", "Response Code")}</th>
						<th class="px-2 py-2 text-left font-medium" style="color: var(--color-muted)">{t("webhooks.delivery_created_at", "Created At")}</th>
						<th class="px-2 py-2 text-left font-medium" style="color: var(--color-muted)">{t("webhooks.delivery_delivered_at", "Delivered At")}</th>
						<th class="px-2 py-2 text-left font-medium" style="color: var(--color-muted)"></th>
					</tr>
				</thead>
				<tbody>
					{#each deliveries as delivery (delivery.id)}
						<tr class="border-b" style="border-color: var(--color-border)">
							<td class="px-2 py-2" style="color: var(--color-foreground)">{delivery.event_type}</td>
							<td class="px-2 py-2">
								<Badge variant={deliveryStatusVariant(delivery.status)}>{deliveryStatusLabel(delivery.status)}</Badge>
							</td>
							<td class="px-2 py-2" style="color: var(--color-foreground)">{delivery.attempts}</td>
							<td class="px-2 py-2 max-w-[200px] truncate" title={delivery.last_error ?? ""} style="color: var(--color-muted)">
								{delivery.last_error ?? "—"}
							</td>
							<td class="px-2 py-2" style="color: var(--color-foreground)">{delivery.last_response_code ?? "—"}</td>
							<td class="px-2 py-2 whitespace-nowrap" style="color: var(--color-foreground)">{new Date(delivery.created_at).toLocaleString()}</td>
							<td class="px-2 py-2 whitespace-nowrap" style="color: var(--color-foreground)">{delivery.delivered_at ? new Date(delivery.delivered_at).toLocaleString() : "—"}</td>
							<td class="px-2 py-2">
								{#if delivery.status === "failed"}
									<Button size="sm" variant="outline" onclick={() => retryDelivery(delivery)} disabled={retryingDeliveryId === delivery.id}>
										<RefreshCw class="h-3.5 w-3.5 mr-1" />
										{retryingDeliveryId === delivery.id ? t("common.loading", "Loading...") : t("webhooks.delivery_retry", "Retry")}
									</Button>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
	{#snippet footer()}
		<div class="mt-4 flex justify-end gap-2">
			<Button variant="ghost" onclick={() => showDeliveries = false}>{t("common.close", "Close")}</Button>
		</div>
	{/snippet}
</SlideOver>
