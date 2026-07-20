<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import SearchInput from '$lib/components/ui/SearchInput.svelte';
	import { Dialog, Select } from 'bits-ui';
	import { api } from '$lib/api';
	import type { Component } from 'svelte';
	import type { Tenant, DataTableColumn, DataTableRowAction, DataTableBulkAction } from '$lib/api/types';
	import { t } from '$lib/stores/i18n.svelte';
	import { escapeHtml } from '$lib/utils';
	import { Pencil, Ban, CheckCircle, Trash2, Plus, X, RefreshCw } from 'lucide-svelte';

	let {} = $props();

	let tenants = $state<Tenant[]>([]);
	let loading = $state(true);
	let filter = $state('');
	let filterStatus = $state('all');
	let errorMessage = $state('');
	let saving = $state(false);

	let page = $state(0);
	let pageSize = $state(10);
	let sortKey = $state('');
	let sortDirection = $state<'asc' | 'desc'>('asc');
	let selectedIds = $state(new Set<string>());

	let showCreateModal = $state(false);
	let createName = $state('');
	let createSlug = $state('');
	let createDomain = $state('');

	let showEditModal = $state(false);
	let editingTenant = $state<Tenant | null>(null);
	let editName = $state('');
	let editDomain = $state('');

	let confirmOpen = $state(false);
	let confirmAction = $state<
		| { type: 'suspend'; tenant: Tenant }
		| { type: 'unsuspend'; tenant: Tenant }
		| { type: 'delete'; tenant: Tenant }
		| null
	>(null);

	const statusItems = [
		{ value: 'all', label: t('tenants.status_all', 'All Status') },
		{ value: 'active', label: t('tenants.status_active', 'Active') },
		{ value: 'suspended', label: t('tenants.status_suspended', 'Suspended') }
	];

	async function load() {
		loading = true;
		errorMessage = '';
		const result = await api<Tenant[]>('/control/tenants');
		if (result.ok) {
			tenants = result.data;
		} else {
			errorMessage = result.error;
		}
		loading = false;
	}

	$effect(() => {
		load();
	});

	function autoSlug(name: string): string {
		return name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
	}

	function openCreate() {
		createName = '';
		createSlug = '';
		createDomain = '';
		showCreateModal = true;
	}

	async function createTenant() {
		saving = true;
		errorMessage = '';
		const slug = createSlug || autoSlug(createName);
		if (createDomain && !/^([a-z0-9-]+\.)*[a-z0-9-]+\.[a-z]+$/.test(createDomain)) {
			errorMessage = 'Please enter a valid domain (e.g. example.com).';
			saving = false;
			return;
		}
		const result = await api<Tenant>('/control/tenants', {
			method: 'POST',
			body: { name: createName, slug, domain: createDomain || undefined }
		});
		if (result.ok) {
			tenants = [result.data, ...tenants];
			showCreateModal = false;
		} else {
			errorMessage = result.error;
		}
		saving = false;
	}

	function openEdit(tenant: Tenant) {
		editingTenant = tenant;
		editName = tenant.name;
		editDomain = tenant.domain ?? '';
		showEditModal = true;
	}

	async function editTenant() {
		if (!editingTenant) return;
		saving = true;
		errorMessage = '';
		const result = await api<Tenant>(`/control/tenants/${editingTenant.id}`, {
			method: 'PATCH',
			body: { name: editName, domain: editDomain || undefined }
		});
		if (result.ok) {
			tenants = tenants.map((t) => (t.id === editingTenant!.id ? { ...t, ...result.data } : t));
			showEditModal = false;
			editingTenant = null;
		} else {
			errorMessage = result.error;
		}
		saving = false;
	}

	async function suspendTenant(id: string) {
		errorMessage = '';
		const result = await api<Tenant>(`/control/tenants/${id}/suspend`, { method: 'POST' });
		if (result.ok) {
			tenants = tenants.map((t) => (t.id === id ? { ...t, status: 'suspended' } : t));
		} else {
			errorMessage = result.error;
		}
	}

	async function unsuspendTenant(id: string) {
		errorMessage = '';
		const result = await api<Tenant>(`/control/tenants/${id}/unsuspend`, { method: 'POST' });
		if (result.ok) {
			tenants = tenants.map((t) => (t.id === id ? { ...t, status: 'active' } : t));
		} else {
			errorMessage = result.error;
		}
	}

	async function deleteTenant(id: string) {
		errorMessage = '';
		const result = await api<{ status: string }>(`/control/tenants/${id}`, { method: 'DELETE' });
		if (result.ok) {
			tenants = tenants.filter((t) => t.id !== id);
		} else {
			errorMessage = result.error;
		}
	}

	function openConfirm(type: 'suspend' | 'unsuspend' | 'delete', tenant: Tenant) {
		confirmAction = { type, tenant };
		confirmOpen = true;
	}

	let filtered = $derived(
		(filter || filterStatus !== 'all')
			? tenants.filter(
					(t) =>
						(filter
							? t.name.toLowerCase().includes(filter.toLowerCase()) ||
								t.slug.toLowerCase().includes(filter.toLowerCase()) ||
								(t.domain ?? '').toLowerCase().includes(filter.toLowerCase())
							: true) &&
						(filterStatus !== 'all' ? t.status === filterStatus : true)
				)
			: tenants
	);

	let sorted = $derived.by(() => {
		if (!sortKey) return filtered;
		const copy = [...filtered];
		copy.sort((a, b) => {
			const aVal = a[sortKey as keyof Tenant] ?? '';
			const bVal = b[sortKey as keyof Tenant] ?? '';
			const cmp = String(aVal).localeCompare(String(bVal));
			return sortDirection === 'asc' ? cmp : -cmp;
		});
		return copy;
	});

	let totalRows = $derived(filtered.length);

	let paginatedTenants = $derived.by(() => {
		const start = page * pageSize;
		return sorted.slice(start, start + pageSize);
	});

	const columns: DataTableColumn[] = [
		{ key: 'name', label: t('tenants.name'), sortable: true, class: 'font-medium' },
		{ key: 'slug', label: t('tenants.slug'), sortable: true, class: 'font-mono text-xs' },
		{ key: 'domain', label: t('tenants.domain'), render: (v) => v ? escapeHtml(v) : '\u2014', class: 'text-muted-foreground text-xs' },
		{
			key: 'status',
			label: t('tenants.status'),
			sortable: true,
			render: (v) => {
				const s = escapeHtml(v);
				const cls = s === 'active' ? 'bg-emerald-500/12 text-emerald-600' : s === 'suspended' ? 'bg-tertiary/12 text-tertiary' : '';
				return `<span class="rounded-full px-2 py-0.5 text-xs font-medium ${cls}">${s}</span>`;
			}
		},
		{
			key: 'created_at',
			label: t('tenants.created'),
			sortable: true,
			render: (v) => `<span>${escapeHtml(new Date(v as string).toLocaleDateString())}</span>`,
			class: 'text-muted-foreground text-xs'
		}
	];

	function buildRowActions(): DataTableRowAction[] {
		return [
			{
				label: t('tenants.edit_tenant'),
				icon: Pencil,
				onClick: (row) => openEdit(row as unknown as Tenant)
			},
			{
				label: t('tenants.suspend_tenant'),
				icon: Ban,
				onClick: (row) => openConfirm('suspend', row as unknown as Tenant),
				disabled: (row) => (row as unknown as Tenant).status !== 'active'
			},
			{
				label: t('tenants.unsuspend_tenant'),
				icon: CheckCircle,
				onClick: (row) => openConfirm('unsuspend', row as unknown as Tenant),
				disabled: (row) => (row as unknown as Tenant).status !== 'suspended'
			},
			{
				label: t('tenants.delete_tenant'),
				icon: Trash2,
				onClick: (row) => openConfirm('delete', row as unknown as Tenant),
				variant: 'destructive'
			}
		];
	}

	let rowActions = $derived(buildRowActions());

	let bulkActions: DataTableBulkAction[] = $derived([
		{
			label: t('tenants.suspend_selected', 'Suspend Selected'),
			icon: Ban,
			onClick: async (ids: string[]) => {
				for (const id of ids) {
					await suspendTenant(id);
				}
				selectedIds = new Set();
			}
		},
		{
			label: t('tenants.delete_selected', 'Delete Selected'),
			icon: Trash2,
			variant: 'destructive',
			onClick: async (ids: string[]) => {
				for (const id of ids) {
					await deleteTenant(id);
				}
				selectedIds = new Set();
			}
		}
	]);

	function resetCreateForm() {
		createName = '';
		createSlug = '';
		createDomain = '';
	}

	function resetEditForm() {
		editName = '';
		editDomain = '';
		editingTenant = null;
	}
