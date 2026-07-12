<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Lock, Unlock } from 'lucide-svelte';

	type ConnectedAccount = {
		provider: string;
		email: string;
		connected_at: string;
	};

	let accounts = $state<ConnectedAccount[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			accounts = await api.getMeConnectedAccounts() as unknown as ConnectedAccount[];
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function disconnect(provider: string) {
		errorMessage = '';
		try {
			await api.deleteMeConnectedAccount(provider);
			accounts = accounts.filter((a) => a.provider !== provider);
		} catch {
			errorMessage = t('common.error');
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('me.connectedAccounts.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.connectedAccounts.title')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button class="btn btn-secondary flex items-center gap-1.5 text-xs" onclick={load}>
		<RefreshCw class="h-3 w-3" />
		{t('common.refresh')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="max-w-xl space-y-2">
		{#each accounts as account (account.provider)}
			<div class="flex items-center justify-between rounded-lg border p-4" style="border-color: var(--color-border)">
				<div>
					<p class="font-medium text-sm capitalize">{account.provider}</p>
					<p class="text-xs" style="color: var(--color-muted)">{account.email} • {new Date(account.connected_at).toLocaleDateString()}</p>
				</div>
				<button class="rounded px-2 py-1 text-xs hover:bg-black/5" onclick={() => disconnect(account.provider)}>
					<Unlock class="h-3 w-3 inline mr-1" style="color: var(--color-danger)" />
					{t('me.connectedAccounts.disconnect')}
				</button>
			</div>
		{:else}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('me.connectedAccounts.empty')}</p>
		{/each}
	</div>
{/if}
