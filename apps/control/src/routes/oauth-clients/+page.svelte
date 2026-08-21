<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { api } from '$lib/api';
	import type { DataTableColumn, DataTableRowAction, DataTableBulkAction } from '$lib/api/types';
	import { t, translations } from '$lib/stores/i18n.svelte';
	import { Dialog } from 'bits-ui';
	import { KeyRound, Plus, Trash2, RefreshCw, CheckCircle, XCircle, Copy, Check } from 'lucide-svelte';

	// RBAC CRUD: OAuth Clients management

	interface OAuthClient {
		id: string;
		name: string;
		provider: string;
		client_id: string;
		client_secret: string;
		redirect_uri: string;
		enabled: boolean;
		created_at: string;
	}

	interface OAuthProviderDef {
		key: string;
		label: string;
		description: string;
	}

	let {} = $props();

	let clients = $state<OAuthClient[]>([]);
	let loading = $state(true);
	let apiAvailable = $state(true);
	let errorMessage = $state('');

	let page = $state(0);
	let pageSize = $state(10);
	let sortKey = $state('');
	let sortDirection = $state<'asc' | 'desc'>('asc');
	let selectedIds = $state(new Set<string>());

	let showCreateModal = $state(false);
	let createName = $state('');
	let createProvider = $state('google');
	let createClientId = $state('');
	let createClientSecret = $state('');
	let createRedirectUri = $state('');

	let providers = $state<OAuthProviderDef[]>([]);

	let confirmDeleteOpen = $state(false);
	let pendingDeleteId = $state<string | null>(null);
	let pendingBulkDeleteIds = $state<string[]>([]);
	let confirmBulkDeleteOpen = $state(false);
	let toggling = $state<string | null>(null);
	let showSecretReveal = $state(false);
	let revealedSecret = $state('');
	let secretCopied = $state(false);

	async function load() {
		const result = await api<OAuthClient[]>('/control/oauth-clients');
		if (result.ok) {
			clients = Array.isArray(result.data) ? result.data : [];
			apiAvailable = true;
		} else {
			apiAvailable = false;
			errorMessage = result.error || '';
		}
		loading = false;
	}

	async function loadProviders() {
		const result = await api<OAuthProviderDef[]>('/control/oauth-providers');
		if (result.ok && Array.isArray(result.data) && result.data.length > 0) {
			providers = result.data;
		}
	}

	$effect(() => {
		load();
		loadProviders();
		const interval = setInterval(() => {
			if (document.visibilityState === 'visible') load();
		}, 30_000);
		return () => clearInterval(interval);
	});

	const columns = $derived.by<DataTableColumn[]>(() => {
		translations();
		return [
			{ key: 'name', label: t('oauth_clients.name', 'Name'), sortable: true },
			{ key: 'provider', label: t('oauth_clients.provider', 'Provider'), sortable: true },
			{
				key: 'client_id',
				label: t('oauth_clients.client_id', 'Client ID'),
				sortable: true,
				render: (val) => `<span class="font-mono text-xs text-muted-foreground truncate max-w-[120px] inline-block">${val}</span>`
			},
			{
				key: 'redirect_uri',
				label: t('oauth_clients.redirect_uri', 'Redirect URI'),
				render: (val) => `<span class="text-xs text-muted-foreground truncate max-w-[150px] inline-block">${val || '—'}</span>`
			},
			{
				key: 'enabled',
				label: t('common.status', 'Status'),
				sortable: true,
				render: (val) => val
					? `<span class="bg-emerald-500/15 text-emerald-600 rounded-full px-2 py-0.5 text-xs font-medium">${t('common.enabled', 'Enabled')}</span>`
					: `<span class="bg-muted text-muted-foreground rounded-full px-2 py-0.5 text-xs font-medium">${t('common.disabled', 'Disabled')}</span>`
			},
			{
				key: 'created_at',
				label: t('common.created', 'Created'),
				sortable: true,
				render: (val) => val ? new Date(String(val)).toLocaleDateString() : '—'
			}
		];
	});

	let rowActions = $derived.by<DataTableRowAction[]>(() => {
		translations();
		return [
			{
				label: t('oauth_clients.rotate_secret', 'Rotate Secret'),
				icon: RefreshCw,
				onClick: async (row) => {
					const id = (row as unknown as OAuthClient).id;
					const result = await api<{ client_secret?: string }>(`/control/oauth-clients/${id}/rotate-secret`, { method: 'POST' });
					if (result.ok) {
						if (result.data?.client_secret) {
							revealedSecret = result.data.client_secret;
							showSecretReveal = true;
							load();
						} else {
							errorMessage = t('oauth_clients.rotate_error', 'Failed to rotate secret');
						}
					} else {
						errorMessage = result.error || t('oauth_clients.rotate_error', 'Failed to rotate secret');
					}
				}
			},
			{
				label: t('common.enable', 'Enable'),
				icon: CheckCircle,
				onClick: async (row) => {
					const client = row as unknown as OAuthClient;
					toggling = client.id;
					const previous = client.enabled;
					clients = clients.map((c) => c.id === client.id ? { ...c, enabled: true } : c);
					const result = await api(`/control/oauth-clients/${client.id}`, { method: 'PATCH', body: { enabled: true } });
					if (!result.ok) {
						clients = clients.map((c) => c.id === client.id ? { ...c, enabled: previous } : c);
						errorMessage = result.error || t('oauth_clients.toggle_error', 'Failed to update OAuth client.');
					}
					toggling = null;
					load();
				},
				hidden: (row) => (row as unknown as OAuthClient).enabled
			},
			{
				label: t('common.disable', 'Disable'),
				icon: XCircle,
				onClick: async (row) => {
					const client = row as unknown as OAuthClient;
					toggling = client.id;
					const previous = client.enabled;
					clients = clients.map((c) => c.id === client.id ? { ...c, enabled: false } : c);
					const result = await api(`/control/oauth-clients/${client.id}`, { method: 'PATCH', body: { enabled: false } });
					if (!result.ok) {
						clients = clients.map((c) => c.id === client.id ? { ...c, enabled: previous } : c);
						errorMessage = result.error || t('oauth_clients.toggle_error', 'Failed to update OAuth client.');
					}
					toggling = null;
					load();
				},
				hidden: (row) => !(row as unknown as OAuthClient).enabled
			},
			{
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onClick: (row) => {
					pendingDeleteId = (row as unknown as OAuthClient).id;
					confirmDeleteOpen = true;
				}
			}
		];
	});

	let bulkActions = $derived.by<DataTableBulkAction[]>(() => {
		translations();
		return [
			{
				label: t('oauth_clients.delete_selected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onClick: async (ids: string[]) => {
					pendingBulkDeleteIds = ids;
					confirmBulkDeleteOpen = true;
				}
			}
		];
	});

	async function createClient() {
		if (!createName.trim() || !createClientId.trim() || !createClientSecret.trim()) {
			errorMessage = t('oauth_clients.required_fields', 'Name, Client ID, and Client Secret are required');
			return;
		}
		const result = await api<OAuthClient>('/control/oauth-clients', {
			method: 'POST',
			body: {
				name: createName,
				provider: createProvider,
				client_id: createClientId,
				client_secret: createClientSecret,
				redirect_uri: createRedirectUri
			}
		});
		if (result.ok) {
			showCreateModal = false;
			revealedSecret = result.data?.client_secret ?? createClientSecret;
			createName = '';
			createClientId = '';
			createClientSecret = '';
			createRedirectUri = '';
			showSecretReveal = true;
			load();
		} else {
			errorMessage = result.error || t('oauth_clients.create_error', 'Failed to create OAuth client');
		}
	}

	async function deleteClient(id: string) {
		const result = await api(`/control/oauth-clients/${id}`, { method: 'DELETE' });
		if (result.ok) {
			load();
		} else {
			errorMessage = result.error || t('oauth_clients.delete_error', 'Failed to delete OAuth client');
		}
	}

	async function bulkDeleteClients(ids: string[]) {
		await Promise.allSettled(ids.map((id) => deleteClient(id)));
		selectedIds = new Set();
		load();
	}

	function openCreateModal() {
		createName = '';
		createProvider = 'google';
		createClientId = '';
		createClientSecret = '';
		createRedirectUri = '';
		errorMessage = '';
		showCreateModal = true;
	}

	async function copyRevealedSecret() {
		try {
			await navigator.clipboard.writeText(revealedSecret);
			secretCopied = true;
			setTimeout(() => (secretCopied = false), 2000);
		} catch {
			// Clipboard API may be unavailable in some contexts.
		}
	}

	function closeSecretReveal(open: boolean) {
		if (!open) {
			revealedSecret = '';
			secretCopied = false;
			showSecretReveal = false;
		}
	}
