<script lang="ts">
	import { onMount } from 'svelte';
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Wallet as WalletIcon } from 'lucide-svelte';

	type Wallet = {
		points: number;
		credits: number;
	};

	let wallet = $state<Wallet | null>(null);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			wallet = await request<Wallet>('/me/wallet');
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('me.wallet.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.wallet.title')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else if wallet}
	<div class="grid gap-4 sm:grid-cols-2">
		<div class="rounded-lg border p-6" style="border-color: var(--color-border); background: var(--color-surface)">
			<p class="text-sm" style="color: var(--color-muted)">{t('me.wallet.credits')}</p>
			<p class="mt-1 text-3xl font-bold">{wallet.credits}</p>
		</div>
		<div class="rounded-lg border p-6" style="border-color: var(--color-border); background: var(--color-surface)">
			<p class="text-sm" style="color: var(--color-muted)">{t('me.wallet.points')}</p>
			<p class="mt-1 text-3xl font-bold">{wallet.points}</p>
		</div>
	</div>
{/if}
