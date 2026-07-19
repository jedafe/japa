<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { api } from "$lib/api";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent } from "$lib/components/ui/card";
	import { Tabs } from "$lib/components/ui/tabs";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Badge } from "$lib/components/ui/badge";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { Select } from "$lib/components/ui/select";
	import { Zap, Plus, RefreshCw, ToggleLeft, ToggleRight, Eye, Pencil, Trash2 } from "lucide-svelte";

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

	let showViewModal = $state(false);
	let viewTarget = $state<FeatureFlag | null>(null);

	const canManageFlags = $derived(hasPermission("admin.feature_flags.manage"));

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
					<p class="font-medium text-foreground">${row.name}</p>
					<p class="font-mono text-xs text-muted">${row.key}</p>
				</div>
			`
		},
		{
			key: "description",
			label: "Description",
			render: (value: string) => `<span class="text-sm text-muted">${value || "—"}</span>`
		},
		{
			key: "environment",
			label: "Environment",
			sortable: true,
			render: (value: string) => {
				const cls =
					value === "production"
						? "bg-danger/15 text-danger border-danger"
						: value === "staging"
							? "bg-warning/15 text-warning border-warning"
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
					<div class="h-2 w-16 rounded-full bg-surface">
						<div class="h-2 rounded-full bg-primary" style="width: ${value}%"></div>
					</div>
					<span class="text-sm text-foreground-alt">${value}%</span>
				</div>
			`
		},
		{
			key: "isEnabled",
			label: "Status",
			sortable: true,
			render: (value: boolean) => {
				const cls = value
					? "bg-success/15 text-success border-success"
					: "bg-surface text-foreground border";
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${value ? "Enabled" : "Disabled"}</span>`;
			}
		},
		{
			key: "actions",
			label: "",
			class: "w-24",
			component: FlagActionsCell
		}
	];

	async function loadFlags() {
		loading = true;
		const result = await api<FeatureFlag[]>("/admin/feature-flags");
		if (result.ok) {
			flags = Array.isArray(result.data) ? result.data : [];
		}
		loading = false;
	}

	function openView(flag: FeatureFlag) {
		viewTarget = flag;
		showViewModal = true;
	}

	async function submitFlag() {
		if (!form.name.trim() || !form.key.trim()) return;
		saving = true;
		errorMessage = "";
		const result = await api("/admin/feature-flags", {
			method: "POST",
			body: {
				name: form.name.trim(),
				key: form.key.trim(),
				description: form.description.trim(),
				environment: form.environment,
				rolloutPercentage: form.rolloutPercentage,
				isEnabled: false
			}
		});
		if (result.ok) {
			createModalOpen = false;
			resetForm();
			successMessage = "Feature flag created successfully";
			loadFlags();
		} else {
			errorMessage = result.error ?? "Failed to create feature flag";
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
		const result = await api(`/admin/feature-flags/${editingFlag.id}`, {
			method: "PUT",
			body: {
				name: form.name.trim(),
				description: form.description.trim(),
				environment: form.environment,
				rolloutPercentage: form.rolloutPercentage,
				isEnabled: editingFlag.isEnabled
			}
		});
		if (result.ok) {
			editModalOpen = false;
			editingFlag = null;
			resetForm();
			successMessage = "Feature flag updated successfully";
			loadFlags();
		} else {
			errorMessage = result.error ?? "Failed to update feature flag";
		}
		saving = false;
	}

	async function toggleFlag(id: string) {
		const flag = flags.find((f) => f.id === id);
		if (!flag) return;
		const result = await api(`/admin/feature-flags/${id}`, {
			method: "PATCH",
			body: { enabled: !flag.isEnabled }
		});
		if (result.ok) {
			successMessage = `Feature flag "${flag.name}" ${flag.isEnabled ? "disabled" : "enabled"}`;
			loadFlags();
		} else {
			errorMessage = result.error ?? "Failed to toggle flag";
		}
	}

	function openDeleteConfirm(id: string) {
		deletingId = id;
		deleteConfirmOpen = true;
	}

	async function confirmDelete() {
		if (!deletingId) return;
		saving = true;
		const result = await api(`/admin/feature-flags/${deletingId}`, { method: "DELETE" });
		if (result.ok) {
			successMessage = "Feature flag deleted";
			loadFlags();
		} else {
			errorMessage = result.error ?? "Failed to delete feature flag";
		}
		deleteConfirmOpen = false;
		deletingId = null;
		saving = false;
	}

	function resetForm() {
		form = { name: "", key: "", description: "", environment: "production", rolloutPercentage: 100 };
	}

	$effect(() => {
		loadFlags();
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
		<div class="rounded-lg border border-danger bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{errorMessage}
		</div>
	{/if}
	{#if successMessage}
		<div class="rounded-lg border border-success bg-success-50 px-4 py-3 text-sm text-success-700">
			{successMessage}
		</div>
	{/if}

	<!-- Summary Cards -->
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-muted">{t('app.feature-flags.total_flags')}</p>
						<p class="text-2xl font-bold text-foreground">{flags.length}</p>
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
						<p class="text-sm font-medium text-muted">{t('app.feature-flags.enabled')}</p>
						<p class="text-2xl font-bold text-success-600">{flags.filter((f) => f.isEnabled).length}</p>
					</div>
					<div class="rounded-lg bg-success/15 p-3">
						<ToggleRight class="h-6 w-6 text-success-600" />
					</div>
				</div>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-muted">{t('app.feature-flags.disabled')}</p>
						<p class="text-2xl font-bold text-muted">{flags.filter((f) => !f.isEnabled).length}</p>
					</div>
					<div class="rounded-lg bg-surface p-3">
						<ToggleLeft class="h-6 w-6 text-muted" />
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
				<p class="py-8 text-center text-sm text-muted">{t('app.feature-flags.loading')}</p>
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
			<label for="flag-name" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.feature-flags.name')}</label>
			<input
				id="flag-name"
				type="text"
				class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				placeholder="e.g., New Dashboard"
				bind:value={form.name}
			/>
		</div>
		<div>
			<label for="flag-key" class="mb-1 block text-sm font-medium text-foreground-alt">Key</label>
			<input
				id="flag-key"
				type="text"
				class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm font-mono placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				placeholder="e.g., new_dashboard"
				bind:value={form.key}
			/>
		</div>
		<div>
			<label for="flag-desc" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.feature-flags.description')}</label>
			<textarea
				id="flag-desc"
				class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				placeholder="What does this flag control?"
				rows="2"
				bind:value={form.description}
			></textarea>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="flag-env" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.feature-flags.environment')}</label>
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
				<label for="flag-rollout" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.feature-flags.rollout')}</label>
				<input
					id="flag-rollout"
					type="number"
					min="0"
					max="100"
					class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					bind:value={form.rolloutPercentage}
				/>
			</div>
		</div>
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => { createModalOpen = false; resetForm(); }}>{t('app.feature-flags.cancel')}</Button>
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
			<label for="edit-flag-key" class="mb-1 block text-sm font-medium text-foreground-alt">Key</label>
			<input
				id="edit-flag-key"
				type="text"
				class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm font-mono text-muted"
				value={editingFlag?.key}
				disabled
			/>
		</div>
		<div>
			<label for="edit-flag-name" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.feature-flags.name')}</label>
			<input
				id="edit-flag-name"
				type="text"
				class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				placeholder="e.g., New Dashboard"
				bind:value={form.name}
			/>
		</div>
		<div>
			<label for="edit-flag-desc" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.feature-flags.description')}</label>
			<textarea
				id="edit-flag-desc"
				class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				placeholder="What does this flag control?"
				rows="2"
				bind:value={form.description}
			></textarea>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="edit-flag-env" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.feature-flags.environment')}</label>
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
				<label for="edit-flag-rollout" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.feature-flags.rollout')}</label>
				<input
					id="edit-flag-rollout"
					type="number"
					min="0"
					max="100"
					class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					bind:value={form.rolloutPercentage}
				/>
			</div>
		</div>
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => { editModalOpen = false; editingFlag = null; resetForm(); }}>{t('app.feature-flags.cancel')}</Button>
			<Button onclick={submitEditFlag} disabled={saving || !form.name.trim()}>
				{saving ? "Saving..." : "Save Changes"}
			</Button>
		</div>
	</div>
</Modal>

<!-- View Modal -->
<Modal bind:open={showViewModal} title="Feature Flag Details">
	{#if viewTarget}
		<div class="space-y-4">
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Name</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.name}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Key</p>
					<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{viewTarget.key}</p>
				</div>
				<div class="col-span-2">
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Description</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">{viewTarget.description || "—"}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Environment</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.environment}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Rollout</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.rolloutPercentage}%</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Status</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.isEnabled ? "Enabled" : "Disabled"}</p>
				</div>
			</div>
		</div>
		<div class="mt-6 flex justify-end">
			<Button variant="outline" onclick={() => (showViewModal = false)}>Close</Button>
		</div>
	{/if}
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={deleteConfirmOpen} title="Confirm Delete" description="This action cannot be undone">
	<div class="space-y-4">
		<p class="text-sm text-foreground-alt">
			Are you sure you want to delete this feature flag? All configuration and rollout data will be permanently removed.
		</p>
		<div class="flex justify-end gap-3">
			<Button variant="outline" onclick={() => (deleteConfirmOpen = false)}>{t('app.feature-flags.cancel')}</Button>
			<Button variant="destructive" onclick={confirmDelete} disabled={saving}>
				{saving ? "Deleting..." : "Delete Flag"}
			</Button>
		</div>
	</div>
</Modal>

{#snippet FlagActionsCell(row: FeatureFlag)}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView(row)}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canManageFlags}
			<Button
				variant="ghost"
				size="sm"
				onclick={() => toggleFlag(row.id)}
				title={row.isEnabled ? "Disable" : "Enable"}
			>
				{#if row.isEnabled}
					<ToggleRight class="h-3.5 w-3.5 text-success" />
				{:else}
					<ToggleLeft class="h-3.5 w-3.5" />
				{/if}
			</Button>
			<Button variant="ghost" size="sm" onclick={() => openEditModal(row.id)}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => openDeleteConfirm(row.id)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
