<script lang="ts">
	import { onMount } from "svelte";
	import { apiFetch } from "$lib/api/client";
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent } from "$lib/components/ui/card";
	import { Tabs } from "$lib/components/ui/tabs";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Badge } from "$lib/components/ui/badge";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { Select } from "$lib/components/ui/select";
	import { Webhook, Plus, RefreshCw, Send, CheckCircle, XCircle } from "lucide-svelte";

	type WebhookEndpoint = {
		id: string;
		name: string;
		url: string;
		events: string[];
		isActive: boolean;
		secret: string;
		lastTriggeredAt: string | null;
		lastStatus: number | null;
		createdAt: string;
	};

	type WebhookLog = {
		id: string;
		webhookId: string;
		webhookName: string;
		event: string;
		status: number;
		duration: number;
		requestBody: string;
		responseBody: string;
		createdAt: string;
	};

	let endpoints = $state<WebhookEndpoint[]>([]);
	let logs = $state<WebhookLog[]>([]);
	let loading = $state(true);
	let activeTab = $state("endpoints");
	let filters = $state<Record<string, string>>({});
	let createModalOpen = $state(false);
	let editModalOpen = $state(false);
	let editingEndpoint = $state<WebhookEndpoint | null>(null);
	let deleteConfirmOpen = $state(false);
	let deletingId = $state<string | null>(null);
	let testModalOpen = $state(false);
	let testingId = $state<string | null>(null);
	let testing = $state(false);
	let saving = $state(false);
	let errorMessage = $state("");
	let successMessage = $state("");

	let form = $state({
		name: "",
		url: "",
		events: [] as string[],
		isActive: true
	});

	const availableEvents = [
		"user.created",
		"user.updated",
		"user.deleted",
		"post.created",
		"post.updated",
		"post.deleted",
		"comment.created",
		"comment.deleted",
		"order.completed",
		"payment.received",
		"ticket.created",
		"ticket.updated"
	];

	let tabs = $derived([
		{ value: "endpoints", label: "Endpoints", icon: Webhook, count: endpoints.length },
		{ value: "logs", label: "Delivery Logs", icon: Send, count: logs.length }
	]);

	const endpointColumns = [
		{
			key: "name",
			label: "Name",
			sortable: true,
			render: (_: any, row: any) => `
				<div>
					<p class="font-medium text-gray-900">${row.name}</p>
					<p class="font-mono text-xs text-gray-500 truncate max-w-[300px]">${row.url}</p>
				</div>
			`
		},
		{
			key: "events",
			label: "Events",
			render: (value: string[]) => `
				<div class="flex flex-wrap gap-1">
					${(value || [])
						.slice(0, 3)
						.map(
							(e) =>
								`<span class="inline-flex items-center rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-700">${e}</span>`
						)
						.join("")}
					${(value || []).length > 3 ? `<span class="text-xs text-gray-500">+${value.length - 3} more</span>` : ""}
				</div>
			`
		},
		{
			key: "isActive",
			label: "Status",
			sortable: true,
			render: (value: boolean) => {
				const cls = value
					? "bg-success-100 text-success-800 border-success-200"
					: "bg-gray-100 text-gray-800 border-gray-200";
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${value ? "Active" : "Inactive"}</span>`;
			}
		},
		{
			key: "lastTriggeredAt",
			label: "Last Triggered",
			sortable: true,
			render: (value: string | null) => value ? `<span class="text-sm text-gray-600">${new Date(value).toLocaleDateString()}</span>` : `<span class="text-sm text-gray-400">Never</span>`
		},
		{
			key: "actions",
			label: "",
			class: "w-32",
			render: (_: any, row: any) => `
				<div class="flex items-center gap-1">
					<button onclick="window.__testWebhook('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-primary-700 hover:bg-primary-50">Test</button>
					<button onclick="window.__editWebhook('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100">Edit</button>
					<button onclick="window.__deleteWebhook('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-danger-700 hover:bg-danger-50">Delete</button>
				</div>
			`
		}
	];

	const logColumns = [
		{
			key: "webhookName",
			label: "Endpoint",
			sortable: true,
			render: (value: string) => `<span class="font-medium text-gray-900">${value}</span>`
		},
		{
			key: "event",
			label: "Event",
			sortable: true,
			render: (value: string) => `<span class="font-mono text-xs text-gray-700">${value}</span>`
		},
		{
			key: "status",
			label: "Status",
			sortable: true,
			render: (value: number) => {
				const success = value >= 200 && value < 300;
				const cls = success
					? "bg-success-100 text-success-800 border-success-200"
					: "bg-danger-100 text-danger-800 border-danger-200";
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${value}</span>`;
			}
		},
		{
			key: "duration",
			label: "Duration",
			sortable: true,
			render: (value: number) => `<span class="text-sm text-gray-600">${value}ms</span>`
		},
		{
			key: "createdAt",
			label: "Time",
			sortable: true,
			render: (value: string) => `<span class="text-sm text-gray-600">${new Date(value).toLocaleString()}</span>`
		}
	];

	async function loadEndpoints() {
		const result = await apiFetch<WebhookEndpoint[]>("/webhooks");
		if (result.data) {
			endpoints = Array.isArray(result.data) ? result.data : [];
		}
	}

	async function loadLogs() {
		const result = await apiFetch<WebhookLog[]>("/webhooks/logs");
		if (result.data) {
			logs = Array.isArray(result.data) ? result.data : [];
		}
	}

	async function loadData() {
		loading = true;
		await Promise.all([loadEndpoints(), loadLogs()]);
		loading = false;
	}

	async function submitEndpoint() {
		if (!form.name.trim() || !form.url.trim()) return;
		saving = true;
		errorMessage = "";
		const result = await apiFetch("/webhooks", {
			method: "POST",
			body: JSON.stringify({
				name: form.name.trim(),
				url: form.url.trim(),
				events: form.events,
				isActive: form.isActive
			})
		});
		if (result.data) {
			createModalOpen = false;
			resetForm();
			successMessage = "Webhook endpoint created successfully";
			loadEndpoints();
		} else {
			errorMessage = result.error?.message ?? "Failed to create webhook";
		}
		saving = false;
	}

	function openEditModal(id: string) {
		const endpoint = endpoints.find((e) => e.id === id);
		if (endpoint) {
			editingEndpoint = endpoint;
			form = {
				name: endpoint.name,
				url: endpoint.url,
				events: [...endpoint.events],
				isActive: endpoint.isActive
			};
			editModalOpen = true;
		}
	}

	async function submitEditEndpoint() {
		if (!editingEndpoint || !form.name.trim()) return;
		saving = true;
		errorMessage = "";
		const result = await apiFetch(`/webhooks/${editingEndpoint.id}`, {
			method: "PUT",
			body: JSON.stringify({
				name: form.name.trim(),
				url: form.url.trim(),
				events: form.events,
				isActive: form.isActive
			})
		});
		if (result.data) {
			editModalOpen = false;
			editingEndpoint = null;
			resetForm();
			successMessage = "Webhook endpoint updated successfully";
			loadEndpoints();
		} else {
			errorMessage = result.error?.message ?? "Failed to update webhook";
		}
		saving = false;
	}

	function openTestModal(id: string) {
		testingId = id;
		testModalOpen = true;
	}

	async function runTest() {
		if (!testingId) return;
		testing = true;
		const result = await apiFetch(`/webhooks/${testingId}/test`, { method: "POST" });
		if (!result.error) {
			successMessage = "Test webhook sent successfully";
			loadLogs();
		} else {
			errorMessage = result.error?.message ?? "Test webhook failed";
		}
		testModalOpen = false;
		testingId = null;
		testing = false;
	}

	function openDeleteConfirm(id: string) {
		deletingId = id;
		deleteConfirmOpen = true;
	}

	async function confirmDelete() {
		if (!deletingId) return;
		saving = true;
		const result = await apiFetch(`/webhooks/${deletingId}`, { method: "DELETE" });
		if (!result.error) {
			successMessage = "Webhook endpoint deleted";
			loadEndpoints();
		} else {
			errorMessage = result.error?.message ?? "Failed to delete webhook";
		}
		deleteConfirmOpen = false;
		deletingId = null;
		saving = false;
	}

	function toggleEvent(event: string) {
		const idx = form.events.indexOf(event);
		if (idx >= 0) {
			form.events = form.events.filter((e) => e !== event);
		} else {
			form.events = [...form.events, event];
		}
	}

	function resetForm() {
		form = { name: "", url: "", events: [], isActive: true };
	}

	onMount(() => {
		loadData();
		(window as any).__editWebhook = openEditModal;
		(window as any).__deleteWebhook = openDeleteConfirm;
		(window as any).__testWebhook = openTestModal;
	});
