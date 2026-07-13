<script lang="ts">
	import { onMount } from 'svelte';
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, AlertTriangle } from 'lucide-svelte';

	let loading = $state(true);
	let errorMessage = $state('');
	let deactivating = $state(false);
	let successMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		successMessage = '';
		try {
			await request<void>('/me');
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function deactivateAccount() {
		deactivating = true;
		errorMessage = '';
		successMessage = '';
		try {
			await request<void>('/me/deactivate', { method: 'POST', body: JSON.stringify({}) });
			successMessage = t('me.deactivate.success');
		} catch {
			errorMessage = t('common.error');
		} finally {
			deactivating = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('me.deactivate.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.deactivate.title')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}
{#if successMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-success); background-color: color-mix(in srgb, var(--color-success) 8%, transparent); color: var(--color-success)">
		{successMessage}
	</div>
{/if}

<div class="max-w-xl rounded-lg border p-6" style="border-color: var(--color-border); background: var(--color-surface)">
	<div class="flex items-center gap-3 mb-4">
		<AlertTriangle class="h-5 w-5" style="color: var(--color-danger)" />
		<p class="font-medium">{t('me.deactivate.warning')}</p>
	</div>
	<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('me.deactivate.description')}</p>
	<button class="btn btn-danger text-xs" onclick={deactivateAccount} disabled={deactivating}>
		{deactivating ? t('common.loading') : t('me.deactivate.submit')}
	</button>
</div>
