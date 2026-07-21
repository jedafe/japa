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
	import { FileText, Plus, Eye, BarChart3, RefreshCw, Trash2, Pencil } from "lucide-svelte";
	import { api } from "$lib/api";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	type Form = {
		id: string;
		title: string;
		slug: string;
		space_id?: string;
		space_name?: string;
		description?: string;
		submission_count: number;
		is_active: boolean;
		fields_count?: number;
		created_at: string;
		updated_at?: string;
	};

	let { data } = $props();

	let activeTab = $state("all");
	let forms = $state<Form[]>([]);
	let loading = $state(true);
	let filters = $state<Record<string, string>>({});
	let selectedIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

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
		{ value: "all", label: t('forms.tab_all', 'All Forms'), icon: FileText },
		{ value: "active", label: t('forms.tab_active', 'Active'), icon: Eye },
		{ value: "inactive", label: t('forms.tab_inactive', 'Inactive'), icon: FileText }
	];

	const columns = [
		{
			key: "title",
			label: "Form",
			sortable: true,
			render: (_: any, row: any) => `
				<div>
					<p class="font-medium text-foreground">${row.title}</p>
					<p class="text-xs font-mono text-muted">${row.slug}</p>
				</div>
			`
		},
		{
			key: "space_name",
			label: "Space",
			sortable: true,
			render: (value: string) => `<span class="text-sm text-foreground-alt">${value || t('forms.global', 'Global')}</span>`
		},
		{
			key: "submission_count",
			label: "Submissions",
			sortable: true,
			render: (value: number) => `<span class="font-medium text-foreground">${(value ?? 0).toLocaleString()}</span>`
		},
		{
			key: "fields_count",
			label: t('forms.fields_count', 'Fields'),
			sortable: true,
			render: (value: number) => `<span class="text-sm text-foreground-alt">${value ?? "—"}</span>`
		},
		{
			key: "is_active",
			label: "Status",
			sortable: true,
			render: (value: boolean) => {
				const variant = value ? "success" : "secondary";
				const classes = value ? "bg-success/15 text-success border-success" : "bg-surface text-foreground border";
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes}">${value ? t('forms.status.active', 'Active') : t('forms.status.inactive', 'Inactive')}</span>`;
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
			label: "",
			class: "w-32",
			component: FormActionsCell
		}
	];

	let filteredForms = $derived.by(() => {
		let result = [...forms];
		if (activeTab === "active") result = result.filter((f) => f.is_active);
		if (activeTab === "inactive") result = result.filter((f) => !f.is_active);
		return result;
	});

	async function loadForms() {
		loading = true;
		try {
			const result = await api<Form[]>("/forms");
			if (result.ok) {
				forms = result.data ?? [];
			}
		} catch {
			// API not available
		} finally {
			loading = false;
		}
	}

	function handleFilter(values: Record<string, string>) {
		filters = values;
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
		editTitle = form.title;
		editDescription = form.description ?? "";
		editIsActive = form.is_active;
		showEditModal = true;
	}

	async function saveEdit() {
		if (!editTarget || !editTitle.trim()) return;
		savingEdit = true;
		try {
			const result = await api(`/forms/${editTarget.id}`, {
				method: "PATCH",
				body: { name: editTitle.trim(), description: editDescription.trim() || undefined, status: editIsActive ? "active" : "draft" }
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
			await api(`/forms/${deleteTarget.id}`, { method: "DELETE" });
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
			await Promise.all(ids.map((id) => api(`/forms/${id}`, { method: "DELETE" })));
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
			const result = await api("/forms", {
				method: "POST",
				body: {
					name: createTitle.trim(),
					slug: createSlug.trim(),
					description: createDescription.trim() || undefined,
					status: createIsActive ? "active" : "draft"
				}
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
			loadForms();
		}
	});
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
					{ value: "global", label: t('forms.global', 'Global') },
					{ value: "space", label: t('forms.space_specific', 'Space-specific') }
				]
			}
		]}
		onsearch={handleFilter}
		onclear={() => { filters = {}; }}
	/>

	<Card>
		<CardContent class="p-0">
			{#if loading}
				<p class="py-8 text-center text-sm text-muted">{t('forms.loading_forms')}</p>
			{:else}
				<DataTable
					columns={columns}
					data={filteredForms}
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
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
		<Card>
			<CardContent class="p-4">
				<p class="text-sm text-muted">{t('forms.total_forms')}</p>
				<p class="text-2xl font-bold text-foreground">{forms.length}</p>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-4">
				<p class="text-sm text-muted">{t('forms.active_forms')}</p>
				<p class="text-2xl font-bold text-foreground">{forms.filter((f) => f.is_active).length}</p>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-4">
				<p class="text-sm text-muted">{t('forms.total_submissions')}</p>
				<p class="text-2xl font-bold text-foreground">{forms.reduce((sum, f) => sum + (f.submission_count ?? 0), 0).toLocaleString()}</p>
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
				{t('forms.delete_confirm', 'Are you sure you want to delete')} <strong>{deleteTarget.title}</strong>?
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
				<p class="mt-1 text-sm font-medium text-foreground">{viewTarget.title}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('forms.slug', 'Slug')}</p>
				<p class="mt-1 text-sm font-mono text-foreground">{viewTarget.slug}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('forms.filter_space', 'Space')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{viewTarget.space_name || t('forms.global', 'Global')}</p>
			</div>
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('forms.submissions')}</p>
					<p class="mt-1 text-sm font-medium text-foreground">{(viewTarget.submission_count ?? 0).toLocaleString()}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('forms.fields')}</p>
					<p class="mt-1 text-sm font-medium text-foreground">{viewTarget.fields_count ?? "—"}</p>
				</div>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('forms.status')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{viewTarget.is_active ? t('forms.status.active', 'Active') : t('forms.status.inactive', 'Inactive')}</p>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end">
		<Button variant="outline" onclick={() => (showViewModal = false)}>{t('forms.cancel')}</Button>
	</div>
</Modal>

<!-- Action Cell -->
{#snippet FormActionsCell(row: Form)}
	<div class="flex items-center gap-1">
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
