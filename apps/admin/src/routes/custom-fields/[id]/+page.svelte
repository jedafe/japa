<script lang="ts">
	import { page } from '$app/state';
	import { api, fieldGroupsAdminApiExtended, userManagementApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Plus, Pencil, Trash2, ArrowLeft, GripVertical } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';

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
	let modalOpen = $state(false);
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
	];

	const fieldTypeLabel = (ft: string) => fieldTypes.find((f) => f.value === ft)?.label ?? ft;

	const needsOptions = $derived(fieldType === 'select' || fieldType === 'multiselect');

	async function load() {
		loading = true;
		error = '';
		try {
			// Load group info
			const groupResult = await api<FieldGroup>(`/admin/field-groups/${fieldTypeId}`);
			if (groupResult.ok) {
				group = groupResult.data;
			}

			// Load fields in the group
			const fieldsResult = await api<CustomField[]>(`/admin/field-groups/${fieldTypeId}/fields`);
			if (fieldsResult.ok) {
				fields = (Array.isArray(fieldsResult.data) ? fieldsResult.data : []) as CustomField[];
			} else {
				error = fieldsResult.error;
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
		modalOpen = true;
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
		modalOpen = true;
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
			modalOpen = false;
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
				Add Field
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
				No fields in this group yet. Click "Add Field" to create one.
			</p>
		</div>
	{:else}
		<!-- Fields table -->
		<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
			<table class="w-full text-sm">
				<thead style="background-color: var(--color-muted-muted)">
					<tr>
						<th class="w-8 px-3 py-3"></th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted)">Label</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted)">Key</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted)">Type</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted)">Required</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted)">Visible</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted)">System</th>
						<th class="w-20 px-4 py-3 text-left font-medium" style="color: var(--color-muted)">{t('common.actions')}</th>
					</tr>
				</thead>
				<tbody>
					{#each fields as f (f.id)}
						<tr class="border-t" style="border-color: var(--color-border)">
							<td class="px-3 py-3 text-center" style="color: var(--color-muted)">
								<GripVertical class="h-4 w-4 inline" />
							</td>
							<td class="px-4 py-3 font-medium" style="color: var(--color-foreground)">{f.label}</td>
							<td class="px-4 py-3 font-mono text-xs" style="color: var(--color-muted)">{f.key}</td>
							<td class="px-4 py-3">
								<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium" style="background-color: var(--color-muted-muted); color: var(--color-muted)">
									{fieldTypeLabel(f.field_type)}
								</span>
							</td>
							<td class="px-4 py-3">
								{#if f.is_required}
									<span class="text-xs font-medium" style="color: var(--color-destructive)">Yes</span>
								{:else}
									<span class="text-xs" style="color: var(--color-muted)">—</span>
								{/if}
							</td>
							<td class="px-4 py-3">
								{#if f.show_in_frontend}
									<span class="text-xs" style="color: var(--color-muted)">Yes</span>
								{:else}
									<span class="text-xs" style="color: var(--color-muted)">—</span>
								{/if}
							</td>
							<td class="px-4 py-3">
								{#if f.is_system}
									<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium" style="background-color: var(--color-primary-muted); color: var(--color-primary)">System</span>
								{:else}
									<span class="text-xs" style="color: var(--color-muted)">—</span>
								{/if}
							</td>
							<td class="px-4 py-3">
								<div class="flex items-center gap-1">
									{#if canEdit}
										<button onclick={() => openEditField(f)} class="p-1 rounded hover:opacity-70" title={t('common.edit')}>
											<Pencil class="h-4 w-4" style="color: var(--color-muted)" />
										</button>
									{/if}
									{#if canDelete && !f.is_system}
										<button onclick={() => confirmDeleteField(f)} class="p-1 rounded hover:opacity-70" title={t('common.delete')}>
											<Trash2 class="h-4 w-4" style="color: var(--color-destructive)" />
										</button>
									{/if}
								</div>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<!-- Add/Edit Field Modal -->
<Modal bind:open={modalOpen} title={editingField ? `Edit Field: ${editingField.label}` : 'Add Field'} confirmLabel={saving ? t('common.saving') : t('common.save')} onConfirm={saveField}>
	<div class="space-y-4">
		<!-- Label -->
		<div>
			<label for="fld-label" class="mb-1 block text-sm font-medium">Label</label>
			<input
				id="fld-label"
				type="text"
				bind:value={label}
				onblur={autoKey}
				placeholder="e.g. Phone Number, Bio, Profile Photo"
				required
				class="w-full rounded-lg border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
			/>
		</div>

		<!-- Key -->
		<div>
			<label for="fld-key" class="mb-1 block text-sm font-medium">Key</label>
			<input
				id="fld-key"
				type="text"
				bind:value={key}
				placeholder="e.g. phone_number, bio, profile_photo"
				required
				disabled={!!editingField}
				class="w-full rounded-lg border px-3 py-2 font-mono text-sm disabled:opacity-50"
				style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
			/>
			<p class="mt-1 text-xs" style="color: var(--color-muted)">Machine-readable identifier (snake_case). {editingField ? 'Cannot be changed after creation.' : 'Auto-generated from label.'}</p>
		</div>

		<!-- Field Type -->
		<div>
			<label for="fld-type" class="mb-1 block text-sm font-medium">Field Type</label>
			<select
				id="fld-type"
				bind:value={fieldType}
				disabled={!!editingField}
				class="w-full rounded-lg border px-3 py-2 text-sm disabled:opacity-50"
				style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
			>
				{#each fieldTypes as ft}
					<option value={ft.value}>{ft.label}</option>
				{/each}
			</select>
			{#if editingField}
				<p class="mt-1 text-xs" style="color: var(--color-muted)">Field type cannot be changed after creation.</p>
			{/if}
		</div>

		<!-- Options for select / multiselect -->
		{#if needsOptions}
			<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
				<span class="mb-2 block text-sm font-medium">Options</span>
				<div class="flex gap-2">
					<input
						type="text"
						class="w-32 rounded-md border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
						placeholder="Value"
						bind:value={newOptionValue}
						onkeydown={handleOptionKeydown}
					/>
					<input
						type="text"
						class="flex-1 rounded-md border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
						placeholder="Label (displayed to users)"
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
						{#each options as opt}
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
</Modal>

<!-- Delete Field Confirmation -->
<Modal bind:open={showDeleteConfirm} title="Delete Field" confirmLabel={t('common.delete')} onConfirm={executeDeleteField}>
	<p class="text-sm" style="color: var(--color-muted)">
		Delete <strong>{deleteTarget?.label}</strong>? All stored values for this field will be lost. This action cannot be undone.
	</p>
</Modal>
