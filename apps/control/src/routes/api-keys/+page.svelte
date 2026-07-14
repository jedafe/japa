<script lang="ts">
	import { api } from '$lib/api';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import Pagination from '$lib/components/ui/Pagination.svelte';
	import { Key, Search, Ban, Trash2, Plus, Pencil, Copy, Check } from 'lucide-svelte';
	import { Dialog, Tabs, Tooltip, Select } from 'bits-ui';
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
	let filterTier = $state('all');
	let activeTab = $state('active');
	// svelte-ignore state_referenced_locally
	let errorMessage = $state(data.error ?? '');
	let confirmOpen = $state(false);
	let confirmAction = $state<{ type: 'revoke' | 'suspend'; key: AdminApiKey } | null>(null);

	// Pagination
	let page = $state(0);
	let pageSize = $state(10);

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

	const tierItems = [
		{ value: 'all', label: 'All Tiers' },
		{ value: 'standard', label: 'Standard' },
		{ value: 'premium', label: 'Premium' },
		{ value: 'unlimited', label: 'Unlimited' }
	];

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
		keys
			.filter((k) => {
				if (activeTab === 'active' && k.status !== 'active') return false;
				if (filterTier !== 'all' && k.rate_limit_tier !== filterTier) return false;
				if (filter) {
					const q = filter.toLowerCase();
					if (
						!k.name?.toLowerCase().includes(q) &&
						!k.key_prefix?.toLowerCase().includes(q) &&
						!k.user_id?.toLowerCase().includes(q)
					)
						return false;
				}
				return true;
			})
	);

	let totalPages = $derived(Math.max(1, Math.ceil(filtered.length / pageSize)));
	let paginatedKeys = $derived.by(() => {
		const start = page * pageSize;
		return filtered.slice(start, start + pageSize);
	});

	let activeCount = $derived(keys.filter((k) => k.status === 'active').length);

	function statusColor(status: string): string {
		switch (status) {
			case 'active':
				return 'text-emerald-500';
			case 'suspended':
				return 'text-amber-500';
			case 'revoked':
				return 'text-destructive';
			default:
				return 'text-muted-foreground';
		}
	}

	function statusBg(status: string): string {
		switch (status) {
			case 'active':
				return 'bg-emerald-500/10';
			case 'suspended':
				return 'bg-amber-500/10';
			case 'revoked':
				return 'bg-destructive/10';
			default:
				return 'bg-muted';
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
	<div class="border-destructive bg-destructive/10 text-destructive mb-4 rounded-xl border px-4 py-3 text-sm">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex items-center gap-3">
	<div class="relative">
		<Search class="text-muted-foreground absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" />
		<input
			bind:value={filter}
			placeholder="Filter by name, prefix, or user..."
			class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 h-input rounded-9px w-72 border px-3 py-2.5 pl-9 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
		/>
	</div>

	<Select.Root type="single" bind:value={filterTier} items={tierItems}>
		<Select.Trigger class="border-border-input bg-background rounded-9px border px-3 py-2 text-sm" aria-label="Filter tier">
			<Select.Value placeholder="All Tiers" />
		</Select.Trigger>
		<Select.Portal>
			<Select.Content class="border-border-input bg-background shadow-popover z-50 rounded-xl border px-1 py-1">
				<Select.Viewport class="p-1">
					{#each tierItems as item}
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

	<span class="text-muted-foreground text-xs">
		{filtered.length} key{filtered.length !== 1 ? 's' : ''}
	</span>
	<div class="flex-1"></div>
	<button
		class="bg-foreground text-background flex items-center gap-1.5 rounded-9px px-3 py-2 text-sm font-medium transition-colors hover:opacity-90"
		onclick={() => (createModalOpen = true)}
	>
		<Plus class="h-4 w-4" />
		Create API Key
	</button>
</div>

<Tabs.Root bind:value={activeTab}>
	<Tabs.List class="bg-muted inline-flex gap-1 rounded-9px p-1">
		<Tabs.Trigger value="active" class="data-[state=active]:bg-background data-[state=active]:shadow-mini rounded-7px px-4 py-2 text-sm font-medium transition-all">
			Active Keys ({activeCount})
		</Tabs.Trigger>
		<Tabs.Trigger value="all" class="data-[state=active]:bg-background data-[state=active]:shadow-mini rounded-7px px-4 py-2 text-sm font-medium transition-all">
			All Keys ({keys.length})
		</Tabs.Trigger>
	</Tabs.List>

	<Tabs.Content value="active" class="mt-4">
		{#if loading}
			<div class="space-y-3">
				{#each Array(5) as _}
					<div class="bg-muted h-16 animate-pulse rounded-lg"></div>
				{/each}
			</div>
		{:else if filtered.length === 0}
			<div class="border-border-card bg-background-alt rounded-9px border p-8 text-center">
				<Key class="text-muted-foreground mx-auto h-8 w-8" />
				<p class="text-muted-foreground mt-3 text-sm">
					{filter || filterTier !== 'all' ? 'No keys match your filter.' : 'No active API keys.'}
				</p>
			</div>
		{:else}
			<div class="border-border-card bg-background-alt shadow-card overflow-hidden rounded-xl">
				<table class="text-foreground w-full text-left text-sm">
					<thead>
						<tr class="border-border-card border-b">
							<th class="text-muted-foreground px-4 py-3 font-medium">Name</th>
							<th class="text-muted-foreground px-4 py-3 font-medium">Key Prefix</th>
							<th class="text-muted-foreground px-4 py-3 font-medium">User</th>
							<th class="text-muted-foreground px-4 py-3 font-medium">Tier</th>
							<th class="text-muted-foreground px-4 py-3 font-medium">Status</th>
							<th class="text-muted-foreground px-4 py-3 text-right font-medium">Requests</th>
							<th class="text-muted-foreground px-4 py-3 font-medium">Actions</th>
						</tr>
					</thead>
					<tbody>
						{#each paginatedKeys as key (key.id)}
							<tr class="border-border-card border-t">
								<td class="px-4 py-3 font-medium">{key.name}</td>
								<td class="font-muted-foreground px-4 py-3 font-mono text-xs">{key.key_prefix}...</td>
								<td class="text-muted-foreground px-4 py-3 text-xs">
									{key.user_id ? key.user_id.slice(0, 8) + '...' : 'System'}
								</td>
								<td class="px-4 py-3">
									<span class="bg-accent text-accent-foreground rounded-5px px-1.5 py-0.5 text-xs">
										{key.rate_limit_tier}
									</span>
								</td>
								<td class="px-4 py-3">
									<span class="{statusBg(key.status)} {statusColor(key.status)} rounded-full px-2 py-0.5 text-xs font-medium">
										{key.status}
									</span>
								</td>
								<td class="px-4 py-3 text-right font-mono text-xs">
									{formatNumber(key.request_count)}
								</td>
								<td class="px-4 py-3">
									<div class="flex items-center gap-1">
										{#if key.status !== 'revoked'}
											<Tooltip.Root>
												<Tooltip.Trigger>
													<button
														class="text-accent-foreground hover:bg-accent flex items-center gap-1 rounded-5px px-2 py-1 text-xs font-medium transition-colors"
														onclick={() => openEditModal(key)}
													>
														<Pencil class="h-3 w-3" />
													</button>
												</Tooltip.Trigger>
												<Tooltip.Portal>
													<Tooltip.Content side="top" sideOffset={4} class="border-dark-10 bg-background shadow-popover z-50 rounded-input border px-3 py-2 text-xs">
														Edit key
													</Tooltip.Content>
												</Tooltip.Portal>
											</Tooltip.Root>
										{/if}
										{#if key.status === 'active'}
											<Tooltip.Root>
												<Tooltip.Trigger>
													<button
														class="text-amber-500 hover:bg-amber-500/10 flex items-center gap-1 rounded-5px px-2 py-1 text-xs font-medium transition-colors"
														onclick={() => (confirmAction = { type: 'suspend', key })}
													>
														<Ban class="h-3 w-3" />
													</button>
												</Tooltip.Trigger>
												<Tooltip.Portal>
													<Tooltip.Content side="top" sideOffset={4} class="border-dark-10 bg-background shadow-popover z-50 rounded-input border px-3 py-2 text-xs">
														Suspend key
													</Tooltip.Content>
												</Tooltip.Portal>
											</Tooltip.Root>
										{/if}
										{#if key.status !== 'revoked'}
											<Tooltip.Root>
												<Tooltip.Trigger>
													<button
														class="text-destructive hover:bg-destructive/10 flex items-center gap-1 rounded-5px px-2 py-1 text-xs font-medium transition-colors"
														onclick={() => (confirmAction = { type: 'revoke', key })}
													>
														<Trash2 class="h-3 w-3" />
													</button>
												</Tooltip.Trigger>
												<Tooltip.Portal>
													<Tooltip.Content side="top" sideOffset={4} class="border-dark-10 bg-background shadow-popover z-50 rounded-input border px-3 py-2 text-xs">
														Revoke key
													</Tooltip.Content>
												</Tooltip.Portal>
											</Tooltip.Root>
										{/if}
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
	</Tabs.Content>

	<Tabs.Content value="all" class="mt-4">
		{#if loading}
			<div class="space-y-3">
				{#each Array(5) as _}
					<div class="bg-muted h-16 animate-pulse rounded-lg"></div>
				{/each}
			</div>
		{:else if filtered.length === 0}
			<div class="border-border-card bg-background-alt rounded-9px border p-8 text-center">
				<Key class="text-muted-foreground mx-auto h-8 w-8" />
				<p class="text-muted-foreground mt-3 text-sm">
					{filter || filterTier !== 'all' ? 'No keys match your filter.' : 'No API keys exist yet.'}
				</p>
			</div>
		{:else}
			<div class="border-border-card bg-background-alt shadow-card overflow-hidden rounded-xl">
				<table class="text-foreground w-full text-left text-sm">
					<thead>
						<tr class="border-border-card border-b">
							<th class="text-muted-foreground px-4 py-3 font-medium">Name</th>
							<th class="text-muted-foreground px-4 py-3 font-medium">Key Prefix</th>
							<th class="text-muted-foreground px-4 py-3 font-medium">User</th>
							<th class="text-muted-foreground px-4 py-3 font-medium">Tier</th>
							<th class="text-muted-foreground px-4 py-3 font-medium">Status</th>
							<th class="text-muted-foreground px-4 py-3 text-right font-medium">Requests</th>
							<th class="text-muted-foreground px-4 py-3 font-medium">Actions</th>
						</tr>
					</thead>
					<tbody>
						{#each paginatedKeys as key (key.id)}
							<tr class="border-border-card border-t">
								<td class="px-4 py-3 font-medium">{key.name}</td>
								<td class="text-muted-foreground px-4 py-3 font-mono text-xs">{key.key_prefix}...</td>
								<td class="text-muted-foreground px-4 py-3 text-xs">
									{key.user_id ? key.user_id.slice(0, 8) + '...' : 'System'}
								</td>
								<td class="px-4 py-3">
									<span class="bg-accent text-accent-foreground rounded-5px px-1.5 py-0.5 text-xs">
										{key.rate_limit_tier}
									</span>
								</td>
								<td class="px-4 py-3">
									<span class="{statusBg(key.status)} {statusColor(key.status)} rounded-full px-2 py-0.5 text-xs font-medium">
										{key.status}
									</span>
								</td>
								<td class="px-4 py-3 text-right font-mono text-xs">
									{formatNumber(key.request_count)}
								</td>
								<td class="px-4 py-3">
									<div class="flex items-center gap-1">
										{#if key.status !== 'revoked'}
											<Tooltip.Root>
												<Tooltip.Trigger>
													<button
														class="text-accent-foreground hover:bg-accent flex items-center gap-1 rounded-5px px-2 py-1 text-xs font-medium transition-colors"
														onclick={() => openEditModal(key)}
													>
														<Pencil class="h-3 w-3" />
													</button>
												</Tooltip.Trigger>
												<Tooltip.Portal>
													<Tooltip.Content side="top" sideOffset={4} class="border-dark-10 bg-background shadow-popover z-50 rounded-input border px-3 py-2 text-xs">
														Edit key
													</Tooltip.Content>
												</Tooltip.Portal>
											</Tooltip.Root>
										{/if}
										{#if key.status === 'active'}
											<Tooltip.Root>
												<Tooltip.Trigger>
													<button
														class="text-amber-500 hover:bg-amber-500/10 flex items-center gap-1 rounded-5px px-2 py-1 text-xs font-medium transition-colors"
														onclick={() => (confirmAction = { type: 'suspend', key })}
													>
														<Ban class="h-3 w-3" />
													</button>
												</Tooltip.Trigger>
												<Tooltip.Portal>
													<Tooltip.Content side="top" sideOffset={4} class="border-dark-10 bg-background shadow-popover z-50 rounded-input border px-3 py-2 text-xs">
														Suspend key
													</Tooltip.Content>
												</Tooltip.Portal>
											</Tooltip.Root>
										{/if}
										{#if key.status !== 'revoked'}
											<Tooltip.Root>
												<Tooltip.Trigger>
													<button
														class="text-destructive hover:bg-destructive/10 flex items-center gap-1 rounded-5px px-2 py-1 text-xs font-medium transition-colors"
														onclick={() => (confirmAction = { type: 'revoke', key })}
													>
														<Trash2 class="h-3 w-3" />
													</button>
												</Tooltip.Trigger>
												<Tooltip.Portal>
													<Tooltip.Content side="top" sideOffset={4} class="border-dark-10 bg-background shadow-popover z-50 rounded-input border px-3 py-2 text-xs">
														Revoke key
													</Tooltip.Content>
												</Tooltip.Portal>
											</Tooltip.Root>
										{/if}
									</div>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</Tabs.Content>
</Tabs.Root>

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
		<Dialog.Overlay class="bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out fixed inset-0 z-50" />
		<Dialog.Content
			class="border-border-card bg-background shadow-popover focus:outline-none fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-xl border p-6"
			role="dialog"
			aria-modal="true"
			onOpenAutoFocus={(e) => e.preventDefault()}
		>
			<Dialog.Title class="text-foreground text-lg font-semibold">
				Create API Key
			</Dialog.Title>
			<p class="text-muted-foreground mt-1 text-sm">
				Generate a new API key for a developer.
			</p>

			{#if errorMessage}
				<div class="bg-destructive/10 text-destructive mt-3 rounded-md p-3 text-sm">
					{errorMessage}
				</div>
			{/if}

			<div class="mt-4 space-y-4">
				<div>
					<label for="create-name" class="text-foreground block text-sm font-medium">
						Name <span class="text-destructive">*</span>
					</label>
					<input
						id="create-name"
						type="text"
						bind:value={createForm.name}
						placeholder="e.g., Production Backend"
						class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 mt-1 block w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
					/>
				</div>

				<div>
					<label for="create-scopes" class="text-foreground block text-sm font-medium">
						Scopes
					</label>
					<input
						id="create-scopes"
						type="text"
						bind:value={createForm.scopes}
						placeholder="read,write,admin"
						class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 mt-1 block w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
					/>
					<p class="text-muted-foreground mt-1 text-xs">Comma-separated list of scopes</p>
				</div>

				<div>
					<label for="create-tier" class="text-foreground block text-sm font-medium">
						Rate Limit Tier
					</label>
					<select
						id="create-tier"
						bind:value={createForm.rate_limit_tier}
						class="border-border-input bg-background text-foreground mt-1 block w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
					>
						<option value="standard">Standard</option>
						<option value="premium">Premium</option>
						<option value="unlimited">Unlimited</option>
					</select>
				</div>

				<div>
					<label for="create-expires" class="text-foreground block text-sm font-medium">
						Expires At
					</label>
					<input
						id="create-expires"
						type="datetime-local"
						bind:value={createForm.expires_at}
						class="border-border-input bg-background text-foreground mt-1 block w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
					/>
					<p class="text-muted-foreground mt-1 text-xs">Leave empty for no expiration</p>
				</div>
			</div>

			<div class="mt-6 flex justify-end gap-3">
				<Dialog.Close class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors">
					Cancel
				</Dialog.Close>
				<button
					type="button"
					class="bg-foreground text-background rounded-9px px-4 py-2 text-sm font-medium transition-colors hover:opacity-90 disabled:opacity-50"
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
		<Dialog.Overlay class="bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out fixed inset-0 z-50" />
		<Dialog.Content
			class="border-border-card bg-background shadow-popover focus:outline-none fixed left-1/2 top-1/2 z-50 w-full max-w-lg -translate-x-1/2 -translate-y-1/2 rounded-xl border p-6"
			role="dialog"
			aria-modal="true"
			onOpenAutoFocus={(e) => e.preventDefault()}
		>
			<Dialog.Title class="text-foreground text-lg font-semibold">
				API Key Created
			</Dialog.Title>

			<div class="bg-amber-500/10 mt-3 rounded-md p-3">
				<p class="text-amber-500 text-sm font-medium">
					Save this key now. It will not be shown again.
				</p>
			</div>

			<div class="mt-4">
				<label for="one-time-key" class="text-foreground block text-sm font-medium">
					Your API Key
				</label>
				<div class="mt-1 flex items-center gap-2">
					<input
						id="one-time-key"
						type="text"
						readonly
						value={oneTimeKeyValue}
						class="border-border-input bg-background text-foreground flex-1 rounded-9px border px-3 py-2 font-mono text-sm focus:shadow-date-field-focus focus:outline-none"
					/>
					<button
						type="button"
						class="border-border-input bg-background-alt hover:bg-muted text-foreground flex items-center gap-1.5 rounded-9px border px-3 py-2 text-sm font-medium transition-colors"
						onclick={copyToClipboard}
					>
						{#if copiedToClipboard}
							<Check class="h-4 w-4 text-emerald-500" />
							Copied
						{:else}
							<Copy class="h-4 w-4" />
							Copy to Clipboard
						{/if}
					</button>
				</div>
			</div>

			<div class="mt-6 flex justify-end">
				<Dialog.Close class="bg-foreground text-background rounded-9px px-4 py-2 text-sm font-medium transition-colors hover:opacity-90">
					Done
				</Dialog.Close>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<!-- Edit API Key Modal -->
<Dialog.Root bind:open={editModalOpen} onOpenChange={(isOpen) => { if (!isOpen) editForm = { id: '', name: '', scopes: '' }; }}>
	<Dialog.Portal>
		<Dialog.Overlay class="bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out fixed inset-0 z-50" />
		<Dialog.Content
			class="border-border-card bg-background shadow-popover focus:outline-none fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-xl border p-6"
			role="dialog"
			aria-modal="true"
			onOpenAutoFocus={(e) => e.preventDefault()}
		>
			<Dialog.Title class="text-foreground text-lg font-semibold">
				Edit API Key
			</Dialog.Title>

			{#if errorMessage}
				<div class="bg-destructive/10 text-destructive mt-3 rounded-md p-3 text-sm">
					{errorMessage}
				</div>
			{/if}

			<div class="mt-4 space-y-4">
				<div>
					<label for="edit-name" class="text-foreground block text-sm font-medium">
						Name <span class="text-destructive">*</span>
					</label>
					<input
						id="edit-name"
						type="text"
						bind:value={editForm.name}
						placeholder="e.g., Production Backend"
						class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 mt-1 block w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
					/>
				</div>

				<div>
					<label for="edit-scopes" class="text-foreground block text-sm font-medium">
						Scopes
					</label>
					<input
						id="edit-scopes"
						type="text"
						bind:value={editForm.scopes}
						placeholder="read,write,admin"
						class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 mt-1 block w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
					/>
					<p class="text-muted-foreground mt-1 text-xs">Comma-separated list of scopes</p>
				</div>
			</div>

			<div class="mt-6 flex justify-end gap-3">
				<Dialog.Close class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors">
					Cancel
				</Dialog.Close>
				<button
					type="button"
					class="bg-foreground text-background rounded-9px px-4 py-2 text-sm font-medium transition-colors hover:opacity-90 disabled:opacity-50"
					disabled={editLoading || !editForm.name.trim()}
					onclick={updateKey}
				>
					{editLoading ? 'Saving...' : 'Save Changes'}
				</button>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
