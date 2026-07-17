<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { onMount } from 'svelte';
	import { api, type DeveloperApiKey } from '$lib/api';
	import { Key, Plus, Trash2, Copy, Check, Shield, AlertTriangle, Webhook as WebhookIcon } from 'lucide-svelte';

	let { data } = $props();

	let keys = $state<DeveloperApiKey[]>([]);
	let loading = $state(true);
	let showCreate = $state(false);
	let newKeyName = $state('');
	let newKeyScopes = $state(['api.read.*']);
	let createdKey = $state<string | null>(null);
	let copiedKey = $state(false);
	let errorMessage = $state('');

	let availableScopes = $state<{ value: string; label: string; description: string }[]>([]);

	function copyKey() {
		if (createdKey) {
			navigator.clipboard.writeText(createdKey);
			copiedKey = true;
			setTimeout(() => (copiedKey = false), 2000);
		}
	}

	function formatDate(dateStr: string): string {
		return new Date(dateStr).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
	}

	async function revokeKey(id: string) {
		try {
			await api.revokeDeveloperKey(id);
			keys = keys.filter((k) => k.id !== id);
		} catch {
			errorMessage = 'Failed to revoke key.';
		}
	}

	async function createKey() {
		if (!newKeyName.trim()) return;
		try {
			const result = await api.createDeveloperKey({ name: newKeyName, scopes: newKeyScopes });
			createdKey = result.key;
			keys = [...keys, result.key_info];
			newKeyName = '';
			showCreate = false;
		} catch {
			errorMessage = 'Failed to create key.';
		}
	}

	onMount(async () => {
		try {
			keys = await api.listDeveloperKeys();
		} catch (e) {
			errorMessage = 'Failed to load API keys.';
		} finally {
			loading = false;
		}

		try {
			const scopes = await api.listDeveloperScopes();
			if (Array.isArray(scopes)) {
				availableScopes = scopes;
			}
		} catch {
			availableScopes = [
				{ value: 'api.read.*', label: 'Read All', description: 'Read access to all resources' },
				{ value: 'content.read', label: 'Read Content', description: 'Read content and articles' },
				{ value: 'content.create', label: 'Create Content', description: 'Create new content' },
				{ value: 'content.edit', label: 'Edit Content', description: 'Edit existing content' },
				{ value: 'content.delete', label: 'Delete Content', description: 'Delete content' },
				{ value: 'spaces.read', label: 'Read Spaces', description: 'Read spaces and communities' },
				{ value: 'spaces.create', label: 'Create Spaces', description: 'Create new spaces' },
				{ value: 'forum.read', label: 'Read Forum', description: 'Read forum threads and replies' },
				{ value: 'forum.create', label: 'Create Forum', description: 'Create forum threads and replies' },
				{ value: 'users.read', label: 'Read Users', description: 'Read user profiles' },
				{ value: 'media.upload', label: 'Upload Media', description: 'Upload media files' },
				{ value: 'notifications.read', label: 'Read Notifications', description: 'Read notifications' },
				{ value: 'search', label: 'Search', description: 'Use search functionality' }
			];
		}
	});</script>

<svelte:head>
	<title>Developer Portal | JapaConnect</title>
</svelte:head>

