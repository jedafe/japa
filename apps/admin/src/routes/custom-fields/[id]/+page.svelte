<script lang="ts">
	import { page } from '$app/state';
	import { api, fieldGroupsAdminApiExtended, userManagementApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Plus, Pencil, Trash2, ArrowLeft, GripVertical } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import { DataTable } from '$lib/components/ui/data-table';
	import type { ActionItem, Column } from '$lib/components/ui/data-table';

	type FieldOption = { value: string; label: string };

	type CustomField = {
		id: string;
		label: string;
		key: string;
		field_type: string;
		is_required: boolean;
		is_system: boolean;
		show_in_frontend: boolean;
		filterable: boolean;
		sortable: boolean;
		searchable: boolean;
		position: number;
		options: FieldOption[] | null;
	};

	type FieldGroup = {
		id: string;
		name: string;
		entity_type: string;
		field_count?: number;
	};

	const fieldTypeId = $derived(page.params.id ?? '');

	// --- Group + fields state ---
	let group = $state<FieldGroup | null>(null);
	let fields = $state<CustomField[]>([]);
	let loading = $state(true);
	let error = $state('');

	// --- Entity type labels ---
	let entityTypeOptions = $state<Array<{ value: string; label: string }>>([]);

	const canManage = $derived(hasPermission('custom_fields.field.create'));
	const canEdit = $derived(hasPermission('custom_fields.field.edit'));
	const canDelete = $derived(hasPermission('custom_fields.field.delete'));

	// --- Add/Edit field modal ---
	let slideOverOpen = $state(false);
	let editingField = $state<CustomField | null>(null);
	let saving = $state(false);
	let fieldError = $state('');

	// Field form state
	let label = $state('');
	let key = $state('');
	let fieldType = $state('text');
	let isRequired = $state(false);
	let showInFrontend = $state(true);
	let filterable = $state(false);
	let sortable = $state(false);
	let searchable = $state(false);
	let options = $state<FieldOption[]>([]);
	let newOptionValue = $state('');
	let newOptionLabel = $state('');

	// --- Delete confirm ---
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<CustomField | null>(null);

	// All supported field types (must match backend validFieldTypes)
	const fieldTypes: Array<{ value: string; label: string }> = [
		{ value: 'text', label: 'Text' },
		{ value: 'textarea', label: 'Textarea' },
		{ value: 'rich_text', label: 'Rich Text' },
		{ value: 'number', label: 'Number' },
		{ value: 'currency', label: 'Currency' },
		{ value: 'percentage', label: 'Percentage' },
		{ value: 'boolean', label: 'Boolean (Toggle)' },
		{ value: 'date', label: 'Date' },
		{ value: 'datetime', label: 'Date & Time' },
		{ value: 'time', label: 'Time' },
		{ value: 'duration', label: 'Duration' },
		{ value: 'select', label: 'Select (Single)' },
		{ value: 'multiselect', label: 'Multi-Select' },
		{ value: 'url', label: 'URL' },
		{ value: 'email', label: 'Email' },
		{ value: 'phone', label: 'Phone' },
		{ value: 'color', label: 'Color Picker' },
		{ value: 'rating', label: 'Rating (1-5)' },
		{ value: 'image_single', label: 'Image (Single Upload)' },
		{ value: 'image_multi', label: 'Images (Multi Upload)' },
		{ value: 'media', label: 'Media (File Upload)' },
		{ value: 'platform_embed', label: 'Platform Embed' },
		{ value: 'reference', label: 'Reference (Link to Entity)' },
		{ value: 'repeater', label: 'Repeater (Add Multiple Items)' },
		{ value: 'group', label: 'Group (Visual Container)' },
	];

	const fieldTypeLabel = (ft: string) => fieldTypes.find((f) => f.value === ft)?.label ?? ft;

	const needsOptions = $derived(fieldType === 'select' || fieldType === 'multiselect');

	const columns: Column[] = [
		{
			key: 'label',
			label: t('fieldGroups.label'),
			sortable: true,
			render: (v: string) => `<span class="font-medium" style="color: var(--color-foreground)">${v}</span>`
		},
		{
			key: 'key',
			label: t('fieldGroups.key'),
			sortable: true,
			render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>`
		},
		{
			key: 'field_type',
			label: t('fieldGroups.field_type'),
			sortable: true,
			render: (v: string) => `<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium" style="background-color: var(--color-muted-muted); color: var(--color-muted)">${fieldTypeLabel(v)}</span>`
		},
		{
			key: 'is_required',
			label: t('fieldGroups.required'),
			render: (v: boolean) => v ? `<span class="text-xs font-medium" style="color: var(--color-destructive)">${t('common.yes')}</span>` : `<span class="text-xs" style="color: var(--color-muted)">—</span>`
		},
		{
			key: 'show_in_frontend',
			label: t('fieldGroups.visible'),
			render: (v: boolean) => v ? `<span class="text-xs" style="color: var(--color-muted)">${t('common.yes')}</span>` : `<span class="text-xs" style="color: var(--color-muted)">—</span>`
		},
		{
			key: 'is_system',
			label: t('fieldGroups.system'),
			render: (v: boolean) => v ? `<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium" style="background-color: var(--color-primary-muted); color: var(--color-primary)">${t('fieldGroups.system')}</span>` : `<span class="text-xs" style="color: var(--color-muted)">—</span>`
		}
	];

	const rowActions: ActionItem[] = [
		{ label: t('common.edit'), icon: Pencil, onclick: (row: CustomField) => openEditField(row), show: () => canEdit },
		{ label: t('common.delete'), icon: Trash2, variant: 'destructive', onclick: (row: CustomField) => confirmDeleteField(row), show: (row: CustomField) => canDelete && !row.is_system }
	];

	async function load() {
		loading = true;
		error = '';
		try {
			// Load group info + fields in a single round trip via the
			// admin detail endpoint, which returns { group, fields }.
			const groupResult = await api<{ group: FieldGroup; fields: CustomField[] }>(`/admin/field-groups/${fieldTypeId}`);
			if (groupResult.ok && groupResult.data) {
				group = groupResult.data.group;
				fields = (Array.isArray(groupResult.data.fields) ? groupResult.data.fields : []) as CustomField[];
			} else if (!groupResult.ok) {
				error = groupResult.error || t('common.error');
			}

			// Load entity type labels (for display)
			if (entityTypeOptions.length === 0) {
				const typesResult = await userManagementApi.listCustomFieldEntityTypes();
				if (typesResult.ok && Array.isArray(typesResult.data)) {
					entityTypeOptions = typesResult.data;
				}
			}
		} catch {
			error = t('common.error');
		} finally {
			loading = false;
		}
	}

	function entityLabel(value: string): string {
		return entityTypeOptions.find((e) => e.value === value)?.label ?? value;
	}

	function openAddField() {
		editingField = null;
		label = '';
		key = '';
		fieldType = 'text';
		isRequired = false;
		showInFrontend = true;
		filterable = false;
		sortable = false;
		searchable = false;
		options = [];
		newOptionValue = '';
		newOptionLabel = '';
		fieldError = '';
		slideOverOpen = true;
	}

	function openEditField(f: CustomField) {
		editingField = f;
		label = f.label;
		key = f.key;
		fieldType = f.field_type;
		isRequired = f.is_required;
		showInFrontend = f.show_in_frontend;
		filterable = f.filterable;
		sortable = f.sortable;
		searchable = f.searchable;
		options = f.options ?? [];
		newOptionValue = '';
		newOptionLabel = '';
		fieldError = '';
		slideOverOpen = true;
	}

	function autoKey() {
		if (!editingField) {
			key = label
				.toLowerCase()
				.replace(/[^a-z0-9]+/g, '_')
				.replace(/^_|_$/g, '');
		}
	}

	function addOption() {
		const v = newOptionValue.trim();
		const l = newOptionLabel.trim();
		if (v && l && !options.some((o) => o.value === v)) {
			options = [...options, { value: v, label: l }];
			newOptionValue = '';
			newOptionLabel = '';
		}
	}

	function removeOption(opt: FieldOption) {
		options = options.filter((o) => o !== opt);
	}

	function handleOptionKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			e.preventDefault();
			addOption();
		}
	}

	async function saveField() {
		if (!label.trim() || !key.trim()) {
			fieldError = 'Label and key are required';
			return;
		}
		if (needsOptions && options.length === 0) {
			fieldError = 'Select and multiselect fields require at least one option';
			return;
		}

		saving = true;
		fieldError = '';
		try {
			const body: Record<string, unknown> = {
				label: label.trim(),
				key: key.trim(),
				field_type: fieldType,
				is_required: isRequired,
				show_in_frontend: showInFrontend,
				filterable,
				sortable,
				searchable,
				position: editingField?.position ?? fields.length,
			};
			if (needsOptions) {
				body.options = options;
			}

			if (editingField) {
				const result = await fieldGroupsAdminApiExtended.updateField(fieldTypeId, editingField.id, body);
				if (!result.ok) {
					fieldError = result.error;
					saving = false;
					return;
				}
			} else {
				const result = await fieldGroupsAdminApiExtended.addField(fieldTypeId, body);
				if (!result.ok) {
					fieldError = result.error;
					saving = false;
					return;
				}
			}
			slideOverOpen = false;
			await load();
		} catch {
			fieldError = t('common.error');
		} finally {
			saving = false;
		}
	}

	function confirmDeleteField(f: CustomField) {
		deleteTarget = f;
		showDeleteConfirm = true;
	}

	async function executeDeleteField() {
		if (!deleteTarget) return;
		showDeleteConfirm = false;
		try {
			await fieldGroupsAdminApiExtended.deleteField(fieldTypeId, deleteTarget.id);
			deleteTarget = null;
			await load();
		} catch {
			error = t('common.error');
		}
	}

	load();
