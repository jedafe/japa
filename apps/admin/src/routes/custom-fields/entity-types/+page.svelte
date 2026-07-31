<script lang="ts">
	import { entityTypesAdminApi, type CustomFieldEntityType } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Plus, Pencil, Trash2 } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';

	let entityTypes = $state<CustomFieldEntityType[]>([]);
	let loading = $state(false);
	let errorMessage = $state('');
	let modalOpen = $state(false);
	let editingType = $state<CustomFieldEntityType | null>(null);
	let value = $state('');
	let label = $state('');
	let sortOrder = $state(0);
	let saving = $state(false);
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<CustomFieldEntityType | null>(null);

	const canManage = $derived(hasPermission('custom_fields.group.create'));

	const columns = [
		{ key: 'value', label: 'Value', sortable: true },
		{ key: 'label', label: 'Label', sortable: true },
		{ key: 'sort_order', label: 'Sort Order', sortable: true },
		{ key: 'is_system', label: 'System', sortable: true },
		{ key: 'actions', label: t('common.actions'), class: 'w-24' }
	];

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await entityTypesAdminApi.list();
			if (result.ok) {
				entityTypes = (Array.isArray(result.data) ? result.data : []) as CustomFieldEntityType[];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		editingType = null;
		value = '';
		label = '';
		sortOrder = entityTypes.length;
		modalOpen = true;
	}

	function openEdit(et: CustomFieldEntityType) {
		editingType = et;
		value = et.value;
		label = et.label;
		sortOrder = et.sort_order;
		modalOpen = true;
	}

	async function save() {
		if (!label.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			if (editingType) {
				const result = await entityTypesAdminApi.update(editingType.id, {
					label: label.trim(),
					sort_order: sortOrder
				});
				if (!result.ok) {
					errorMessage = result.error;
					saving = false;
					return;
				}
			} else {
				if (!value.trim()) {
					errorMessage = 'Value is required';
					saving = false;
					return;
				}
				const result = await entityTypesAdminApi.create({
					value: value.trim().toLowerCase().replace(/\s+/g, '_'),
					label: label.trim(),
					sort_order: sortOrder
				});
				if (!result.ok) {
					errorMessage = result.error;
					saving = false;
					return;
				}
			}
			modalOpen = false;
			await load();
		} catch {
			errorMessage = t('common.error');
		} finally {
			saving = false;
		}
	}

	function confirmDelete(et: CustomFieldEntityType) {
		deleteTarget = et;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		errorMessage = '';
		try {
			await entityTypesAdminApi.delete(deleteTarget.id);
			showDeleteConfirm = false;
			deleteTarget = null;
			await load();
		} catch {
			errorMessage = t('common.error');
		}
	}

	load();
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">Entity Types</h1>
			<p class="mt-1 text-sm" style="color: var(--color-muted)">
				Manage the entity types that custom field groups can be assigned to.
			</p>
		</div>
		{#if canManage}
			<button
				onclick={openCreate}
				class="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white"
				style="background-color: var(--color-primary)"
			>
				<Plus class="h-4 w-4" />
				{t('common.create')}
			</button>
		{/if}
	</div>

	{#if errorMessage}
		<div class="rounded-lg p-3 text-sm" style="background-color: var(--color-destructive-muted); color: var(--color-destructive)">
			{errorMessage}
		</div>
	{/if}

	{#if loading}
		<p class="text-sm" style="color: var(--color-muted)">{t('common.loading')}…</p>
	{:else}
		<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
			<table class="w-full text-sm">
				<thead style="background-color: var(--color-muted-muted)">
					<tr>
						{#each columns as col}
							<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted)">{col.label}</th>
						{/each}
					</tr>
				</thead>
				<tbody>
					{#each entityTypes as et (et.id)}
						<tr class="border-t" style="border-color: var(--color-border)">
							<td class="px-4 py-3 font-mono text-xs" style="color: var(--color-foreground)">{et.value}</td>
							<td class="px-4 py-3" style="color: var(--color-foreground)">{et.label}</td>
							<td class="px-4 py-3" style="color: var(--color-foreground)">{et.sort_order}</td>
							<td class="px-4 py-3">
								{#if et.is_system}
									<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium" style="background-color: var(--color-primary-muted); color: var(--color-primary)">System</span>
								{:else}
									<span class="text-xs" style="color: var(--color-muted)">—</span>
								{/if}
							</td>
							<td class="px-4 py-3">
								<div class="flex items-center gap-2">
									{#if canManage}
										<button onclick={() => openEdit(et)} class="p-1 rounded hover:opacity-70" title={t('common.edit')}>
											<Pencil class="h-4 w-4" style="color: var(--color-muted)" />
										</button>
										{#if !et.is_system}
											<button onclick={() => confirmDelete(et)} class="p-1 rounded hover:opacity-70" title={t('common.delete')}>
												<Trash2 class="h-4 w-4" style="color: var(--color-destructive)" />
											</button>
										{/if}
									{/if}
								</div>
							</td>
						</tr>
					{:else}
						<tr class="border-t" style="border-color: var(--color-border)">
							<td colspan={columns.length} class="px-4 py-8 text-center" style="color: var(--color-muted)">
								No entity types found.
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<Modal bind:open={modalOpen} title={editingType ? t('common.edit') : t('common.create')} confirmLabel={t('common.save')} onConfirm={save}>
	<div class="space-y-4">
		{#if !editingType}
			<div>
				<label for="et-value" class="mb-1 block text-sm font-medium">Value</label>
				<input
					id="et-value"
					type="text"
					bind:value
					placeholder="e.g. event, project, course"
					required
					class="w-full rounded-lg border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
				/>
				<p class="mt-1 text-xs" style="color: var(--color-muted)">Machine-readable token (snake_case). Cannot be changed after creation.</p>
			</div>
		{/if}
		<div>
			<label for="et-label" class="mb-1 block text-sm font-medium">Label</label>
			<input
				id="et-label"
				type="text"
				bind:value={label}
				placeholder="e.g. Event, Project, Course"
				required
				class="w-full rounded-lg border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
			/>
		</div>
		<div>
			<label for="et-sort" class="mb-1 block text-sm font-medium">Sort Order</label>
			<input
				id="et-sort"
				type="number"
				bind:value={sortOrder}
				min="0"
				class="w-full rounded-lg border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
			/>
		</div>
		{#if errorMessage}
			<p class="text-sm" style="color: var(--color-destructive)">{errorMessage}</p>
		{/if}
	</div>
</Modal>

<Modal bind:open={showDeleteConfirm} title="Delete Entity Type" confirmLabel={t('common.delete')} onConfirm={executeDelete}>
	<p class="text-sm" style="color: var(--color-muted)">
		Delete <strong>{deleteTarget?.label}</strong>? Field groups assigned to this entity type will remain but will no longer appear in the entity type dropdown.
	</p>
</Modal>
