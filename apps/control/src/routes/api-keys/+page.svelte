<script lang="ts">
	import { api } from '$lib/api';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { Key, Search, Ban, Trash2, Plus, Pencil, Copy, Check } from 'lucide-svelte';
	import { Dialog } from 'bits-ui';
	import type { PageData } from './$types';

	let { data = {} as PageData } = $props();

	interface AdminApiKey {
		id: string;
		name: string;
		key_prefix: string;
		user_id: string;
		rate_limit_tier: string;
		status: string;
		request_count: number;
		scopes: string[];
		created_at: string;
		last_used_at?: string;
		expires_at?: string;
		allowed_ips?: string[];
	}

	// svelte-ignore state_referenced_locally
	let keys = $state<AdminApiKey[]>(data.keys ?? []);
	let loading = $state(false);
	let filter = $state('');
	// svelte-ignore state_referenced_locally
	let errorMessage = $state(data.error ?? '');
	let confirmOpen = $state(false);
	let confirmAction = $state<{ type: 'revoke' | 'suspend'; key: AdminApiKey } | null>(null);

	// Create modal state
	let createModalOpen = $state(false);
	let createForm = $state({ name: '', scopes: '', rate_limit_tier: 'standard', expires_at: '' });
	let createLoading = $state(false);

	// One-time key display modal state
	let oneTimeKeyModalOpen = $state(false);
	let oneTimeKeyValue = $state('');
	let copiedToClipboard = $state(false);

	// Edit modal state
	let editModalOpen = $state(false);
	let editForm = $state({ id: '', name: '', scopes: '' });
	let editLoading = $state(false);

	async function suspendKey(id: string) {
		const result = await api(`/control/api-keys/${id}/suspend`, { method: 'POST' });
		if (result.ok) {
			keys = keys.map((k) => (k.id === id ? { ...k, status: 'suspended' } : k));
		} else {
			errorMessage = result.error || 'Failed to suspend key.';
		}
	}

	async function revokeKey(id: string) {
		if (!confirm('Permanently revoke this key? The developer will lose access immediately.')) return;
		const result = await api(`/control/api-keys/${id}`, { method: 'DELETE' });
		if (result.ok) {
			keys = keys.map((k) => (k.id === id ? { ...k, status: 'revoked' } : k));
		} else {
			errorMessage = result.error || 'Failed to revoke key.';
		}
	}

	async function createKey() {
		if (!createForm.name.trim()) {
			errorMessage = 'Name is required.';
			return;
		}

		createLoading = true;
		errorMessage = '';

		const body: Record<string, unknown> = { name: createForm.name.trim() };
		if (createForm.scopes.trim()) {
			body.scopes = createForm.scopes.split(',').map((s) => s.trim()).filter(Boolean);
		}
		if (createForm.rate_limit_tier && createForm.rate_limit_tier !== 'standard') {
			body.rate_limit_tier = createForm.rate_limit_tier;
		}
		if (createForm.expires_at) {
			body.expires_at = new Date(createForm.expires_at).toISOString();
		}

		const result = await api<{ id: string; key: string } & AdminApiKey>('/control/api-keys', {
			method: 'POST',
			body
		});

		createLoading = false;

		if (result.ok) {
			const { key, ...keyData } = result.data;
			oneTimeKeyValue = key;
			createModalOpen = false;
			createForm = { name: '', scopes: '', rate_limit_tier: 'standard', expires_at: '' };
			keys = [{ ...keyData }, ...keys];
			oneTimeKeyModalOpen = true;
		} else {
			errorMessage = result.error || 'Failed to create API key.';
		}
	}

	async function updateKey() {
		if (!editForm.name.trim()) {
			errorMessage = 'Name is required.';
			return;
		}

		editLoading = true;
		errorMessage = '';

		const body: Record<string, unknown> = { name: editForm.name.trim() };
		if (editForm.scopes.trim()) {
			body.scopes = editForm.scopes.split(',').map((s) => s.trim()).filter(Boolean);
		}

		const result = await api<{ id: string }>(`/control/api-keys/${editForm.id}`, {
			method: 'PATCH',
			body
		});

		editLoading = false;

		if (result.ok) {
			keys = keys.map((k) =>
				k.id === editForm.id
					? { ...k, name: editForm.name.trim(), scopes: body.scopes as string[] || k.scopes }
					: k
			);
			editModalOpen = false;
		} else {
			errorMessage = result.error || 'Failed to update API key.';
		}
	}

	function openEditModal(key: AdminApiKey) {
		editForm = {
			id: key.id,
			name: key.name,
			scopes: (key.scopes || []).join(', ')
		};
		editModalOpen = true;
	}

	function copyToClipboard() {
		navigator.clipboard.writeText(oneTimeKeyValue);
		copiedToClipboard = true;
		setTimeout(() => (copiedToClipboard = false), 2000);
	}

	let filtered = $derived(
		filter
			? keys.filter(
					(k) =>
						k.name?.toLowerCase().includes(filter.toLowerCase()) ||
						k.key_prefix?.toLowerCase().includes(filter.toLowerCase()) ||
						k.user_id?.toLowerCase().includes(filter.toLowerCase())
				)
			: keys
	);

	function statusColor(status: string): string {
		switch (status) {
			case 'active':
				return '#22c55e';
			case 'suspended':
				return '#f59e0b';
			case 'revoked':
				return '#ef4444';
			default:
				return 'var(--color-muted)';
		}
	}

	function formatNumber(n: number): string {
		if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
		if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K';
		return n.toString();
	}
