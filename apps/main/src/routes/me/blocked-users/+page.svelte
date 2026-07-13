<script lang="ts">
	import { onMount } from 'svelte';
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Shield, Trash2 } from 'lucide-svelte';

	type BlockedUser = { user_id: string; blocked_at: string };

	let blockedUsers = $state<BlockedUser[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			blockedUsers = await request<BlockedUser[]>('/me/blocked-users');
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function unblock(userId: string) {
		errorMessage = '';
		try {
			await request<void>(`/me/blocked-users/${userId}`, { method: 'DELETE' });
			blockedUsers = blockedUsers.filter((u) => u.user_id !== userId);
		} catch {
			errorMessage = t('common.error');
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('me.blocked.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.blocked.title')}</h1>

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
		{#each blockedUsers as u (u.user_id)}
			<div class="flex items-center justify-between rounded-lg border p-4" style="border-color: var(--color-border)">
				<span class="text-sm font-mono">{u.user_id}</span>
				<button class="rounded px-2 py-1 text-xs hover:bg-black/5" onclick={() => unblock(u.user_id)}>
					{t('me.blocked.unblock')}
				</button>
			</div>
		{:else}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('me.blocked.empty')}</p>
		{/each}
	</div>
{/if}
