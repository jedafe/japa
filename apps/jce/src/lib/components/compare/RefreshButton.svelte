<script lang="ts">
	import { onDestroy } from 'svelte';
	import { t } from '$lib/i18n';
	import { listIngestionJobs, refreshComparison } from '$lib/api/jce';
	import type { IngestionJob } from '$lib/types/jce';
	import { RefreshCw } from 'lucide-svelte';

	interface Props {
		comparisonID: string;
		sourceKind?: string;
	}

	let { comparisonID, sourceKind }: Props = $props();

	let refreshing = $state(false);
	let lastJob = $state<IngestionJob | null>(null);
	let error = $state('');
	let timeout = $state<ReturnType<typeof setTimeout> | null>(null);

	function stopPolling() {
		if (timeout) {
			clearTimeout(timeout);
			timeout = null;
		}
	}

	async function poll() {
		if (!lastJob) return;
		try {
			const jobs = await listIngestionJobs(comparisonID);
			const found = jobs?.find((j) => j.id === lastJob?.id);
			if (found) lastJob = found;
			const active = found && (found.status === 'pending' || found.status === 'running');
			if (active) {
				timeout = setTimeout(poll, 2000);
			} else {
				refreshing = false;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('jce.refresh.error', 'Could not poll refresh job');
			refreshing = false;
		}
	}

	async function onClick() {
		if (refreshing) return;
		stopPolling();
		error = '';
		refreshing = true;
		const job = await refreshComparison(comparisonID, sourceKind);
		if (job) {
			lastJob = job;
			poll();
		} else {
			refreshing = false;
			error = t('jce.refresh.error', 'Could not start refresh');
		}
	}

	onDestroy(stopPolling);
</script>

{#if lastJob}
	<p class="text-xs" style="color: var(--color-muted)">
		{t('jce.refresh.status', 'Status: {status}', { status: lastJob.status })}
		{#if lastJob.refreshed_count > 0}({lastJob.refreshed_count}){/if}
	</p>
{/if}
{#if error}
	<p class="text-xs text-red-400">{error}</p>
{/if}
<button
	type="button"
	class="inline-flex items-center gap-1.5 rounded-[var(--radius-button)] border px-3 py-1.5 text-xs font-medium transition-colors disabled:opacity-50"
	style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-text)"
	onclick={onClick}
	disabled={refreshing}
>
	<RefreshCw size={12} class={refreshing ? 'animate-spin' : ''} />
	{refreshing ? t('jce.refresh.refreshing', 'Refreshing...') : t('jce.refresh.button', 'Refresh data')}
</button>
