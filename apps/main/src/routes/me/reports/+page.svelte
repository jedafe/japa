<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Eye, Trash2 } from 'lucide-svelte';

	type Report = {
		id: string;
		content_type: string;
		status: string;
		created_at: string;
	};

	let { data } = $props();
	let reports = $derived(data.data ?? []);
</script>

<svelte:head>
	<title>{t('me.reports.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.reports.title')}</h1>

<div class="mb-4 flex justify-end">
	<button class="btn btn-secondary flex items-center gap-1.5 text-xs" onclick={() => invalidateAll()}>
		<RefreshCw class="h-3 w-3" />
		{t('common.refresh')}
	</button>
</div>

{#if !data.data}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('me.reports.type')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('me.reports.status')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('me.reports.created')}</th>
				</tr>
			</thead>
			<tbody>
				{#each reports as report (report.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3">{report.content_type}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {report.status === 'resolved' ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : report.status === 'pending' ? 'bg-amber-500/15 text-amber-700 dark:text-amber-400' : 'bg-slate-500/15 text-slate-700 dark:text-slate-400'}">
								{report.status}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(report.created_at).toLocaleDateString()}</td>
					</tr>
				{:else}
					<tr>
						<td colspan="3" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('me.reports.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
