<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Settings } from 'lucide-svelte';

	type Preferences = Record<string, unknown>;

	let { data } = $props();
	let preferences = $derived(data.data ?? {});
	let saving = $state(false);

	async function save() {
		saving = true;
		try {
			await request<void>('/me/preferences', { method: 'PATCH', body: JSON.stringify(preferences) });
			await invalidateAll();
		} catch {
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>{t('me.preferences.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.preferences.title')}</h1>

{#if !data.data}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="max-w-xl rounded-lg border p-6" style="border-color: var(--color-border); background: var(--color-surface)">
		<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('me.preferences.description')}</p>
		<pre class="mb-4 overflow-x-auto rounded-md border p-4 text-xs" style="border-color: var(--color-border); background: var(--color-background)">{JSON.stringify(preferences, null, 2)}</pre>
		<button class="btn btn-primary text-xs" onclick={save} disabled={saving}>
			{saving ? t('common.saving') : t('common.save')}
		</button>
	</div>
{/if}
