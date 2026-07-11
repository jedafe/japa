<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { apiFetch } from '$lib/api/client';
	import type { ApiKey, CreateApiKeyResponse } from '$lib/api/types';
	import { Key, Copy, Trash2, Plus, AlertTriangle, Check, X, Eye } from 'lucide-svelte';

	let { data } = $props<{ data: { keys: ApiKey[] } }>();

	let keys = $state<ApiKey[]>([]);
	let showCreateModal = $state(false);
	let showKeyModal = $state(false);
	let createdKey = $state<CreateApiKeyResponse | null>(null);
	let copied = $state(false);

	$effect(() => {
		keys = data.keys;
	});

	// Create form state
	let newName = $state('');
	let newScopes = $state<string[]>(['read']);
	let creating = $state(false);
	let createError = $state('');

	// Revoke state
	let revokingId = $state<string | null>(null);
	let confirmRevokeId = $state<string | null>(null);

	const availableScopes = ['read', 'write', 'admin'];

	function toggleScope(scope: string) {
		if (newScopes.includes(scope)) {
			newScopes = newScopes.filter((s) => s !== scope);
		} else {
			newScopes = [...newScopes, scope];
		}
	}

	async function createKey() {
		if (!newName.trim()) {
			createError = 'Key name is required.';
			return;
		}
		if (newScopes.length === 0) {
			createError = 'Select at least one scope.';
			return;
		}

		creating = true;
		createError = '';

		const result = await apiFetch<CreateApiKeyResponse>('/developer/keys', {
			method: 'POST',
			body: JSON.stringify({
				name: newName.trim(),
				scopes: newScopes,
				rate_limit_tier: 'standard',
			}),
		});

		if (result.data) {
			createdKey = result.data;
			showKeyModal = true;
			showCreateModal = false;
			// Add the new key to the list (without the full secret)
			keys = [
				{
					id: result.data.id,
					name: result.data.name,
					key_prefix: result.data.key_prefix,
					scopes: result.data.scopes,
					rate_limit_tier: 'standard',
					status: result.data.status,
					last_used_at: null,
					request_count: 0,
					expires_at: null,
					created_at: result.data.created_at,
					updated_at: result.data.created_at,
				},
				...keys,
			];
			newName = '';
			newScopes = ['read'];
		} else {
			createError = result.error?.message ?? 'Failed to create API key.';
		}

		creating = false;
	}

	async function revokeKey(id: string) {
		revokingId = id;
		const result = await apiFetch(`/developer/keys/${id}`, { method: 'DELETE' });
		if (result.status === 200 || result.status === 204) {
			keys = keys.map((k) => (k.id === id ? { ...k, status: 'revoked' } : k));
		}
		revokingId = null;
		confirmRevokeId = null;
	}

	function copyKey() {
		if (!createdKey?.key) return;
		navigator.clipboard.writeText(createdKey.key);
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}

	function formatDate(dateStr: string | null): string {
		if (!dateStr) return '—';
		return new Date(dateStr).toLocaleDateString('en-US', {
			year: 'numeric',
			month: 'short',
			day: 'numeric',
		});
	}

	function maskKey(prefix: string): string {
		return `${prefix}...`;
	}

	function statusColor(status: string): string {
		switch (status) {
			case 'active':
				return 'var(--color-success)';
			case 'revoked':
				return 'var(--color-danger)';
			default:
				return 'var(--color-muted)';
		}
	}

	function closeCreateModal() {
		showCreateModal = false;
		newName = '';
		newScopes = ['read'];
		createError = '';
	}

	function closeKeyModal() {
		showKeyModal = false;
		createdKey = null;
		copied = false;
	}
</script>

<svelte:head>
	<title>API Keys — JapaConnect Docs</title>
</svelte:head>

