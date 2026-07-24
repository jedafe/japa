<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { Tabs } from "$lib/components/ui/tabs";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Badge } from "$lib/components/ui/badge";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { FileText, Plus, Eye, BarChart3, RefreshCw, Trash2, Pencil, ExternalLink, Settings } from "lucide-svelte";
	import { goto } from '$app/navigation';
	import { api, formsAdminApi, spacesAdminApi } from "$lib/api";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	type Form = {
		id: string;
		name: string;
		slug: string;
		space_id?: string;
		description?: string;
		status: 'active' | 'draft';
		is_multi_step?: boolean;
		submit_label?: string;
		success_message?: string;
		is_system?: boolean;
		created_at: string;
	};

	let { data } = $props();

	let activeTab = $state("all");
	let forms = $state<Form[]>([]);
	let loading = $state(true);
	let filters = $state<Record<string, string>>({});
	let selectedIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	// Space filter
	let availableSpaces = $state<Array<{ id: string; name: string }>>([]);
	let selectedSpaceId = $state("");
	let selectedTypeFilter = $state("");

	// Delete confirmation
	let showDeleteModal = $state(false);
	let deleteTarget = $state<Form | null>(null);
	let deleting = $state(false);

	// View modal
	let showViewModal = $state(false);
	let viewTarget = $state<Form | null>(null);

	// Edit modal
	let showEditModal = $state(false);
	let editTarget = $state<Form | null>(null);
	let editTitle = $state("");
	let editDescription = $state("");
	let editIsActive = $state(true);
	let savingEdit = $state(false);

	// Create modal
	let showCreateModal = $state(false);
	let createTitle = $state("");
	let createSlug = $state("");
	let createDescription = $state("");
	let createIsActive = $state(true);
	let savingCreate = $state(false);

	// Backend gates form mutations on forms.create/edit/delete
	const canManageForms = $derived(hasPermission("forms.create"));

	const tabs = [
		{ value: "all", label: t('forms.tabAll', 'All Forms'), icon: FileText },
		{ value: "system", label: t('forms.tabSystem', 'System'), icon: Settings },
		{ value: "active", label: t('forms.tabActive', 'Active'), icon: Eye },
		{ value: "inactive", label: t('forms.tabInactive', 'Inactive'), icon: FileText }
	];

	const columns = [
		{
			key: "name",
			label: t('forms.columnForm', 'Form'),
			sortable: true,
		render: (_: any, row: any) => `
			<div>
				<a href="/forms/${row.id}" class="font-medium text-foreground hover:underline cursor-pointer" onclick="event.stopPropagation()">${row.name}</a>
				<p class="text-xs font-mono text-muted">${row.slug}</p>
				${row.is_system ? '<span class="mt-1 inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-semibold" style="border-color: var(--color-info); color: var(--color-info)">System</span>' : ''}
			</div>
		`
		},
		{
			key: "space_id",
			label: t('forms.columnSpace', 'Space'),
			sortable: true,
			render: (value: any, row: any) => `<span class="text-sm text-foreground-alt">${row.space_id ? 'Space-specific' : t('forms.global', 'Global')}</span>`
		},
		{
			key: "is_system",
			label: t('forms.columnType', 'Type'),
			sortable: true,
			render: (value: boolean) => {
				if (value) {
					return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold bg-info/15 text-info border-info">${t('forms.type.system', 'System')}</span>`;
				}
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold bg-surface text-foreground border">${t('forms.type.custom', 'Custom')}</span>`;
			}
		},
		{
			key: "status",
			label: t('forms.columnStatus', 'Status'),
			sortable: true,
			render: (value: string) => {
				const isActive = value === 'active';
				const classes = isActive ? "bg-success/15 text-success border-success" : "bg-surface text-foreground border";
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes}">${isActive ? t('forms.status.active', 'Active') : t('forms.status.inactive', 'Inactive')}</span>`;
			}
		},
		{
			key: "created_at",
			label: t('forms.created', 'Created'),
			sortable: true,
			render: (value: string) => new Date(value).toLocaleDateString("en-US", { year: "numeric", month: "short", day: "numeric" })
		},
		{
			key: "actions",
			label: t('forms.columnActions', 'Actions'),
			class: "w-32",
			componentSnippet: FormActionsCell
		}
	];

	let filteredForms = $derived.by(() => {
		let result = [...forms];
		if (activeTab === "system") result = result.filter((f) => f.is_system === true);
		if (activeTab === "active") result = result.filter((f) => f.status === 'active');
		if (activeTab === "inactive") result = result.filter((f) => f.status !== 'active');
		if (selectedTypeFilter === "system") result = result.filter((f) => f.is_system === true);
		if (selectedTypeFilter === "custom") result = result.filter((f) => f.is_system !== true);
		return result;
	});

	async function loadForms(spaceId?: string) {
		loading = true;
		try {
			const params = spaceId ? `space_id=${spaceId}` : undefined;
			const result = await formsAdminApi.list(params);
			if (result.ok) {
				const raw = result.data;
				forms = Array.isArray(raw) ? raw : (raw as any)?.data ?? [];
			}
		} catch {
			// API not available
		} finally {
			loading = false;
		}
	}

	function handleFilter(values: Record<string, string>) {
		filters = values;
		selectedSpaceId = values.space ?? "";
		selectedTypeFilter = values.type ?? "";
		loadForms(selectedSpaceId || undefined);
	}

	function openView(form: Form) {
		viewTarget = form;
		showViewModal = true;
	}

	function confirmDelete(form: Form) {
		deleteTarget = form;
		showDeleteModal = true;
	}

	function openEdit(form: Form) {
		editTarget = form;
		editTitle = form.name;
		editDescription = form.description ?? "";
		editIsActive = form.status === 'active';
		showEditModal = true;
	}

	async function saveEdit() {
		if (!editTarget || !editTitle.trim()) return;
		savingEdit = true;
		try {
			const result = await formsAdminApi.update(editTarget.id, {
				name: editTitle.trim(),
				description: editDescription.trim() || undefined,
				status: editIsActive ? "active" : "draft"
			});
			if (result.ok) {
				showEditModal = false;
				loadForms();
			}
		} catch {
			// API not available
		} finally {
			savingEdit = false;
		}
	}

	async function deleteForm() {
		if (!deleteTarget) return;
		deleting = true;
		try {
			await formsAdminApi.delete(deleteTarget.id);
			forms = forms.filter((f) => f.id !== deleteTarget!.id);
			showDeleteModal = false;
			deleteTarget = null;
		} catch {
			// API not available
		} finally {
			deleting = false;
		}
	}

	async function bulkDelete() {
		const ids = Array.from(selectedIds);
		if (ids.length === 0) return;
		bulkDeleting = true;
		try {
			await Promise.all(ids.map((id) => formsAdminApi.delete(id)));
			selectedIds = new Set();
			loadForms();
		} catch {
			// API not available
		} finally {
			bulkDeleting = false;
		}
	}

	// slugify turns a title into a URL-safe slug (mirrors backend slug rules).
	function slugify(value: string): string {
		return value
			.toLowerCase()
			.trim()
			.replace(/[^a-z0-9\s-]/g, "")
			.replace(/\s+/g, "-")
			.replace(/-+/g, "-")
			.replace(/^-|-$/g, "") || "form";
	}

	function openCreate() {
		createTitle = "";
		createSlug = "";
		createDescription = "";
		createIsActive = true;
		showCreateModal = true;
	}

	// Auto-fill the slug from the title until the user edits it manually.
	let slugTouched = $state(false);
	function onTitleInput() {
		if (!slugTouched) createSlug = slugify(createTitle);
	}

	async function createForm() {
		if (!createTitle.trim() || !createSlug.trim()) return;
		savingCreate = true;
		try {
			const result = await formsAdminApi.create({
				name: createTitle.trim(),
				slug: createSlug.trim(),
				description: createDescription.trim() || undefined,
				status: createIsActive ? "active" : "draft"
			});
			if (result.ok) {
				showCreateModal = false;
				loadForms();
			}
		} catch {
			// API not available
		} finally {
			savingCreate = false;
		}
	}

	let initialized = false;
	$effect(() => {
		if (!initialized) {
			initialized = true;
			loadSpaces();
			loadForms();
		}
	});

	async function loadSpaces() {
		try {
			const result = await spacesAdminApi.list();
			if (result.ok) {
				const raw = result.data as any;
				const items = raw?.items ?? (Array.isArray(raw) ? raw : raw?.data?.items ?? []);
				availableSpaces = items.map((s: any) => ({ id: s.id, name: s.name }));
			}
		} catch {
			// ignore — space filter just won't populate
		}
	}
