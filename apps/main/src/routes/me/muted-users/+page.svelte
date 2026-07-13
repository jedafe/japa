<script lang="ts">
	import { onMount } from 'svelte';
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, UserX, VolumeX } from 'lucide-svelte';

	type MutedUser = { user_id: string; muted_at: string };
	type MutedSpace = { space_id: string; muted_at: string };

	let mutedUsers = $state<MutedUser[]>([]);
	let mutedSpaces = $state<MutedSpace[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const [mutedUsersData, mutedSpacesData] = await Promise.all([
				request<MutedUser[]>('/me/muted-users'),
				request<MutedSpace[]>('/me/muted-spaces')
			]);
			mutedUsers = mutedUsersData ?? [];
			mutedSpaces = mutedSpacesData ?? [];
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function unmuteUser(userId: string) {
		errorMessage = '';
		try {
			await request<void>(`/me/muted-users/${userId}`, { method: 'DELETE' });
			mutedUsers = mutedUsers.filter((u) => u.user_id !== userId);
		} catch {
			errorMessage = t('common.error');
		}
	}

	async function unmuteSpace(spaceId: string) {
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
	<title>{t('me.muted.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.muted.title')}</h1>

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
	<div class="grid gap-6 lg:grid-cols-2">
		<div class="rounded-lg border" style="border-color: var(--color-border)">
			<div class="border-b px-4 py-3" style="border-color: var(--color-border); background: var(--color-surface)">
				<h2 class="font-medium flex items-center gap-2"><UserX class="h-4 w-4" style="color: var(--color-muted)" /> {t('me.muted.users')}</h2>
			</div>
			{#if mutedUsers.length === 0}
				<p class="px-4 py-6 text-center text-sm" style="color: var(--color-muted)">{t('me.muted.users_empty')}</p>
			{:else}
				<div class="divide-y" style="border-color: var(--color-border)">
					{#each mutedUsers as u (u.user_id)}
						<div class="flex items-center justify-between px-4 py-3">
							<span class="text-sm font-mono">{u.user_id}</span>
							<button class="text-xs underline" onclick={() => unmuteUser(u.user_id)}>{t('me.muted.unmute')}</button>
						</div>
					{/each}
				</div>
			{/if}
		</div>
		<div class="rounded-lg border" style="border-color: var(--color-border)">
			<div class="border-b px-4 py-3" style="border-color: var(--color-border); background: var(--color-surface)">
				<h2 class="font-medium flex items-center gap-2"><VolumeX class="h-4 w-4" style="color: var(--color-muted)" /> {t('me.muted.spaces')}</h2>
			</div>
			{#if mutedSpaces.length === 0}
				<p class="px-4 py-6 text-center text-sm" style="color: var(--color-muted)">{t('me.muted.spaces_empty')}</p>
			{:else}
				<div class="divide-y" style="border-color: var(--color-border)">
					{#each mutedSpaces as s (s.space_id)}
						<div class="flex items-center justify-between px-4 py-3">
							<span class="text-sm font-mono">{s.space_id}</span>
							<button class="text-xs underline" onclick={() => unmuteSpace(s.space_id)}>{t('me.muted.unmute')}</button>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	</div>
{/if}
