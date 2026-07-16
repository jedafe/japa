<script lang="ts">
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, CheckCircle, XCircle } from 'lucide-svelte';

	type JoinRequest = {
		id: string;
		user_id: string;
		space_id: string;
		status: string;
		message?: string;
		requested_at: string;
	};

	let requests = $state<JoinRequest[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<JoinRequest[]>('/admin/join-requests');
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

	async function approveRequest(id: string) {
		errorMessage = '';
		try {
			await api(`/admin/join-requests/${id}/approve`, { method: 'POST', body: {} });
			load();
		} catch {
			errorMessage = t('common.error');
		}
	}

	async function rejectRequest(id: string) {
		errorMessage = '';
		try {
			await api(`/admin/join-requests/${id}/reject`, { method: 'POST', body: {} });
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
	<title>{t('joinRequests.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('joinRequests.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('joinRequests.description')}</p>

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
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('joinRequests.user')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('joinRequests.space')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('joinRequests.message')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('joinRequests.status')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('joinRequests.requested')}</th>
					<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each requests as req (req.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-mono text-xs">{req.user_id}</td>
						<td class="px-4 py-3 text-xs">{req.space_id}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{req.message || '—'}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {req.status === 'approved' ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : req.status === 'rejected' ? 'bg-red-500/15 text-red-700 dark:text-red-400' : 'bg-amber-500/15 text-amber-700 dark:text-amber-400'}">
								{req.status}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(req.requested_at).toLocaleDateString()}</td>
						<td class="px-4 py-3 text-right">
							{#if req.status === 'pending'}
								<div class="flex items-center justify-end gap-1">
									<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => approveRequest(req.id)}>
										<CheckCircle class="h-3 w-3 inline mr-1" style="color: var(--color-success)" />
										{t('joinRequests.approve')}
									</button>
									<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => rejectRequest(req.id)}>
										<XCircle class="h-3 w-3 inline mr-1" style="color: var(--color-danger)" />
										{t('joinRequests.reject')}
									</button>
								</div>
							{/if}
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="6" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('joinRequests.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