</script>

<PageHeader title={t('oauth_clients.title', 'OAuth Clients')} description={t('oauth_clients.description', 'Manage OAuth client applications for social login integrations')} />

<div class="mb-4 flex justify-end">
	<button
		type="button"
		class="bg-foreground text-background hover:bg-foreground/90 inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-colors"
		onclick={openCreateModal}
	>
		<Plus class="size-4" />
		{t('oauth_clients.create', 'Create Client')}
	</button>
</div>

{#if !apiAvailable}
	<div class="border-destructive bg-destructive/10 text-destructive mb-4 flex items-center gap-2 rounded-xl border px-4 py-3 text-sm">
		{t('common.api_unavailable', 'Unable to load data.')}
	</div>
{/if}

{#if errorMessage}
	<div class="border-destructive bg-destructive/10 text-destructive mb-4 rounded-xl border px-4 py-3 text-sm">
		{errorMessage}
	</div>
{/if}

<DataTable
	{columns}
	rows={clients}
	totalRows={clients.length}
	bind:page
	bind:pageSize
	bind:sortKey
	bind:sortDirection
	bind:selectedIds
	{loading}
	{rowActions}
	{bulkActions}
	selectable={true}
	emptyMessage={t('oauth_clients.empty', 'No OAuth clients configured')}
	emptyIcon={KeyRound}
/>

{#if showCreateModal}
	<Dialog.Root bind:open={showCreateModal}>
		<Dialog.Content class="bg-background border-border-card shadow-popover fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-xl border p-6">
			<Dialog.Title class="text-foreground mb-4 text-lg font-semibold">{t('oauth_clients.create_title', 'Create OAuth Client')}</Dialog.Title>
			<form onsubmit={(e) => { e.preventDefault(); createClient(); }} class="space-y-4">
				<div>
					<label for="oauth-name" class="text-foreground mb-1 block text-sm font-medium">{t('oauth_clients.name_label', 'Name')}</label>
					<input id="oauth-name" type="text" bind:value={createName} required class="border-border-input bg-background text-foreground w-full rounded-lg border px-3 py-2 text-sm" placeholder="My App" />
				</div>
				<div>
					<label for="oauth-provider" class="text-foreground mb-1 block text-sm font-medium">{t('oauth_clients.provider_label', 'Provider')}</label>
					<select id="oauth-provider" bind:value={createProvider} class="border-border-input bg-background text-foreground w-full rounded-lg border px-3 py-2 text-sm">
						{#each providers as p (p.key)}
							<option value={p.key}>{p.label}</option>
						{/each}
					</select>
					<p class="text-muted-foreground mt-1 text-xs">
						{providers.find((p) => p.key === createProvider)?.description ?? ''}
					</p>
				</div>
				<div>
					<label for="oauth-client-id" class="text-foreground mb-1 block text-sm font-medium">{t('oauth_clients.client_id_label', 'Client ID')}</label>
					<input id="oauth-client-id" type="text" bind:value={createClientId} required class="border-border-input bg-background text-foreground w-full rounded-lg border px-3 py-2 text-sm font-mono" placeholder="xxxxxxxxxx.apps.googleusercontent.com" />
				</div>
				<div>
					<label for="oauth-client-secret" class="text-foreground mb-1 block text-sm font-medium">{t('oauth_clients.client_secret_label', 'Client Secret')}</label>
					<input id="oauth-client-secret" type="text" bind:value={createClientSecret} required class="border-border-input bg-background text-foreground w-full rounded-lg border px-3 py-2 text-sm font-mono" placeholder="GOCSPX-xxxxxxxxxxxxx" />
					<p class="text-muted-foreground mt-1 text-xs">
						{t('oauth_clients.client_secret_hint', 'Used to authenticate with the OAuth provider. Stored encrypted. Revealed once after creation.')}
					</p>
				</div>
				<div>
					<label for="oauth-redirect" class="text-foreground mb-1 block text-sm font-medium">{t('oauth_clients.redirect_uri_label', 'Redirect URI')}</label>
					<input id="oauth-redirect" type="text" bind:value={createRedirectUri} class="border-border-input bg-background text-foreground w-full rounded-lg border px-3 py-2 text-sm" placeholder="https://example.com/auth/callback" />
				</div>
				<div class="flex justify-end gap-2 pt-2">
					<button type="button" class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-lg border px-4 py-2 text-sm font-medium transition-colors" onclick={() => (showCreateModal = false)}>
						{t('common.cancel', 'Cancel')}
					</button>
					<button type="submit" class="bg-foreground text-background hover:bg-foreground/90 rounded-lg px-4 py-2 text-sm font-medium transition-colors">
						{t('common.create', 'Create')}
					</button>
				</div>
			</form>
		</Dialog.Content>
	</Dialog.Root>
{/if}

{#if pendingDeleteId}
	<ConfirmModal
		bind:open={confirmDeleteOpen}
		title={t('oauth_clients.delete_title', 'Delete OAuth Client')}
		consequence={t('oauth_clients.delete_confirm', 'This will permanently delete the OAuth client. This action cannot be undone.')}
		confirmText={t('common.delete', 'Delete')}
		onConfirm={async () => { await deleteClient(pendingDeleteId!); pendingDeleteId = null; }}
		onCancel={() => (pendingDeleteId = null)}
	/>
{/if}

{#if pendingBulkDeleteIds.length > 0}
	<ConfirmModal
		bind:open={confirmBulkDeleteOpen}
		title={t('oauth_clients.bulk_delete_title', 'Delete Selected Clients')}
		consequence={t('oauth_clients.bulk_delete_confirm', 'This will permanently delete {n} OAuth client(s).').replace('{n}', String(pendingBulkDeleteIds.length))}
		confirmText={t('common.delete', 'Delete')}
		onConfirm={async () => { await bulkDeleteClients(pendingBulkDeleteIds); pendingBulkDeleteIds = []; }}
		onCancel={() => (pendingBulkDeleteIds = [])}
	/>
{/if}

{#if showSecretReveal}
	<Dialog.Root bind:open={showSecretReveal} onOpenChange={closeSecretReveal}>
		<Dialog.Portal>
			<Dialog.Overlay class="fixed inset-0 z-50 bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out" />
			<Dialog.Content
				class="border-border-card bg-background shadow-popover fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-xl border p-6 focus:outline-none"
				role="dialog"
				aria-modal="true"
			>
				<Dialog.Title class="text-foreground mb-2 text-lg font-semibold">{t('oauth_clients.secret_reveal_title', 'Client Secret')}</Dialog.Title>
				<p class="text-muted-foreground text-sm">{t('oauth_clients.secret_reveal_desc', 'Copy this secret now. It will not be shown again.')}</p>
				<div class="mt-4 flex items-center gap-2">
					<input
						type="text"
						readonly
						value={revealedSecret}
						class="border-border-input bg-background text-foreground w-full rounded-lg border px-3 py-2 text-sm font-mono"
					/>
					<button
						type="button"
						class="bg-foreground text-background hover:bg-foreground/90 inline-flex h-9 w-9 items-center justify-center rounded-lg transition-colors"
						onclick={copyRevealedSecret}
						aria-label={t('common.copy', 'Copy')}
					>
						{#if secretCopied}
							<Check class="size-4" />
						{:else}
							<Copy class="size-4" />
						{/if}
					</button>
				</div>
				<div class="mt-4 flex justify-end">
					<button
						type="button"
						class="bg-foreground text-background hover:bg-foreground/90 rounded-lg px-4 py-2 text-sm font-medium transition-colors"
						onclick={() => (showSecretReveal = false)}
					>
						{t('common.close', 'Close')}
					</button>
				</div>
			</Dialog.Content>
		</Dialog.Portal>
	</Dialog.Root>
{/if}