</script>

<svelte:head>
	<title>{group?.name ?? 'Field Group'} — JapaConnect Admin</title>
</svelte:head>

<div class="space-y-6">
	<!-- Back link -->
	<a href="/custom-fields" class="inline-flex items-center gap-1.5 text-sm hover:opacity-70" style="color: var(--color-muted)">
		<ArrowLeft class="h-4 w-4" />
		{t('fieldGroups.title', 'Custom Fields')}
	</a>

	<!-- Header -->
	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">
				{group?.name ?? 'Loading…'}
			</h1>
			<p class="mt-1 text-sm" style="color: var(--color-muted)">
				{#if group}
					<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium" style="background-color: var(--color-primary-muted); color: var(--color-primary)">
						{entityLabel(group.entity_type)}
					</span>
					<span class="ml-2">{fields.length} field{fields.length === 1 ? '' : 's'}</span>
				{/if}
			</p>
		</div>
		{#if canManage}
			<button
				onclick={openAddField}
				class="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white"
				style="background-color: var(--color-primary)"
			>
				<Plus class="h-4 w-4" />
				{t('fieldGroups.add_field')}
			</button>
		{/if}
	</div>

	{#if error}
		<div class="rounded-lg p-3 text-sm" style="background-color: var(--color-destructive-muted); color: var(--color-destructive)">
			{error}
		</div>
	{/if}

	{#if loading}
		<p class="text-sm" style="color: var(--color-muted)">{t('common.loading')}…</p>
	{:else if fields.length === 0}
		<div class="rounded-lg border p-8 text-center" style="border-color: var(--color-border)">
			<p class="text-sm" style="color: var(--color-muted)">
				{t('fieldGroups.no_fields_yet')}
			</p>
		</div>
	{:else}
		<!-- Fields table -->
		<DataTable {columns} data={fields} actions={rowActions} pageSize={50} emptyMessage={t('fieldGroups.no_fields_yet')} />
	{/if}
</div>

<!-- Add/Edit Field SlideOver -->
<SlideOver bind:open={slideOverOpen} title={editingField ? `${t('fieldGroups.edit_field')}: ${editingField.label}` : t('fieldGroups.add_field')}>
	<div class="space-y-4">
		<!-- Label -->
		<div>
			<label for="fld-label" class="mb-1 block text-sm font-medium">{t('fieldGroups.label')}</label>
			<input
				id="fld-label"
				type="text"
				bind:value={label}
				onblur={autoKey}
				placeholder={t('fieldGroups.label_placeholder')}
				required
				class="w-full rounded-lg border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
			/>
		</div>

		<!-- Key -->
		<div>
			<label for="fld-key" class="mb-1 block text-sm font-medium">{t('fieldGroups.key')}</label>
			<input
				id="fld-key"
				type="text"
				bind:value={key}
				placeholder={t('fieldGroups.key_placeholder')}
				required
				disabled={!!editingField}
				class="w-full rounded-lg border px-3 py-2 font-mono text-sm disabled:opacity-50"
				style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
			/>
			<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('fieldGroups.key_help')} {editingField ? t('fieldGroups.key_immutable') : t('fieldGroups.key_auto')}</p>
		</div>

		<!-- Field Type -->
		<div>
			<label for="fld-type" class="mb-1 block text-sm font-medium">{t('fieldGroups.field_type')}</label>
			<select
				id="fld-type"
				bind:value={fieldType}
				disabled={!!editingField}
				class="w-full rounded-lg border px-3 py-2 text-sm disabled:opacity-50"
				style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
			>
				{#each fieldTypes as ft (ft)}
					<option value={ft.value}>{ft.label}</option>
				{/each}
			</select>
			{#if editingField}
				<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('fieldGroups.type_immutable')}</p>
			{/if}
		</div>

		<!-- Options for select / multiselect -->
		{#if needsOptions}
			<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
				<span class="mb-2 block text-sm font-medium">{t('fieldGroups.options')}</span>
				<div class="flex gap-2">
					<input
						type="text"
						class="w-32 rounded-md border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
						placeholder={t('fieldGroups.option_value')}
						bind:value={newOptionValue}
						onkeydown={handleOptionKeydown}
					/>
					<input
						type="text"
						class="flex-1 rounded-md border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
						placeholder={t('fieldGroups.option_label')}
						bind:value={newOptionLabel}
						onkeydown={handleOptionKeydown}
					/>
					<button
						type="button"
						class="rounded-md border px-3 py-2 text-sm font-medium hover:opacity-70"
						style="border-color: var(--color-border)"
						onclick={addOption}
					>
						Add
					</button>
				</div>
				{#if options.length > 0}
					<div class="mt-2 flex flex-wrap gap-2">
						{#each options as opt (opt)}
							<span class="inline-flex items-center gap-1 rounded-full px-3 py-1 text-xs" style="background-color: var(--color-muted-muted)">
								<span style="color: var(--color-muted)">{opt.value}</span>: {opt.label}
								<button type="button" class="ml-0.5 opacity-50 hover:opacity-100" onclick={() => removeOption(opt)}>&times;</button>
							</span>
						{/each}
					</div>
				{/if}
			</div>
		{/if}

		<!-- Checkboxes -->
		<div class="flex flex-wrap gap-4">
			<label class="flex items-center gap-2 text-sm">
				<input type="checkbox" bind:checked={isRequired} class="rounded" />
				Required
			</label>
			<label class="flex items-center gap-2 text-sm">
				<input type="checkbox" bind:checked={showInFrontend} class="rounded" />
				Show in frontend
			</label>
			<label class="flex items-center gap-2 text-sm">
				<input type="checkbox" bind:checked={filterable} class="rounded" />
				Filterable
			</label>
			<label class="flex items-center gap-2 text-sm">
				<input type="checkbox" bind:checked={sortable} class="rounded" />
				Sortable
			</label>
			<label class="flex items-center gap-2 text-sm">
				<input type="checkbox" bind:checked={searchable} class="rounded" />
				Searchable
			</label>
		</div>

		{#if fieldError}
			<p class="text-sm" style="color: var(--color-destructive)">{fieldError}</p>
		{/if}
	</div>
	{#snippet footer()}
		<div class="mt-6 flex justify-end gap-2">
			<button class="rounded-md px-4 py-2 text-sm font-medium transition-colors" style="border: 1px solid var(--color-border)" onclick={() => (slideOverOpen = false)}>{t('common.cancel')}</button>
			<button class="rounded-md px-4 py-2 text-sm font-medium transition-colors" style="background: var(--color-primary); color: var(--color-primary-foreground)" onclick={saveField} disabled={saving}>{saving ? t('common.saving') : t('common.save')}</button>
		</div>
	{/snippet}
</SlideOver>

<!-- Delete Field Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t('fieldGroups.delete_field')} confirmLabel={t('common.delete')} onConfirm={executeDeleteField}>
	<p class="text-sm" style="color: var(--color-muted)">
		{t('common.delete')} <strong>{deleteTarget?.label}</strong>? {t('fieldGroups.delete_field_confirm')}
	</p>
</Modal>
