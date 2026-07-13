<script lang="ts">
	import { onMount } from 'svelte';
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Lock } from 'lucide-svelte';

	type ApiKey = {
		id: string;
		name: string;
		prefix: string;
		created_at: string;
		last_used_at?: string;
	};

	let keys = $state<ApiKey[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			keys = await request<ApiKey[]>('/me/api-keys');
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('me.apiKeys.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.apiKeys.title')}</h1>

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
		{#each keys as key (key.id)}
			<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
				<div class="flex items-center justify-between">
					<p class="font-medium text-sm">{key.name}</p>
					<span class="text-xs font-mono" style="color: var(--color-muted)">{key.prefix}...</span>
				</div>
				<p class="text-xs" style="color: var(--color-muted)">				{t('me.apiKeys.created')}: {new Date(key.created_at).toLocaleDateString()} {key.last_used_at ? '• ' + t('me.apiKeys.last_used') + ': ' + new Date(key.last_used_at).toLocaleDateString() : ''}</p>
			</div>
		{:else}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('me.apiKeys.empty')}</p>
		{/each}
	</div>
{/if}
