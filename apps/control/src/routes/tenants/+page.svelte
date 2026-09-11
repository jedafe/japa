<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import SearchInput from '$lib/components/ui/SearchInput.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { api } from '$lib/api';
	import { t, translations } from '$lib/stores/i18n.svelte';
	import { escapeHtml } from '$lib/utils';
	import type { DataTableColumn, DataTableRowAction, DataTableBulkAction, Tenant } from '$lib/api/types';
	import { Dialog, Select } from 'bits-ui';
	import { Building, Plus, Pencil, Trash2, PauseCircle, PlayCircle, Settings } from 'lucide-svelte';
	import TenantSettingsDialog from '$lib/components/TenantSettingsDialog.svelte';

	const DEFAULT_TENANT_ID = '00000000-0000-0000-0000-000000000001';

	let tenants = $state<Tenant[]>([]);
	let loading = $state(true);
	let apiAvailable = $state(true);
	let errorMessage = $state('');

	let filter = $state('');
	let statusFilter = $state('all');

	let page = $state(0);
	let pageSize = $state(10);
	let sortKey = $state('');
	let sortDirection = $state<'asc' | 'desc'>('asc');
	let selectedIds = $state(new Set<string>());

	let createModalOpen = $state(false);
	let createName = $state('');
	let createSlug = $state('');
	let createDomain = $state('');
	let createLoading = $state(false);

	let editModalOpen = $state(false);
	let editForm = $state<{ id: string; name: string; domain: string }>({ id: '', name: '', domain: '' });
	let editLoading = $state(false);

	let confirmOpen = $state(false);
	let confirmType = $state<'delete' | 'suspend' | 'unsuspend' | 'bulk-delete' | 'bulk-suspend' | 'bulk-unsuspend'>('delete');
	let confirmTenant = $state<Tenant | null>(null);
	let confirming = $state(false);

	let toggling = $state<string | null>(null);
	let settingsTenant = $state<Tenant | null>(null);
	let settingsDialogOpen = $state(false);

	const statusOptions = $derived([
		{ value: 'all', label: t('tenants.status_all', 'All Status') },
		{ value: 'active', label: t('tenants.status_active', 'Active') },
		{ value: 'suspended', label: t('tenants.status_suspended', 'Suspended') }
	]);

	async function load() {
		loading = true;
		errorMessage = '';
		const status = statusFilter === 'all' ? '' : statusFilter;
		try {
			const result = await api<{ data: Tenant[]; total: number }>(
				`/control/tenants?status=${encodeURIComponent(status)}&page_size=1000&page_offset=0`
			);
			if (result.ok) {
				tenants = Array.isArray(result.data?.data) ? result.data.data : [];
				apiAvailable = true;
			} else {
				apiAvailable = false;
				errorMessage = result.error || t('tenants.load_error', 'Failed to load tenants.');
			}
		} catch {
			apiAvailable = false;
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		load();
	});

	function slugify(name: string): string {
		return name
			.toLowerCase()
			.trim()
			.replace(/[^\w\s-]/g, '')
			.replace(/\s+/g, '-')
			.replace(/-+/g, '-')
			.slice(0, 64);
	}

	function statusClasses(status: string): string {
		switch (status) {
			case 'active':
				return 'bg-emerald-500/10 text-emerald-600';
			case 'suspended':
				return 'bg-amber-500/10 text-amber-500';
			case 'deleted':
				return 'bg-destructive/10 text-destructive';
			default:
				return 'bg-muted text-muted-foreground';
		}
	}

	function isDefault(t: Tenant | null): boolean {
		return t?.id === DEFAULT_TENANT_ID;
	}

	let filtered = $derived.by<Tenant[]>(() => {
		translations();
		let list = tenants;
		if (filter.trim()) {
			const q = filter.toLowerCase();
			list = list.filter((t) =>
				t.name.toLowerCase().includes(q) ||
				t.slug.toLowerCase().includes(q) ||
				(t.domain ?? '').toLowerCase().includes(q)
			);
		}
		if (sortKey) {
			list = [...list].sort((a, b) => {
				const aVal = (a as unknown as Record<string, unknown>)[sortKey];
				const bVal = (b as unknown as Record<string, unknown>)[sortKey];
				const cmp = String(aVal ?? '').localeCompare(String(bVal ?? ''));
				return sortDirection === 'asc' ? cmp : -cmp;
			});
		}
		return list;
	});

	let totalRows = $derived(filtered.length);

	let columns = $derived.by<DataTableColumn[]>(() => {
		translations();
		return [
			{
				key: 'name',
				label: t('tenants.name', 'Name'),
				sortable: true,
				render: (_val, row) => `<span class="font-medium">${escapeHtml((row as unknown as Tenant).name)}</span>`
			},
			{
				key: 'slug',
				label: t('tenants.slug', 'Slug'),
				sortable: true,
				render: (val) => `<span class="font-mono text-xs text-muted-foreground">${escapeHtml(String(val))}</span>`
			},
			{
				key: 'domain',
				label: t('tenants.domain', 'Domain'),
				sortable: true,
				render: (val) => val ? `<span class="text-muted-foreground text-xs">${escapeHtml(String(val))}</span>` : '—'
			},
			{
				key: 'status',
				label: t('tenants.status', 'Status'),
				sortable: true,
				render: (val) => {
					const status = escapeHtml(String(val));
					return `<span class="${statusClasses(status)} inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium">${status}</span>`;
				}
			},
			{
				key: 'created_at',
				label: t('tenants.created', 'Created'),
				sortable: true,
				render: (val) => val ? `<span class="text-muted-foreground text-xs">${new Date(String(val)).toLocaleDateString()}</span>` : '—'
			}
		];
	});

	let rowActions = $derived.by<DataTableRowAction[]>(() => {
		translations();
		return [
			{
				label: t('tenants.edit_tenant', 'Edit tenant'),
				icon: Pencil,
				onClick: (row) => openEditModal(row as unknown as Tenant),
				disabled: (row) => (row as unknown as Tenant).status === 'deleted'
			},
			{
				label: t('tenants.settings', 'Settings'),
				icon: Settings,
				onClick: (row) => openSettings(row as unknown as Tenant)
			},
			{
				label: t('tenants.suspend_tenant', 'Suspend tenant'),
				icon: PauseCircle,
				onClick: (row) => openConfirm('suspend', row as unknown as Tenant),
				hidden: (row) => (row as unknown as Tenant).status !== 'active',
				disabled: (row) => isDefault(row as unknown as Tenant)
			},
			{
				label: t('tenants.unsuspend_tenant', 'Unsuspend tenant'),
				icon: PlayCircle,
				onClick: (row) => openConfirm('unsuspend', row as unknown as Tenant),
				hidden: (row) => (row as unknown as Tenant).status !== 'suspended',
				disabled: (row) => isDefault(row as unknown as Tenant)
			},
			{
				label: t('tenants.delete_tenant', 'Delete tenant'),
				icon: Trash2,
				variant: 'destructive',
				onClick: (row) => openConfirm('delete', row as unknown as Tenant),
				hidden: (row) => (row as unknown as Tenant).status === 'deleted',
				disabled: (row) => isDefault(row as unknown as Tenant)
			}
		];
	});

	let bulkActions = $derived.by<DataTableBulkAction[]>(() => {
		translations();
		return [
			{
				label: t('tenants.suspend_selected', 'Suspend Selected'),
				icon: PauseCircle,
				onClick: (ids) => openBulkConfirm('bulk-suspend', ids)
			},
			{
				label: t('tenants.unsuspend_selected', 'Unsuspend Selected'),
				icon: PlayCircle,
				onClick: (ids) => openBulkConfirm('bulk-unsuspend', ids)
			},
			{
				label: t('tenants.delete_selected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onClick: (ids) => openBulkConfirm('bulk-delete', ids)
			}
		];
	});

	function openCreateModal() {
		createName = '';
		createSlug = '';
		createDomain = '';
		errorMessage = '';
		createModalOpen = true;
	}

	function openSettings(tenant: Tenant) {
		settingsTenant = tenant;
		settingsDialogOpen = true;
	}

	function openEditModal(tenant: Tenant) {
		editForm = { id: tenant.id, name: tenant.name, domain: tenant.domain ?? '' };
		errorMessage = '';
		editModalOpen = true;
	}

	function openConfirm(type: typeof confirmType, tenant: Tenant) {
		confirmType = type;
		confirmTenant = tenant;
		confirmOpen = true;
	}

	function openBulkConfirm(type: 'bulk-delete' | 'bulk-suspend' | 'bulk-unsuspend', ids: string[]) {
		confirmType = type;
		confirmTenant = null;
		pendingBulkIds = ids;
		confirmOpen = true;
	}

	let pendingBulkIds = $state<string[]>([]);

	async function handleCreate() {
		const name = createName.trim();
		if (!name) {
			errorMessage = t('tenants.name_required', 'Name is required.');
			return;
		}
		createLoading = true;
		errorMessage = '';
		const slug = createSlug.trim() || slugify(name);
		const domain = createDomain.trim() || undefined;
		const result = await api<Tenant>('/control/tenants', {
			method: 'POST',
			body: { name, slug, domain: domain || null }
		});
		createLoading = false;
		if (result.ok) {
			createModalOpen = false;
			createName = '';
			createSlug = '';
			createDomain = '';
			await load();
		} else {
			errorMessage = result.error || t('tenants.create_error', 'Failed to create tenant.');
		}
	}

	async function handleUpdate() {
		const name = editForm.name.trim();
		if (!name || !editForm.id) {
			errorMessage = t('tenants.name_required', 'Name is required.');
			return;
		}
		editLoading = true;
		errorMessage = '';
		const domain = editForm.domain.trim() || null;
		const result = await api<Tenant>(`/control/tenants/${editForm.id}`, {
			method: 'PATCH',
			body: { name, domain }
		});
		editLoading = false;
		if (result.ok) {
			editModalOpen = false;
			tenants = tenants.map((t) => (t.id === editForm.id ? { ...t, name, domain } : t));
		} else {
			errorMessage = result.error || t('tenants.save_error', 'Failed to save tenant.');
		}
	}

	async function handleConfirmAction() {
		if (confirmType.startsWith('bulk-')) {
			await handleBulkAction();
			return;
		}
		if (!confirmTenant) return;
		const id = confirmTenant.id;
		confirming = true;
		try {
			if (confirmType === 'delete') {
				const result = await api(`/control/tenants/${id}`, { method: 'DELETE' });
				if (result.ok) {
					tenants = tenants.map((t) => (t.id === id ? { ...t, status: 'deleted' } : t));
					confirmOpen = false;
				} else {
					errorMessage = result.error || t('tenants.delete_error', 'Failed to delete tenant.');
				}
			} else if (confirmType === 'suspend') {
				toggling = id;
				const result = await api(`/control/tenants/${id}/suspend`, { method: 'POST' });
				toggling = null;
				if (result.ok) {
					tenants = tenants.map((t) => (t.id === id ? { ...t, status: 'suspended' } : t));
					confirmOpen = false;
				} else {
					errorMessage = result.error || t('tenants.suspend_error', 'Failed to suspend tenant.');
				}
			} else if (confirmType === 'unsuspend') {
				toggling = id;
				const result = await api(`/control/tenants/${id}/unsuspend`, { method: 'POST' });
				toggling = null;
				if (result.ok) {
					tenants = tenants.map((t) => (t.id === id ? { ...t, status: 'active' } : t));
					confirmOpen = false;
				} else {
					errorMessage = result.error || t('tenants.unsuspend_error', 'Failed to unsuspend tenant.');
				}
			}
		} finally {
			confirming = false;
		}
	}

	async function handleBulkAction() {
		confirming = true;
		const ids = pendingBulkIds;
		try {
			if (confirmType === 'bulk-delete') {
				await Promise.allSettled(ids.map((id) => api(`/control/tenants/${id}`, { method: 'DELETE' })));
				tenants = tenants.map((t) => (ids.includes(t.id) ? { ...t, status: 'deleted' } : t));
			} else if (confirmType === 'bulk-suspend') {
				await Promise.allSettled(ids.map((id) => api(`/control/tenants/${id}/suspend`, { method: 'POST' })));
				tenants = tenants.map((t) => (ids.includes(t.id) ? { ...t, status: 'suspended' } : t));
			} else if (confirmType === 'bulk-unsuspend') {
				await Promise.allSettled(ids.map((id) => api(`/control/tenants/${id}/unsuspend`, { method: 'POST' })));
				tenants = tenants.map((t) => (ids.includes(t.id) ? { ...t, status: 'active' } : t));
			}
			selectedIds = new Set();
			confirmOpen = false;
		} finally {
			confirming = false;
		}
	}

	let confirmTitle = $derived.by(() => {
		translations();
		switch (confirmType) {
			case 'delete':
			case 'bulk-delete':
				return t('tenants.delete_title', 'Delete Tenant');
			case 'suspend':
			case 'bulk-suspend':
				return t('tenants.suspend_title', 'Suspend Tenant');
			case 'unsuspend':
			case 'bulk-unsuspend':
				return t('tenants.unsuspend_title', 'Unsuspend Tenant');
			default:
				return '';
		}
	});

	let confirmConsequence = $derived.by(() => {
		translations();
		if (confirmType.startsWith('bulk-')) {
			const count = pendingBulkIds.length;
			if (confirmType === 'bulk-delete') return t('tenants.bulk_delete_confirm', 'Delete {count} selected tenants? This cannot be undone.').replace('{count}', String(count));
			if (confirmType === 'bulk-suspend') return t('tenants.bulk_suspend_confirm', 'Suspend {count} selected tenants?').replace('{count}', String(count));
			if (confirmType === 'bulk-unsuspend') return t('tenants.bulk_unsuspend_confirm', 'Unsuspend {count} selected tenants?').replace('{count}', String(count));
		}
		if (!confirmTenant) return '';
		if (confirmType === 'delete') return t('tenants.delete_confirm', "Permanently delete tenant '{name}' ({slug}). This cannot be undone.").replace('{name}', confirmTenant.name).replace('{slug}', confirmTenant.slug);
		if (confirmType === 'suspend') return t('tenants.suspend_confirm', "This will suspend tenant '{name}'. Users will lose access.").replace('{name}', confirmTenant.name);
		if (confirmType === 'unsuspend') return t('tenants.unsuspend_confirm', "This will restore tenant '{name}'.").replace('{name}', confirmTenant.name);
		return '';
	});

	let confirmText = $derived.by(() => {
		translations();
		switch (confirmType) {
			case 'delete':
			case 'bulk-delete':
				return t('tenants.delete_text', 'DELETE');
			case 'suspend':
			case 'bulk-suspend':
				return t('tenants.suspend_text', 'SUSPEND');
			case 'unsuspend':
			case 'bulk-unsuspend':
				return t('tenants.unsuspend_text', 'UNSUSPEND');
			default:
				return '';
		}
	});

	function handleStatusFilterChange(value: string | undefined) {
		if (value !== undefined) {
			statusFilter = value;
			page = 0;
			load();
		}
	}
</script>

<PageHeader title={t('tenants.title')} description={t('tenants.description')} />

{#if errorMessage}
	<div class="border-destructive bg-destructive/10 text-destructive mb-4 rounded-xl border px-4 py-3 text-sm">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex flex-wrap items-center gap-3">
	<SearchInput
		bind:value={filter}
		placeholder={t('tenants.search_placeholder', 'Search tenants...')}
		class="w-72"
	/>

	<Select.Root type="single" bind:value={statusFilter} items={statusOptions} onValueChange={handleStatusFilterChange}>
		<Select.Trigger class="border-border-input bg-background rounded-9px border px-3 py-2 text-sm" aria-label={t('tenants.status')}>
			<Select.Value placeholder={t('tenants.status_all')} />
		</Select.Trigger>
		<Select.Portal>
			<Select.Content class="border-border-input bg-background shadow-popover z-50 rounded-xl border px-1 py-1">
				<Select.Viewport class="p-1">
					{#each statusOptions as item}
						<Select.Item
							class="data-highlighted:bg-accent rounded-9px flex h-9 cursor-pointer items-center px-3 text-sm outline-none"
							value={item.value}
							label={item.label}
						>
							{#snippet children({ selected })}
								{item.label}
								{#if selected}<span class="ml-auto">✓</span>{/if}
							{/snippet}
						</Select.Item>
					{/each}
				</Select.Viewport>
			</Select.Content>
		</Select.Portal>
	</Select.Root>

	<span class="text-muted-foreground text-xs">
		{totalRows} {totalRows !== 1 ? t('tenants.tenant_count', 'tenants') : t('tenants.tenant_count', 'tenant')}
	</span>

	<div class="flex-1"></div>
	<button
		class="bg-foreground text-background flex items-center gap-1.5 rounded-9px px-3 py-2 text-sm font-medium transition-colors hover:opacity-90"
		onclick={openCreateModal}
	>
		<Plus class="h-4 w-4" />
		{t('tenants.create_tenant', 'Create Tenant')}
	</button>
</div>

{#if !apiAvailable && !loading}
	<div class="text-muted-foreground py-8 text-center text-sm">{t('tenants.no_tenants', 'No tenants exist yet.')}</div>
{/if}

<DataTable
	columns={columns}
	rows={filtered}
	{totalRows}
	{loading}
	bind:page
	bind:pageSize
	bind:sortKey
	bind:sortDirection
	bind:selectedIds
	rowActions={rowActions}
	bulkActions={bulkActions}
	emptyMessage={t('tenants.no_match', 'No tenants match your filter.')}
	emptyIcon={Building}
/>

<Dialog.Root bind:open={createModalOpen}>
	<Dialog.Portal>
		<Dialog.Overlay class="bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out fixed inset-0 z-50" />
		<Dialog.Content
			class="border-border-card bg-background shadow-popover fixed left-1/2 top-1/2 z-50 w-full max-w-lg -translate-x-1/2 -translate-y-1/2 rounded-xl border p-6 focus:outline-none"
			role="dialog"
			aria-modal="true"
			onOpenAutoFocus={(e) => e.preventDefault()}
		>
			<Dialog.Title class="text-foreground text-lg font-semibold">{t('tenants.create_title', 'Create Tenant')}</Dialog.Title>
			<div class="mt-4 space-y-4">
				<div>
					<label for="tenant-name" class="text-foreground mb-1 block text-sm font-medium">{t('tenants.name_label', 'Name *')}</label>
					<input
						id="tenant-name"
						type="text"
						bind:value={createName}
						class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						placeholder={t('tenants.name_placeholder', 'My Organization')}
					/>
				</div>
				<div>
					<label for="tenant-slug" class="text-foreground mb-1 block text-sm font-medium">{t('tenants.slug_label', 'Slug *')}</label>
					<input
						id="tenant-slug"
						type="text"
						bind:value={createSlug}
						class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						placeholder={t('tenants.slug_placeholder', 'my-organization')}
					/>
					<p class="text-muted-foreground mt-1 text-xs">{t('tenants.slug_hint', 'Auto-generated from name if left empty')}</p>
				</div>
				<div>
					<label for="tenant-domain" class="text-foreground mb-1 block text-sm font-medium">{t('tenants.domain_label', 'Domain')}</label>
					<input
						id="tenant-domain"
						type="text"
						bind:value={createDomain}
						class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						placeholder={t('tenants.domain_placeholder', 'example.com')}
					/>
				</div>
			</div>
			<div class="mt-6 flex justify-end gap-3">
				<Dialog.Close
					class="border-border-input bg-background hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors"
					onclick={() => (createModalOpen = false)}
				>
					{t('tenants.cancel', 'Cancel')}
				</Dialog.Close>
				<button
					class="bg-foreground text-background rounded-9px px-4 py-2 text-sm font-medium transition-colors hover:opacity-90 disabled:opacity-50"
					onclick={handleCreate}
					disabled={createLoading}
				>
					{createLoading ? t('tenants.creating', 'Creating...') : t('tenants.create', 'Create')}
				</button>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<Dialog.Root bind:open={editModalOpen}>
	<Dialog.Portal>
		<Dialog.Overlay class="bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out fixed inset-0 z-50" />
		<Dialog.Content
			class="border-border-card bg-background shadow-popover fixed left-1/2 top-1/2 z-50 w-full max-w-lg -translate-x-1/2 -translate-y-1/2 rounded-xl border p-6 focus:outline-none"
			role="dialog"
			aria-modal="true"
			onOpenAutoFocus={(e) => e.preventDefault()}
		>
			<Dialog.Title class="text-foreground text-lg font-semibold">{t('tenants.edit_title', 'Edit Tenant')}</Dialog.Title>
			<div class="mt-4 space-y-4">
				<div>
					<label for="edit-tenant-name" class="text-foreground mb-1 block text-sm font-medium">{t('tenants.name_label', 'Name *')}</label>
					<input
						id="edit-tenant-name"
						type="text"
						bind:value={editForm.name}
						class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
					/>
				</div>
				<div>
					<label for="edit-tenant-slug" class="text-foreground mb-1 block text-sm font-medium">{t('tenants.slug_label', 'Slug *')}</label>
					<input
						id="edit-tenant-slug"
						type="text"
						value={editForm.id}
						disabled
						class="border-border-input bg-muted text-muted-foreground w-full rounded-9px border px-3 py-2 text-sm"
					/>
					<p class="text-muted-foreground mt-1 text-xs">{t('tenants.slug_cannot_change', 'Slug cannot be changed')}</p>
				</div>
				<div>
					<label for="edit-tenant-domain" class="text-foreground mb-1 block text-sm font-medium">{t('tenants.domain_label', 'Domain')}</label>
					<input
						id="edit-tenant-domain"
						type="text"
						bind:value={editForm.domain}
						class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
					/>
				</div>
			</div>
			<div class="mt-6 flex justify-end gap-3">
				<Dialog.Close
					class="border-border-input bg-background hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors"
					onclick={() => (editModalOpen = false)}
				>
					{t('tenants.cancel', 'Cancel')}
				</Dialog.Close>
				<button
					class="bg-foreground text-background rounded-9px px-4 py-2 text-sm font-medium transition-colors hover:opacity-90 disabled:opacity-50"
					onclick={handleUpdate}
					disabled={editLoading}
				>
					{editLoading ? t('tenants.saving', 'Saving...') : t('tenants.save_changes', 'Save Changes')}
				</button>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<ConfirmModal
	bind:open={confirmOpen}
	title={confirmTitle}
	consequence={confirmConsequence}
	confirmText={confirmText}
	onConfirm={handleConfirmAction}
/>

{#if settingsTenant}
	<TenantSettingsDialog
		bind:open={settingsDialogOpen}
		tenantId={settingsTenant.id}
		tenantName={settingsTenant.name}
	/>
{/if}
