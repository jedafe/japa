<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import Pagination from '$lib/components/ui/Pagination.svelte';
	import { Dialog, Select, Tooltip } from 'bits-ui';
	import { api } from '$lib/api';
	import type { Tenant } from '$lib/api/types';
	import { Building2, Search, RefreshCw, Plus, Pencil, Ban, CheckCircle, Trash2, X } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data = {} as PageData } = $props();

	// svelte-ignore state_referenced_locally
	let tenants = $state<Tenant[]>(data.tenants ?? []);
	let loading = $state(false);
	let filter = $state('');
	let filterStatus = $state('all');
	// svelte-ignore state_referenced_locally
	let errorMessage = $state(data.error ?? '');
	let saving = $state(false);

	// Pagination
	let page = $state(0);
	let pageSize = $state(10);

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
		{ value: 'all', label: 'All Status' },
		{ value: 'active', label: 'Active' },
		{ value: 'suspended', label: 'Suspended' },
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

	let totalPages = $derived(Math.max(1, Math.ceil(filtered.length / pageSize)));
	let paginatedTenants = $derived.by(() => {
		const start = page * pageSize;
		return filtered.slice(start, start + pageSize);
	});

	let activeTenants = $derived(filtered.filter((t) => t.status === 'active'));
	let suspendedTenants = $derived(filtered.filter((t) => t.status === 'suspended'));
</script>

<PageHeader title="Tenants" description="Manage tenant organizations across the platform" />

