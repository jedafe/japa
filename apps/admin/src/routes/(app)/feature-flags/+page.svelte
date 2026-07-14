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
	import { Zap, Plus, RefreshCw, ToggleLeft, ToggleRight } from "lucide-svelte";

	type FeatureFlag = {
		id: string;
		name: string;
		key: string;
		description: string;
		isEnabled: boolean;
		environment: string;
		rolloutPercentage: number;
		createdAt: string;
		updatedAt: string;
	};

	let flags = $state<FeatureFlag[]>([]);
	let loading = $state(true);
	let activeTab = $state("all");
	let filters = $state<Record<string, string>>({});
	let createModalOpen = $state(false);
	let editModalOpen = $state(false);
	let editingFlag = $state<FeatureFlag | null>(null);
	let deleteConfirmOpen = $state(false);
	let deletingId = $state<string | null>(null);
	let saving = $state(false);
	let errorMessage = $state("");
	let successMessage = $state("");

	let form = $state({
		name: "",
		key: "",
		description: "",
		environment: "production",
		rolloutPercentage: 100
	});

	let tabs = $derived([
		{ value: "all", label: "All Flags", icon: Zap, count: flags.length },
		{ value: "enabled", label: "Enabled", icon: ToggleRight, count: flags.filter((f) => f.isEnabled).length },
		{ value: "disabled", label: "Disabled", icon: ToggleLeft, count: flags.filter((f) => !f.isEnabled).length }
	]);

	let filteredFlags = $derived.by(() => {
		let result = [...flags];
		if (activeTab === "enabled") result = result.filter((f) => f.isEnabled);
		if (activeTab === "disabled") result = result.filter((f) => !f.isEnabled);
		if (filters.environment) result = result.filter((f) => f.environment === filters.environment);
		return result;
	});

	const flagColumns = [
		{
			key: "name",
			label: "Name",
			sortable: true,
			render: (_: any, row: any) => `
				<div>
					<p class="font-medium text-gray-900">${row.name}</p>
					<p class="font-mono text-xs text-gray-500">${row.key}</p>
				</div>
			`
		},
		{
			key: "description",
			label: "Description",
			render: (value: string) => `<span class="text-sm text-gray-500">${value || "—"}</span>`
		},
		{
			key: "environment",
			label: "Environment",
			sortable: true,
			render: (value: string) => {
				const cls =
					value === "production"
						? "bg-danger-100 text-danger-800 border-danger-200"
						: value === "staging"
							? "bg-warning-100 text-warning-800 border-warning-200"
							: "bg-info-100 text-info-800 border-info-200";
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${value}</span>`;
			}
		},
		{
			key: "rolloutPercentage",
			label: "Rollout",
			sortable: true,
			render: (value: number) => `
				<div class="flex items-center gap-2">
					<div class="h-2 w-16 rounded-full bg-gray-200">
						<div class="h-2 rounded-full bg-primary-600" style="width: ${value}%"></div>
					</div>
					<span class="text-sm text-gray-600">${value}%</span>
				</div>
			`
		},
		{
			key: "isEnabled",
			label: "Status",
			sortable: true,
			render: (value: boolean) => {
				const cls = value
					? "bg-success-100 text-success-800 border-success-200"
					: "bg-gray-100 text-gray-800 border-gray-200";
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${value ? "Enabled" : "Disabled"}</span>`;
			}
		},
		{
			key: "actions",
			label: "",
			class: "w-24",
			render: (_: any, row: any) => `
				<div class="flex items-center gap-1">
					<button onclick="window.__editFlag('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100">Edit</button>
					<button onclick="window.__deleteFlag('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-danger-700 hover:bg-danger-50">Delete</button>
				</div>
			`
		}
	];

	async function loadFlags() {
		loading = true;
		const result = await apiFetch<FeatureFlag[]>("/feature-flags");
		if (result.data) {
			flags = Array.isArray(result.data) ? result.data : [];
		}
		loading = false;
	}

	async function submitFlag() {
		if (!form.name.trim() || !form.key.trim()) return;
		saving = true;
		errorMessage = "";
		const result = await apiFetch("/feature-flags", {
			method: "POST",
			body: JSON.stringify({
				name: form.name.trim(),
				key: form.key.trim(),
				description: form.description.trim(),
				environment: form.environment,
				rolloutPercentage: form.rolloutPercentage,
				isEnabled: false
			})
		});
		if (result.data) {
			createModalOpen = false;
			resetForm();
			successMessage = "Feature flag created successfully";
			loadFlags();
		} else {
			errorMessage = result.error?.message ?? "Failed to create feature flag";
		}
		saving = false;
	}

	function openEditModal(id: string) {
		const flag = flags.find((f) => f.id === id);
		if (flag) {
			editingFlag = flag;
			form = {
				name: flag.name,
				key: flag.key,
				description: flag.description,
				environment: flag.environment,
				rolloutPercentage: flag.rolloutPercentage
			};
			editModalOpen = true;
		}
	}

	async function submitEditFlag() {
		if (!editingFlag || !form.name.trim()) return;
		saving = true;
		errorMessage = "";
		const result = await apiFetch(`/feature-flags/${editingFlag.id}`, {
			method: "PUT",
			body: JSON.stringify({
				name: form.name.trim(),
				description: form.description.trim(),
				environment: form.environment,
				rolloutPercentage: form.rolloutPercentage
			})
		});
		if (result.data) {
			editModalOpen = false;
			editingFlag = null;
			resetForm();
			successMessage = "Feature flag updated successfully";
			loadFlags();
		} else {
			errorMessage = result.error?.message ?? "Failed to update feature flag";
		}
		saving = false;
	}

	async function toggleFlag(id: string) {
		const flag = flags.find((f) => f.id === id);
		if (!flag) return;
		const result = await apiFetch(`/feature-flags/${id}/toggle`, { method: "POST" });
		if (!result.error) {
			successMessage = `Feature flag "${flag.name}" ${flag.isEnabled ? "disabled" : "enabled"}`;
			loadFlags();
		} else {
			errorMessage = result.error?.message ?? "Failed to toggle flag";
		}
	}

	function openDeleteConfirm(id: string) {
		deletingId = id;
		deleteConfirmOpen = true;
	}

	async function confirmDelete() {
		if (!deletingId) return;
		saving = true;
		const result = await apiFetch(`/feature-flags/${deletingId}`, { method: "DELETE" });
		if (!result.error) {
			successMessage = "Feature flag deleted";
			loadFlags();
		} else {
			errorMessage = result.error?.message ?? "Failed to delete feature flag";
		}
		deleteConfirmOpen = false;
		deletingId = null;
		saving = false;
	}

	function resetForm() {
		form = { name: "", key: "", description: "", environment: "production", rolloutPercentage: 100 };
	}

	onMount(() => {
		loadFlags();
		(window as any).__editFlag = openEditModal;
		(window as any).__deleteFlag = openDeleteConfirm;
	});