<div class="mx-auto max-w-4xl">
	<!-- Header -->
	<div class="mb-8">
		<h1 class="text-3xl font-bold tracking-tight">Developer API Keys</h1>
		<p class="mt-2 text-sm" style="color: var(--color-muted)">
			Create and manage API keys for programmatic access to the JapaConnect API.
			Keys are shown only once at creation — store them securely.
		</p>
	</div>

	<!-- Create button -->
	<div class="mb-6 flex items-center justify-between">
		<span class="text-sm" style="color: var(--color-muted)">
			{keys.length} key{keys.length !== 1 ? 's' : ''}
		</span>
		<button
			class="flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
			style="background-color: var(--color-primary)"
			onclick={() => (showCreateModal = true)}
		>
			<Plus class="h-4 w-4" />
			Create New Key
		</button>
	</div>

	<!-- Keys table -->
	{#if keys.length === 0}
		<div
			class="rounded-lg border p-12 text-center"
			style="border-color: var(--color-border); background-color: var(--color-surface)"
		>
			<Key class="mx-auto h-10 w-10" style="color: var(--color-muted)" />
			<h3 class="mt-4 text-lg font-semibold">No API keys yet</h3>
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				Create your first API key to start integrating with the JapaConnect API.
			</p>
			<button
				class="mt-6 inline-flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
				style="background-color: var(--color-primary)"
				onclick={() => (showCreateModal = true)}
			>
				<Plus class="h-4 w-4" />
				Create Your First Key
			</button>
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
						<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Key</th>
						<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Scopes</th>
						<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Status</th>
						<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Created</th>
						<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Last Used</th>
						<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each keys as key (key.id)}
						<tr style="border-bottom: 1px solid var(--color-border)">
							<td class="px-4 py-3 font-medium">{key.name}</td>
							<td class="px-4 py-3 font-mono text-xs">{maskKey(key.key_prefix)}</td>
							<td class="px-4 py-3">
								<div class="flex flex-wrap gap-1">
									{#each key.scopes as scope}
										<span
											class="rounded px-1.5 py-0.5 text-xs"
											style="background: color-mix(in srgb, var(--color-primary) 8%, transparent); color: var(--color-primary)"
										>
											{scope}
										</span>
									{/each}
								</div>
							</td>
							<td class="px-4 py-3">
								<span
									class="rounded-full px-2 py-0.5 text-xs font-medium"
									style="background: color-mix(in srgb, {statusColor(key.status)} 12%, transparent); color: {statusColor(key.status)}"
								>
									{key.status}
								</span>
							</td>
							<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">
								{formatDate(key.created_at)}
							</td>
							<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">
								{formatDate(key.last_used_at)}
							</td>
							<td class="px-4 py-3">
								{#if key.status === 'active'}
									{#if confirmRevokeId === key.id}
										<div class="flex items-center gap-2">
											<span class="text-xs" style="color: var(--color-danger)">Revoke?</span>
											<button
												class="rounded px-2 py-1 text-xs font-medium transition-colors"
												style="color: var(--color-danger)"
												disabled={revokingId === key.id}
												onclick={() => revokeKey(key.id)}
											>
												{revokingId === key.id ? 'Revoking...' : 'Yes'}
											</button>
											<button
												class="rounded px-2 py-1 text-xs transition-colors"
												style="color: var(--color-muted)"
												onclick={() => (confirmRevokeId = null)}
											>
												No
											</button>
										</div>
									{:else}
										<button
											class="flex items-center gap-1 rounded px-2 py-1 text-xs font-medium transition-colors"
											style="color: var(--color-danger)"
											onclick={() => (confirmRevokeId = key.id)}
										>
											<Trash2 class="h-3 w-3" />
											Revoke
										</button>
									{/if}
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<!-- Create Key Modal -->
{#if showCreateModal}
	<!-- svelte-ignore a11y_no_static_element_interactions a11y_click_events_have_key_events -->
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onclick={closeCreateModal}>
		<div
			class="w-full max-w-md rounded-lg border p-6 shadow-xl"
			style="border-color: var(--color-border); background-color: var(--color-background)"
			onclick={(e) => e.stopPropagation()}
		>
			<div class="mb-4 flex items-center justify-between">
				<h2 class="text-lg font-semibold">Create New API Key</h2>
				<button
					class="rounded p-1 transition-colors hover:bg-black/5 dark:hover:bg-white/10"
					onclick={closeCreateModal}
				>
					<X class="h-4 w-4" style="color: var(--color-muted)" />
				</button>
			</div>

			{#if createError}
				<div
					class="mb-4 flex items-center gap-2 rounded-lg border p-3 text-sm"
					style="border-color: color-mix(in srgb, var(--color-danger) 30%, transparent); background: color-mix(in srgb, var(--color-danger) 6%, transparent); color: var(--color-danger)"
				>
					<AlertTriangle class="h-4 w-4 flex-shrink-0" />
					{createError}
				</div>
			{/if}

			<div class="space-y-4">
				<div>
					<label for="key-name" class="mb-1 block text-sm font-medium">Key Name</label>
					<input
						id="key-name"
						type="text"
						class="w-full rounded-md border px-3 py-2 text-sm outline-none focus:ring-2"
						style="border-color: var(--color-border); background: var(--color-background); --tw-ring-color: var(--color-primary)"
						placeholder="e.g. production-server, ci-pipeline"
						bind:value={newName}
					/>
					<p class="mt-1 text-xs" style="color: var(--color-muted)">
						A descriptive name to help you identify this key later.
					</p>
				</div>

				<div>
					<!-- svelte-ignore a11y_label_has_associated_control -->
					<label class="mb-2 block text-sm font-medium">Scopes</label>
					<div class="flex flex-wrap gap-2">
						{#each availableScopes as scope}
							<button
								class="rounded-md border px-3 py-1.5 text-sm font-medium transition-colors"
								style={newScopes.includes(scope)
									? 'border-color: var(--color-primary); background: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)'
									: 'border-color: var(--color-border); color: var(--color-muted)'}
								onclick={() => toggleScope(scope)}
							>
								{scope}
							</button>
						{/each}
					</div>
					<p class="mt-1 text-xs" style="color: var(--color-muted)">
						Select the permissions this key will have.
					</p>
				</div>
			</div>

			<div class="mt-6 flex justify-end gap-3">
				<button
					class="rounded-md border px-4 py-2 text-sm font-medium transition-colors"
					style="border-color: var(--color-border); color: var(--color-muted)"
					onclick={closeCreateModal}
				>
					Cancel
				</button>
				<button
					class="rounded-md px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
					style="background-color: var(--color-primary)"
					disabled={creating}
					onclick={createKey}
				>
					{creating ? 'Creating...' : 'Create Key'}
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- Key Created Modal -->
{#if showKeyModal && createdKey}
	<!-- svelte-ignore a11y_no_static_element_interactions a11y_click_events_have_key_events -->
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onclick={closeKeyModal}>
		<div
			class="w-full max-w-lg rounded-lg border p-6 shadow-xl"
			style="border-color: var(--color-border); background-color: var(--color-background)"
			onclick={(e) => e.stopPropagation()}
		>
			<div class="mb-4 flex items-center gap-3">
				<div
					class="flex h-10 w-10 items-center justify-center rounded-full"
					style="background: color-mix(in srgb, var(--color-success) 12%, transparent)"
				>
					<Check class="h-5 w-5" style="color: var(--color-success)" />
				</div>
				<div>
					<h2 class="text-lg font-semibold">API Key Created</h2>
					<p class="text-sm" style="color: var(--color-muted)">
						Copy your key now — it won't be shown again.
					</p>
				</div>
			</div>

			<div
				class="mb-4 flex items-center gap-2 rounded-lg border p-3"
				style="border-color: color-mix(in srgb, var(--color-warning) 30%, transparent); background: color-mix(in srgb, var(--color-warning) 6%, transparent)"
			>
				<AlertTriangle class="h-4 w-4 flex-shrink-0" style="color: var(--color-warning)" />
				<p class="text-sm" style="color: var(--color-warning)">
					This is the only time you'll see this key. Store it securely.
				</p>
			</div>

			<div
				class="mb-4 flex items-center gap-2 rounded-lg border p-3 font-mono text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
			>
				<Eye class="h-4 w-4 flex-shrink-0" style="color: var(--color-muted)" />
				<code class="flex-1 break-all">{createdKey.key}</code>
				<button
					class="flex-shrink-0 rounded p-1 transition-colors hover:bg-black/5 dark:hover:bg-white/10"
					onclick={copyKey}
				>
					{#if copied}
						<Check class="h-4 w-4" style="color: var(--color-success)" />
					{:else}
						<Copy class="h-4 w-4" style="color: var(--color-muted)" />
					{/if}
				</button>
			</div>

			<div class="flex justify-end">
				<button
					class="rounded-md px-4 py-2 text-sm font-medium transition-colors"
					style="background-color: var(--color-surface); color: var(--color-foreground)"
					onclick={closeKeyModal}
				>
					I've saved my key
				</button>
			</div>
		</div>
	</div>
{/if}
