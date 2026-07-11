<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { Key, Search, Ban, Trash2 } from 'lucide-svelte';

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
	}

	let keys = $state<AdminApiKey[]>([]);
	let loading = $state(true);
	let filter = $state('');
	let errorMessage = $state('');
	let confirmOpen = $state(false);
	let confirmAction = $state<{ type: 'revoke' | 'suspend'; key: AdminApiKey } | null>(null);

	onMount(async () => {
		const result = await api<AdminApiKey[]>('/control/api-keys');
		if (result.ok) {
			keys = result.data;
		} else {
			errorMessage = result.error || 'Failed to load API keys.';
		}
		loading = false;
	});

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
		const result = await api(`/control/api-keys/${id}/suspend`, { method: 'POST' });
		if (result.ok) {
			keys = keys.map((k) => (k.id === id ? { ...k, status: 'revoked' } : k));
		} else {
			errorMessage = result.error || 'Failed to revoke key.';
		}
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
