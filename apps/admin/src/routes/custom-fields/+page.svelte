<script lang="ts">
	// /custom-fields — migrated to the EntityPage scaffold.
	//
	// The field-group list + group CRUD is driven declaratively by EntityPage.
	// The field-editor sub-flow (add/edit/delete/reorder fields within a group)
	// is kept as a page-local <SlideOver> triggered by a custom "Manage Fields"
	// row action — mirrors the tags page's moderators SlideOver.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { TableSkeleton } from '$lib/components/ui/skeleton';
	import { api, fieldGroupsAdminApiExtended, userManagementApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Trash2, List, Pencil, Plus, GripVertical, ArrowUp, ArrowDown } from 'lucide-svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	type FieldGroup = {
		id: string;
		name: string;
		slug?: string;
		entity_type: string;
		field_count: number;
		is_system: boolean;
		description?: string;
		created_at: string;
	};

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

	// Entity types are fetched from the backend so the form select + filter
	// options always match the backend's supported entity classes (no hardcoded
	// option lists in the frontend).
	let entityTypeOptions = $state<Array<{ value: string; label: string }>>([]);

	$effect(() => {
		void loadEntityTypeOptions();
	});

	async function loadEntityTypeOptions() {
		try {
			const result = await userManagementApi.listCustomFieldEntityTypes();
			if (result.ok && Array.isArray(result.data)) {
				entityTypeOptions = result.data as Array<{ value: string; label: string }>;
			}
		} catch {
			entityTypeOptions = [];
		}
	}

	function entityLabel(value: string): string {
		return entityTypeOptions.find((o) => o.value === value)?.label ?? value;
	}

	// Bulk delete is fully scaffold-driven: EntityPage's built-in "Delete
	// Selected" runs config.remove per row and, when rows come back inUse and
	// config.forceRemove is declared, offers a second force-delete confirm
	// (DELETE ?force=true) — no page-local bulk handler needed.
	let entityController = $state<EntityPageController | undefined>(undefined);

	// ── Entity type autocomplete helpers (filter pre-loaded options client-side) ──
	function fetchEntityTypeOptions(q: string): Promise<{ id: string; label: string }[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? entityTypeOptions
			: entityTypeOptions.filter((o) => o.label.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((o) => ({ id: o.value, label: o.label })));
	}

	function fetchEntityTypeOptionsInitial(): Promise<{ id: string; label: string }[]> {
		return Promise.resolve(entityTypeOptions.map((o) => ({ id: o.value, label: o.label })));
	}

	function getEntityTypeSelectedLabel(id: string): string {
		return entityTypeOptions.find((o) => o.value === id)?.label ?? id;
	}

	// `fields` drives EntityForm rendering + edit-form seeding. entity_type is an
	// autocomplete whose options are loaded dynamically, so the array is rebuilt when
	// entityTypeOptions updates (mirrors the /users dynamic-filter pattern).
	const fields = $derived<FieldDef[]>([
		{ key: 'name', label: t('fieldGroups.name', 'Name'), type: 'text', required: true },
		{ key: 'slug', label: t('fieldGroups.slug', 'Slug'), type: 'text' },
		{
			key: 'entity_type',
			label: t('fieldGroups.entity_type', 'Entity Type'),
			type: 'autocomplete',
			placeholder: t('fieldGroups.entity_type_placeholder', 'Search entity type…'),
			emptyMessage: t('common.no_results', 'No results found'),
			fetcher: fetchEntityTypeOptions,
			initialFetcher: fetchEntityTypeOptionsInitial,
			getSelectedLabel: getEntityTypeSelectedLabel,
			required: true
		},
		{ key: 'description', label: t('fieldGroups.description', 'Description'), type: 'textarea' }
	]);

	const config = $derived<EntityPageConfig>({
		title: t('fieldGroups.title', 'Field Groups'),
		description: t('fieldGroups.description', 'Manage custom field groups'),
		createLabel: t('fieldGroups.create', 'Create Group'),
		createPermission: 'custom_fields.group.create',
		editPermission: 'custom_fields.group.edit',
		deletePermission: 'custom_fields.group.delete',
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('fieldGroups.name', 'Name'), sortable: true },
			{ key: 'slug', label: t('fieldGroups.slug', 'Slug'), sortable: true },
			{
				key: 'entity_type',
				label: t('fieldGroups.entity_type', 'Entity Type'),
				sortable: true,
				componentSnippet: EntityTypeCell
			},
			{
				key: 'field_count',
				label: t('fieldGroups.fields', 'Fields'),
				sortable: true,
				render: (v: number) => String(v ?? 0)
			},
			{
				key: 'is_system',
				label: t('fieldGroups.type', 'Type'),
				sortable: true,
				componentSnippet: SystemBadge
			}
		],
		filters: [
			{
				key: 'entity_type',
				label: t('fieldGroups.entity_type', 'Entity Type'),
				options: entityTypeOptions
			}
		],
		actions: [
			{
				label: t('fieldGroups.manage_fields', 'Manage Fields'),
				icon: List,
				onclick: (row: FieldGroup) => openFieldEditor(row)
			},
			{
				label: t('common.view', 'View'),
				onclick: (_row: FieldGroup, ctx) => ctx?.openView(_row)
			},
			{
				label: t('common.edit', 'Edit'),
				onclick: (_row: FieldGroup, ctx) => ctx?.openEdit(_row),
				show: (row: FieldGroup) => !row.is_system
			},
			{
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (_row: FieldGroup, ctx) => ctx?.openDelete(_row),
				show: (row: FieldGroup) => !row.is_system
			}
		],
		fields,
		load: async ({ q, filter, page, pageSize, sort }) => {
			const params = new URLSearchParams();
			if (filter.entity_type) params.set('entity_type', filter.entity_type);
			if (q) params.set('q', q);
			// page is 0-based from EntityPage; backend expects 1-based.
			params.set('page', String(page + 1));
			params.set('page_size', String(pageSize));
			if (sort) params.set('sort', sort);
			const result = await fieldGroupsAdminApiExtended.list(params.toString() || undefined);
			if (result.ok) {
				const body = result.data;
				const data = (Array.isArray(body?.data) ? body.data : []) as FieldGroup[];
				const meta = body?.meta;
				return {
					data,
					total: meta?.total ?? data.length,
					totalPages: meta?.total_pages ?? 1
				};
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load field groups'));
		},
		create: async (body) => {
			const payload: Record<string, unknown> = {
				name: String(body.name ?? '').trim(),
				entity_type: String(body.entity_type ?? '').trim()
			};
			const slug = String(body.slug ?? '').trim();
			if (slug) payload.slug = slug;
			const desc = String(body.description ?? '').trim();
			if (desc) payload.description = desc;
			const res = await fieldGroupsAdminApiExtended.create(payload);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const payload: Record<string, unknown> = {
				name: String(body.name ?? '').trim(),
				entity_type: String(body.entity_type ?? '').trim()
			};
			const slug = String(body.slug ?? '').trim();
			if (slug) payload.slug = slug;
			const desc = String(body.description ?? '').trim();
			if (desc) payload.description = desc;
			const res = await fieldGroupsAdminApiExtended.update(id, payload);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await fieldGroupsAdminApiExtended.delete(id);
			return res.ok
				? { ok: true }
				: { ok: false, error: res.error, inUse: res.code === 'in_use' };
		},
		forceRemove: async (id) => {
			const res = await fieldGroupsAdminApiExtended.delete(id, true);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: GroupView
	});

	// ── Field-editor sub-flow (page-local SlideOver) ────────────────────────
	const canManageFields = $derived(hasPermission('custom_fields.field.create'));
	const canEditField = $derived(hasPermission('custom_fields.field.edit'));
	const canDeleteField = $derived(hasPermission('custom_fields.field.delete'));

	let showFieldEditor = $state(false);
	let editorGroup = $state<FieldGroup | null>(null);
	let groupFields = $state<CustomField[]>([]);
	let fieldsLoading = $state(false);
	let fieldError = $state('');

	// Add/Edit field modal
	let fieldModalOpen = $state(false);
	let editingField = $state<CustomField | null>(null);
	let savingField = $state(false);
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

	// Delete field confirm
	let showDeleteFieldConfirm = $state(false);
	let deleteFieldTarget = $state<CustomField | null>(null);

	// All supported field types (must match backend validFieldTypes).
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
		{ value: 'group', label: 'Group (Visual Container)' }
	];

	const fieldTypeLabel = (ft: string) => fieldTypes.find((f) => f.value === ft)?.label ?? ft;
	const needsOptions = $derived(fieldType === 'select' || fieldType === 'multiselect');

	async function openFieldEditor(group: FieldGroup) {
		editorGroup = group;
		showFieldEditor = true;
		fieldError = '';
		await loadGroupFields();
	}

	async function loadGroupFields() {
		if (!editorGroup) return;
		fieldsLoading = true;
		fieldError = '';
		try {
			const result = await api<{ group: FieldGroup; fields: CustomField[] }>(
				`/admin/field-groups/${editorGroup.id}`
			);
			if (result.ok && result.data) {
				editorGroup = result.data.group ?? editorGroup;
				groupFields = (Array.isArray(result.data.fields) ? result.data.fields : []) as CustomField[];
			} else if (!result.ok) {
				fieldError = result.error ?? t('common.error', 'Failed to load fields');
			}
		} catch {
			fieldError = t('common.error', 'Failed to load fields');
		} finally {
			fieldsLoading = false;
		}
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
		fieldModalOpen = true;
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
		fieldModalOpen = true;
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
		if (!editorGroup) return;
		if (!label.trim() || !key.trim()) {
			fieldError = t('fieldGroups.label_key_required', 'Label and key are required');
			return;
		}
		if (needsOptions && options.length === 0) {
			fieldError = t('fieldGroups.options_required', 'Select and multiselect fields require at least one option');
			return;
		}
		savingField = true;
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
				position: editingField?.position ?? groupFields.length
			};
			if (needsOptions) body.options = options;

			if (editingField) {
				const result = await fieldGroupsAdminApiExtended.updateField(editorGroup.id, editingField.id, body);
				if (!result.ok) {
					fieldError = result.error ?? t('common.error', 'Failed to save field');
					savingField = false;
					return;
				}
			} else {
				const result = await fieldGroupsAdminApiExtended.addField(editorGroup.id, body);
				if (!result.ok) {
					fieldError = result.error ?? t('common.error', 'Failed to add field');
					savingField = false;
					return;
				}
			}
			fieldModalOpen = false;
			await loadGroupFields();
			entityController?.reload();
		} catch {
			fieldError = t('common.error', 'Failed to save field');
		} finally {
			savingField = false;
		}
	}

	function confirmDeleteField(f: CustomField) {
		deleteFieldTarget = f;
		showDeleteFieldConfirm = true;
	}

	async function executeDeleteField() {
		if (!editorGroup || !deleteFieldTarget) return;
		showDeleteFieldConfirm = false;
		try {
			await fieldGroupsAdminApiExtended.deleteField(editorGroup.id, deleteFieldTarget.id);
			deleteFieldTarget = null;
			await loadGroupFields();
			entityController?.reload();
		} catch {
			fieldError = t('common.error', 'Failed to delete field');
		}
	}

	async function moveField(index: number, dir: -1 | 1) {
		if (!editorGroup) return;
		const newIndex = index + dir;
		if (newIndex < 0 || newIndex >= groupFields.length) return;
		const reordered = [...groupFields];
		const [moved] = reordered.splice(index, 1);
		reordered.splice(newIndex, 0, moved);
		groupFields = reordered;
		try {
			await fieldGroupsAdminApiExtended.reorderFields(editorGroup.id, {
				ordered_field_ids: reordered.map((f) => f.id)
			});
			await loadGroupFields();
			entityController?.reload();
		} catch {
			fieldError = t('common.error', 'Failed to reorder fields');
			await loadGroupFields();
		}
	}
