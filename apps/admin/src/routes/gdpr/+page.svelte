<script lang="ts">
	import { api, gdprAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Trash2 } from 'lucide-svelte';

	type DeletionRequest = {
		id: string;
		user_id: string;
		status: string;
		reason: string;
		requested_at: string;
		executed_at?: string;
	};

	let requests = $state<DeletionRequest[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<DeletionRequest[]>('/admin/gdpr/deletion-requests');
			if (result.ok) {
				requests = result.data ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function executeDeletion(id: string) {
		errorMessage = '';
		try {
			await api(`/admin/gdpr/deletion-requests/${id}/execute`, { method: 'POST', body: {} });
			load();
		} catch {
			errorMessage = t('common.error');
		}
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('gdpr.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('gdpr.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('gdpr.description')}</p>

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
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('gdpr.user')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('gdpr.reason')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('gdpr.status')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('gdpr.requested')}</th>
					<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each requests as req (req.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-mono text-xs">{req.user_id}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{req.reason || '—'}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {req.status === 'executed' ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : req.status === 'pending' ? 'bg-amber-500/15 text-amber-700 dark:text-amber-400' : 'bg-slate-500/15 text-slate-700 dark:text-slate-400'}">
								{req.status}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(req.requested_at).toLocaleString()}</td>
						<td class="px-4 py-3 text-right">
							{#if req.status === 'pending'}
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => executeDeletion(req.id)}>
									{t('gdpr.execute')}
								</button>
							{/if}
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('gdpr.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
