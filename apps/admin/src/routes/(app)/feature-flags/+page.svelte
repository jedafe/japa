<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { FEATURE_ENVIRONMENTS } from '$lib/constants/feature-flags';
	import { api } from "$lib/api";

	const environmentOptions = $derived(FEATURE_ENVIRONMENTS.map((env) => ({ value: env, label: env.charAt(0).toUpperCase() + env.slice(1) })));

	import { hasPermission } from "$lib/stores/permissions.svelte";
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent } from "$lib/components/ui/card";
	import { Tabs } from "$lib/components/ui/tabs";
	import { DataTable } from "$lib/components/ui/data-table";
	import type { ActionItem, FilterDef } from "$lib/components/ui/data-table";
	import { SlideOver } from "$lib/components/ui/slide-over";
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
	let createSlideOverOpen = $state(false);
	let editSlideOverOpen = $state(false);
	let editingFlag = $state<FeatureFlag | null>(null);
	let deleteConfirmOpen = $state(false);
	let deletingId = $state<string | null>(null);
	let saving = $state(false);
	let toggling = $state(false);
	let errorMessage = $state("");
	let successMessage = $state("");

	let showViewSlideOver = $state(false);
	let viewTarget = $state<FeatureFlag | null>(null);
	let selectedIds = $state<Set<string>>(new Set());

	const canManageFlags = $derived(hasPermission("admin.feature_flags.manage"));

	let form = $state({
		name: "",
		key: "",
		description: "",
		environment: "production",
		rolloutPercentage: 100
	});

	let tabs = $derived([
		{ value: "all", label: t('app.feature-flags.all_flags', 'All Flags'), icon: Zap, count: flags.length },
		{ value: "enabled", label: t('app.feature-flags.enabled', 'Enabled'), icon: ToggleRight, count: flags.filter((f) => f.isEnabled).length },
		{ value: "disabled", label: t('app.feature-flags.disabled', 'Disabled'), icon: ToggleLeft, count: flags.filter((f) => !f.isEnabled).length }
	]);

	let filteredFlags = $derived.by(() => {
		let result = [...flags];
		if (activeTab === "enabled") result = result.filter((f) => f.isEnabled);
		if (activeTab === "disabled") result = result.filter((f) => !f.isEnabled);
		return result;
	});

	const flagFilters: FilterDef[] = $derived([
		{
			key: "environment",
			label: t('app.feature-flags.filter_environment', 'Environment'),
			options: environmentOptions
		}
	]);

	const flagColumns = [
		{
			key: "name",
			label: t('app.feature-flags.name', 'Name'),
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
			label: t('app.feature-flags.description', 'Description'),
			render: (value: string) => `<span class="text-sm text-muted">${value || "—"}</span>`
		},
		{
			key: "environment",
			label: t('app.feature-flags.environment', 'Environment'),
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
			label: t('app.feature-flags.rollout', 'Rollout'),
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
			label: t('app.feature-flags.label_status', 'Status'),
			sortable: true,
			render: (value: boolean) => {
				const cls = value
					? "bg-success/15 text-success border-success"
					: "bg-surface text-foreground border";
				const label = value ? t('app.feature-flags.status_enabled', 'Enabled') : t('app.feature-flags.status_disabled', 'Disabled');
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${label}</span>`;
			}
		}
	];

	const flagRowActions: ActionItem[] = [
		{ label: t('common.view', 'View'), icon: Eye, onclick: (row: FeatureFlag) => openView(row) },
		{
			label: t('app.feature-flags.disable', 'Disable'),
			icon: ToggleRight,
			onclick: (row: FeatureFlag) => toggleFlag(row.id),
			show: (row: FeatureFlag) => canManageFlags && row.isEnabled
		},
		{
			label: t('app.feature-flags.enable', 'Enable'),
			icon: ToggleLeft,
			onclick: (row: FeatureFlag) => toggleFlag(row.id),
			show: (row: FeatureFlag) => canManageFlags && !row.isEnabled
		},
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row: FeatureFlag) => openEditModal(row.id),
			show: () => canManageFlags
		},
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row: FeatureFlag) => openDeleteConfirm(row.id),
			show: () => canManageFlags
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
		showViewSlideOver = true;
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
			createSlideOverOpen = false;
			resetForm();
			successMessage = t('app.feature-flags.created_successfully', 'Feature flag created successfully');
			loadFlags();
		} else {
			errorMessage = result.error ?? t('app.feature-flags.create_failed', 'Failed to create feature flag');
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
			editSlideOverOpen = true;
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
			editSlideOverOpen = false;
			editingFlag = null;
			resetForm();
			successMessage = t('app.feature-flags.updated_successfully', 'Feature flag updated successfully');
			loadFlags();
		} else {
			errorMessage = result.error ?? t('app.feature-flags.update_failed', 'Failed to update feature flag');
		}
		saving = false;
	}

	async function toggleFlag(id: string) {
		if (toggling) return;
		const flag = flags.find((f) => f.id === id);
		if (!flag) return;
		toggling = true;
		errorMessage = "";
		const result = await api(`/admin/feature-flags/${id}`, {
			method: "PATCH",
			body: { enabled: !flag.isEnabled }
		});
		if (result.ok) {
			successMessage = `${t('app.feature-flags.flag', 'Feature flag')} "${flag.name}" ${flag.isEnabled ? t('app.feature-flags.status_disabled', 'disabled') : t('app.feature-flags.status_enabled', 'enabled')}`;
			loadFlags();
		} else {
			errorMessage = result.error ?? t('app.feature-flags.toggle_failed', 'Failed to toggle flag');
		}
		toggling = false;
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
			successMessage = t('app.feature-flags.deleted', 'Feature flag deleted');
			loadFlags();
		} else {
			errorMessage = result.error ?? t('app.feature-flags.delete_failed', 'Failed to delete feature flag');
		}
		deleteConfirmOpen = false;
		deletingId = null;
		saving = false;
	}

	async function bulkDelete() {
		saving = true;
		errorMessage = "";
		let failed = 0;
		for (const id of selectedIds) {
			const result = await api(`/admin/feature-flags/${id}`, { method: "DELETE" });
			if (!result.ok) failed++;
		}
		if (failed > 0) {
			errorMessage = `${t('app.feature-flags.failed_to_delete_n', 'Failed to delete')} ${failed} ${failed > 1 ? t('app.feature-flags.flags', 'flags') : t('app.feature-flags.flag', 'flag')}`;
		} else {
			successMessage = `${selectedIds.size} ${selectedIds.size > 1 ? t('app.feature-flags.flags', 'flags') : t('app.feature-flags.flag', 'flag')} ${t('app.feature-flags.deleted_suffix', 'deleted')}`;
		}
		selectedIds = new Set();
		await loadFlags();
		saving = false;
	}

	function resetForm() {
		form = { name: "", key: "", description: "", environment: "production", rolloutPercentage: 100 };
	}

	$effect(() => {
		loadFlags();
	});

	$effect(() => {
		let successTimer: ReturnType<typeof setTimeout> | undefined;
		let errorTimer: ReturnType<typeof setTimeout> | undefined;
		if (successMessage) {
			const msg = successMessage;
			successTimer = setTimeout(() => { if (successMessage === msg) successMessage = ''; }, 4000);
		}
		if (errorMessage) {
			const msg = errorMessage;
			errorTimer = setTimeout(() => { if (errorMessage === msg) errorMessage = ''; }, 6000);
		}
		return () => {
			if (successTimer) clearTimeout(successTimer);
			if (errorTimer) clearTimeout(errorTimer);
		};
	});
</script>

<svelte:head>
	<title>{t('app.feature-flags.page_title', 'Feature Flags — JapaConnect Admin')}</title>
</svelte:head>

<PageHeader
	title={t('app.feature-flags.title', 'Feature Flags')}
	description={t('app.feature-flags.manage_description', 'Manage feature flags and rollout percentages')}
	actions={[
		{ label: t('app.feature-flags.create_flag', 'Create Flag'), icon: Plus, onclick: () => (createSlideOverOpen = true) },
		{ label: t('app.feature-flags.refresh', 'Refresh'), icon: RefreshCw, variant: "ghost", onclick: loadFlags }
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

	<Card>
		<CardContent class="p-0">
			{#if loading}
				<p class="py-8 text-center text-sm text-muted">{t('app.feature-flags.loading')}</p>
			{:else}
				<DataTable
					columns={flagColumns}
					data={filteredFlags}
					filters={flagFilters}
					actions={flagRowActions}
					pageSize={20}
					emptyMessage={t('app.feature-flags.no_flags_found', 'No feature flags found')}
					selectable
					bind:selectedIds
				>
					{#snippet bulkActions()}
						{#if canManageFlags}
							<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={saving}>
								<Trash2 class="h-3.5 w-3.5 mr-1" />
								{saving ? t('app.feature-flags.deleting', 'Deleting...') : t('app.feature-flags.delete_selected', 'Delete Selected')}
							</Button>
						{/if}
					{/snippet}
				</DataTable>
			{/if}
		</CardContent>
	</Card>
</div>

<!-- Create Flag SlideOver -->
<SlideOver bind:open={createSlideOverOpen} title={t('app.feature-flags.create_title', 'Create Feature Flag')}>
	<div class="space-y-4">
		<div>
			<label for="flag-name" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.feature-flags.name', 'Name')}</label>
			<input
				id="flag-name"
				type="text"
				class="w-full rounded-lg border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				placeholder={t('app.feature-flags.placeholder_name', 'e.g., New Dashboard')}
				bind:value={form.name}
			/>
		</div>
		<div>
			<label for="flag-key" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.feature-flags.key', 'Key')}</label>
			<input
				id="flag-key"
				type="text"
				class="w-full rounded-lg border bg-surface px-3 py-2 text-sm font-mono placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				placeholder={t('app.feature-flags.placeholder_key', 'e.g., new_dashboard')}
				bind:value={form.key}
			/>
		</div>
		<div>
			<label for="flag-desc" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.feature-flags.description', 'Description')}</label>
			<textarea
				id="flag-desc"
				class="w-full rounded-lg border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				placeholder={t('app.feature-flags.placeholder_description', 'What does this flag control?')}
				rows="2"
				bind:value={form.description}
			></textarea>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="flag-env" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.feature-flags.environment', 'Environment')}</label>
				<Select
					bind:value={form.environment}
					options={environmentOptions}
				/>
			</div>
			<div>
				<label for="flag-rollout" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.feature-flags.rollout', 'Rollout')}</label>
				<input
					id="flag-rollout"
					type="number"
					min="0"
					max="100"
					class="w-full rounded-lg border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					bind:value={form.rolloutPercentage}
				/>
			</div>
		</div>
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => { createSlideOverOpen = false; resetForm(); }}>{t('app.feature-flags.cancel', 'Cancel')}</Button>
			<Button onclick={submitFlag} disabled={saving || !form.name.trim() || !form.key.trim()}>
				{saving ? t('app.feature-flags.creating', 'Creating...') : t('app.feature-flags.create_flag', 'Create Flag')}
			</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- Edit Flag SlideOver -->
<SlideOver bind:open={editSlideOverOpen} title={t('app.feature-flags.edit_title', 'Edit Feature Flag')}>
	<div class="space-y-4">
		<div>
			<label for="edit-flag-key" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.feature-flags.key', 'Key')}</label>
			<input
				id="edit-flag-key"
				type="text"
				class="w-full rounded-lg border bg-surface px-3 py-2 text-sm font-mono text-muted"
				value={editingFlag?.key}
				disabled
			/>
		</div>
		<div>
			<label for="edit-flag-name" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.feature-flags.name', 'Name')}</label>
			<input
				id="edit-flag-name"
				type="text"
				class="w-full rounded-lg border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				placeholder={t('app.feature-flags.placeholder_name', 'e.g., New Dashboard')}
				bind:value={form.name}
			/>
		</div>
		<div>
			<label for="edit-flag-desc" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.feature-flags.description', 'Description')}</label>
			<textarea
				id="edit-flag-desc"
				class="w-full rounded-lg border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				placeholder={t('app.feature-flags.placeholder_description', 'What does this flag control?')}
				rows="2"
				bind:value={form.description}
			></textarea>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="edit-flag-env" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.feature-flags.environment', 'Environment')}</label>
				<Select
					bind:value={form.environment}
					options={environmentOptions}
				/>
			</div>
			<div>
				<label for="edit-flag-rollout" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.feature-flags.rollout', 'Rollout')}</label>
				<input
					id="edit-flag-rollout"
					type="number"
					min="0"
					max="100"
					class="w-full rounded-lg border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					bind:value={form.rolloutPercentage}
				/>
			</div>
		</div>
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => { editSlideOverOpen = false; editingFlag = null; resetForm(); }}>{t('app.feature-flags.cancel', 'Cancel')}</Button>
			<Button onclick={submitEditFlag} disabled={saving || !form.name.trim()}>
				{saving ? t('app.feature-flags.saving', 'Saving...') : t('app.feature-flags.save_changes', 'Save Changes')}
			</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- View SlideOver -->
<SlideOver bind:open={showViewSlideOver} title={t('app.feature-flags.details_title', 'Feature Flag Details')}>
	{#if viewTarget}
		<div class="space-y-4">
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.feature-flags.name', 'Name')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.name}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.feature-flags.key', 'Key')}</p>
					<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{viewTarget.key}</p>
				</div>
				<div class="col-span-2">
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.feature-flags.description', 'Description')}</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">{viewTarget.description || "—"}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.feature-flags.environment', 'Environment')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.environment}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.feature-flags.rollout', 'Rollout')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.rolloutPercentage}%</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.feature-flags.label_status', 'Status')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.isEnabled ? t('app.feature-flags.status_enabled', 'Enabled') : t('app.feature-flags.status_disabled', 'Disabled')}</p>
				</div>
			</div>
		</div>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end">
			<Button variant="outline" onclick={() => (showViewSlideOver = false)}>{t('app.feature-flags.close', 'Close')}</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- Delete Confirmation Modal -->
<Modal bind:open={deleteConfirmOpen} title={t('app.feature-flags.confirm_delete_title', 'Confirm Delete')} description={t('app.feature-flags.confirm_delete_description', 'This action cannot be undone')}>
	<div class="space-y-4">
		<p class="text-sm text-foreground-alt">
			{t('app.feature-flags.confirm_delete_body', 'Are you sure you want to delete this feature flag? All configuration and rollout data will be permanently removed.')}
		</p>
		<div class="flex justify-end gap-3">
			<Button variant="outline" onclick={() => (deleteConfirmOpen = false)}>{t('app.feature-flags.cancel', 'Cancel')}</Button>
			<Button variant="destructive" onclick={confirmDelete} disabled={saving}>
				{saving ? t('app.feature-flags.deleting', 'Deleting...') : t('app.feature-flags.delete_flag', 'Delete Flag')}
			</Button>
		</div>
	</div>
</Modal>
