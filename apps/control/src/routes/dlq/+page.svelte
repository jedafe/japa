<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { api } from '$lib/api';
	import type { DlqEntry } from '$lib/api/types';
	import { t } from '$lib/stores/i18n.svelte';
	import { AlertTriangle, RefreshCw, RotateCcw, RotateCw, Trash2 } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data = {} as PageData } = $props();

	// svelte-ignore state_referenced_locally
	let entries = $state<DlqEntry[]>(data.entries ?? []);
	let loading = $state(false);
	let filter = $state('');
	let retrying = $state<string | null>(null);
	let deleting = $state<string | null>(null);
	let confirmOpen = $state(false);
	let confirmRetry = $state<DlqEntry | null>(null);
	let confirmDiscard = $state<DlqEntry | null>(null);
	let confirmDiscardAll = $state(false);

	async function load() {
		loading = true;
		const result = await api<DlqEntry[]>('/control/queue/dlq');
		if (result.ok) {
			entries = result.data;
		}
		loading = false;
	}

	async function retryTask(taskId: string) {
		retrying = taskId;
		await api(`/control/queue/dlq/${taskId}/retry`, { method: 'POST' });
		retrying = null;
		confirmRetry = null;
		await load();
	}

	async function discardTask(taskId: string) {
		deleting = taskId;
		await api(`/control/queue/dlq/${taskId}`, { method: 'DELETE' });
		deleting = null;
		confirmDiscard = null;
		entries = entries.filter((e) => e.task_id !== taskId);
	}

	async function discardAll() {
		deleting = '__all__';
		await api('/control/queue/dlq/discard-all', { method: 'POST' });
		deleting = null;
		confirmDiscardAll = false;
		entries = [];
	}

	let filtered = $derived(
		filter
			? entries.filter(
					(e) =>
						e.task_type.toLowerCase().includes(filter.toLowerCase()) ||
						e.error_message.toLowerCase().includes(filter.toLowerCase())
				)
			: entries
	);
</script>

<PageHeader title={t('dlq.title')} description={t('dlq.description')} />

<div class="mb-4 flex items-center gap-3">
	<input
		bind:value={filter}
		placeholder="Filter by task type or error..."
		class="w-72 rounded-lg border py-2 px-3 text-sm outline-none"
		style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
	/>
	<button class="btn btn-secondary text-xs" onclick={load}>
		<RefreshCw class="h-3 w-3" />
		{t('common.refresh')}
	</button>
	{#if entries.length > 0}
		<button
			class="btn text-xs"
			style="color: var(--color-danger); border: 1px solid var(--color-danger)"
			onclick={() => (confirmDiscardAll = true)}
		>
			<Trash2 class="h-3 w-3" />
			Discard All
		</button>
	{/if}
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else if filtered.length === 0}
	<div class="rounded-lg border p-8 text-center" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<p class="text-sm" style="color: var(--color-muted)">No dead letter queue entries.</p>
	</div>
{:else}
	<div class="overflow-hidden rounded-lg border" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<table class="w-full text-left text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Task Type</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Error</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Retries</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Failed At</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Actions</th>
				</tr>
			</thead>
			<tbody>
				{#each filtered as entry (entry.task_id)}
					<tr style="border-bottom: 1px solid var(--color-border)">
						<td class="px-4 py-3">
							<span class="rounded px-1.5 py-0.5 text-xs" style="background: color-mix(in srgb, var(--color-warning) 12%, transparent); color: var(--color-warning)">
								{entry.task_type}
							</span>
						</td>
						<td class="max-w-xs truncate px-4 py-3 text-xs" style="color: var(--color-danger)">
							{entry.error_message}
						</td>
						<td class="px-4 py-3 text-xs">{entry.retry_count}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">
							{new Date(entry.failed_at).toLocaleString()}
						</td>
						<td class="px-4 py-3">
							<div class="flex items-center gap-2">
								<button
									class="flex items-center gap-1 rounded px-2 py-1 text-xs font-medium transition-colors"
									style="color: var(--color-primary)"
									disabled={retrying === entry.task_id || deleting === entry.task_id}
									onclick={() => (confirmRetry = entry)}
								>
									<RotateCw class="h-3 w-3 {retrying === entry.task_id ? 'animate-spin' : ''}" />
									Retry
								</button>
								<button
									class="flex items-center gap-1 rounded px-2 py-1 text-xs font-medium transition-colors"
									style="color: var(--color-danger)"
									disabled={retrying === entry.task_id || deleting === entry.task_id}
									onclick={() => (confirmDiscard = entry)}
								>
									<Trash2 class="h-3 w-3 {deleting === entry.task_id ? 'animate-pulse' : ''}" />
									Discard
								</button>
							</div>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

{#if confirmRetry}
	<ConfirmModal
		bind:open={confirmOpen}
		title="Retry Dead Letter Task"
		consequence="Re-queue task {confirmRetry.task_type} for retry. This may fail again if the underlying issue persists."
		confirmText="RETRY"
		onConfirm={() => { if (confirmRetry) retryTask(confirmRetry.task_id); }}
		onCancel={() => { confirmRetry = null; }}
	/>
{/if}

{#if confirmDiscard}
	<ConfirmModal
		bind:open={confirmOpen}
		title="Discard DLQ Task"
		consequence="Permanently discard task {confirmDiscard.task_type} ({confirmDiscard.task_id}). This cannot be undone."
		confirmText="DISCARD"
		onConfirm={() => { if (confirmDiscard) discardTask(confirmDiscard.task_id); }}
		onCancel={() => { confirmDiscard = null; }}
	/>
{/if}

{#if confirmDiscardAll}
	<ConfirmModal
		bind:open={confirmOpen}
		title="Discard All DLQ Tasks"
		consequence="Permanently discard ALL {entries.length} dead letter queue entries. This cannot be undone."
		confirmText="DISCARD ALL"
		onConfirm={discardAll}
		onCancel={() => { confirmDiscardAll = false; }}
	/>
{/if}
