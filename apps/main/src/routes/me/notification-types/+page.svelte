<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Shield, CheckCircle } from 'lucide-svelte';

	type NotificationType = {
		key: string;
		label: string;
		description?: string;
		enabled: boolean;
	};

	let { data } = $props();
	let types = $derived(data.data ?? []);
	let errorMessage = $state('');

	async function toggle(key: string, enabled: boolean) {
		errorMessage = '';
		try {
			await request<void>(`/me/notification-types/${key}`, { method: 'PATCH', body: JSON.stringify({ enabled }) });
			await invalidateAll();
		} catch {
			errorMessage = t('common.error');
		}
	}
</script>

<svelte:head>
	<title>{t('me.notificationTypes.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.notificationTypes.title')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

{#if !data.data}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="max-w-xl space-y-2">
		{#each types as nt (nt.key)}
			<div class="flex items-center justify-between rounded-lg border p-4" style="border-color: var(--color-border)">
				<div>
					<p class="font-medium text-sm">{nt.label}</p>
					{#if nt.description}
						<p class="text-xs" style="color: var(--color-muted)">{nt.description}</p>
					{/if}
				</div>
				<input type="checkbox" checked={nt.enabled} onchange={(e) => toggle(nt.key, (e.target as HTMLInputElement).checked)} class="rounded" />
			</div>
		{:else}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('me.notificationTypes.empty')}</p>
		{/each}
	</div>
{/if}
