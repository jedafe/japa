<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /forms — migrated to shared components (ActionMenu, FilterPanel,
	// Modal, bulkActionsList). Tabbed page (all/system/active/inactive);
	// keeps page-local Modals for create/edit (forms with slug auto-fill) and
	// delete confirmation. View detail opens in a Modal.
	import { t, locale } from '$lib/stores/i18n.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import type { ActionItem, BulkAction } from '$lib/components/ui/data-table';
	import { FileText, Plus, Eye, RefreshCw, Trash2, Pencil, ExternalLink, Settings } from 'lucide-svelte';
	import { goto } from '$app/navigation';
	import { formsAdminApi, spacesAdminApi } from '$lib/api';
	import { hasPermission } from '$lib/stores/permissions.svelte';

	type Form = {
		id: string;
		name: string;
		slug: string;
		space_id?: string;
		space_name?: string;
		description?: string;
		status: 'active' | 'draft';
		is_multi_step?: boolean;
		submit_label?: string;
		success_message?: string;
		is_system?: boolean;
		submission_fee_credits?: number;
		created_at: string;
	};

	let activeTab = $state('all');
	let forms = $state<Form[]>([]);
	let loading = $state(true);
	let loadError = $state('');
	let filterValue = $state<Record<string, string>>({});
	let selectedIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	// Space filter
	let availableSpaces = $state<Array<{ id: string; name: string }>>([]);

	// Delete confirmation Modal
	let showDeleteModal = $state(false);
	let deleteTarget = $state<Form | null>(null);
	let deleting = $state(false);

	// View Modal
	let showViewModal = $state(false);
	let viewTarget = $state<Form | null>(null);

	// Edit Modal
	let showEditModal = $state(false);
	let editTarget = $state<Form | null>(null);
	let editTitle = $state('');
	let editDescription = $state('');
	let editIsActive = $state(true);
	let editIsMultiStep = $state(false);
	let editSubmitLabel = $state('');
	let editSuccessMessage = $state('');
	let editSubmissionFeeCredits = $state(0);
	let savingEdit = $state(false);

	// Create Modal
	let showCreateModal = $state(false);
	let createTitle = $state('');
	let createSlug = $state('');
	let createDescription = $state('');
	let createIsActive = $state(true);
	let createIsMultiStep = $state(false);
	let createSubmitLabel = $state('');
	let createSuccessMessage = $state('');
	let createSubmissionFeeCredits = $state(0);
	let savingCreate = $state(false);
	let slugTouched = $state(false);

	const canManageForms = $derived(hasPermission('forms.create'));

	const tabs = [
		{ value: 'all', label: t('forms.tabAll', 'All Forms'), icon: FileText },
		{ value: 'system', label: t('forms.tabSystem', 'System'), icon: Settings },
		{ value: 'active', label: t('forms.tabActive', 'Active'), icon: Eye },
		{ value: 'inactive', label: t('forms.tabInactive', 'Inactive'), icon: FileText }
	];

	const columns = [
		{
			key: 'name',
			label: t('forms.columnForm', 'Form'),
			render: (_: any, row: any) => `
				<div>
					<a href="/forms/${row.id}" class="font-medium text-foreground hover:underline cursor-pointer" onclick="event.stopPropagation()">${row.name}</a>
					<p class="text-xs font-mono text-muted">${row.slug}</p>
					${row.is_system ? `<span class="mt-1 inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-semibold" style="border-color: var(--color-info); color: var(--color-info)">${t('common.system', 'System')}</span>` : ''}
				</div>
			`
		},
		{
			key: 'space_name',
			label: t('forms.columnSpace', 'Space'),
			render: (value: any, row: any) => `<span class="text-sm text-foreground-alt">${row.space_name || (row.space_id ? row.space_id.substring(0, 8) + '...' : t('forms.global', 'Global'))}</span>`
		},
		{
			key: 'is_system',
			label: t('forms.columnType', 'Type'),
			render: (v: boolean) => v
				? `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold bg-info/15 text-info border-info">${t('forms.type.system', 'System')}</span>`
				: `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold bg-surface text-foreground border">${t('forms.type.custom', 'Custom')}</span>`
		},
		{
			key: 'status',
			label: t('forms.columnStatus', 'Status'),
			render: (v: string) => {
				const isActive = v !== 'draft';
				const cls = isActive ? 'bg-success/15 text-success border-success' : 'bg-surface text-foreground border';
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${isActive ? t('forms.status.active', 'Active') : t('forms.status.inactive', 'Inactive')}</span>`;
			}
		},
		{
			key: 'created_at',
			label: t('forms.created', 'Created'),
			render: (v: string) => new Date(v).toLocaleDateString(locale.current, { year: 'numeric', month: 'short', day: 'numeric' })
		}
	];

	const rowActions: ActionItem[] = [
		{ label: t('forms.manage', 'Manage'), icon: ExternalLink, onclick: (row: Form) => goto(`/forms/${row.id}`) },
		{ label: t('common.view', 'View'), icon: Eye, onclick: (row) => openView(row) },
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row) => openEdit(row),
			show: () => canManageForms
		},
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row) => confirmDelete(row),
			// System forms are consumed by bespoke pages keyed on their slug —
			// the backend rejects deletion (403 system_form_protected), so the
			// menu shouldn't offer it in the first place.
			show: (row: Form) => canManageForms && !row.is_system
		}
	];

	const bulkActions: BulkAction[] = [
		{ label: t('forms.delete_selected', 'Delete Selected'), icon: Trash2, variant: 'destructive', onclick: bulkDelete }
	];

	let filteredForms = $derived.by(() => {
		let result = [...forms];
		if (activeTab === 'system') result = result.filter((f) => f.is_system === true);
		if (activeTab === 'active') result = result.filter((f) => f.status !== 'draft');
		if (activeTab === 'inactive') result = result.filter((f) => f.status === 'draft');
		if (filterValue.type === 'system') result = result.filter((f) => f.is_system === true);
		if (filterValue.type === 'custom') result = result.filter((f) => f.is_system !== true);
		return result;
	});

	const spaceFilterOptions = $derived.by(() => [
		{ value: '', label: t('forms.all_spaces', 'All Spaces') },
		...availableSpaces.map((s) => ({ value: s.id, label: s.name }))
	]);

	const typeFilterOptions = $derived([
		{ value: '', label: t('forms.all_types', 'All Types') },
		{ value: 'system', label: t('forms.type_system', 'System') },
		{ value: 'custom', label: t('forms.type_custom', 'Custom') }
	]);

	const filterDefs = $derived([
		{ key: 'space', label: t('forms.filter_space', 'Space'), options: spaceFilterOptions },
		{ key: 'type', label: t('forms.filter_type', 'Type'), options: typeFilterOptions }
	]);

	async function loadForms(spaceId?: string) {
		loading = true;
		loadError = '';
		try {
			const params = spaceId ? `space_id=${spaceId}` : undefined;
			const result = await formsAdminApi.list(params);
			if (result.ok) {
				const raw = result.data;
				forms = Array.isArray(raw) ? raw : (raw as any)?.data ?? [];
			} else {
				loadError = t('forms.load_error', 'Could not load forms — check the API connection and try Refresh.');
			}
		} catch {
			loadError = t('forms.load_error', 'Could not load forms — check the API connection and try Refresh.');
		} finally {
			loading = false;
		}
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
		editDescription = form.description ?? '';
		editIsActive = form.status !== 'draft';
		editIsMultiStep = form.is_multi_step ?? false;
		editSubmitLabel = form.submit_label ?? '';
		editSuccessMessage = form.success_message ?? '';
		editSubmissionFeeCredits = form.submission_fee_credits ?? 0;
		showEditModal = true;
	}

	async function saveEdit() {
		if (!editTarget || !editTitle.trim()) return;
		savingEdit = true;
		try {
			const result = await formsAdminApi.update(editTarget.id, {
				name: editTitle.trim(),
				description: editDescription.trim() || undefined,
				status: editIsActive ? 'published' : 'draft',
				is_multi_step: editIsMultiStep,
				submit_label: editSubmitLabel.trim() || undefined,
				success_message: editSuccessMessage.trim() || undefined,
				submission_fee_credits: editSubmissionFeeCredits
			});
			if (result.ok) {
				showEditModal = false;
				loadForms(filterValue.space || undefined);
			}
		} catch { /* API not available */ } finally {
			savingEdit = false;
		}
	}

	let pendingBulkIds = $state<Set<string>>(new Set());

	function bulkDelete(ids: Set<string>) {
		if (ids.size === 0 || bulkDeleting) return;
		// Skip system forms — the backend rejects them with 403; offering
		// them in a bulk delete would produce confusing partial failures.
		const deletable = new Set([...ids].filter((id) => forms.find((f) => f.id === id)?.is_system !== true));
		if (deletable.size === 0) return;
		// Confirm first, then refetch — the list is only patched after the
		// server confirms, matching the invalidate-on-confirm pattern.
		pendingBulkIds = deletable;
		deleteTarget = null;
		showDeleteModal = true;
	}

	async function deleteForm() {
		if (pendingBulkIds.size > 0) {
			bulkDeleting = true;
			try {
				await Promise.all([...pendingBulkIds].map((id) => formsAdminApi.delete(id)));
			} catch { /* API not available */ } finally {
				bulkDeleting = false;
			}
			pendingBulkIds = new Set();
			showDeleteModal = false;
			selectedIds = new Set();
			loadForms(filterValue.space || undefined);
			return;
		}
		if (!deleteTarget) return;
		deleting = true;
		try {
			await formsAdminApi.delete(deleteTarget.id);
			forms = forms.filter((f) => f.id !== deleteTarget!.id);
			showDeleteModal = false;
			deleteTarget = null;
		} catch { /* API not available */ } finally {
			deleting = false;
		}
	}

	function slugify(value: string): string {
		return value.toLowerCase().trim().replace(/[^a-z0-9\s-]/g, '').replace(/\s+/g, '-').replace(/-+/g, '-').replace(/^-|-$/g, '') || 'form';
	}

	function openCreate() {
		createTitle = '';
		createSlug = '';
		createDescription = '';
		createIsActive = true;
		createIsMultiStep = false;
		createSubmitLabel = '';
		createSuccessMessage = '';
		createSubmissionFeeCredits = 0;
		slugTouched = false;
		showCreateModal = true;
	}

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
				status: createIsActive ? 'published' : 'draft',
				is_multi_step: createIsMultiStep,
				submit_label: createSubmitLabel.trim() || undefined,
				success_message: createSuccessMessage.trim() || undefined,
				submission_fee_credits: createSubmissionFeeCredits
			});
			if (result.ok) {
				showCreateModal = false;
				loadForms(filterValue.space || undefined);
			}
		} catch { /* API not available */ } finally {
			savingCreate = false;
		}
	}

	// Refetch when space filter changes
	$effect(() => {
		void filterValue.space;
		loadForms(filterValue.space || undefined);
	});

	let initialized = false;
	$effect(() => {
		if (!initialized) {
			initialized = true;
			loadSpaces();
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
		} catch { /* ignore */ }
	}
</script>

<svelte:head>
	<title>{t('forms.title', 'Forms')} — {siteName()} Admin</title>
</svelte:head>

<PageHeader
	title={t('forms.title', 'Forms')}
	description={t('forms.description', 'Manage platform forms and collect user submissions')}
	actions={[
		...(canManageForms ? [{ label: t('forms.create', 'Create Form'), icon: Plus, onclick: openCreate }] : []),
		{ label: t('common.refresh', 'Refresh'), icon: RefreshCw, onclick: () => loadForms(filterValue.space || undefined), variant: 'outline' as const }
	]}
/>

<div class="space-y-6">
	<Tabs {tabs} bind:value={activeTab} tabParam="tab" tabPreserve={[]} />

	{#if loadError}
		<div class="rounded-lg border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive" role="alert">
			{loadError}
		</div>
	{/if}

	<Card>
		<CardContent class="p-0">
			<DataTable
				columns={columns}
				data={filteredForms}
				{loading}
				selectable
				bind:selectedIds
				pageSize={20}
				emptyMessage={t('forms.no_forms', 'No forms found')}
				filters={filterDefs}
				bind:filterValue
				actions={rowActions}
				bulkActionsList={bulkActions}
			/>
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
				<p class="text-2xl font-bold text-foreground">{forms.filter((f) => f.status !== 'draft').length}</p>
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
<Modal bind:open={showCreateModal} title={t('forms.create', 'Create Form')}>
	<div class="space-y-4">
		<div>
			<label for="form-create-title" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.title_label', 'Title')}</label>
			<input id="form-create-title" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" placeholder={t('forms.title_placeholder', 'e.g., Feedback Survey')} bind:value={createTitle} oninput={onTitleInput} />
		</div>
		<div>
			<label for="form-create-slug" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.slug_label', 'Slug')}</label>
			<input id="form-create-slug" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm font-mono placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" placeholder="feedback-survey" bind:value={createSlug} oninput={() => (slugTouched = true)} />
		</div>
		<div>
			<label for="form-create-desc" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.description_label', 'Description')}</label>
			<textarea id="form-create-desc" rows="3" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={createDescription}></textarea>
		</div>
		<div>
			<label for="form-create-submit-label" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.submit_label', 'Submit Button Label')}</label>
			<input id="form-create-submit-label" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" placeholder={t('forms.submit_label_placeholder', 'e.g., Submit')} bind:value={createSubmitLabel} />
		</div>
		<div>
			<label for="form-create-success-msg" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.success_message', 'Success Message')}</label>
			<textarea id="form-create-success-msg" rows="2" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" placeholder={t('forms.success_message_placeholder', 'e.g., Thank you for your submission!')} bind:value={createSuccessMessage}></textarea>
		</div>
		<div>
			<label for="form-create-fee-credits" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.fee_credits', 'Submission Fee (credits)')}</label>
			<input id="form-create-fee-credits" type="number" min="0" step="1" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={createSubmissionFeeCredits} />
			<p class="mt-1 text-xs text-muted">{t('forms.fee_credits_hint', '0 = free submission')}</p>
		</div>
		<div class="flex items-center gap-2">
			<input id="form-create-active" type="checkbox" bind:checked={createIsActive} class="rounded" />
			<label for="form-create-active" class="text-sm text-foreground-alt">{t('forms.active_label', 'Active')}</label>
		</div>
		<div class="flex items-center gap-2">
			<input id="form-create-multi-step" type="checkbox" bind:checked={createIsMultiStep} class="rounded" />
			<label for="form-create-multi-step" class="text-sm text-foreground-alt">{t('forms.multi_step_label', 'Multi-step form')}</label>
		</div>
		{#if createIsMultiStep}
			<p class="rounded-md border border-info/30 bg-info/10 px-3 py-2 text-xs text-info">
				{t('forms.multi_step_hint', 'After creating the form, add steps on the form detail page, then add fields to each step.')}
			</p>
		{/if}
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-3">
			<Button variant="outline" onclick={() => (showCreateModal = false)}>{t('common.cancel', 'Cancel')}</Button>
			<Button onclick={createForm} disabled={savingCreate || !createTitle.trim() || !createSlug.trim()}>
				{savingCreate ? t('common.creating', 'Creating...') : t('common.create', 'Create Form')}
			</Button>
		</div>
	{/snippet}
</Modal>

<!-- Edit Modal -->
<Modal bind:open={showEditModal} title={t('forms.edit', 'Edit Form')}>
	{#if editTarget}
		<div class="space-y-4">
			<div>
				<label for="form-edit-title" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.title_label', 'Title')}</label>
				<input id="form-edit-title" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={editTitle} />
			</div>
			<div>
				<label for="form-edit-desc" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.description_label', 'Description')}</label>
				<textarea id="form-edit-desc" rows="3" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={editDescription}></textarea>
			</div>
			<div>
				<label for="form-edit-submit-label" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.submit_label', 'Submit Button Label')}</label>
				<input id="form-edit-submit-label" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={editSubmitLabel} />
			</div>
			<div>
				<label for="form-edit-success-msg" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.success_message', 'Success Message')}</label>
				<textarea id="form-edit-success-msg" rows="2" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={editSuccessMessage}></textarea>
			</div>
			<div>
				<label for="form-edit-fee-credits" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.fee_credits', 'Submission Fee (credits)')}</label>
				<input id="form-edit-fee-credits" type="number" min="0" step="1" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={editSubmissionFeeCredits} />
				<p class="mt-1 text-xs text-muted">{t('forms.fee_credits_hint', '0 = free submission')}</p>
			</div>
			<div class="flex items-center gap-2">
				<input id="form-edit-active" type="checkbox" bind:checked={editIsActive} class="rounded" />
				<label for="form-edit-active" class="text-sm text-foreground-alt">{t('forms.active_label', 'Active')}</label>
			</div>
			<div class="flex items-center gap-2">
				<input id="form-edit-multi-step" type="checkbox" bind:checked={editIsMultiStep} class="rounded" />
				<label for="form-edit-multi-step" class="text-sm text-foreground-alt">{t('forms.multi_step_label', 'Multi-step form')}</label>
			</div>
		</div>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end gap-3">
			<Button variant="outline" onclick={() => (showEditModal = false)}>{t('common.cancel', 'Cancel')}</Button>
			<Button onclick={saveEdit} disabled={savingEdit || !editTitle.trim()}>
				{savingEdit ? t('common.saving', 'Saving...') : t('common.save_changes', 'Save Changes')}
			</Button>
		</div>
	{/snippet}
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
			{deleting ? t('common.deleting', 'Deleting...') : t('forms.delete_button', 'Delete Form')}
		</Button>
	</div>
</Modal>

<!-- View Modal -->
<Modal bind:open={showViewModal} title={t('forms.details', 'Form Details')}>
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
	{#snippet footer()}
		<div class="flex justify-end">
			<Button variant="outline" onclick={() => (showViewModal = false)}>{t('forms.cancel')}</Button>
		</div>
	{/snippet}
</Modal>