</script>

<svelte:head>
	<title>Feature Flags — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title="Feature Flags"
	description="Manage feature flags and rollout percentages"
	actions={[
		{ label: "Create Flag", icon: Plus, onclick: () => (createModalOpen = true) },
		{ label: "Refresh", icon: RefreshCw, variant: "ghost", onclick: loadFlags }
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
						<p class="text-sm font-medium text-gray-500">Total Flags</p>
						<p class="text-2xl font-bold text-gray-900">{flags.length}</p>
					</div>
					<div class="rounded-lg bg-primary-100 p-3">
						<Zap class="h-6 w-6 text-primary-600" />
					</div>
				</div>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-gray-500">Enabled</p>
						<p class="text-2xl font-bold text-success-600">{flags.filter((f) => f.isEnabled).length}</p>
					</div>
					<div class="rounded-lg bg-success-100 p-3">
						<ToggleRight class="h-6 w-6 text-success-600" />
					</div>
				</div>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-gray-500">Disabled</p>
						<p class="text-2xl font-bold text-gray-400">{flags.filter((f) => !f.isEnabled).length}</p>
					</div>
					<div class="rounded-lg bg-gray-100 p-3">
						<ToggleLeft class="h-6 w-6 text-gray-400" />
					</div>
				</div>
			</CardContent>
		</Card>
	</div>

	<Tabs {tabs} bind:value={activeTab} />

	<FilterBar
		bind:value={filters}
		filters={[
			{
				key: "environment",
				label: "Environment",
				options: [
					{ value: "production", label: "Production" },
					{ value: "staging", label: "Staging" },
					{ value: "development", label: "Development" }
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
					columns={flagColumns}
					data={filteredFlags}
					pageSize={20}
					emptyMessage="No feature flags found"
				/>
			{/if}
		</CardContent>
	</Card>
</div>

<!-- Create Flag Modal -->
<Modal bind:open={createModalOpen} title="Create Feature Flag" description="Add a new feature flag to control feature rollouts">
	<div class="space-y-4">
		<div>
			<label for="flag-name" class="mb-1 block text-sm font-medium text-gray-700">Name</label>
			<input
				id="flag-name"
				type="text"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				placeholder="e.g., New Dashboard"
				bind:value={form.name}
			/>
		</div>
		<div>
			<label for="flag-key" class="mb-1 block text-sm font-medium text-gray-700">Key</label>
			<input
				id="flag-key"
				type="text"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm font-mono placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				placeholder="e.g., new_dashboard"
				bind:value={form.key}
			/>
		</div>
		<div>
			<label for="flag-desc" class="mb-1 block text-sm font-medium text-gray-700">Description</label>
			<textarea
				id="flag-desc"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				placeholder="What does this flag control?"
				rows="2"
				bind:value={form.description}
			></textarea>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="flag-env" class="mb-1 block text-sm font-medium text-gray-700">Environment</label>
				<Select
					bind:value={form.environment}
					options={[
						{ value: "production", label: "Production" },
						{ value: "staging", label: "Staging" },
						{ value: "development", label: "Development" }
					]}
				/>
			</div>
			<div>
				<label for="flag-rollout" class="mb-1 block text-sm font-medium text-gray-700">Rollout %</label>
				<input
					id="flag-rollout"
					type="number"
					min="0"
					max="100"
					class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
					bind:value={form.rolloutPercentage}
				/>
			</div>
		</div>
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => { createModalOpen = false; resetForm(); }}>Cancel</Button>
			<Button onclick={submitFlag} disabled={saving || !form.name.trim() || !form.key.trim()}>
				{saving ? "Creating..." : "Create Flag"}
			</Button>
		</div>
	</div>
</Modal>

<!-- Edit Flag Modal -->
<Modal bind:open={editModalOpen} title="Edit Feature Flag" description="Update feature flag configuration">
	<div class="space-y-4">
		<div>
			<label for="edit-flag-key" class="mb-1 block text-sm font-medium text-gray-700">Key</label>
			<input
				id="edit-flag-key"
				type="text"
				class="w-full rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 text-sm font-mono text-gray-500"
				value={editingFlag?.key}
				disabled
			/>
		</div>
		<div>
			<label for="edit-flag-name" class="mb-1 block text-sm font-medium text-gray-700">Name</label>
			<input
				id="edit-flag-name"
				type="text"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				placeholder="e.g., New Dashboard"
				bind:value={form.name}
			/>
		</div>
		<div>
			<label for="edit-flag-desc" class="mb-1 block text-sm font-medium text-gray-700">Description</label>
			<textarea
				id="edit-flag-desc"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				placeholder="What does this flag control?"
				rows="2"
				bind:value={form.description}
			></textarea>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="edit-flag-env" class="mb-1 block text-sm font-medium text-gray-700">Environment</label>
				<Select
					bind:value={form.environment}
					options={[
						{ value: "production", label: "Production" },
						{ value: "staging", label: "Staging" },
						{ value: "development", label: "Development" }
					]}
				/>
			</div>
			<div>
				<label for="edit-flag-rollout" class="mb-1 block text-sm font-medium text-gray-700">Rollout %</label>
				<input
					id="edit-flag-rollout"
					type="number"
					min="0"
					max="100"
					class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
					bind:value={form.rolloutPercentage}
				/>
			</div>
		</div>
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => { editModalOpen = false; editingFlag = null; resetForm(); }}>Cancel</Button>
			<Button onclick={submitEditFlag} disabled={saving || !form.name.trim()}>
				{saving ? "Saving..." : "Save Changes"}
			</Button>
		</div>
	</div>
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={deleteConfirmOpen} title="Confirm Delete" description="This action cannot be undone">
	<div class="space-y-4">
		<p class="text-sm text-gray-600">
			Are you sure you want to delete this feature flag? All configuration and rollout data will be permanently removed.
		</p>
		<div class="flex justify-end gap-3">
			<Button variant="outline" onclick={() => (deleteConfirmOpen = false)}>Cancel</Button>
			<Button variant="destructive" onclick={confirmDelete} disabled={saving}>
				{saving ? "Deleting..." : "Delete Flag"}
			</Button>
		</div>
	</div>
</Modal>