<main class="mx-auto max-w-4xl px-4 py-8">
	<header class="mb-8">
		<div class="flex items-center gap-3">
			<div
				class="flex h-10 w-10 items-center justify-center rounded-xl"
				style="background: color-mix(in srgb, var(--color-primary) 12%, transparent)"
			>
				<Key class="h-5 w-5" style="color: var(--color-primary)" />
			</div>
			<div>
				<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">
					Developer Portal
				</h1>
				<p class="text-sm" style="color: var(--color-muted)">
					Manage your API keys for programmatic access.
				</p>
			</div>
		</div>
	</header>

	<nav class="mb-6 flex gap-1 rounded-lg border p-1" style="border-color: var(--color-border); background: var(--color-surface)">
		<a
			href="/dashboard/developer"
			class="flex items-center gap-2 rounded-md px-3 py-1.5 text-sm font-medium"
			style="background-color: var(--color-primary); color: white"
		>
			<Key class="h-3.5 w-3.5" />
			API Keys
		</a>
		<a
			href="/dashboard/webhooks"
			class="flex items-center gap-2 rounded-md px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
			style="color: var(--color-muted)"
		>
			<WebhookIcon class="h-3.5 w-3.5" />
			Webhooks
		</a>
	</nav>

	{#if errorMessage}
		<div
			class="mb-6 flex items-center gap-2 rounded-lg border p-3 text-sm"
			style="border-color: color-mix(in srgb, var(--color-danger) 30%, transparent); background: color-mix(in srgb, var(--color-danger) 6%, transparent); color: var(--color-danger)"
		>
			<AlertTriangle class="h-4 w-4 flex-shrink-0" />
			{errorMessage}
		</div>
	{/if}

	{#if createdKey}
		<div
			class="mb-6 rounded-xl border p-5"
			style="border-color: color-mix(in srgb, #22c55e 30%, transparent); background: color-mix(in srgb, #22c55e 6%, transparent)"
		>
			<p class="font-medium" style="color: #16a34a">{t('dashboard.developer.api_key_created')}</p>
			<div class="mt-3 flex items-center gap-2">
				<code
					class="flex-1 break-all rounded-lg px-3 py-2 text-sm"
					style="background: color-mix(in srgb, #22c55e 10%, transparent); color: var(--color-foreground)"
					>{createdKey}</code
				>
				<button
					class="flex h-8 w-8 items-center justify-center rounded-lg transition-colors"
					style="background: color-mix(in srgb, #22c55e 12%, transparent)"
					onclick={copyKey}
				>
					{#if copiedKey}
						<Check class="h-4 w-4" style="color: #16a34a" />
					{:else}
						<Copy class="h-4 w-4" style="color: #16a34a" />
					{/if}
				</button>
			</div>
			<p class="mt-2 text-xs" style="color: #16a34a">
				Copy this key now. It will not be shown again.
			</p>
			<button
				class="mt-3 text-sm font-medium underline"
				style="color: #16a34a"
				onclick={() => (createdKey = null)}
			>
				Done
			</button>
		</div>
	{/if}

	<div
		class="mb-6 rounded-xl border p-4"
		style="border-color: color-mix(in srgb, var(--color-primary) 20%, transparent); background: color-mix(in srgb, var(--color-primary) 5%, transparent)"
	>
		<div class="flex items-start gap-3">
			<Shield class="mt-0.5 h-4 w-4 flex-shrink-0" style="color: var(--color-primary)" />
			<div class="text-xs" style="color: var(--color-muted)">
				<p class="font-medium" style="color: var(--color-foreground)">{t('dashboard.developer.api_access')}</p>
				<p class="mt-1">
					Use your API keys to access JapaConnect programmatically. Keys are scoped to specific
					permissions and can be revoked at any time.
				</p>
			</div>
		</div>
	</div>

	<div class="flex items-center justify-between">
		<h2 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('dashboard.developer.my_api_keys')}</h2>
		<button
			class="flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
			style="background-color: var(--color-primary)"
			onclick={() => (showCreate = true)}
		>
			<Plus class="h-4 w-4" />
			Create New Key
		</button>
	</div>

	{#if loading}
		<div class="mt-4 space-y-3">
			{#each Array(3) as _}
				<div
					class="h-20 animate-pulse rounded-xl"
					style="background-color: var(--color-surface)"
				></div>
			{/each}
		</div>
	{:else if keys.length === 0}
		<div
			class="mt-4 rounded-xl border p-8 text-center"
			style="border-color: var(--color-border); background: var(--color-surface)"
		>
			<Key class="mx-auto h-8 w-8" style="color: var(--color-muted)" />
			<p class="mt-3 text-sm" style="color: var(--color-muted)">
				No API keys yet. Create one to get started.
			</p>
		</div>
	{:else}
		<div class="mt-4 space-y-3">
			{#each keys as key (key.id)}
				<div
					class="rounded-xl border p-4 transition-all hover:shadow-sm"
					style="border-color: var(--color-border); background: var(--color-surface)"
				>
					<div class="flex items-start justify-between">
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-2">
								<p class="font-medium" style="color: var(--color-foreground)">{key.name}</p>
								<span
									class="rounded-full px-2 py-0.5 text-xs font-medium"
									style="background: color-mix(in srgb, {key.status === 'active'
										? '#22c55e'
										: '#ef4444'} 12%, transparent); color: {key.status === 'active'
										? '#16a34a'
										: '#dc2626'}"
								>
									{key.status}
								</span>
							</div>
							<p class="mt-1 font-mono text-xs" style="color: var(--color-muted)">
								{key.key_prefix}...
							</p>
							<div class="mt-2 flex flex-wrap gap-1">
								{#each key.scopes as scope}
									<span
										class="rounded px-1.5 py-0.5 text-xs"
										style="background: color-mix(in srgb, var(--color-primary) 8%, transparent); color: var(--color-primary)"
									>
										{scope}
									</span>
								{/each}
							</div>
							<p class="mt-2 text-xs" style="color: var(--color-muted)">
								Created {formatDate(key.created_at)}
								{#if key.last_used_at}
									· Last used {formatDate(key.last_used_at)}
								{/if}
							</p>
						</div>
						{#if key.status !== 'revoked'}
							<button
								class="flex items-center gap-1 rounded-lg px-3 py-1.5 text-xs font-medium transition-colors"
								style="color: var(--color-danger)"
								onclick={() => revokeKey(key.id)}
							>
								<Trash2 class="h-3.5 w-3.5" />
								Revoke
							</button>
						{/if}
					</div>
				</div>
			{/each}
		</div>
	{/if}

	{#if showCreate}
		<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
			<div
				class="mx-4 w-full max-w-md rounded-xl p-6 shadow-xl"
				style="background: var(--color-surface); border: 1px solid var(--color-border)"
			>
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">
					Create API Key
				</h3>
				<p class="mt-1 text-sm" style="color: var(--color-muted)">
					Give your key a name to identify it later.
				</p>

				<input
					bind:value={newKeyName}
					placeholder="e.g. My Integration"
					class="mt-4 w-full rounded-lg border px-3 py-2 text-sm outline-none transition-colors focus:ring-2"
					style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground); --tw-ring-color: var(--color-primary)"
				/>

				<div class="mt-4">
					<p class="mb-2 text-xs font-medium" style="color: var(--color-muted)">{t('dashboard.developer.scopes')}</p>
					<div class="flex flex-wrap gap-2">
						{#each availableScopes as scope}
							<label class="flex items-center gap-1.5 text-xs" style="color: var(--color-foreground)" title={scope.description}>
								<input
									type="checkbox"
									checked={newKeyScopes.includes(scope.value)}
									onchange={() => {
										if (newKeyScopes.includes(scope.value)) {
											newKeyScopes = newKeyScopes.filter((s) => s !== scope.value);
										} else {
											newKeyScopes = [...newKeyScopes, scope.value];
										}
									}}
									class="rounded"
								/>
								{scope.label}
							</label>
						{/each}
					</div>
				</div>

				<div class="mt-6 flex justify-end gap-2">
					<button
						class="rounded-lg px-4 py-2 text-sm font-medium transition-colors"
						style="color: var(--color-muted)"
						onclick={() => (showCreate = false)}
					>
						Cancel
					</button>
					<button
						class="rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
						style="background-color: var(--color-primary)"
						onclick={createKey}
					>
						Create Key
					</button>
				</div>
			</div>
		</div>
	{/if}
</main>