</script>

<svelte:head>
	<title>{t('forms.title', 'Forms')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('forms.title', 'Forms')}
	description={t('forms.description', 'Manage platform forms and collect user submissions')}
	actions={[
		...(canManageForms
			? [{ label: t('forms.create', 'Create Form'), icon: Plus, onclick: openCreate }]
			: []),
		{ label: t('common.refresh', 'Refresh'), icon: RefreshCw, onclick: loadForms, variant: "outline" }
	]}
/>

<div class="space-y-6">
	<Tabs {tabs} bind:value={activeTab} />

	<FilterBar
		bind:value={filters}
		filters={[
			{
				key: "space",
				label: t('forms.filter_space', 'Space'),
				options: [
					{ value: "", label: t('forms.all_spaces', 'All Spaces') },
					...availableSpaces.map((s) => ({ value: s.id, label: s.name }))
				]
			},
			{
				key: "type",
				label: t('forms.filter_type', 'Type'),
				options: [
					{ value: "", label: t('forms.all_types', 'All Types') },
					{ value: "system", label: t('forms.type_system', 'System') },
					{ value: "custom", label: t('forms.type_custom', 'Custom') }
				]
			}
		]}
		onsearch={handleFilter}
		onclear={() => { filters = {}; selectedSpaceId = ""; selectedTypeFilter = ""; loadForms(); }}
	/>

	<Card>
		<CardContent class="p-0">
			{#if loading}
				<p class="py-8 text-center text-sm text-muted">{t('forms.loading_forms')}</p>
		{:else}
			<DataTable
				columns={columns}
				data={filteredForms}
				loading={loading}
				selectable
				bind:selectedIds
				pageSize={20}
				emptyMessage={t('forms.no_forms', 'No forms found')}
			>
				{#snippet bulkActions()}
						<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting}>
							<Trash2 class="mr-1 h-3.5 w-3.5" />
							{bulkDeleting ? t('common.deleting', 'Deleting...') : `Delete Selected (${selectedIds.size})`}
						</Button>
					{/snippet}
				</DataTable>
			{/if}
		</CardContent>
	</Card>

	<!-- Summary Stats -->
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-4">
		<Card>
			<CardContent class="p-4">
				<p class="text-sm text-muted">{t('forms.statTotal', 'Total Forms')}</p>
				<p class="text-2xl font-bold text-foreground">{forms.length}</p>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-4">
				<p class="text-sm text-muted">{t('forms.statActive', 'Active Forms')}</p>
				<p class="text-2xl font-bold text-foreground">{forms.filter((f) => f.status === 'active').length}</p>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-4">
				<p class="text-sm text-muted">{t('forms.statInactive', 'Inactive Forms')}</p>
				<p class="text-2xl font-bold text-foreground">{forms.filter((f) => f.status !== 'active').length}</p>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-4">
				<p class="text-sm text-muted">{t('forms.statSystem', 'System Forms')}</p>
				<p class="text-2xl font-bold text-foreground">{forms.filter((f) => f.is_system).length}</p>
			</CardContent>
		</Card>
	</div>
</div>

<!-- Create Modal -->
<Modal bind:open={showCreateModal} title={t('forms.create', 'Create Form')} size="sm">
	<div class="space-y-4">
		<div>
			<label for="form-create-title" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.title_label', 'Title')}</label>
			<input
				id="form-create-title"
				type="text"
				class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				placeholder={t('forms.title_placeholder', 'e.g., Feedback Survey')}
				bind:value={createTitle}
				oninput={onTitleInput}
			/>
		</div>
		<div>
			<label for="form-create-slug" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.slug_label', 'Slug')}</label>
			<input
				id="form-create-slug"
				type="text"
				class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm font-mono placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				placeholder="feedback-survey"
				bind:value={createSlug}
				oninput={() => (slugTouched = true)}
			/>
		</div>
		<div>
			<label for="form-create-desc" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.description_label', 'Description')}</label>
			<textarea
				id="form-create-desc"
				rows="3"
				class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				bind:value={createDescription}
			></textarea>
		</div>
		<div class="flex items-center gap-2">
			<input id="form-create-active" type="checkbox" bind:checked={createIsActive} class="rounded" />
			<label for="form-create-active" class="text-sm text-foreground-alt">{t('forms.active_label', 'Active')}</label>
		</div>
	</div>
	<div class="mt-6 flex justify-end gap-3">
		<Button variant="outline" onclick={() => (showCreateModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={createForm} disabled={savingCreate || !createTitle.trim() || !createSlug.trim()}>
			{savingCreate ? t('common.creating', 'Creating...') : t('common.create', 'Create Form')}
		</Button>
	</div>
</Modal>

<!-- Edit Modal -->
<Modal bind:open={showEditModal} title={t('forms.edit', 'Edit Form')} size="sm">
	{#if editTarget}
		<div class="space-y-4">
			<div>
				<label for="form-edit-title" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.title_label', 'Title')}</label>
				<input
					id="form-edit-title"
					type="text"
					class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					bind:value={editTitle}
				/>
			</div>
			<div>
				<label for="form-edit-desc" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.description_label', 'Description')}</label>
				<textarea
					id="form-edit-desc"
					rows="3"
					class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					bind:value={editDescription}
				></textarea>
			</div>
			<div class="flex items-center gap-2">
				<input id="form-edit-active" type="checkbox" bind:checked={editIsActive} class="rounded" />
				<label for="form-edit-active" class="text-sm text-foreground-alt">{t('forms.active_label', 'Active')}</label>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-3">
		<Button variant="outline" onclick={() => (showEditModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={saveEdit} disabled={savingEdit || !editTitle.trim()}>
			{savingEdit ? t('common.saving', 'Saving...') : t('common.save_changes', 'Save Changes')}
		</Button>
	</div>
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={showDeleteModal} title={t('forms.delete', 'Delete Form')} description={t('forms.delete_warning', 'This action cannot be undone. All submissions will be lost.')} size="sm">
	{#if deleteTarget}
		<div class="rounded-lg bg-danger-50 p-4">
			<p class="text-sm text-danger-700">
				{t('forms.delete_confirm', 'Are you sure you want to delete')} <strong>{deleteTarget.name}</strong>?
				{t('forms.delete_submissions_lost', 'This will permanently remove the form and all its submissions.')}
			</p>
		</div>
	{/if}
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => (showDeleteModal = false)}>{t('forms.cancel')}</Button>
		<Button variant="destructive" onclick={deleteForm} disabled={deleting}>
			{#if deleting}
				{t('common.deleting', 'Deleting...')}
			{:else}
				{t('forms.delete_button', 'Delete Form')}
			{/if}
		</Button>
	</div>
</Modal>

<!-- View Modal -->
<Modal bind:open={showViewModal} title={t('forms.details', 'Form Details')} size="sm">
	{#if viewTarget}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('forms.title')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{viewTarget.name}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('forms.slug', 'Slug')}</p>
				<p class="mt-1 text-sm font-mono text-foreground">{viewTarget.slug}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('forms.filter_space', 'Space')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{(viewTarget as any).space_id ? 'Space-specific' : t('forms.global', 'Global')}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('forms.status')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{viewTarget.status === 'active' ? t('forms.status.active', 'Active') : t('forms.status.inactive', 'Inactive')}</p>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end">
		<Button variant="outline" onclick={() => (showViewModal = false)}>{t('forms.cancel')}</Button>
	</div>
</Modal>

<!-- Action Cell -->
{#snippet FormActionsCell({ value, row }: { value: any; row: Form })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => goto(`/forms/${row.id}`)} title="Manage form">
			<ExternalLink class="h-3.5 w-3.5" />
		</Button>
		<Button variant="ghost" size="sm" onclick={() => openView(row)}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canManageForms}
			<Button variant="ghost" size="sm" onclick={() => openEdit(row)}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => confirmDelete(row)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
