<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw } from 'lucide-svelte';

	type DataExport = {
		id: string;
		user_id: string;
		status: string;
		format: string;
		created_at: string;
		completed_at?: string;
		download_url?: string;
	};

	let exports = $state<DataExport[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<DataExport[]>('/admin/data-exports');
			if (result.ok) {
				exports = result.data ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function retryExport(id: string) {
		errorMessage = '';
		try {
			await api(`/admin/gdpr/exports/${id}/retry`, { method: 'POST', body: {} });
			load();
		} catch {
			errorMessage = t('common.error');
		}
	}

	function statusColor(status: string) {
		switch (status) {
			case 'completed': return 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400';
			case 'processing': return 'bg-blue-500/15 text-blue-700 dark:text-blue-400';
			case 'failed': return 'bg-red-500/15 text-red-700 dark:text-red-400';
			default: return 'bg-amber-500/15 text-amber-700 dark:text-amber-400';
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('dataExports.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('dataExports.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('dataExports.description')}</p>

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
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('dataExports.user')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('dataExports.status')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('dataExports.format')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('dataExports.created')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('dataExports.completed')}</th>
				</tr>
			</thead>
			<tbody>
				{#each exports as exp (exp.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-mono text-xs">{exp.user_id}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {statusColor(exp.status)}">
								{exp.status}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{exp.format}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(exp.created_at).toLocaleString()}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{exp.completed_at ? new Date(exp.completed_at).toLocaleString() : '—'}</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('dataExports.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
