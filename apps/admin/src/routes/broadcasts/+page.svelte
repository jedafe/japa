<script lang="ts">
	import { onMount } from 'svelte';
	import { api, broadcastsAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Plus, Send, Trash2 } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';

	type Broadcast = {
		id: string;
		title: string;
		body: string;
		status: string;
		target_audience: string;
		sent_at?: string;
		created_at: string;
	};

	let broadcasts = $state<Broadcast[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	let showCreateModal = $state(false);
	let title = $state('');
	let body = $state('');
	let targetAudience = $state('all');
	let sending = $state(false);

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<Broadcast[]>('/admin/broadcasts');
			if (result.ok) {
				broadcasts = result.data ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function createBroadcast() {
		if (!title.trim() || !body.trim()) return;
		sending = true;
		errorMessage = '';
		try {
			const result = await api('/admin/broadcasts', {
				method: 'POST',
				body: { title: title.trim(), body: body.trim(), target_audience: targetAudience }
			});
			if (result.ok) {
				showCreateModal = false;
				title = '';
				body = '';
				targetAudience = 'all';
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			sending = false;
		}
	}

	async function sendBroadcast(id: string) {
		errorMessage = '';
		try {
			await api(`/admin/broadcasts/${id}/send`, { method: 'POST', body: {} });
			load();
		} catch {
			errorMessage = t('common.error');
		}
	}

	function statusColor(status: string) {
		switch (status) {
			case 'sent': return 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400';
			case 'draft': return 'bg-amber-500/15 text-amber-700 dark:text-amber-400';
			case 'scheduled': return 'bg-blue-500/15 text-blue-700 dark:text-blue-400';
			default: return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('broadcasts.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('broadcasts.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('broadcasts.description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button class="btn btn-primary flex items-center gap-1.5 text-xs" onclick={() => { showCreateModal = true; }}>
		<Plus class="h-3 w-3" />
		{t('broadcasts.create')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('broadcasts.title')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('broadcasts.audience')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('broadcasts.status')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('broadcasts.created')}</th>
					<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each broadcasts as bc (bc.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium">{bc.title}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{bc.target_audience}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {statusColor(bc.status)}">
								{bc.status}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(bc.created_at).toLocaleDateString()}</td>
						<td class="px-4 py-3 text-right">
							{#if bc.status === 'draft'}
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10 flex items-center gap-1 inline-flex" onclick={() => sendBroadcast(bc.id)}>
									<Send class="h-3 w-3" style="color: var(--color-success)" />
									{t('broadcasts.send')}
								</button>
							{/if}
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('broadcasts.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<!-- Create Modal -->
<Modal bind:open={showCreateModal} title={t('broadcasts.create')} onConfirm={createBroadcast} confirmLabel={sending ? t('common.saving') : t('broadcasts.create')}>
	<div class="space-y-4">
		<div>
			<label for="bc-title" class="mb-1 block text-sm font-medium">{t('broadcasts.title')}</label>
			<input id="bc-title" type="text" bind:value={title} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="bc-body" class="mb-1 block text-sm font-medium">{t('broadcasts.body')}</label>
			<textarea id="bc-body" bind:value={body} rows="4" required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"></textarea>
		</div>
		<div>
			<label for="bc-audience" class="mb-1 block text-sm font-medium">{t('broadcasts.audience')}</label>
			<select id="bc-audience" bind:value={targetAudience} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)">
				<option value="all">{t('broadcasts.audience_all')}</option>
				<option value="active">{t('broadcasts.audience_active')}</option>
				<option value="premium">{t('broadcasts.audience_premium')}</option>
			</select>
		</div>
	</div>
</Modal>