</script>

<svelte:head>
	<title>Webhooks — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title="Webhooks"
	description="Manage webhook endpoints and delivery logs"
	actions={[
		{ label: "Add Endpoint", icon: Plus, onclick: () => (createModalOpen = true) },
		{ label: "Refresh", icon: RefreshCw, variant: "ghost", onclick: loadData }
	]}
/>

<div class="space-y-6">
	{#if errorMessage}
		<div class="rounded-lg border border-danger-200 bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{errorMessage}
		</div>
	{/if}
	{#if successMessage}
		<div class="rounded-lg border border-success-200 bg-success-50 px-4 py-3 text-sm text-success-700">
			{successMessage}
		</div>
	{/if}

	<!-- Summary Cards -->
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-gray-500">Total Endpoints</p>
						<p class="text-2xl font-bold text-gray-900">{endpoints.length}</p>
					</div>
					<div class="rounded-lg bg-primary-100 p-3">
						<Webhook class="h-6 w-6 text-primary-600" />
					</div>
				</div>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-gray-500">Active</p>
						<p class="text-2xl font-bold text-success-600">{endpoints.filter((e) => e.isActive).length}</p>
					</div>
					<div class="rounded-lg bg-success-100 p-3">
						<CheckCircle class="h-6 w-6 text-success-600" />
					</div>
				</div>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-gray-500">Recent Deliveries</p>
						<p class="text-2xl font-bold text-gray-900">{logs.length}</p>
					</div>
					<div class="rounded-lg bg-info-100 p-3">
						<Send class="h-6 w-6 text-info-600" />
					</div>
				</div>
			</CardContent>
		</Card>
	</div>

	<Tabs {tabs} bind:value={activeTab} />

	{#if activeTab === "endpoints"}
		<Card>
			<CardContent class="p-0">
				{#if loading}
					<p class="py-8 text-center text-sm text-gray-500">Loading...</p>
				{:else}
					<DataTable
						columns={endpointColumns}
						data={endpoints}
						pageSize={20}
						emptyMessage="No webhook endpoints configured"
					/>
				{/if}
			</CardContent>
		</Card>
	{:else if activeTab === "logs"}
		<FilterBar
			bind:value={filters}
			filters={[
				{
					key: "status",
					label: "Status",
					options: [
						{ value: "success", label: "Success (2xx)" },
						{ value: "failed", label: "Failed (4xx/5xx)" }
					]
				}
			]}
		/>

		<Card>
			<CardContent class="p-0">
				{#if loading}
					<p class="py-8 text-center text-sm text-gray-500">Loading...</p>
				{:else}
					<DataTable
						columns={logColumns}
						data={logs}
						pageSize={25}
						emptyMessage="No delivery logs yet"
					/>
				{/if}
			</CardContent>
		</Card>
	{/if}
</div>

<!-- Create Endpoint Modal -->
<Modal bind:open={createModalOpen} title="Add Webhook Endpoint" description="Configure a new webhook delivery endpoint">
	<div class="space-y-4">
		<div>
			<label for="wh-name" class="mb-1 block text-sm font-medium text-gray-700">Name</label>
			<input
				id="wh-name"
				type="text"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				placeholder="e.g., Slack Notifications"
				bind:value={form.name}
			/>
		</div>
		<div>
			<label for="wh-url" class="mb-1 block text-sm font-medium text-gray-700">URL</label>
			<input
				id="wh-url"
				type="url"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm font-mono placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				placeholder="https://example.com/webhook"
				bind:value={form.url}
			/>
		</div>
		<div>
			<label class="mb-1 block text-sm font-medium text-gray-700">Events</label>
			<div class="flex flex-wrap gap-2">
				{#each availableEvents as event}
					<button
						type="button"
						class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium transition-colors {form.events.includes(event)
							? 'border-primary-300 bg-primary-50 text-primary-700'
							: 'border-gray-200 bg-gray-50 text-gray-600 hover:bg-gray-100'}"
						onclick={() => toggleEvent(event)}
					>
						{event}
					</button>
				{/each}
			</div>
		</div>
		<div class="flex items-center gap-2">
			<input
				id="wh-active"
				type="checkbox"
				bind:checked={form.isActive}
				class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
			/>
			<label for="wh-active" class="text-sm font-medium text-gray-700">Active</label>
		</div>
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => { createModalOpen = false; resetForm(); }}>Cancel</Button>
			<Button onclick={submitEndpoint} disabled={saving || !form.name.trim() || !form.url.trim()}>
				{saving ? "Creating..." : "Create Endpoint"}
			</Button>
		</div>
	</div>
</Modal>

<!-- Edit Endpoint Modal -->
<Modal bind:open={editModalOpen} title="Edit Webhook Endpoint" description="Update webhook endpoint configuration">
	<div class="space-y-4">
		<div>
			<label for="edit-wh-name" class="mb-1 block text-sm font-medium text-gray-700">Name</label>
			<input
				id="edit-wh-name"
				type="text"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				placeholder="e.g., Slack Notifications"
				bind:value={form.name}
			/>
		</div>
		<div>
			<label for="edit-wh-url" class="mb-1 block text-sm font-medium text-gray-700">URL</label>
			<input
				id="edit-wh-url"
				type="url"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm font-mono placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				placeholder="https://example.com/webhook"
				bind:value={form.url}
			/>
		</div>
		<div>
			<label class="mb-1 block text-sm font-medium text-gray-700">Events</label>
			<div class="flex flex-wrap gap-2">
				{#each availableEvents as event}
					<button
						type="button"
						class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium transition-colors {form.events.includes(event)
							? 'border-primary-300 bg-primary-50 text-primary-700'
							: 'border-gray-200 bg-gray-50 text-gray-600 hover:bg-gray-100'}"
						onclick={() => toggleEvent(event)}
					>
						{event}
					</button>
				{/each}
			</div>
		</div>
		<div class="flex items-center gap-2">
			<input
				id="edit-wh-active"
				type="checkbox"
				bind:checked={form.isActive}
				class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
			/>
			<label for="edit-wh-active" class="text-sm font-medium text-gray-700">Active</label>
		</div>
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => { editModalOpen = false; editingEndpoint = null; resetForm(); }}>Cancel</Button>
			<Button onclick={submitEditEndpoint} disabled={saving || !form.name.trim()}>
				{saving ? "Saving..." : "Save Changes"}
			</Button>
		</div>
	</div>
</Modal>

<!-- Test Webhook Modal -->
<Modal bind:open={testModalOpen} title="Test Webhook" description="Send a test event to this endpoint">
	<div class="space-y-4">
		<p class="text-sm text-gray-600">
			This will send a <code class="rounded bg-gray-100 px-1.5 py-0.5 text-xs font-mono">webhook.test</code> event to the endpoint. Check the delivery logs for the result.
		</p>
		<div class="flex justify-end gap-3">
			<Button variant="outline" onclick={() => (testModalOpen = false)}>Cancel</Button>
			<Button onclick={runTest} disabled={testing}>
				{testing ? "Sending..." : "Send Test"}
			</Button>
		</div>
	</div>
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={deleteConfirmOpen} title="Confirm Delete" description="This action cannot be undone">
	<div class="space-y-4">
		<p class="text-sm text-gray-600">
			Are you sure you want to delete this webhook endpoint? All delivery history will be permanently removed.
		</p>
		<div class="flex justify-end gap-3">
			<Button variant="outline" onclick={() => (deleteConfirmOpen = false)}>Cancel</Button>
			<Button variant="destructive" onclick={confirmDelete} disabled={saving}>
				{saving ? "Deleting..." : "Delete Endpoint"}
			</Button>
		</div>
	</div>
</Modal>
