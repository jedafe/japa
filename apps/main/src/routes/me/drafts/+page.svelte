<script lang="ts">
	import { onMount } from 'svelte';
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, FileText, Calendar } from 'lucide-svelte';

	type Draft = {
		id: string;
		title: string;
		content_type: string;
		status: string;
		updated_at: string;
		scheduled_at?: string;
	};

	let drafts = $state<Draft[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			drafts = await request<Draft[]>('/me/drafts');
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('me.drafts.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.drafts.title')}</h1>

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
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('me.drafts.title')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('me.drafts.type')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('me.drafts.scheduled')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('me.drafts.updated')}</th>
				</tr>
			</thead>
			<tbody>
				{#each drafts as draft (draft.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium">{draft.title}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{draft.content_type}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">
							{draft.scheduled_at ? new Date(draft.scheduled_at).toLocaleString() : '—'}
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(draft.updated_at).toLocaleString()}</td>
					</tr>
				{:else}
					<tr>
						<td colspan="4" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('me.drafts.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