</script>

<svelte:head>
	<title>{t('custom_fields.title', 'Custom Fields')} — JapaConnect Admin</title>
</svelte:head>

<EntityPage {config} bind:controller={entityController} />

<!-- Field-editor SlideOver (page-local sub-flow) -->
<SlideOver bind:open={showFieldEditor} title={t('fieldGroups.manage_fields', 'Manage Fields')} size="lg">
	{#if editorGroup}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('fieldGroups.field_group', 'Field Group')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{editorGroup.name}
				</p>
				<p class="mt-0.5 text-xs" style="color: var(--color-muted)">
					{entityLabel(editorGroup.entity_type)}
				</p>
			</div>

			{#if fieldError}
				<div
					class="rounded-lg p-2 text-sm"
					style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
				>
					{fieldError}
				</div>
			{/if}

			{#if canManageFields}
				<Button onclick={openAddField} size="sm">
					<Plus class="mr-1 h-3.5 w-3.5" />
					{t('fieldGroups.add_field', 'Add Field')}
				</Button>
			{/if}

			{#if fieldsLoading}
				<TableSkeleton rows={4} columns={3} />
			{:else if groupFields.length === 0}
				<div class="py-8 text-center text-sm" style="color: var(--color-muted)">
					{t('fieldGroups.no_fields_yet', 'No fields yet')}
				</div>
			{:else}
				<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
					<table class="w-full text-sm">
						<thead>
							<tr style="border-bottom: 1px solid var(--color-border)">
								<th class="w-8 px-2 py-2"></th>
								<th class="px-3 py-2 text-left font-medium" style="color: var(--color-muted)">{t('fieldGroups.label', 'Label')}</th>
								<th class="px-3 py-2 text-left font-medium" style="color: var(--color-muted)">{t('fieldGroups.key', 'Key')}</th>
								<th class="px-3 py-2 text-left font-medium" style="color: var(--color-muted)">{t('fieldGroups.field_type', 'Type')}</th>
								<th class="w-24 px-3 py-2 text-left font-medium" style="color: var(--color-muted)">{t('common.actions', 'Actions')}</th>
							</tr>
						</thead>
						<tbody>
							{#each groupFields as f, i (f.id ?? `field-${i}`)}
								<tr class="border-t" style="border-color: var(--color-border)">
									<td class="px-2 py-2 text-center" style="color: var(--color-muted)">
										<div class="flex flex-col items-center">
											<button
												type="button"
												class="p-0.5 hover:opacity-70"
												onclick={() => moveField(i, -1)}
												disabled={i === 0}
												title={t('common.move_up', 'Move up')}
											>
												<ArrowUp class="h-3 w-3" />
											</button>
											<GripVertical class="h-3 w-3" />
											<button
												type="button"
												class="p-0.5 hover:opacity-70"
												onclick={() => moveField(i, 1)}
												disabled={i === groupFields.length - 1}
												title={t('common.move_down', 'Move down')}
											>
												<ArrowDown class="h-3 w-3" />
											</button>
										</div>
									</td>
									<td class="px-3 py-2 font-medium" style="color: var(--color-foreground)">{f.label}</td>
									<td class="px-3 py-2 font-mono text-xs" style="color: var(--color-muted)">{f.key}</td>
									<td class="px-3 py-2">
										<span
											class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
											style="background: var(--color-surface); color: var(--color-muted)"
										>
											{fieldTypeLabel(f.field_type)}
										</span>
									</td>
									<td class="px-3 py-2">
										<div class="flex items-center gap-1">
											{#if canEditField}
												<button type="button" class="p-1 rounded hover:opacity-70" onclick={() => openEditField(f)} title={t('common.edit', 'Edit')}>
													<Pencil class="h-3.5 w-3.5" style="color: var(--color-muted)" />
												</button>
											{/if}
											{#if canDeleteField && !f.is_system}
												<button type="button" class="p-1 rounded hover:opacity-70" onclick={() => confirmDeleteField(f)} title={t('common.delete', 'Delete')}>
													<Trash2 class="h-3.5 w-3.5" style="color: var(--color-danger)" />
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
	{/if}

	{#snippet footer()}
		<Button variant="outline" onclick={() => (showFieldEditor = false)}>
			{t('common.close', 'Close')}
		</Button>
	{/snippet}
</SlideOver>

<!-- Add/Edit Field Modal -->
<Modal bind:open={fieldModalOpen} title={editingField ? `${t('fieldGroups.edit_field', 'Edit Field')}: ${editingField.label}` : t('fieldGroups.add_field', 'Add Field')}>
	<div class="space-y-4">
		<div>
			<label for="fld-label" class="mb-1 block text-sm font-medium">{t('fieldGroups.label', 'Label')}</label>
			<input
				id="fld-label"
				type="text"
				bind:value={label}
				onblur={autoKey}
				placeholder={t('fieldGroups.label_placeholder', 'Field label')}
				required
				class="w-full rounded-lg border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
			/>
		</div>
		<div>
			<label for="fld-key" class="mb-1 block text-sm font-medium">{t('fieldGroups.key', 'Key')}</label>
			<input
				id="fld-key"
				type="text"
				bind:value={key}
				placeholder={t('fieldGroups.key_placeholder', 'field_key')}
				required
				disabled={!!editingField}
				class="w-full rounded-lg border px-3 py-2 font-mono text-sm disabled:opacity-50"
				style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
			/>
			<p class="mt-1 text-xs" style="color: var(--color-muted)">
				{t('fieldGroups.key_help', 'Unique identifier for this field.')}
				{editingField ? t('fieldGroups.key_immutable', 'Key cannot be changed after creation.') : t('fieldGroups.key_auto', 'Auto-generated from the label.')}
			</p>
		</div>
		<div>
			<label for="fld-type" class="mb-1 block text-sm font-medium">{t('fieldGroups.field_type', 'Field Type')}</label>
			<AutocompleteSelect
				id="fld-type"
				bind:value={fieldType}
				disabled={!!editingField}
				options={fieldTypes}
				class="w-full"
			/>
			{#if editingField}
				<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('fieldGroups.type_immutable', 'Field type cannot be changed after creation.')}</p>
			{/if}
		</div>
		{#if needsOptions}
			<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
				<span class="mb-2 block text-sm font-medium">{t('fieldGroups.options', 'Options')}</span>
				<div class="flex gap-2">
					<input
						type="text"
						class="w-32 rounded-md border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
						placeholder={t('fieldGroups.option_value', 'value')}
						bind:value={newOptionValue}
						onkeydown={handleOptionKeydown}
					/>
					<input
						type="text"
						class="flex-1 rounded-md border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
						placeholder={t('fieldGroups.option_label', 'label')}
						bind:value={newOptionLabel}
						onkeydown={handleOptionKeydown}
					/>
					<button
						type="button"
						class="rounded-md border px-3 py-2 text-sm font-medium hover:opacity-70"
						style="border-color: var(--color-border)"
						onclick={addOption}
					>
						{t('common.add', 'Add')}
					</button>
				</div>
				{#if options.length > 0}
					<div class="mt-2 flex flex-wrap gap-2">
						{#each options as opt (opt.value)}
							<span class="inline-flex items-center gap-1 rounded-full px-3 py-1 text-xs" style="background: var(--color-surface)">
								<span style="color: var(--color-muted)">{opt.value}</span>: {opt.label}
								<button type="button" class="ml-0.5 opacity-50 hover:opacity-100" onclick={() => removeOption(opt)}>&times;</button>
							</span>
						{/each}
					</div>
				{/if}
			</div>
		{/if}
		<div class="flex flex-wrap gap-4">
			<label class="flex items-center gap-2 text-sm">
				<input type="checkbox" bind:checked={isRequired} class="rounded" />
				{t('fieldGroups.required', 'Required')}
			</label>
			<label class="flex items-center gap-2 text-sm">
				<input type="checkbox" bind:checked={showInFrontend} class="rounded" />
				{t('fieldGroups.visible', 'Show in frontend')}
			</label>
			<label class="flex items-center gap-2 text-sm">
				<input type="checkbox" bind:checked={filterable} class="rounded" />
				{t('fieldGroups.filterable', 'Filterable')}
			</label>
			<label class="flex items-center gap-2 text-sm">
				<input type="checkbox" bind:checked={sortable} class="rounded" />
				{t('fieldGroups.sortable', 'Sortable')}
			</label>
			<label class="flex items-center gap-2 text-sm">
				<input type="checkbox" bind:checked={searchable} class="rounded" />
				{t('fieldGroups.searchable', 'Searchable')}
			</label>
		</div>
		{#if fieldError}
			<p class="text-sm" style="color: var(--color-danger)">{fieldError}</p>
		{/if}
		<div class="mt-2 flex justify-end gap-2">
			<Button variant="outline" onclick={() => (fieldModalOpen = false)} disabled={savingField}>
				{t('common.cancel', 'Cancel')}
			</Button>
			<Button onclick={saveField} disabled={savingField}>
				{savingField ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
			</Button>
		</div>
	</div>
</Modal>

<!-- Delete Field Confirmation -->
<Modal bind:open={showDeleteFieldConfirm} title={t('fieldGroups.delete_field', 'Delete Field')}>
	{#if deleteFieldTarget}
		<p class="text-sm" style="color: var(--color-muted)">
			{t('common.delete', 'Delete')} <strong>{deleteFieldTarget.label}</strong>?
			{t('fieldGroups.delete_field_confirm', 'This cannot be undone.')}
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteFieldConfirm = false)}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button variant="destructive" onclick={executeDeleteField}>
			{t('common.delete', 'Delete')}
		</Button>
	</div>
</Modal>

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

<!-- EntityTypeCell renders the human-readable label for the entity_type token -->
{#snippet EntityTypeCell({ value }: { value: any; row: FieldGroup })}
	{entityLabel(value)}
{/snippet}

<!-- SystemBadge renders a badge indicating whether the group is system-defined -->
{#snippet SystemBadge({ value }: { value: any; row: FieldGroup })}
	{#if value}
		<span
			class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
			style="background: color-mix(in srgb, var(--color-primary) 12%, transparent); color: var(--color-primary)"
		>
			{t('fieldGroups.system', 'System')}
		</span>
	{:else}
		<span
			class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
			style="background: var(--color-surface); color: var(--color-muted)"
		>
			{t('fieldGroups.custom', 'Custom')}
		</span>
	{/if}
{/snippet}

<!-- View slide-over body showing the group's fields -->
{#snippet GroupView({ row }: { row: FieldGroup })}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('fieldGroups.name', 'Name')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('fieldGroups.slug', 'Slug')}
				</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug ?? '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('fieldGroups.entity_type', 'Entity Type')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{entityLabel(row.entity_type)}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('fieldGroups.fields', 'Fields')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.field_count ?? 0}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('fieldGroups.type', 'Type')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{row.is_system ? t('fieldGroups.system', 'System') : t('fieldGroups.custom', 'Custom')}
				</p>
			</div>
		</div>
		{#if row.description}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('fieldGroups.description', 'Description')}
				</p>
				<p class="mt-1 whitespace-pre-line text-sm" style="color: var(--color-foreground)">{row.description}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('common.created', 'Created')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{new Date(row.created_at).toLocaleString()}
			</p>
		</div>
	</div>
{/snippet}
