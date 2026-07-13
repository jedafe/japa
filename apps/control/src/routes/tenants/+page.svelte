<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { api } from '$lib/api';
	import type { Tenant } from '$lib/api/types';
	import {
		Building2,
		Search,
		RefreshCw,
		Plus,
		Pencil,
		Ban,
		CheckCircle,
		Trash2,
		X
	} from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data = {} as PageData } = $props();

	// svelte-ignore state_referenced_locally
	let tenants = $state<Tenant[]>(data.tenants ?? []);
	let loading = $state(false);
	let filter = $state('');
	// svelte-ignore state_referenced_locally
	let errorMessage = $state(data.error ?? '');
	let saving = $state(false);

	// Create modal
	let showCreateModal = $state(false);
	let createName = $state('');
	let createSlug = $state('');
	let createDomain = $state('');

	// Edit modal
	let showEditModal = $state(false);
	let editingTenant = $state<Tenant | null>(null);
	let editName = $state('');
	let editDomain = $state('');

	// Confirm modal state
	let confirmOpen = $state(false);
	let confirmAction = $state<
		| { type: 'suspend'; tenant: Tenant }
		| { type: 'unsuspend'; tenant: Tenant }
		| { type: 'delete'; tenant: Tenant }
		| null
	>(null);

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
		return name
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-|-$/g, '');
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
		const result = await api<{ status: string }>(`/control/tenants/${id}`, {
			method: 'DELETE'
		});
		if (result.ok) {
			tenants = tenants.filter((t) => t.id !== id);
		} else {
			errorMessage = result.error;
		}
	}

	function statusColor(status: string): string {
		switch (status) {
			case 'active':
				return 'var(--color-success)';
			case 'suspended':
				return 'var(--color-warning)';
			default:
				return 'var(--color-muted)';
		}
	}

	let filtered = $derived(
		filter
			? tenants.filter(
					(t) =>
						t.name.toLowerCase().includes(filter.toLowerCase()) ||
						t.slug.toLowerCase().includes(filter.toLowerCase()) ||
						(t.domain ?? '').toLowerCase().includes(filter.toLowerCase())
				)
			: tenants
	);
</script>

<PageHeader title="Tenants" description="Manage tenant organizations across the platform" />

{#if errorMessage}
	<div
		class="mb-4 rounded-lg border p-3 text-sm"
		style="border-color: color-mix(in srgb, var(--color-danger) 30%, transparent); background: color-mix(in srgb, var(--color-danger) 6%, transparent); color: var(--color-danger)"
	>
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex items-center gap-3">
	<div class="relative">
		<Search
			class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2"
			style="color: var(--color-muted)"
		/>
		<input
			bind:value={filter}
			placeholder="Search tenants..."
			class="w-72 rounded-lg border py-2 pl-9 pr-3 text-sm outline-none transition-colors focus:ring-2"
			style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground); --tw-ring-color: var(--color-primary)"
		/>
	</div>
	<span class="text-xs" style="color: var(--color-muted)">
		{filtered.length} tenant{filtered.length !== 1 ? 's' : ''}
	</span>
	<div class="flex-1"></div>
	<button class="btn btn-primary flex items-center gap-1.5 text-xs" onclick={openCreate}>
		<Plus class="h-3 w-3" />
		Create Tenant
	</button>
	<button class="btn btn-secondary flex items-center gap-1.5 text-xs" onclick={load}>
		<RefreshCw class="h-3 w-3" />
		Refresh
	</button>
</div>