</script>

<PageHeader title={t('tenants.title')} description={t('tenants.description')} />

{#if errorMessage}
	<div class="border-destructive/30 bg-destructive/6 text-destructive mb-4 rounded-lg border p-3 text-sm">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex items-center gap-3">
	<SearchInput
		bind:value={filter}
		placeholder={t('tenants.search_placeholder')}
		class="w-72"
	/>
	<Select.Root type="single" bind:value={filterStatus} items={statusItems}>
		<Select.Trigger
			class="border-border-input bg-background data-placeholder:text-muted-foreground/50 rounded-9px flex h-10 items-center border px-3 text-sm transition-colors hover:border-border-input-hover"
			aria-label={t('tenants.status')}
			onclick={() => { page = 0; }}
		>
			<Select.Value placeholder={t('tenants.status_all')} />
		</Select.Trigger>
		<Select.Portal>
			<Select.Content class="border-border-input bg-background shadow-popover z-50 rounded-xl border px-1 py-1" sideOffset={4}>
				<Select.Viewport class="p-1">
					{#each statusItems as item}
						<Select.Item class="data-highlighted:bg-accent rounded-9px flex h-9 cursor-pointer items-center px-3 text-sm outline-none" value={item.value} label={item.label}>
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
	<div class="flex-1"></div>
	<button class="bg-foreground hover:bg-foreground/90 text-background flex items-center gap-1.5 rounded-9px px-3 py-2 text-xs font-medium transition-colors" onclick={openCreate}>
		<Plus class="size-3" />
		{t('tenants.create_tenant')}
	</button>
	<button class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border flex items-center gap-1.5 px-3 py-2 text-xs font-medium transition-colors" onclick={load}>
		<RefreshCw class="size-3" />
		{t('common.refresh')}
	</button>
</div>

<DataTable
	{columns}
	rows={paginatedTenants as unknown as Record<string, unknown>[]}
	{totalRows}
	bind:page
	bind:pageSize
	bind:sortKey
	bind:sortDirection
	bind:selectedIds
	{rowActions}
	{bulkActions}
	{loading}
	emptyMessage={t('tenants.no_tenants', 'No tenants found')}
/>

<Dialog.Root bind:open={showCreateModal} onOpenChange={(isOpen) => { if (!isOpen) resetCreateForm(); }}>
	<Dialog.Portal>
		<Dialog.Overlay class="bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out fixed inset-0 z-50" />
		<Dialog.Content class="border-border-card bg-background shadow-popover fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-xl border p-6" onOpenAutoFocus={(e) => e.preventDefault()}>
			<div class="mb-4 flex items-center justify-between">
				<Dialog.Title class="text-foreground text-lg font-semibold">{t('tenants.create_title')}</Dialog.Title>
				<Dialog.Close class="hover:bg-dark-04 rounded-9px p-1.5 transition-colors"><X class="size-4" /></Dialog.Close>
			</div>
			<form onsubmit={(e) => { e.preventDefault(); createTenant(); }}>
				<div class="space-y-4">
					<div>
						<label for="create-name" class="text-foreground mb-1 block text-sm font-medium">{t('tenants.name_label')}</label>
						<input id="create-name" type="text" bind:value={createName} required class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus" placeholder={t('tenants.name_placeholder')} oninput={() => { if (!createSlug) createSlug = autoSlug(createName); }} />
					</div>
					<div>
						<label for="create-slug" class="text-foreground mb-1 block text-sm font-medium">{t('tenants.slug_label')}</label>
						<input id="create-slug" type="text" bind:value={createSlug} required class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm" placeholder={t('tenants.slug_placeholder')} />
						<p class="text-muted-foreground mt-1 text-xs">{t('tenants.slug_hint')}</p>
					</div>
					<div>
						<label for="create-domain" class="text-foreground mb-1 block text-sm font-medium">{t('tenants.domain_label')}</label>
						<input id="create-domain" type="text" bind:value={createDomain} class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm" placeholder={t('tenants.domain_placeholder')} />
					</div>
				</div>
				<div class="mt-6 flex justify-end gap-3">
					<Dialog.Close class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium">{t('common.cancel')}</Dialog.Close>
					<button type="submit" class="bg-foreground hover:bg-foreground/90 text-background rounded-9px px-4 py-2 text-sm font-medium disabled:opacity-50" disabled={saving || !createName.trim()}>
						{saving ? t('tenants.creating') : t('tenants.create')}
					</button>
				</div>
			</form>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<Dialog.Root bind:open={showEditModal} onOpenChange={(isOpen) => { if (!isOpen) resetEditForm(); }}>
	<Dialog.Portal>
		<Dialog.Overlay class="bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out fixed inset-0 z-50" />
		<Dialog.Content class="border-border-card bg-background shadow-popover fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-xl border p-6" onOpenAutoFocus={(e) => e.preventDefault()}>
			<div class="mb-4 flex items-center justify-between">
				<Dialog.Title class="text-foreground text-lg font-semibold">{t('tenants.edit_title')}</Dialog.Title>
				<Dialog.Close class="hover:bg-dark-04 rounded-9px p-1.5 transition-colors"><X class="size-4" /></Dialog.Close>
			</div>
			<form onsubmit={(e) => { e.preventDefault(); editTenant(); }}>
				<div class="space-y-4">
					<div>
						<label for="edit-name" class="text-foreground mb-1 block text-sm font-medium">{t('tenants.name')}</label>
						<input id="edit-name" type="text" bind:value={editName} required class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm" />
					</div>
					<div>
						<label for="edit-slug" class="text-foreground mb-1 block text-sm font-medium">{t('tenants.slug')}</label>
						<input id="edit-slug" type="text" value={editingTenant?.slug ?? ''} disabled class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm opacity-60" />
						<p class="text-muted-foreground mt-1 text-xs">{t('tenants.slug_cannot_change')}</p>
					</div>
					<div>
						<label for="edit-domain" class="text-foreground mb-1 block text-sm font-medium">{t('tenants.domain')}</label>
						<input id="edit-domain" type="text" bind:value={editDomain} class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm" placeholder={t('tenants.domain_placeholder')} />
					</div>
				</div>
				<div class="mt-6 flex justify-end gap-3">
					<Dialog.Close class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium">{t('common.cancel')}</Dialog.Close>
					<button type="submit" class="bg-foreground hover:bg-foreground/90 text-background rounded-9px px-4 py-2 text-sm font-medium disabled:opacity-50" disabled={saving || !editName.trim()}>
						{saving ? t('tenants.saving') : t('tenants.save_changes')}
					</button>
				</div>
			</form>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

{#if confirmAction}
	{#if confirmAction.type === 'suspend'}
		<ConfirmModal bind:open={confirmOpen} title={t('tenants.suspend_title')} consequence={t('tenants.suspend_confirm').replace('{name}', confirmAction.tenant.name)} confirmText={t('tenants.suspend_text')} onConfirm={async () => { if (confirmAction?.type === 'suspend') await suspendTenant(confirmAction.tenant.id); confirmAction = null; }} onCancel={() => (confirmAction = null)} />
	{:else if confirmAction.type === 'unsuspend'}
		<ConfirmModal bind:open={confirmOpen} title={t('tenants.unsuspend_title')} consequence={t('tenants.unsuspend_confirm').replace('{name}', confirmAction.tenant.name)} confirmText={t('tenants.unsuspend_text')} onConfirm={async () => { if (confirmAction?.type === 'unsuspend') await unsuspendTenant(confirmAction.tenant.id); confirmAction = null; }} onCancel={() => (confirmAction = null)} />
	{:else if confirmAction.type === 'delete'}
		<ConfirmModal bind:open={confirmOpen} title={t('tenants.delete_title')} consequence={t('tenants.delete_confirm').replace('{name}', confirmAction.tenant.name).replace('{slug}', confirmAction.tenant.slug)} confirmText={t('tenants.delete_text')} onConfirm={async () => { if (confirmAction?.type === 'delete') await deleteTenant(confirmAction.tenant.id); confirmAction = null; }} onCancel={() => (confirmAction = null)} />
	{/if}
{/if}