{#if errorMessage}
	<div class="border-destructive/30 bg-destructive/6 text-destructive mb-4 rounded-lg border p-3 text-sm">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex items-center gap-3">
	<div class="relative">
		<Search class="text-muted-foreground absolute left-3 top-1/2 size-4 -translate-y-1/2" />
		<input
			bind:value={filter}
			placeholder="Search tenants..."
			class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 h-input rounded-9px w-72 border pl-9 pr-3 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
		/>
	</div>
	<Select.Root type="single" bind:value={filterStatus} items={statusItems}>
		<Select.Trigger
			class="border-border-input bg-background data-placeholder:text-muted-foreground/50 rounded-9px flex h-10 items-center border px-3 text-sm transition-colors hover:border-border-input-hover"
			aria-label="Filter status"
			onclick={() => { page = 0; }}
		>
			<Select.Value placeholder="All Status" />
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
	<span class="text-muted-foreground text-xs">{filtered.length} tenant{filtered.length !== 1 ? 's' : ''}</span>
	<div class="flex-1"></div>
	<button class="bg-foreground hover:bg-foreground/90 text-background flex items-center gap-1.5 rounded-9px px-3 py-2 text-xs font-medium transition-colors" onclick={openCreate}>
		<Plus class="size-3" />
		Create Tenant
	</button>
	<button class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border flex items-center gap-1.5 px-3 py-2 text-xs font-medium transition-colors" onclick={load}>
		<RefreshCw class="size-3" />
		Refresh
	</button>
</div>

{#if loading}
	<div class="space-y-3">
		{#each Array(5) as _}
			<div class="bg-muted h-16 animate-pulse rounded-lg"></div>
		{/each}
	</div>
{:else if filtered.length === 0}
	<div class="border-border-card bg-background-alt rounded-9px border p-8 text-center">
		<Building2 class="text-muted-foreground mx-auto size-8" />
		<p class="text-muted-foreground mt-3 text-sm">{filter ? 'No tenants match your filter.' : 'No tenants exist yet.'}</p>
	</div>
{:else}
	<div class="border-border-card bg-background-alt shadow-card overflow-hidden rounded-xl">
		<table class="w-full text-left text-sm">
			<thead>
				<tr class="border-border-card border-b">
					<th class="text-muted-foreground px-4 py-3 font-medium">Name</th>
					<th class="text-muted-foreground px-4 py-3 font-medium">Slug</th>
					<th class="text-muted-foreground px-4 py-3 font-medium">Domain</th>
					<th class="text-muted-foreground px-4 py-3 font-medium">Status</th>
					<th class="text-muted-foreground px-4 py-3 font-medium">Created</th>
					<th class="text-muted-foreground px-4 py-3 font-medium">Actions</th>
				</tr>
			</thead>
			<tbody>
				{#each paginatedTenants as tenant (tenant.id)}
					<tr class="border-border-card border-t">
						<td class="px-4 py-3"><p class="font-medium">{tenant.name}</p></td>
						<td class="px-4 py-3 font-mono text-xs">{tenant.slug}</td>
						<td class="text-muted-foreground px-4 py-3 text-xs">{tenant.domain ?? '—'}</td>
						<td class="px-4 py-3">
							<span
								class="rounded-full px-2 py-0.5 text-xs font-medium {tenant.status === 'active' ? 'bg-green-500/12 text-green-600' : tenant.status === 'suspended' ? 'bg-tertiary/12 text-tertiary' : ''}"
							>
								{tenant.status}
							</span>
						</td>
						<td class="text-muted-foreground px-4 py-3 text-xs">{new Date(tenant.created_at).toLocaleDateString()}</td>
						<td class="px-4 py-3">
							<div class="flex items-center gap-1">
								<Tooltip.Root>
									<Tooltip.Trigger>
										<button class="text-accent-foreground hover:bg-accent rounded-5px p-1.5 transition-colors" onclick={() => openEdit(tenant)}>
											<Pencil class="size-3" />
										</button>
									</Tooltip.Trigger>
									<Tooltip.Portal><Tooltip.Content side="top" sideOffset={4} class="border-dark-10 bg-background shadow-popover z-50 rounded-input border px-3 py-2 text-xs">Edit tenant</Tooltip.Content></Tooltip.Portal>
								</Tooltip.Root>
								{#if tenant.status === 'active'}
									<Tooltip.Root>
										<Tooltip.Trigger>
											<button class="text-tertiary hover:bg-tertiary/10 rounded-5px p-1.5 transition-colors" onclick={() => (confirmAction = { type: 'suspend', tenant })}>
												<Ban class="size-3" />
											</button>
										</Tooltip.Trigger>
										<Tooltip.Portal><Tooltip.Content side="top" sideOffset={4} class="border-dark-10 bg-background shadow-popover z-50 rounded-input border px-3 py-2 text-xs">Suspend tenant</Tooltip.Content></Tooltip.Portal>
									</Tooltip.Root>
								{:else if tenant.status === 'suspended'}
									<Tooltip.Root>
										<Tooltip.Trigger>
											<button class="text-green-500 hover:bg-green-500/10 rounded-5px p-1.5 transition-colors" onclick={() => (confirmAction = { type: 'unsuspend', tenant })}>
												<CheckCircle class="size-3" />
											</button>
										</Tooltip.Trigger>
										<Tooltip.Portal><Tooltip.Content side="top" sideOffset={4} class="border-dark-10 bg-background shadow-popover z-50 rounded-input border px-3 py-2 text-xs">Unsuspend tenant</Tooltip.Content></Tooltip.Portal>
									</Tooltip.Root>
								{/if}
								<Tooltip.Root>
									<Tooltip.Trigger>
										<button class="text-destructive hover:bg-destructive/10 rounded-5px p-1.5 transition-colors" onclick={() => (confirmAction = { type: 'delete', tenant })}>
											<Trash2 class="size-3" />
										</button>
									</Tooltip.Trigger>
									<Tooltip.Portal><Tooltip.Content side="top" sideOffset={4} class="border-dark-10 bg-background shadow-popover z-50 rounded-input border px-3 py-2 text-xs">Delete tenant</Tooltip.Content></Tooltip.Portal>
								</Tooltip.Root>
							</div>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>

	<Pagination
		bind:page
		{totalPages}
		totalItems={filtered.length}
		bind:pageSize
	/>
{/if}

<!-- Create Modal -->
<Dialog.Root bind:open={showCreateModal}>
	<Dialog.Portal>
		<Dialog.Overlay class="bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out fixed inset-0 z-50" />
		<Dialog.Content class="border-border-card bg-background shadow-popover fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-xl border p-6" onOpenAutoFocus={(e) => e.preventDefault()}>
			<div class="mb-4 flex items-center justify-between">
				<Dialog.Title class="text-foreground text-lg font-semibold">Create Tenant</Dialog.Title>
				<Dialog.Close class="hover:bg-dark-04 rounded-9px p-1.5 transition-colors"><X class="size-4" /></Dialog.Close>
			</div>
			<form onsubmit={(e) => { e.preventDefault(); createTenant(); }}>
				<div class="space-y-4">
					<div>
						<label for="create-name" class="text-foreground mb-1 block text-sm font-medium">Name *</label>
						<input id="create-name" type="text" bind:value={createName} required class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus" placeholder="My Organization" oninput={() => { if (!createSlug) createSlug = autoSlug(createName); }} />
					</div>
					<div>
						<label for="create-slug" class="text-foreground mb-1 block text-sm font-medium">Slug *</label>
						<input id="create-slug" type="text" bind:value={createSlug} required class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm" placeholder="my-organization" />
						<p class="text-muted-foreground mt-1 text-xs">Auto-generated from name if left empty</p>
					</div>
					<div>
						<label for="create-domain" class="text-foreground mb-1 block text-sm font-medium">Domain</label>
						<input id="create-domain" type="text" bind:value={createDomain} class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm" placeholder="example.com" />
					</div>
				</div>
				<div class="mt-6 flex justify-end gap-3">
					<Dialog.Close class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium">Cancel</Dialog.Close>
					<button type="submit" class="bg-foreground hover:bg-foreground/90 text-background rounded-9px px-4 py-2 text-sm font-medium disabled:opacity-50" disabled={saving || !createName.trim()}>
						{saving ? 'Creating...' : 'Create'}
					</button>
				</div>
			</form>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<!-- Edit Modal -->
<Dialog.Root bind:open={showEditModal}>
	<Dialog.Portal>
		<Dialog.Overlay class="bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out fixed inset-0 z-50" />
		<Dialog.Content class="border-border-card bg-background shadow-popover fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-xl border p-6" onOpenAutoFocus={(e) => e.preventDefault()}>
			<div class="mb-4 flex items-center justify-between">
				<Dialog.Title class="text-foreground text-lg font-semibold">Edit Tenant</Dialog.Title>
				<Dialog.Close class="hover:bg-dark-04 rounded-9px p-1.5 transition-colors"><X class="size-4" /></Dialog.Close>
			</div>
			<form onsubmit={(e) => { e.preventDefault(); editTenant(); }}>
				<div class="space-y-4">
					<div>
						<label for="edit-name" class="text-foreground mb-1 block text-sm font-medium">Name</label>
						<input id="edit-name" type="text" bind:value={editName} required class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm" />
					</div>
					<div>
						<label for="edit-slug" class="text-foreground mb-1 block text-sm font-medium">Slug</label>
						<input id="edit-slug" type="text" value={editingTenant?.slug ?? ''} disabled class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm opacity-60" />
						<p class="text-muted-foreground mt-1 text-xs">Slug cannot be changed</p>
					</div>
					<div>
						<label for="edit-domain" class="text-foreground mb-1 block text-sm font-medium">Domain</label>
						<input id="edit-domain" type="text" bind:value={editDomain} class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm" placeholder="example.com" />
					</div>
				</div>
				<div class="mt-6 flex justify-end gap-3">
					<Dialog.Close class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium">Cancel</Dialog.Close>
					<button type="submit" class="bg-foreground hover:bg-foreground/90 text-background rounded-9px px-4 py-2 text-sm font-medium disabled:opacity-50" disabled={saving || !editName.trim()}>
						{saving ? 'Saving...' : 'Save Changes'}
					</button>
				</div>
			</form>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

{#if confirmAction}
	{#if confirmAction.type === 'suspend'}
		<ConfirmModal bind:open={confirmOpen} title="Suspend Tenant" consequence="This will suspend tenant '{confirmAction.tenant.name}'. Users will lose access." confirmText="SUSPEND" onConfirm={async () => { if (confirmAction?.type === 'suspend') await suspendTenant(confirmAction.tenant.id); confirmAction = null; }} onCancel={() => (confirmAction = null)} />
	{:else if confirmAction.type === 'unsuspend'}
		<ConfirmModal bind:open={confirmOpen} title="Unsuspend Tenant" consequence="This will restore tenant '{confirmAction.tenant.name}'." confirmText="UNSUSPEND" onConfirm={async () => { if (confirmAction?.type === 'unsuspend') await unsuspendTenant(confirmAction.tenant.id); confirmAction = null; }} onCancel={() => (confirmAction = null)} />
	{:else if confirmAction.type === 'delete'}
		<ConfirmModal bind:open={confirmOpen} title="Delete Tenant" consequence="Permanently delete tenant '{confirmAction.tenant.name}' ({confirmAction.tenant.slug}). This cannot be undone." confirmText="DELETE" onConfirm={async () => { if (confirmAction?.type === 'delete') await deleteTenant(confirmAction.tenant.id); confirmAction = null; }} onCancel={() => (confirmAction = null)} />
	{/if}
{/if}
