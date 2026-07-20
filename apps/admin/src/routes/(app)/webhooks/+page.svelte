<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Badge } from "$lib/components/ui/badge";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { Webhook, Plus, RefreshCw, Send, Trash2, Pencil, Play } from "lucide-svelte";
	import { webhooksAdminApiExtended } from "$lib/api";
	import { WEBHOOK_EVENT_TYPES } from '$lib/constants/webhooks';
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	type WebhookEndpoint = {
		id: string;
		name: string;
		url: string;
		events: string[];
		is_active: boolean;
		last_triggered_at?: string | null;
		created_at: string;
	};

	let endpoints = $state<WebhookEndpoint[]>([]);
	let loading = $state(true);
	let error = $state("");
	let success = $state("");

	const canManageWebhooks = $derived(hasPermission("webhooks.manage"));

	// Modal state
	let showModal = $state(false);
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

	// Form
	let formName = $state("");
	let formUrl = $state("");
	let formEvents = $state<string[]>([]);
	let formActive = $state(true);

	const availableEvents = [...WEBHOOK_EVENT_TYPES];

	const columns = [
		{ key: "name", label: t("webhooks.name", "Name"), sortable: true },
		{ key: "url", label: t("webhooks.url", "URL"), sortable: true },
		{
			key: "events",
			label: t("webhooks.events", "Events"),
			render: (v: string[]) => (v ?? []).slice(0, 3).join(", ") + ((v ?? []).length > 3 ? ` +${v.length - 3}` : "")
		},
		{ key: "is_active", label: t("common.status", "Status"), sortable: true },
		{
			key: "last_triggered_at",
			label: t("webhooks.last_triggered", "Last Triggered"),
			sortable: true,
			render: (v: string) => v ? new Date(v).toLocaleDateString() : t("webhooks.never", "Never")
		},
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-32",
			component: WebhookActionsCell
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

	function openCreate() {
		modalMode = "create";
		editingEndpoint = null;
		formName = "";
		formUrl = "";
		formEvents = [];
		formActive = true;
		showModal = true;
	}

	function openEdit(ep: WebhookEndpoint) {
		modalMode = "edit";
		editingEndpoint = ep;
		formName = ep.name;
		formUrl = ep.url;
		formEvents = [...(ep.events ?? [])];
		formActive = ep.is_active;
		showModal = true;
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
			const body = { name: formName.trim(), url: formUrl.trim(), events: formEvents, is_active: formActive };
			const result = editingEndpoint
				? await webhooksAdminApiExtended.update(editingEndpoint.id, body)
				: await webhooksAdminApiExtended.create(body);

			if (result.ok) {
				showModal = false;
				success = editingEndpoint ? t("webhooks.updated", "Webhook updated") : t("webhooks.created", "Webhook created");
				loadEndpoints();
			} else {
				error = result.error ?? t("common.error", "Failed to save webhook");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to save webhook");
		} finally {
			saving = false;
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

</script>

<svelte:head>
	<title>{t("webhooks.title", "Webhooks")} — JapaConnect Admin</title>
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

<Card>
	<CardContent>
		{#if loading}
			<TableSkeleton rows={8} columns={columns.length} />
		{:else}
			<DataTable {columns} data={endpoints} pageSize={10} />
		{/if}
	</CardContent>
</Card>

<!-- Create/Edit Modal -->
<Modal bind:open={showModal} title={modalMode === "create" ? t("webhooks.add", "Add Endpoint") : t("webhooks.edit", "Edit Endpoint")}>
	<div class="space-y-4">
		<div>
			<label for="wh-name" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("webhooks.name", "Name")}</label>
			<input id="wh-name" type="text" bind:value={formName} class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="e.g., Slack Notifications" />
		</div>
		<div>
			<label for="wh-url" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("webhooks.url", "URL")}</label>
			<input id="wh-url" type="url" bind:value={formUrl} class="mt-1 w-full rounded-lg border p-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="https://example.com/webhook" />
		</div>
		<div>
			<span class="block text-sm font-medium" style="color: var(--color-foreground)">{t("webhooks.events", "Events")}</span>
			<div class="mt-2 flex flex-wrap gap-2">
				{#each availableEvents as event}
					<button type="button" class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium transition-colors" style={formEvents.includes(event) ? "border-color: var(--color-primary); background: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)" : "border-color: var(--color-border); color: var(--color-muted)"} onclick={() => toggleEvent(event)}>
						{event}
					</button>
				{/each}
			</div>
		</div>
		<div class="flex items-center gap-2">
			<input id="wh-active" type="checkbox" bind:checked={formActive} class="h-4 w-4 rounded" style="accent-color: var(--color-primary)" />
			<label for="wh-active" class="text-sm font-medium" style="color: var(--color-foreground)">{t("webhooks.active", "Active")}</label>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showModal = false}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={saveEndpoint} disabled={saving || !formName.trim() || !formUrl.trim()}>
			{saving ? t("common.saving", "Saving...") : t("common.save", "Save")}
		</Button>
	</div>
</Modal>

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

<!-- WebhookActionsCell component -->
{#snippet WebhookActionsCell(row: WebhookEndpoint)}
	<div class="flex items-center gap-1">
		{#if canManageWebhooks}
			<Button variant="ghost" size="sm" onclick={() => openTest(row)} style="color: var(--color-primary)" title={t("webhooks.test", "Test")}>
				<Play class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => openEdit(row)}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => openDelete(row)} style="color: var(--color-danger)" title={t("common.delete")}>
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
