<script lang="ts">
	import { ActionCell } from '$lib/components/ui/data-table';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { Pencil, Trash2 } from 'lucide-svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { userManagementApi } from '$lib/api';
	import type { Component } from 'svelte';
	import type { UserGroup } from '$lib/api';

	let { value, row, onchange }: { value: unknown; row: UserGroup; onchange?: () => void } = $props();

	let showEditModal = $state(false);
	let editName = $state('');
	let editDescription = $state('');
	let editPermissions = $state<string[]>([]);
	let editError = $state('');
	let editSaving = $state(false);
	let showDeleteConfirm = $state(false);

	const canManage = hasPermission('admin.rbac.manage');

	function openEditModal() {
		editName = row.name;
		editDescription = row.description;
		editPermissions = [...row.permissions];
		editError = '';
		showEditModal = true;
	}

	const actions = $derived(
		canManage
			? [
					{ label: t('common.edit', 'Edit'), icon: Pencil as unknown as Component, onclick: openEditModal },
					{ label: t('common.delete', 'Delete'), icon: Trash2 as unknown as Component, onclick: () => { showDeleteConfirm = true; }, variant: 'destructive' as const }
				]
			: []
	);

	async function handleEditSave() {
		if (!editName.trim()) { editError = t('users.groups.name_required', 'Name is required'); return; }
		editSaving = true;
		editError = '';
		const result = await userManagementApi.updateGroup(row.id, { name: editName, description: editDescription, permissions: editPermissions });
		editSaving = false;
		if (result.ok) {
			showEditModal = false;
			onchange?.();
		} else {
			editError = result.error ?? t('users.groups.save_error', 'Failed to save');
		}
	}

	async function handleDelete() {
		showDeleteConfirm = false;
		const result = await userManagementApi.deleteGroup(row.id);
		if (result.ok) onchange?.();
	}
</script>

<ActionCell {actions} />

<Modal bind:open={showEditModal} title={t('users.groups.edit_title', 'Edit Group')}>
	{#if editError}
		<div class="mb-3 rounded-md border px-3 py-2 text-xs" style="border-color: var(--color-danger); color: var(--color-danger)">{editError}</div>
	{/if}
	<div class="space-y-4">
		<div>
			<label for="edit-group-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('users.groups.name', 'Name')}</label>
			<input
				id="edit-group-name"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
				bind:value={editName}
			/>
		</div>
		<div>
			<label for="edit-group-desc" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('users.groups.description_label', 'Description')}</label>
			<textarea
				id="edit-group-desc"
				rows="2"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
				bind:value={editDescription}
			></textarea>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showEditModal = false}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={handleEditSave} disabled={editSaving}>{editSaving ? t('common.saving', 'Saving...') : t('common.save', 'Save')}</Button>
	</div>
</Modal>

<Modal bind:open={showDeleteConfirm} title={t('common.delete', 'Delete')}>
	<p class="text-sm" style="color: var(--color-foreground)">
		{t('users.groups.delete_confirm', 'Are you sure you want to delete this group?')}
	</p>
	<p class="mt-2 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
	<p class="mt-1 text-sm" style="color: var(--color-danger)">
		{t('common.cannotUndo', 'This action cannot be undone.')}
	</p>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showDeleteConfirm = false}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={handleDelete}>{t('common.delete', 'Delete')}</Button>
	</div>
</Modal>
