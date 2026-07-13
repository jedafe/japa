<script lang="ts">
	import { onMount } from 'svelte';
	import { api, reportsAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Eye } from 'lucide-svelte';

	type Report = {
		id: string;
		title: string;
		report_type: string;
		status: string;
		generated_at: string;
		file_url?: string;
	};

	let reports = $state<Report[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<Report[]>('/admin/reports');
			if (result.ok) {
				reports = result.data ?? [];
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
	<title>{t('reports.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('reports.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('reports.description')}</p>

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
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('reports.title')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('reports.type')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('reports.status')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('reports.generated')}</th>
				</tr>
			</thead>
			<tbody>
				{#each reports as report (report.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium">{report.title}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{report.report_type}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {report.status === 'ready' ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : report.status === 'generating' ? 'bg-blue-500/15 text-blue-700 dark:text-blue-400' : 'bg-amber-500/15 text-amber-700 dark:text-amber-400'}">
								{report.status}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(report.generated_at).toLocaleString()}</td>
					</tr>
				{:else}
					<tr>
						<td colspan="4" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('reports.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