</script>

<PageHeader
	title="API Key Management"
	description="Monitor and control developer API keys across the platform"
/>

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
			placeholder="Filter by name, prefix, or user..."
			class="w-72 rounded-lg border py-2 pl-9 pr-3 text-sm outline-none transition-colors focus:ring-2"
			style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground); --tw-ring-color: var(--color-primary)"
		/>
	</div>
	<span class="text-xs" style="color: var(--color-muted)">
		{filtered.length} key{filtered.length !== 1 ? 's' : ''}
	</span>
	<div class="flex-1"></div>
	<button
		class="flex items-center gap-1.5 rounded-lg px-3 py-2 text-sm font-medium text-white transition-colors"
		style="background: var(--color-primary)"
		onclick={() => (createModalOpen = true)}
	>
		<Plus class="h-4 w-4" />
		Create API Key
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
		<Key class="mx-auto h-8 w-8" style="color: var(--color-muted)" />
		<p class="mt-3 text-sm" style="color: var(--color-muted)">
			{filter ? 'No keys match your filter.' : 'No API keys exist yet.'}
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
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Key Prefix</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">User</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Tier</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Status</th>
					<th class="px-4 py-3 font-medium text-right" style="color: var(--color-muted)">Requests</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Actions</th>
				</tr>
			</thead>
			<tbody>
				{#each filtered as key (key.id)}
					<tr style="border-bottom: 1px solid var(--color-border)">
						<td class="px-4 py-3 font-medium">{key.name}</td>
						<td class="px-4 py-3 font-mono text-xs">{key.key_prefix}...</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">
							{key.user_id ? key.user_id.slice(0, 8) + '...' : 'System'}
						</td>
						<td class="px-4 py-3">
							<span
								class="rounded px-1.5 py-0.5 text-xs"
								style="background: color-mix(in srgb, var(--color-primary) 8%, transparent); color: var(--color-primary)"
							>
								{key.rate_limit_tier}
							</span>
						</td>
						<td class="px-4 py-3">
							<span
								class="rounded-full px-2 py-0.5 text-xs font-medium"
								style="background: color-mix(in srgb, {statusColor(key.status)} 12%, transparent); color: {statusColor(
									key.status
								)}"
							>
								{key.status}
							</span>
						</td>
						<td class="px-4 py-3 text-right font-mono text-xs">
							{formatNumber(key.request_count)}
						</td>
						<td class="px-4 py-3">
							<div class="flex items-center gap-2">
								{#if key.status !== 'revoked'}
									<button
										class="flex items-center gap-1 rounded px-2 py-1 text-xs font-medium transition-colors"
										style="color: var(--color-primary)"
										onclick={() => openEditModal(key)}
									>
										<Pencil class="h-3 w-3" />
										Edit
									</button>
								{/if}
								{#if key.status === 'active'}
									<button
										class="flex items-center gap-1 rounded px-2 py-1 text-xs font-medium transition-colors"
										style="color: #f59e0b"
										onclick={() => (confirmAction = { type: 'suspend', key })}
									>
										<Ban class="h-3 w-3" />
										Suspend
									</button>
								{/if}
								{#if key.status !== 'revoked'}
									<button
										class="flex items-center gap-1 rounded px-2 py-1 text-xs font-medium transition-colors"
										style="color: var(--color-danger)"
										onclick={() => (confirmAction = { type: 'revoke', key })}
									>
										<Trash2 class="h-3 w-3" />
										Revoke
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

{#if confirmAction}
	{#if confirmAction.type === 'revoke'}
		<ConfirmModal
			bind:open={confirmOpen}
			title="Revoke API Key"
			consequence="Permanently revoke key '{confirmAction.key.name}' ({confirmAction.key.key_prefix}...). This cannot be undone."
			confirmText="REVOKE"
			onConfirm={async () => { await revokeKey(confirmAction!.key.id); confirmAction = null; }}
			onCancel={() => (confirmAction = null)}
		/>
	{:else}
		<ConfirmModal
			bind:open={confirmOpen}
			title="Suspend API Key"
			consequence="Suspend key '{confirmAction.key.name}' ({confirmAction.key.key_prefix}...). The key owner will need to generate a new key."
			confirmText="SUSPEND"
			onConfirm={async () => { await suspendKey(confirmAction!.key.id); confirmAction = null; }}
			onCancel={() => (confirmAction = null)}
		/>
	{/if}
{/if}

<!-- Create API Key Modal -->
<Dialog.Root bind:open={createModalOpen} onOpenChange={(isOpen) => { if (!isOpen) createForm = { name: '', scopes: '', rate_limit_tier: 'standard', expires_at: '' }; }}>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-50 bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out" />
		<Dialog.Content
			class="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-lg border p-6 shadow-xl focus:outline-none"
			style="border-color: var(--color-border); background: var(--color-surface)"
			role="dialog"
			aria-modal="true"
			onOpenAutoFocus={(e) => e.preventDefault()}
		>
			<Dialog.Title class="text-lg font-semibold" style="color: var(--color-foreground)">
				Create API Key
			</Dialog.Title>
			<p class="mt-1 text-sm" style="color: var(--color-muted)">
				Generate a new API key for a developer.
			</p>

			{#if errorMessage}
				<div
					class="mt-3 rounded-md p-3 text-sm"
					style="background: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
				>
					{errorMessage}
				</div>
			{/if}

			<div class="mt-4 space-y-4">
				<div>
					<label for="create-name" class="block text-sm font-medium" style="color: var(--color-foreground)">
						Name <span style="color: var(--color-danger)">*</span>
					</label>
					<input
						id="create-name"
						type="text"
						bind:value={createForm.name}
						placeholder="e.g., Production Backend"
						class="mt-1 block w-full rounded-md border px-3 py-2 text-sm outline-none focus:ring-2"
						style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground); --tw-ring-color: var(--color-primary)"
					/>
				</div>

				<div>
					<label for="create-scopes" class="block text-sm font-medium" style="color: var(--color-foreground)">
						Scopes
					</label>
					<input
						id="create-scopes"
						type="text"
						bind:value={createForm.scopes}
						placeholder="read,write,admin"
						class="mt-1 block w-full rounded-md border px-3 py-2 text-sm outline-none focus:ring-2"
						style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground); --tw-ring-color: var(--color-primary)"
					/>
					<p class="mt-1 text-xs" style="color: var(--color-muted)">Comma-separated list of scopes</p>
				</div>

				<div>
					<label for="create-tier" class="block text-sm font-medium" style="color: var(--color-foreground)">
						Rate Limit Tier
					</label>
					<select
						id="create-tier"
						bind:value={createForm.rate_limit_tier}
						class="mt-1 block w-full rounded-md border px-3 py-2 text-sm outline-none focus:ring-2"
						style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground); --tw-ring-color: var(--color-primary)"
					>
						<option value="standard">Standard</option>
						<option value="premium">Premium</option>
						<option value="unlimited">Unlimited</option>
					</select>
				</div>

				<div>
					<label for="create-expires" class="block text-sm font-medium" style="color: var(--color-foreground)">
						Expires At
					</label>
					<input
						id="create-expires"
						type="datetime-local"
						bind:value={createForm.expires_at}
						class="mt-1 block w-full rounded-md border px-3 py-2 text-sm outline-none focus:ring-2"
						style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground); --tw-ring-color: var(--color-primary)"
					/>
					<p class="mt-1 text-xs" style="color: var(--color-muted)">Leave empty for no expiration</p>
				</div>
			</div>

			<div class="mt-6 flex justify-end gap-3">
				<Dialog.Close
					class="rounded-md border px-4 py-2 text-sm font-medium transition-colors"
					style="border-color: var(--color-border); color: var(--color-foreground)"
				>
					Cancel
				</Dialog.Close>
				<button
					type="button"
					class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
					style="background: var(--color-primary)"
					disabled={createLoading || !createForm.name.trim()}
					onclick={createKey}
				>
					{createLoading ? 'Creating...' : 'Create Key'}
				</button>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<!-- One-Time Key Display Modal -->
<Dialog.Root bind:open={oneTimeKeyModalOpen}>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-50 bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out" />
		<Dialog.Content
			class="fixed left-1/2 top-1/2 z-50 w-full max-w-lg -translate-x-1/2 -translate-y-1/2 rounded-lg border p-6 shadow-xl focus:outline-none"
			style="border-color: var(--color-border); background: var(--color-surface)"
			role="dialog"
			aria-modal="true"
			onOpenAutoFocus={(e) => e.preventDefault()}
		>
			<Dialog.Title class="text-lg font-semibold" style="color: var(--color-foreground)">
				API Key Created
			</Dialog.Title>

			<div class="mt-3 rounded-md p-3" style="background: color-mix(in srgb, #f59e0b 8%, transparent)">
				<p class="text-sm font-medium" style="color: #f59e0b">
					Save this key now. It will not be shown again.
				</p>
			</div>

			<div class="mt-4">
				<label for="one-time-key" class="block text-sm font-medium" style="color: var(--color-foreground)">
					Your API Key
				</label>
				<div class="mt-1 flex items-center gap-2">
					<input
						id="one-time-key"
						type="text"
						readonly
						value={oneTimeKeyValue}
						class="flex-1 rounded-md border px-3 py-2 font-mono text-sm focus:ring-2"
						style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground); --tw-ring-color: var(--color-primary)"
					/>
					<button
						type="button"
						class="flex items-center gap-1.5 rounded-md border px-3 py-2 text-sm font-medium transition-colors"
						style="border-color: var(--color-border); color: var(--color-foreground)"
						onclick={copyToClipboard}
					>
						{#if copiedToClipboard}
							<Check class="h-4 w-4" style="color: #22c55e" />
							Copied
						{:else}
							<Copy class="h-4 w-4" />
							Copy to Clipboard
						{/if}
					</button>
				</div>
			</div>

			<div class="mt-6 flex justify-end">
				<Dialog.Close
					class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
					style="background: var(--color-primary)"
				>
					Done
				</Dialog.Close>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<!-- Edit API Key Modal -->
<Dialog.Root bind:open={editModalOpen} onOpenChange={(isOpen) => { if (!isOpen) editForm = { id: '', name: '', scopes: '' }; }}>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-50 bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out" />
		<Dialog.Content
			class="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-lg border p-6 shadow-xl focus:outline-none"
			style="border-color: var(--color-border); background: var(--color-surface)"
			role="dialog"
			aria-modal="true"
			onOpenAutoFocus={(e) => e.preventDefault()}
		>
			<Dialog.Title class="text-lg font-semibold" style="color: var(--color-foreground)">
				Edit API Key
			</Dialog.Title>

			{#if errorMessage}
				<div
					class="mt-3 rounded-md p-3 text-sm"
					style="background: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
				>
					{errorMessage}
				</div>
			{/if}

			<div class="mt-4 space-y-4">
				<div>
					<label for="edit-name" class="block text-sm font-medium" style="color: var(--color-foreground)">
						Name <span style="color: var(--color-danger)">*</span>
					</label>
					<input
						id="edit-name"
						type="text"
						bind:value={editForm.name}
						placeholder="e.g., Production Backend"
						class="mt-1 block w-full rounded-md border px-3 py-2 text-sm outline-none focus:ring-2"
						style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground); --tw-ring-color: var(--color-primary)"
					/>
				</div>

				<div>
					<label for="edit-scopes" class="block text-sm font-medium" style="color: var(--color-foreground)">
						Scopes
					</label>
					<input
						id="edit-scopes"
						type="text"
						bind:value={editForm.scopes}
						placeholder="read,write,admin"
						class="mt-1 block w-full rounded-md border px-3 py-2 text-sm outline-none focus:ring-2"
						style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground); --tw-ring-color: var(--color-primary)"
					/>
					<p class="mt-1 text-xs" style="color: var(--color-muted)">Comma-separated list of scopes</p>
				</div>
			</div>

			<div class="mt-6 flex justify-end gap-3">
				<Dialog.Close
					class="rounded-md border px-4 py-2 text-sm font-medium transition-colors"
					style="border-color: var(--color-border); color: var(--color-foreground)"
				>
					Cancel
				</Dialog.Close>
				<button
					type="button"
					class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
					style="background: var(--color-primary)"
					disabled={editLoading || !editForm.name.trim()}
					onclick={updateKey}
				>
					{editLoading ? 'Saving...' : 'Save Changes'}
				</button>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