{#if loading}
	<div class="space-y-3">
		{#each Array(5) as _}
			<div class="h-16 animate-pulse rounded-lg" style="background-color: var(--color-surface)"></div>
		{/each}
	</div>
{:else if filtered.length === 0}
	<div
		class="rounded-lg border p-8 text-center"
		style="border-color: var(--color-border); background-color: var(--color-surface)"
	>
		<Building2 class="mx-auto h-8 w-8" style="color: var(--color-muted)" />
		<p class="mt-3 text-sm" style="color: var(--color-muted)">
			{filter ? 'No tenants match your filter.' : 'No tenants exist yet.'}
		</p>
	</div>
{:else}
	<div
		class="overflow-hidden rounded-lg border"
		style="border-color: var(--color-border); background-color: var(--color-surface)"
	>
		<table class="w-full text-left text-sm" style="color: var(--color-foreground)">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Name</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Slug</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Domain</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Status</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Created</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Actions</th>
				</tr>
			</thead>
			<tbody>
				{#each filtered as tenant (tenant.id)}
					<tr style="border-bottom: 1px solid var(--color-border)">
						<td class="px-4 py-3">
							<p class="font-medium">{tenant.name}</p>
						</td>
						<td class="px-4 py-3 font-mono text-xs">{tenant.slug}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">
							{tenant.domain ?? '—'}
						</td>
						<td class="px-4 py-3">
							<span
								class="rounded-full px-2 py-0.5 text-xs font-medium"
								style="background: color-mix(in srgb, {statusColor(tenant.status)} 12%, transparent); color: {statusColor(tenant.status)}"
							>
								{tenant.status}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">
							{new Date(tenant.created_at).toLocaleDateString()}
						</td>
						<td class="px-4 py-3">
							<div class="flex items-center gap-1">
								<button
									class="flex items-center gap-1 rounded px-2 py-1 text-xs font-medium transition-colors"
									style="color: var(--color-primary)"
									title="Edit"
									onclick={() => openEdit(tenant)}
								>
									<Pencil class="h-3 w-3" />
								</button>
								{#if tenant.status === 'active'}
									<button
										class="flex items-center gap-1 rounded px-2 py-1 text-xs font-medium transition-colors"
										style="color: var(--color-warning)"
										title="Suspend"
										onclick={() => (confirmAction = { type: 'suspend', tenant })}
									>
										<Ban class="h-3 w-3" />
									</button>
								{:else if tenant.status === 'suspended'}
									<button
										class="flex items-center gap-1 rounded px-2 py-1 text-xs font-medium transition-colors"
										style="color: var(--color-success)"
										title="Unsuspend"
										onclick={() => (confirmAction = { type: 'unsuspend', tenant })}
									>
										<CheckCircle class="h-3 w-3" />
									</button>
								{/if}
								<button
									class="flex items-center gap-1 rounded px-2 py-1 text-xs font-medium transition-colors"
									style="color: var(--color-danger)"
									title="Delete"
									onclick={() => (confirmAction = { type: 'delete', tenant })}
								>
									<Trash2 class="h-3 w-3" />
								</button>
							</div>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<!-- Create Modal -->
{#if showCreateModal}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
		<div
			class="w-full max-w-md rounded-lg border p-6 shadow-xl"
			style="border-color: var(--color-border); background: var(--color-surface)"
		>
			<div class="mb-4 flex items-center justify-between">
				<h2 class="text-lg font-semibold">Create Tenant</h2>
				<button
					class="rounded p-1 transition-colors hover:bg-black/5"
					onclick={() => (showCreateModal = false)}
				>
					<X class="h-4 w-4" style="color: var(--color-muted)" />
				</button>
			</div>
			<form
				onsubmit={(e) => {
					e.preventDefault();
					createTenant();
				}}
			>
				<div class="space-y-4">
					<div>
						<label for="create-name" class="mb-1 block text-sm font-medium">Name *</label>
						<input
							id="create-name"
							type="text"
							bind:value={createName}
							required
							class="w-full rounded-lg border px-3 py-2 text-sm outline-none"
							style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
							placeholder="My Organization"
							oninput={() => {
								if (!createSlug) createSlug = autoSlug(createName);
							}}
						/>
					</div>
					<div>
						<label for="create-slug" class="mb-1 block text-sm font-medium">Slug *</label>
						<input
							id="create-slug"
							type="text"
							bind:value={createSlug}
							required
							class="w-full rounded-lg border px-3 py-2 text-sm outline-none"
							style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
							placeholder="my-organization"
						/>
						<p class="mt-1 text-xs" style="color: var(--color-muted)">
							Auto-generated from name if left empty
						</p>
					</div>
					<div>
						<label for="create-domain" class="mb-1 block text-sm font-medium">Domain</label>
						<input
							id="create-domain"
							type="text"
							bind:value={createDomain}
							class="w-full rounded-lg border px-3 py-2 text-sm outline-none"
							style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
							placeholder="example.com"
						/>
					</div>
				</div>
				<div class="mt-6 flex justify-end gap-3">
					<button
						type="button"
						class="rounded-md border px-4 py-2 text-sm font-medium"
						style="border-color: var(--color-border); color: var(--color-foreground)"
						onclick={() => (showCreateModal = false)}
					>
						Cancel
					</button>
					<button
						type="submit"
						class="rounded-md px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
						style="background: var(--color-primary)"
						disabled={saving || !createName.trim()}
					>
						{saving ? 'Creating...' : 'Create'}
					</button>
				</div>
			</form>
		</div>
	</div>
{/if}

<!-- Edit Modal -->
{#if showEditModal && editingTenant}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
		<div
			class="w-full max-w-md rounded-lg border p-6 shadow-xl"
			style="border-color: var(--color-border); background: var(--color-surface)"
		>
			<div class="mb-4 flex items-center justify-between">
				<h2 class="text-lg font-semibold">Edit Tenant</h2>
				<button
					class="rounded p-1 transition-colors hover:bg-black/5"
					onclick={() => {
						showEditModal = false;
						editingTenant = null;
					}}
				>
					<X class="h-4 w-4" style="color: var(--color-muted)" />
				</button>
			</div>
			<form
				onsubmit={(e) => {
					e.preventDefault();
					editTenant();
				}}
			>
				<div class="space-y-4">
					<div>
						<label for="edit-name" class="mb-1 block text-sm font-medium">Name</label>
						<input
							id="edit-name"
							type="text"
							bind:value={editName}
							required
							class="w-full rounded-lg border px-3 py-2 text-sm outline-none"
							style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
						/>
					</div>
					<div>
						<label for="edit-slug" class="mb-1 block text-sm font-medium">Slug</label>
						<input
							id="edit-slug"
							type="text"
							value={editingTenant.slug}
							disabled
							class="w-full rounded-lg border px-3 py-2 text-sm outline-none opacity-60"
							style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
						/>
						<p class="mt-1 text-xs" style="color: var(--color-muted)">Slug cannot be changed</p>
					</div>
					<div>
						<label for="edit-domain" class="mb-1 block text-sm font-medium">Domain</label>
						<input
							id="edit-domain"
							type="text"
							bind:value={editDomain}
							class="w-full rounded-lg border px-3 py-2 text-sm outline-none"
							style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
							placeholder="example.com"
						/>
					</div>
				</div>
				<div class="mt-6 flex justify-end gap-3">
					<button
						type="button"
						class="rounded-md border px-4 py-2 text-sm font-medium"
						style="border-color: var(--color-border); color: var(--color-foreground)"
						onclick={() => {
							showEditModal = false;
							editingTenant = null;
						}}
					>
						Cancel
					</button>
					<button
						type="submit"
						class="rounded-md px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
						style="background: var(--color-primary)"
						disabled={saving || !editName.trim()}
					>
						{saving ? 'Saving...' : 'Save Changes'}
					</button>
				</div>
			</form>
		</div>
	</div>
{/if}

<!-- Confirm Modals -->
{#if confirmAction}
	{#if confirmAction.type === 'suspend'}
		<ConfirmModal
			bind:open={confirmOpen}
			title="Suspend Tenant"
			consequence="This will suspend tenant '{confirmAction.tenant.name}'. Users will lose access."
			confirmText="SUSPEND"
			onConfirm={async () => {
				if (confirmAction?.type === 'suspend') await suspendTenant(confirmAction.tenant.id);
				confirmAction = null;
			}}
			onCancel={() => (confirmAction = null)}
		/>
	{:else if confirmAction.type === 'unsuspend'}
		<ConfirmModal
			bind:open={confirmOpen}
			title="Unsuspend Tenant"
			consequence="This will restore tenant '{confirmAction.tenant.name}'."
			confirmText="UNSUSPEND"
			onConfirm={async () => {
				if (confirmAction?.type === 'unsuspend') await unsuspendTenant(confirmAction.tenant.id);
				confirmAction = null;
			}}
			onCancel={() => (confirmAction = null)}
		/>
	{:else if confirmAction.type === 'delete'}
		<ConfirmModal
			bind:open={confirmOpen}
			title="Delete Tenant"
			consequence="Permanently delete tenant '{confirmAction.tenant.name}' ({confirmAction.tenant.slug}). This cannot be undone."
			confirmText="DELETE"
			onConfirm={async () => {
				if (confirmAction?.type === 'delete') await deleteTenant(confirmAction.tenant.id);
				confirmAction = null;
			}}
			onCancel={() => (confirmAction = null)}
		/>
	{/if}
{/if}
