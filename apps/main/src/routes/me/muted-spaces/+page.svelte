<script lang="ts">
	import { onMount } from 'svelte';
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, VolumeX } from 'lucide-svelte';

	type MutedSpace = { space_id: string; space_name?: string; muted_at: string };

	let mutedSpaces = $state<MutedSpace[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			mutedSpaces = await request<MutedSpace[]>('/me/muted-spaces');
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function unmute(spaceId: string) {
		errorMessage = '';
		try {
			await request<void>(`/me/muted-spaces/${spaceId}`, { method: 'DELETE' });
			mutedSpaces = mutedSpaces.filter((s) => s.space_id !== spaceId);
		} catch {
			errorMessage = t('common.error');
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('me.mutedSpaces.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.mutedSpaces.title')}</h1>

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
		{#each mutedSpaces as s (s.space_id)}
			<div class="flex items-center justify-between rounded-lg border p-4" style="border-color: var(--color-border)">
				<div class="flex items-center gap-3">
					<VolumeX class="h-4 w-4" style="color: var(--color-muted)" />
					<span class="text-sm">{s.space_name || s.space_id}</span>
				</div>
				<button class="text-xs underline" onclick={() => unmute(s.space_id)}>{t('me.muted.unmute')}</button>
			</div>
		{:else}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('me.mutedSpaces.empty')}</p>
		{/each}
	</div>
{/if}
