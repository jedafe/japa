<script lang="ts">
	import { onMount } from 'svelte';
	import { api, settingsAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw } from 'lucide-svelte';

	type Setting = {
		key: string;
		value: unknown;
		description?: string;
		updated_at: string;
	};

	let settings = $state<Setting[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<Setting[]>('/admin/settings');
			if (result.ok) {
				settings = result.data ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('settings.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('settings.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('settings.description')}</p>

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
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('settings.key')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('settings.value')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('settings.updated')}</th>
				</tr>
			</thead>
			<tbody>
				{#each settings as setting (setting.key)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-mono text-xs">{setting.key}</td>
						<td class="px-4 py-3 text-xs">
							{typeof setting.value === 'object' ? JSON.stringify(setting.value) : String(setting.value)}
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(setting.updated_at).toLocaleString()}</td>
					</tr>
				{:else}
					<tr>
						<td colspan="3" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('settings.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
