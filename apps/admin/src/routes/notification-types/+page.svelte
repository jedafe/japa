<script lang="ts">
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';

	type NotificationType = {
		key: string;
		label: string;
		description: string;
		default_channels: { email: boolean; push: boolean; in_app: boolean };
	};

	let types = $state<NotificationType[]>([]);
	let loading = $state(true);
	let error = $state('');
	let showCreateModal = $state(false);
	let editingType = $state<NotificationType | null>(null);
	let newType = $state({ key: '', label: '', description: '' });
	let saving = $state(false);
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<NotificationType | null>(null);

	const canManageNotificationTypes = $derived(hasPermission('notification_types.manage'));

	function confirmDelete(ntype: NotificationType) {
		deleteTarget = ntype;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		error = '';
		try {
			await api(`/admin/notification-types/${deleteTarget.key}`, { method: 'DELETE' });
			types = types.filter((t) => t.key !== deleteTarget!.key);
			showDeleteConfirm = false;
			deleteTarget = null;
		} catch {
			error = t('common.error');
		}
	}

	$effect(() => {
		load();
	});

	async function load() {
		loading = true;
		error = '';
		try {
			const result = await api<NotificationType[]>('/admin/notification-types');
			if (result.ok) {
				types = result.data ?? [];
			} else {
				error = result.error;
			}
		} catch {
			error = t('common.error');
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		newType = { key: '', label: '', description: '' };
		editingType = null;
		showCreateModal = true;
	}

	function openEdit(ntype: NotificationType) {
		editingType = { ...ntype };
		newType = { key: ntype.key, label: ntype.label, description: ntype.description };
		showCreateModal = true;
	}

	async function save() {
		if (!newType.key || !newType.label) return;
		saving = true;
		try {
			if (editingType) {
				await api(`/admin/notification-types/${editingType.key}`, {
					method: 'PATCH',
					body: newType
				});
			} else {
				await api('/admin/notification-types', {
					method: 'POST',
					body: newType
				});
			}
			showCreateModal = false;
			load();
		} catch {
			error = t('common.error');
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>{t('notification_types.title')} — JapaConnect Admin</title>
</svelte:head>

<div class="mb-6 flex items-center justify-between">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">{t('notification_types.title')}</h1>
		<p class="text-sm" style="color: var(--color-muted)">{t('notification_types.description')}</p>
	</div>
	{#if canManageNotificationTypes}
		<button
			class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
			style="background-color: var(--color-primary)"
			onclick={openCreate}
		>
			{t('notification_types.create')}
		</button>
	{/if}
</div>

{#if error}
	<div class="mb-4 rounded-md border p-3 text-sm" style="border-color: var(--color-danger); background-color: var(--color-danger-bg)">
		{error}
	</div>
{/if}

{#if loading}
	<p class="text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium">{t('notification_types.key')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('notification_types.label')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('notification_types.description')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('notification_types.channels')}</th>
					<th class="px-4 py-3 text-right font-medium">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each types as ntype (ntype.key)}
					<tr style="border-bottom: 1px solid var(--color-border)">
						<td class="px-4 py-3 font-mono text-xs">{ntype.key}</td>
						<td class="px-4 py-3 font-medium">{ntype.label}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{ntype.description || '-'}</td>
						<td class="px-4 py-3">
							<div class="flex gap-1">
								{#if ntype.default_channels?.email}
									<span class="rounded px-1.5 py-0.5 text-xs bg-blue-500/15 text-info dark:text-blue-400">{t('notification-types.email')}</span>
								{/if}
								{#if ntype.default_channels?.push}
									<span class="rounded px-1.5 py-0.5 text-xs bg-purple-500/15 text-purple-700 dark:text-purple-400">{t('notification-types.push')}</span>
								{/if}
								{#if ntype.default_channels?.in_app}
									<span class="rounded px-1.5 py-0.5 text-xs bg-success/15 text-success dark:text-success">{t('notification-types.in_app')}</span>
								{/if}
							</div>
						</td>
						<td class="px-4 py-3 text-right">
							<div class="flex items-center justify-end gap-2">
								{#if canManageNotificationTypes}
									<button
										class="text-xs text-blue-500 hover:text-info"
										onclick={() => openEdit(ntype)}
									>
										{t('common.edit')}
									</button>
									<button
										class="text-xs text-danger hover:text-danger"
										onclick={() => confirmDelete(ntype)}
									>
										{t('common.delete')}
									</button>
								{/if}
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('notification_types.no_types')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

{#if showCreateModal}
	<Modal title={editingType ? t('notification_types.edit') : t('notification_types.create')} onclose={() => { showCreateModal = false; }}>
		<div class="space-y-4">
			<div>
				<label for="type-key" class="mb-1 block text-sm font-medium">{t('notification_types.key')}</label>
				<input
					id="type-key"
					type="text"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					placeholder="e.g. new_comment"
					bind:value={newType.key}
					disabled={!!editingType}
				/>
			</div>
			<div>
				<label for="type-label" class="mb-1 block text-sm font-medium">{t('notification_types.label')}</label>
				<input
					id="type-label"
					type="text"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					placeholder="New Comment"
					bind:value={newType.label}
				/>
			</div>
			<div>
				<label for="type-desc" class="mb-1 block text-sm font-medium">{t('notification_types.description')}</label>
				<textarea
					id="type-desc"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					rows="2"
					bind:value={newType.description}
				></textarea>
			</div>
			<div class="flex justify-end gap-2">
				<button
					class="rounded-md px-4 py-2 text-sm font-medium"
					style="border: 1px solid var(--color-border)"
					onclick={() => { showCreateModal = false; }}
				>
					{t('common.cancel')}
				</button>
				<button
					class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
					style="background-color: var(--color-primary)"
					disabled={saving || !newType.key || !newType.label}
					onclick={save}
				>
					{saving ? t('common.saving') : t('common.save')}
				</button>
			</div>
		</div>
	</Modal>

	<Modal bind:open={showDeleteConfirm} title={t('common.confirmDelete')} confirmLabel={t('common.delete')} onConfirm={executeDelete}>
		<p class="text-sm" style="color: var(--color-muted)">
			{t('notification_types.confirm_delete')} <strong>{deleteTarget?.label}</strong>?
		</p>
	</Modal>
{/if}
