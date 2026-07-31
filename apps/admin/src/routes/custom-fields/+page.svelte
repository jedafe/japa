<script lang="ts">
	import { api, fieldGroupsAdminApiExtended, userManagementApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Plus, Pencil, Trash2, Shield, List } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { FilterBar } from '$lib/components/ui/filter-bar';
	import { Button } from '$lib/components/ui/button';

	type FieldGroup = {
		id: string;
		name: string;
		entity_type: string;
		field_count: number;
		created_at: string;
	};

	type FieldItem = {
		id: string;
		key: string;
		label: string;
		field_type: string;
	};

	type FieldPermission = {
		field_id: string;
		user_group_id: string;
		can_view: boolean;
		can_edit: boolean;
	};

	let groups = $state<FieldGroup[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let selectedIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	let showCreateModal = $state(false);
	let showEditModal = $state(false);
	let editingGroup = $state<FieldGroup | null>(null);
	let saving = $state(false);

	let modalOpen = $state(false);

	let name = $state('');
	let entityType = $state('');

	let activeTab = $state<'groups' | 'permissions'>('groups');
	let selectedGroup = $state<FieldGroup | null>(null);
	let groupFields = $state<FieldItem[]>([]);
	let userGroups = $state<Array<{ id: string; name: string }>>([]);
	let permissions = $state<FieldPermission[]>([]);
	let permissionsLoading = $state(false);
	let savingPermission = $state(false);

	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<FieldGroup | null>(null);

	let searchQuery = $state('');

	const canManageFields = $derived(hasPermission('custom_fields.group.create'));

	// Entity types are fetched from GET /custom-field-entity-types so the
	// filter options always match the backend's supported entity classes
	// (Rule 16 — no hardcoded option lists in the frontend).
	let entityTypeOptions = $state<Array<{ value: string; label: string }>>([]);

	const filterDefs = $derived([
		{
			key: 'entity_type',
			label: t('fieldGroups.entity_type'),
			options: entityTypeOptions
		}
	]);

	let filterValues = $state<Record<string, string>>({});

	function handleFilterSearch(values: Record<string, string>) {
		searchQuery = values.search ?? '';
		filterValues = values;
	}

	function handleFilterClear() {
		searchQuery = '';
		filterValues = {};
	}

	const filteredGroups = $derived(
		searchQuery
			? groups.filter(
					(g) =>
						g.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
						g.entity_type.toLowerCase().includes(searchQuery.toLowerCase())
				)
			: groups
	);

	const columns = [
		{ key: 'name', label: t('fieldGroups.name'), sortable: true },
		{ key: 'entity_type', label: t('fieldGroups.entity_type'), sortable: true },
		{ key: 'field_count', label: t('fieldGroups.fields'), sortable: true },
		{ key: 'actions', label: t('common.actions'), class: 'w-24', componentSnippet: FieldGroupActionsCell }
	];

	function confirmDelete(group: FieldGroup) {
		deleteTarget = group;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		errorMessage = '';
		try {
			await api(`/admin/field-groups/${deleteTarget.id}`, { method: 'DELETE' });
			showDeleteConfirm = false;
			deleteTarget = null;
			// Re-fetch from the server to stay consistent with server truth
			// (Rule 16 — invalidate and re-fetch after server confirms).
			await load();
		} catch {
			errorMessage = t('common.error');
		}
	}

	async function bulkDelete() {
		if (selectedIds.size === 0) return;
		bulkDeleting = true;
		errorMessage = '';
		try {
			for (const id of selectedIds) {
				await api(`/admin/field-groups/${id}`, { method: 'DELETE' });
			}
			selectedIds = new Set();
			load();
		} catch {
			errorMessage = t('common.error');
		}
		bulkDeleting = false;
	}

	function handleSearch(q: string) {
		searchQuery = q;
	}

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const params = new URLSearchParams();
			if (filterValues.entity_type) params.set('entity_type', filterValues.entity_type);
			const qs = params.toString();
			const result = await fieldGroupsAdminApiExtended.list(qs || undefined);
			if (result.ok) {
				groups = (Array.isArray(result.data) ? result.data : []) as FieldGroup[];
			} else {
				errorMessage = result.error;
			}
			// Fetch entity types from the backend (Rule 16) if not already loaded.
			if (entityTypeOptions.length === 0) {
				const typesResult = await userManagementApi.listCustomFieldEntityTypes();
				if (typesResult.ok && Array.isArray(typesResult.data)) {
					entityTypeOptions = typesResult.data;
				}
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		name = '';
		entityType = '';
		modalOpen = true;
	}

	function openEdit(group: FieldGroup) {
		editingGroup = group;
		name = group.name;
		entityType = group.entity_type;
		modalOpen = true;
	}

	async function createGroup() {
		if (!name.trim() || !entityType.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await api('/admin/field-groups', {
				method: 'POST',
				body: { name: name.trim(), entity_type: entityType.trim() }
			});
			if (result.ok) {
				modalOpen = false;
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			saving = false;
		}
	}

	async function updateGroup() {
		if (!editingGroup || !name.trim() || !entityType.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await api(`/admin/field-groups/${editingGroup.id}`, {
				method: 'PATCH',
				body: { name: name.trim(), entity_type: entityType.trim() }
			});
			if (result.ok) {
				modalOpen = false;
				editingGroup = null;
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			saving = false;
		}
	}

	async function openPermissions(group: FieldGroup) {
		selectedGroup = group;
		activeTab = 'permissions';
		permissionsLoading = true;
		try {
			const fieldsResult = await api<FieldItem[]>(`/admin/field-groups/${group.id}/fields`);
			groupFields = fieldsResult.ok ? (fieldsResult.data ?? []) : [];

			const ugResult = await api<{ groups: Array<{ id: string; name: string }>; total: number }>('/admin/user-groups');
			userGroups = ugResult.ok && ugResult.data?.groups ? ugResult.data.groups : [];

			permissions = [];
			for (const field of groupFields) {
				const permResult = await api<FieldPermission[]>(`/custom-fields/${field.id}/permissions`);
				if (permResult.ok && permResult.data) {
					permissions.push(...permResult.data);
				}
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			permissionsLoading = false;
		}
	}

	async function setFieldPermission(fieldId: string, userGroupId: string, canView: boolean, canEdit: boolean) {
		savingPermission = true;
		try {
			await api(`/custom-fields/${fieldId}/permissions`, {
				method: 'PUT',
				body: { user_group_id: userGroupId, can_view: canView, can_edit: canEdit }
			});
		} catch {
			errorMessage = t('common.error');
		} finally {
			savingPermission = false;
		}
	}

	function getPermission(fieldId: string, groupId: string): FieldPermission | undefined {
		return permissions.find((p) => p.field_id === fieldId && p.user_group_id === groupId);
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('custom_fields.title', 'Custom Fields')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('fieldGroups.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('fieldGroups.description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<!-- Tabs -->
<div class="mb-6 flex gap-1 rounded-lg border p-1" style="border-color: var(--color-border)">
	<button
		class="rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'groups' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'groups'; }}
	>
		{t('fieldGroups.title')}
	</button>
	<button
		class="flex items-center gap-1.5 rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'permissions' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'permissions'; }}
	>
		<Shield class="h-3 w-3" />
		{t('fieldGroups.permissions')}
	</button>
</div>

{#if activeTab === 'groups'}
	<div class="mb-4 flex justify-end">
		<button class="btn btn-primary flex items-center gap-1.5 text-xs" onclick={openCreate}>
			<Plus class="h-3 w-3" />
			{t('fieldGroups.create')}
		</button>
	</div>

	<FilterBar filters={filterDefs} onsearch={handleFilterSearch} onclear={handleFilterClear} />

	{#if loading}
		<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
	{:else}
		<DataTable
			{columns}
			data={filteredGroups}
			pageSize={25}
			emptyMessage={t('fieldGroups.empty')}
			searchable
			searchPlaceholder={t('common.search') + ' ' + t('fieldGroups.title').toLowerCase() + '...'}
			onsearch={handleSearch}
			selectable
			bind:selectedIds
		>
			{#snippet bulkActions()}
				<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting || selectedIds.size === 0}>
					<Trash2 class="h-3.5 w-3.5 mr-1" />
					{bulkDeleting ? t('common.deleting', 'Deleting...') : t('common.deleteSelected', 'Delete Selected')} ({selectedIds.size})
				</Button>
			{/snippet}
		</DataTable>
	{/if}

{:else if activeTab === 'permissions'}
	<!-- Group selector -->
	<div class="mb-6 flex items-center gap-3">
		<label for="perm-group-select" class="text-sm font-medium" style="color: var(--color-foreground)">Field Group:</label>
		<select
			id="perm-group-select"
			class="rounded-lg border px-3 py-2 text-sm"
			style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
			value={selectedGroup?.id ?? ''}
			onchange={(e) => {
				const id = (e.target as HTMLSelectElement).value;
				const group = groups.find((g) => g.id === id);
				if (group) openPermissions(group);
			}}
		>
			<option value="" disabled>Select a field group…</option>
			{#each groups as g}
				<option value={g.id}>{g.name}</option>
			{/each}
		</select>
	</div>

	{#if !selectedGroup}
		<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Select a field group above to manage its field permissions.</p>
	{:else if permissionsLoading}
		<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
	{:else}
		<p class="mb-4 text-sm" style="color: var(--color-muted)">
			Permissions for: <strong>{selectedGroup.name}</strong>
		</p>

		{#if groupFields.length === 0}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">No fields in this group. Add fields first from the Field Groups tab.</p>
		{:else if userGroups.length === 0}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">No user groups found. Create user groups first from the User Management section.</p>
		{:else}
			<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
				<table class="w-full text-sm">
					<thead>
						<tr style="border-bottom: 1px solid var(--color-border)">
							<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('custom-fields.field')}</th>
							{#each userGroups as ug}
								<th class="px-4 py-3 text-center font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">
									{ug.name}
									<div class="mt-1 flex justify-center gap-4 text-[10px] font-normal">
										<span>{t('custom-fields.view')}</span>
										<span>{t('custom-fields.edit')}</span>
									</div>
								</th>
							{/each}
						</tr>
					</thead>
					<tbody>
						{#each groupFields as field (field.id)}
							<tr class="border-t" style="border-color: var(--color-border)">
								<td class="px-4 py-3 font-medium">{field.label}</td>
								{#each userGroups as ug}
									{@const perm = getPermission(field.id, ug.id)}
									<td class="px-4 py-3 text-center">
										<div class="flex justify-center gap-3">
											<input
												type="checkbox"
												class="h-4 w-4 rounded"
												checked={perm?.can_view ?? false}
												onchange={(e) => setFieldPermission(field.id, ug.id, (e.target as HTMLInputElement).checked, perm?.can_edit ?? false)}
											/>
											<input
												type="checkbox"
												class="h-4 w-4 rounded"
												checked={perm?.can_edit ?? false}
												onchange={(e) => setFieldPermission(field.id, ug.id, perm?.can_view ?? false, (e.target as HTMLInputElement).checked)}
											/>
										</div>
									</td>
								{/each}
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	{/if}
{/if}

<!-- Create/Edit Modal -->
<Modal bind:open={modalOpen} title={editingGroup ? t('fieldGroups.edit') : t('fieldGroups.create')} onConfirm={editingGroup ? updateGroup : createGroup} confirmLabel={saving ? t('common.saving') : editingGroup ? t('common.save') : t('common.create')}>
	<div class="space-y-4">
		<div>
			<label for="fg-name" class="mb-1 block text-sm font-medium">{t('fieldGroups.name')}</label>
			<input id="fg-name" type="text" bind:value={name} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="fg-entity" class="mb-1 block text-sm font-medium">{t('fieldGroups.entity_type')}</label>
			<select id="fg-entity" bind:value={entityType} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)">
				<option value="" disabled>Select…</option>
				{#each entityTypeOptions as opt}
					<option value={opt.value}>{opt.label}</option>
				{/each}
			</select>
		</div>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t('fieldGroups.confirm_delete')} confirmLabel={t('common.delete')} onConfirm={executeDelete}>
	<p class="text-sm" style="color: var(--color-muted)">
		{t('fieldGroups.delete_confirm')} <strong>{deleteTarget?.name}</strong>?
	</p>
</Modal>

<!-- FieldGroupActionsCell snippet for DataTable -->
{#snippet FieldGroupActionsCell({ value, row }: { value: any; row: FieldGroup })}
	<div class="flex items-center gap-1">
		<a href={`/custom-fields/${row.id}`} class="inline-flex items-center justify-center rounded-md p-1.5 hover:opacity-70" title="Manage fields">
			<List class="h-3.5 w-3.5" style="color: var(--color-foreground)" />
		</a>
		<Button variant="ghost" size="sm" onclick={() => openPermissions(row)} title={t('fieldGroups.permissions')}>
			<Shield class="h-3.5 w-3.5" style="color: var(--color-primary)" />
		</Button>
		{#if canManageFields}
			<Button variant="ghost" size="sm" onclick={() => openEdit(row)}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => confirmDelete(row)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
