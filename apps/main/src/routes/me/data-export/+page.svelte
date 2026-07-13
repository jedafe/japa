<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Download, Shield } from 'lucide-svelte';

	type ExportStatus = {
		request_id: string;
		status: string;
		format: string;
		created_at: string;
		completed_at?: string;
		download_url?: string;
	};

	let { data } = $props();
	let exportStatus = $derived(data.data ?? null);
	let errorMessage = $state('');
	let exporting = $state(false);

	async function requestExport() {
		exporting = true;
		errorMessage = '';
		try {
			await request('/me/data-export', { method: 'POST', body: JSON.stringify({}) });
			await invalidateAll();
		} catch {
			errorMessage = t('common.error');
		} finally {
			exporting = false;
		}
	}
</script>

<svelte:head>
	<title>{t('me.dataExport.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.dataExport.title')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

{#if !data.data}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else if exportStatus}
	<div class="max-w-xl space-y-4 rounded-lg border p-6" style="border-color: var(--color-border); background: var(--color-surface)">
		<div class="flex items-center gap-3">
			<Shield class="h-5 w-5" style="color: var(--color-primary)" />
			<div>
				<p class="font-medium">{t('me.dataExport.status')}: {exportStatus.status}</p>
				<p class="text-xs" style="color: var(--color-muted)">{t('me.dataExport.format')}: {exportStatus.format}</p>
				<p class="text-xs" style="color: var(--color-muted)">{t('me.dataExport.requested')}: {new Date(exportStatus.created_at).toLocaleString()}</p>
			</div>
		</div>
		{#if exportStatus.download_url}
			<a href={exportStatus.download_url} class="btn btn-primary inline-flex items-center gap-2 text-xs">
				<Download class="h-3 w-3" />
				{t('me.dataExport.download')}
			</a>
		{/if}
		{#if exportStatus.status === 'completed' || exportStatus.status === 'failed'}
			<button class="btn btn-secondary text-xs" onclick={requestExport} disabled={exporting}>
				{exporting ? t('common.loading') : t('me.dataExport.request_new')}
			</button>
		{/if}
	</div>
{/if}
