<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { ArrowLeft, Loader2, CheckCircle2, Link2, Unlink } from 'lucide-svelte';
	import { onMount } from 'svelte';

	type ConnectedAccount = {
		provider: string;
		connected: boolean;
		provider_username?: string;
		connected_at?: string;
	};

	let accounts = $state<ConnectedAccount[]>([
		{ provider: 'google', connected: false },
		{ provider: 'github', connected: false }
	]);
	let loading = $state(true);
	let error = $state('');
	let connecting = $state<string | null>(null);
	let disconnecting = $state<string | null>(null);

	onMount(loadAccounts);

	async function loadAccounts() {
		loading = true;
		error = '';
		try {
			const res = await fetch('/api/v1/me/connected-accounts', { credentials: 'include' });
			if (res.ok) {
				const { data } = await res.json();
				// Merge with defaults to ensure all providers are shown
				const connected = data ?? [];
				accounts = accounts.map((a) => {
					const found = connected.find((c: ConnectedAccount) => c.provider === a.provider);
					return found ? { ...a, ...found, connected: true } : { ...a, connected: false };
				});
			} else {
				error = 'Failed to load connected accounts.';
			}
		} catch {
			error = 'Failed to load connected accounts.';
		} finally {
			loading = false;
		}
	}

	async function connectProvider(provider: string) {
		connecting = provider;
		error = '';
		try {
			// Redirect to OAuth initiation
			window.location.href = `/api/v1/auth/oauth/${provider}/initiate`;
		} catch {
			error = `Failed to connect ${provider}.`;
			connecting = null;
		}
	}

	async function disconnectProvider(provider: string) {
		if (!confirm(`Are you sure you want to disconnect your ${provider} account?`)) return;
		disconnecting = provider;
		error = '';
		try {
			const res = await fetch(`/api/v1/me/connected-accounts/${provider}`, {
				method: 'DELETE',
				credentials: 'include'
			});
			if (res.ok) {
				accounts = accounts.map((a) =>
					a.provider === provider ? { ...a, connected: false, provider_username: undefined } : a
				);
			} else {
				error = `Failed to disconnect ${provider}.`;
			}
		} catch {
			error = `Failed to disconnect ${provider}.`;
		} finally {
			disconnecting = null;
		}
	}

	function providerIcon(provider: string): string {
		switch (provider) {
			case 'google': return 'G';
			case 'github': return 'GH';
			default: return '?';
		}
	}

	function providerColor(provider: string): string {
		switch (provider) {
			case 'google': return '#4285f4';
			case 'github': return '#333';
			default: return '#666';
		}
	}
</script>

<svelte:head>
	<title>Connected Accounts | JapaConnect</title>
</svelte:head>

<main class="mx-auto max-w-lg px-4 py-8">
	<a
		href="/settings"
		class="mb-6 inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to Settings
	</a>

	<h1 class="mb-2 text-2xl font-bold" style="color: var(--color-foreground)">{t('settings.connected-accounts.connected_accounts')}</h1>
	<p class="mb-6 text-sm" style="color: var(--color-muted)">
		Connect your social accounts for easier sign-in and profile enrichment.
	</p>

	{#if error}
		<div class="mb-6 rounded-lg border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
			{error}
		</div>
	{/if}

	{#if loading}
		<div class="space-y-3">
			{#each [1, 2] as _}
				<div class="h-20 animate-pulse rounded-lg" style="background-color: var(--color-background)"></div>
			{/each}
		</div>
	{:else}
		<div class="space-y-3">
			{#each accounts as account (account.provider)}
				<div
					class="flex items-center justify-between rounded-xl border p-4"
					style="border-color: var(--color-border); background: var(--color-surface);"
				>
					<div class="flex items-center gap-3">
						<div
							class="flex h-10 w-10 items-center justify-center rounded-lg text-sm font-bold text-white"
							style="background-color: {providerColor(account.provider)}"
						>
							{providerIcon(account.provider)}
						</div>
						<div>
							<div class="text-sm font-medium capitalize" style="color: var(--color-foreground)">
								{account.provider}
							</div>
							{#if account.connected && account.provider_username}
								<p class="text-xs" style="color: var(--color-muted)">
									Connected as {account.provider_username}
								</p>
							{:else}
								<p class="text-xs" style="color: var(--color-muted)">
									Not connected
								</p>
							{/if}
						</div>
					</div>

					{#if account.connected}
						<div class="flex items-center gap-2">
							<CheckCircle2 class="h-4 w-4" style="color: var(--color-success, #16a34a)" />
							<button
								onclick={() => disconnectProvider(account.provider)}
								disabled={disconnecting === account.provider}
								class="rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
								style="border-color: var(--color-danger); color: var(--color-danger)"
							>
								{#if disconnecting === account.provider}
									<Loader2 class="h-4 w-4 animate-spin" />
								{:else}
									Disconnect
								{/if}
							</button>
						</div>
					{:else}
						<button
							onclick={() => connectProvider(account.provider)}
							disabled={connecting === account.provider}
							class="flex items-center gap-2 rounded-lg px-3 py-1.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
							style="background-color: var(--color-primary)"
						>
							{#if connecting === account.provider}
								<Loader2 class="h-4 w-4 animate-spin" />
							{:else}
								<Link2 class="h-4 w-4" />
							{/if}
							Connect
						</button>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</main>
