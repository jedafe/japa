<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Trash2, Monitor } from 'lucide-svelte';

	type Session = {
		id: string;
		ip_address: string;
		user_agent: string;
		created_at: string;
		last_active_at: string;
		is_current: boolean;
	};

	let sessions = $state<Session[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			sessions = await api.getMeSessions() as unknown as Session[];
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function revokeSession(id: string) {
		errorMessage = '';
		try {
			await api.deleteMeSession(id);
			sessions = sessions.filter((s) => s.id !== id);
		} catch {
			errorMessage = t('common.error');
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('me.sessions.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.sessions.title')}</h1>

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
	<div class="space-y-2">
		{#each sessions as session (session.id)}
			<div class="flex items-center justify-between rounded-lg border p-4" style="border-color: var(--color-border)">
				<div class="flex items-center gap-3">
					<Monitor class="h-4 w-4" style="color: var(--color-muted)" />
					<div>
						<p class="text-sm font-medium">{session.user_agent || t('me.sessions.unknown_device')}</p>
						<p class="text-xs" style="color: var(--color-muted)">{session.ip_address} • {new Date(session.last_active_at).toLocaleString()}</p>
					</div>
				</div>
				<div class="flex items-center gap-2">
					{#if session.is_current}
						<span class="text-xs" style="color: var(--color-success)">{t('me.sessions.current')}</span>
					{:else}
						<button class="rounded px-2 py-1 text-xs hover:bg-black/5" onclick={() => revokeSession(session.id)}>
							<Trash2 class="h-3 w-3 inline mr-1" style="color: var(--color-danger)" />
							{t('me.sessions.revoke')}
						</button>
					{/if}
				</div>
			</div>
		{:else}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('me.sessions.empty')}</p>
		{/each}
	</div>
{/if}
