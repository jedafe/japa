<script lang="ts">
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { hasPermission } from '$lib/stores/permissions.svelte';

	type NotificationType = {
		key: string;
		label: string;
		description: string;
		enabled: boolean;
		default_in_app: boolean;
		default_email: boolean;
		default_push: boolean;
	};

	let types = $state<NotificationType[]>([]);
	let loading = $state(true);
	let error = $state('');
	let searchQuery = $state('');
	let showCreateModal = $state(false);
	let showDeleteConfirm = $state(false);
	let editingType = $state<NotificationType | null>(null);
	let deleteTarget = $state<NotificationType | null>(null);
	let newType = $state({
		key: '',
		label: '',
		description: '',
		enabled: true,
		default_in_app: true,
		default_email: false,
		default_push: false
	});
	let saving = $state(false);

	const canManageNotificationTypes = $derived(hasPermission('notifications.manage'));

	const columns = [
		{ key: 'key', label: t('notification_types.key'), sortable: true, render: (v: string) => `<span class="font-mono text-xs">${v}</span>` },
		{ key: 'label', label: t('notification_types.label'), sortable: true },
		{ key: 'description', label: t('common.description'), render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v || '-'}</span>` },
		{
			key: 'enabled',
			label: t('notification_types.enabled'),
			sortable: true,
			render: (_: any, row: NotificationType) => row.enabled
				? `<span class="rounded px-1.5 py-0.5 text-xs bg-success/15 text-success">${t('common.status_active')}</span>`
				: `<span class="rounded px-1.5 py-0.5 text-xs bg-danger/15 text-danger">${t('common.status_inactive')}</span>`
		},
		{
			key: 'default_channels',
			label: t('notification_types.channels'),
			render: (_: any, row: NotificationType) => {
				const badges: string[] = [];
				if (row.default_in_app) badges.push(`<span class="rounded px-1.5 py-0.5 text-xs bg-success/15 text-success">${t('notification-types.in_app')}</span>`);
				if (row.default_email) badges.push(`<span class="rounded px-1.5 py-0.5 text-xs bg-info/15 text-info">${t('notification-types.email')}</span>`);
				if (row.default_push) badges.push(`<span class="rounded px-1.5 py-0.5 text-xs bg-info/15 text-info">${t('notification-types.push')}</span>`);
				return `<div class="flex gap-1">${badges.join('') || '-'}</div>`;
			}
		},
		{
			key: 'actions',
			label: t('common.actions'),
			componentSnippet: NtActionsCell
		}
	];

	const filteredTypes = $derived.by(() => {
		let items = types;
		if (searchQuery) {
			const q = searchQuery.toLowerCase();
			items = items.filter((nt) => nt.key.toLowerCase().includes(q) || nt.label.toLowerCase().includes(q) || nt.description?.toLowerCase().includes(q));
		}
		return items;
	});

	function confirmDelete(ntype: NotificationType) {
		deleteTarget = ntype;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		error = '';
		try {
			const result = await api(`/admin/notification-types/${deleteTarget.key}`, { method: 'DELETE' });
			if (!result.ok) {
				error = result.error;
				return;
			}
			showDeleteConfirm = false;
			deleteTarget = null;
			await load();
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
		newType = {
			key: '',
			label: '',
			description: '',
			enabled: true,
			default_in_app: true,
			default_email: false,
			default_push: false
		};
		editingType = null;
		showCreateModal = true;
	}

	function openEdit(ntype: NotificationType) {
		editingType = { ...ntype };
		newType = {
			key: ntype.key,
			label: ntype.label,
			description: ntype.description,
			enabled: ntype.enabled,
			default_in_app: ntype.default_in_app,
			default_email: ntype.default_email,
			default_push: ntype.default_push
		};
		showCreateModal = true;
	}

	async function save() {
		if (!newType.key || !newType.label) return;
		saving = true;
		error = '';
		try {
			const result = editingType
				? await api(`/admin/notification-types/${editingType.key}`, {
					method: 'PATCH',
					body: {
						label: newType.label,
						description: newType.description || null,
						enabled: newType.enabled,
						default_in_app: newType.default_in_app,
						default_email: newType.default_email,
						default_push: newType.default_push
					}
				})
				: await api('/admin/notification-types', {
					method: 'POST',
					body: {
						key: newType.key,
						label: newType.label,
						description: newType.description || null,
						enabled: newType.enabled,
						default_in_app: newType.default_in_app,
						default_email: newType.default_email,
						default_push: newType.default_push
					}
				});
			if (!result.ok) {
				error = result.error;
				return;
			}
			showCreateModal = false;
			await load();
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
	<DataTable
		columns={columns}
		data={filteredTypes}
		rowKey={(row: NotificationType) => row.key}
		pageSize={25}
		emptyMessage={t('notification_types.no_types')}
		searchable
		onsearch={(q) => { searchQuery = q; }}
	/>
{/if}

{#if showCreateModal}
	<Modal
		bind:open={showCreateModal}
		title={editingType ? t('notification_types.edit') : t('notification_types.create')}
		onConfirm={save}
		confirmLabel={t('common.save')}
	>
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
				<label for="type-desc" class="mb-1 block text-sm font-medium">{t('common.description')}</label>
				<textarea
					id="type-desc"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					rows="2"
					bind:value={newType.description}
				></textarea>
			</div>
			<div class="flex items-center gap-2">
				<input
					id="type-enabled"
					type="checkbox"
					class="h-4 w-4 rounded"
					bind:checked={newType.enabled}
				/>
				<label for="type-enabled" class="text-sm font-medium">{t('notification_types.enabled')}</label>
			</div>
			<div>
				<span class="mb-1 block text-sm font-medium">{t('notification_types.default_channels')}</span>
				<p class="mb-2 text-xs" style="color: var(--color-muted)">{t('notification_types.default_channels_hint')}</p>
				<div class="flex flex-wrap gap-4">
					<label class="flex items-center gap-2 text-sm">
						<input type="checkbox" class="h-4 w-4 rounded" bind:checked={newType.default_in_app} />
						{t('notification-types.in_app')}
					</label>
					<label class="flex items-center gap-2 text-sm">
						<input type="checkbox" class="h-4 w-4 rounded" bind:checked={newType.default_email} />
						{t('notification-types.email')}
					</label>
					<label class="flex items-center gap-2 text-sm">
						<input type="checkbox" class="h-4 w-4 rounded" bind:checked={newType.default_push} />
						{t('notification-types.push')}
					</label>
				</div>
			</div>
		</div>
	</Modal>
{/if}

{#if showDeleteConfirm}
	<Modal bind:open={showDeleteConfirm} title={t('common.confirmDelete')} confirmLabel={t('common.delete')} onConfirm={executeDelete}>
		<p class="text-sm" style="color: var(--color-muted)">
			{t('notification_types.confirm_delete')} <strong>{deleteTarget?.label}</strong>?
		</p>
	</Modal>
{/if}

{#snippet NtActionsCell({ value, row }: { value: any; row: NotificationType })}
	<div class="flex items-center justify-end gap-2">
		{#if canManageNotificationTypes}
			<button class="text-xs" style="color: var(--color-primary)" onclick={() => openEdit(row)}>
				{t('common.edit')}
			</button>
			<button class="text-xs" style="color: var(--color-danger)" onclick={() => confirmDelete(row)}>
				{t('common.delete')}
			</button>
		{/if}
	</div>
{/snippet}
