<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import Pagination from '$lib/components/ui/Pagination.svelte';
	import { Tabs, Tooltip } from 'bits-ui';
	import { api } from '$lib/api';
	import type { DlqEntry } from '$lib/api/types';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, RotateCcw, RotateCw, Trash2 } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data = {} as PageData } = $props();

	// svelte-ignore state_referenced_locally
	let entries = $state<DlqEntry[]>(data.entries ?? []);
	let loading = $state(false);
	let filter = $state('');
	let retrying = $state<string | null>(null);
	let deleting = $state<string | null>(null);
	let confirmRetryOpen = $state(false);
	let confirmRetry = $state<DlqEntry | null>(null);
	let confirmDiscardOpen = $state(false);
	let confirmDiscard = $state<DlqEntry | null>(null);
	let confirmDiscardAllOpen = $state(false);
	let confirmDiscardAll = $state(false);
	let activeTab = $state('all');

	// Pagination
	let page = $state(0);
	let pageSize = $state(10);

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

	let totalPages = $derived(Math.max(1, Math.ceil(filtered.length / pageSize)));
	let paginatedEntries = $derived.by(() => {
		const start = page * pageSize;
		return filtered.slice(start, start + pageSize);
	});

	let byTaskType = $derived(
		Object.entries(
			entries.reduce((acc, e) => {
				(acc[e.task_type] ??= []).push(e);
				return acc;
			}, {} as Record<string, DlqEntry[]>)
		).sort(([, a], [, b]) => b.length - a.length)
	);
</script>

<PageHeader title={t('dlq.title')} description={t('dlq.description')} />

<div class="mb-4 flex items-center gap-3">
	<input
		bind:value={filter}
		placeholder="Filter by task type or error..."
		class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 h-input rounded-9px w-72 border px-3 py-2.5 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
	/>
	<button class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-3 py-1.5 text-xs font-medium transition-colors" onclick={load}>
		<RefreshCw class="size-3" />
		{t('common.refresh')}
	</button>
	{#if entries.length > 0}
		<Tooltip.Root>
			<Tooltip.Trigger>
				<button
					class="border-destructive/30 bg-destructive/10 text-destructive hover:bg-destructive/20 rounded-9px border px-3 py-1.5 text-xs font-medium transition-colors"
					onclick={() => (confirmDiscardAll = true)}
				>
					<Trash2 class="size-3" />
					Discard All
				</button>
			</Tooltip.Trigger>
			<Tooltip.Portal>
				<Tooltip.Content side="top" sideOffset={4} class="border-dark-10 bg-background shadow-popover z-50 rounded-input border px-3 py-2 text-xs">
					Permanently discard all {entries.length} entries
				</Tooltip.Content>
			</Tooltip.Portal>
		</Tooltip.Root>
	{/if}
</div>

<Tabs.Root bind:value={activeTab}>
	<Tabs.List class="bg-muted inline-flex gap-1 rounded-9px p-1">
		<Tabs.Trigger value="all" class="data-[state=active]:bg-background data-[state=active]:shadow-mini rounded-7px px-4 py-2 text-sm font-medium transition-all">
			All ({entries.length})
		</Tabs.Trigger>
		<Tabs.Trigger value="by-type" class="data-[state=active]:bg-background data-[state=active]:shadow-mini rounded-7px px-4 py-2 text-sm font-medium transition-all">
			By Task Type ({byTaskType.length})
		</Tabs.Trigger>
	</Tabs.List>

	<Tabs.Content value="all" class="mt-4">
		{#if loading}
			<p class="text-muted-foreground py-8 text-center text-sm">Loading...</p>
		{:else if filtered.length === 0}
			<div class="border-border-card bg-background-alt rounded-9px border p-8 text-center">
				<p class="text-muted-foreground text-sm">No dead letter queue entries.</p>
			</div>
		{:else}
			<div class="border-border-card bg-background-alt shadow-card overflow-hidden rounded-xl">
				<table class="w-full text-left text-sm">
					<thead>
						<tr class="border-border-card border-b">
							<th class="text-muted-foreground px-4 py-3 font-medium">Task Type</th>
							<th class="text-muted-foreground px-4 py-3 font-medium">Error</th>
							<th class="text-muted-foreground px-4 py-3 font-medium">Retries</th>
							<th class="text-muted-foreground px-4 py-3 font-medium">Failed At</th>
							<th class="text-muted-foreground px-4 py-3 font-medium">Actions</th>
						</tr>
					</thead>
					<tbody>
						{#each paginatedEntries as entry, index (entry.task_id + '-' + index)}
							<tr class="border-border-card border-t">
								<td class="px-4 py-3">
									<span class="bg-tertiary/12 text-tertiary rounded-5px px-1.5 py-0.5 text-xs">{entry.task_type}</span>
								</td>
								<td class="text-destructive max-w-xs truncate px-4 py-3 text-xs">{entry.error_message}</td>
								<td class="px-4 py-3 text-xs">{entry.retry_count}</td>
								<td class="text-muted-foreground px-4 py-3 text-xs">{new Date(entry.failed_at).toLocaleString()}</td>
								<td class="px-4 py-3">
									<div class="flex items-center gap-2">
										<Tooltip.Root>
											<Tooltip.Trigger>
												<button
													class="text-accent-foreground hover:bg-accent flex items-center gap-1 rounded-5px px-2 py-1 text-xs font-medium transition-colors"
													disabled={retrying === entry.task_id || deleting === entry.task_id}
													onclick={() => (confirmRetry = entry)}
												>
													<RotateCw class="size-3 {retrying === entry.task_id ? 'animate-spin' : ''}" />
													Retry
												</button>
											</Tooltip.Trigger>
											<Tooltip.Portal>
												<Tooltip.Content side="top" sideOffset={4} class="border-dark-10 bg-background shadow-popover z-50 rounded-input border px-3 py-2 text-xs">
													Re-queue this task for retry
												</Tooltip.Content>
											</Tooltip.Portal>
										</Tooltip.Root>
										<Tooltip.Root>
											<Tooltip.Trigger>
												<button
													class="text-destructive hover:bg-destructive/10 flex items-center gap-1 rounded-5px px-2 py-1 text-xs font-medium transition-colors"
													disabled={retrying === entry.task_id || deleting === entry.task_id}
													onclick={() => (confirmDiscard = entry)}
												>
													<Trash2 class="size-3 {deleting === entry.task_id ? 'animate-pulse' : ''}" />
													Discard
												</button>
											</Tooltip.Trigger>
											<Tooltip.Portal>
												<Tooltip.Content side="top" sideOffset={4} class="border-dark-10 bg-background shadow-popover z-50 rounded-input border px-3 py-2 text-xs">
													Permanently discard this task
												</Tooltip.Content>
											</Tooltip.Portal>
										</Tooltip.Root>
									</div>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>

			<Pagination
				bind:page
				{totalPages}
				totalItems={filtered.length}
				bind:pageSize
			/>
		{/if}
	</Tabs.Content>

	<Tabs.Content value="by-type" class="mt-4">
		<div class="space-y-4">
			{#each byTaskType as [taskType, items]}
				<div class="border-border-card bg-background-alt shadow-card rounded-xl">
					<div class="border-border-card flex items-center justify-between border-b px-4 py-3">
						<div class="flex items-center gap-2">
							<span class="bg-tertiary/12 text-tertiary rounded-5px px-2 py-0.5 text-xs font-medium">{taskType}</span>
							<span class="bg-muted text-muted-foreground rounded-full px-2 py-0.5 text-[10px] font-medium">{items.length}</span>
						</div>
					</div>
					<div class="divide-border-card divide-y">
						{#each items as entry, idx (entry.task_id + '-' + idx)}
							<div class="flex items-center justify-between px-4 py-3">
								<div class="min-w-0 flex-1">
									<p class="text-destructive truncate text-xs">{entry.error_message}</p>
									<p class="text-muted-foreground text-[10px]">Retries: {entry.retry_count} | {new Date(entry.failed_at).toLocaleString()}</p>
								</div>
								<div class="ml-4 flex items-center gap-1">
									<button
										class="text-accent-foreground hover:bg-accent rounded-5px p-1 transition-colors"
										disabled={retrying === entry.task_id}
										onclick={() => (confirmRetry = entry)}
									>
										<RotateCw class="size-3" />
									</button>
									<button
										class="text-destructive hover:bg-destructive/10 rounded-5px p-1 transition-colors"
										disabled={deleting === entry.task_id}
										onclick={() => (confirmDiscard = entry)}
									>
										<Trash2 class="size-3" />
									</button>
								</div>
							</div>
						{/each}
					</div>
				</div>
			{/each}
		</div>
	</Tabs.Content>
</Tabs.Root>

{#if confirmRetry}
	<ConfirmModal
		bind:open={confirmRetryOpen}
		title="Retry Dead Letter Task"
		consequence="Re-queue task {confirmRetry.task_type} for retry. This may fail again if the underlying issue persists."
		confirmText="RETRY"
		onConfirm={() => { if (confirmRetry) retryTask(confirmRetry.task_id); }}
		onCancel={() => { confirmRetry = null; }}
	/>
{/if}

{#if confirmDiscard}
	<ConfirmModal
		bind:open={confirmDiscardOpen}
		title="Discard DLQ Task"
		consequence="Permanently discard task {confirmDiscard.task_type} ({confirmDiscard.task_id}). This cannot be undone."
		confirmText="DISCARD"
		onConfirm={() => { if (confirmDiscard) discardTask(confirmDiscard.task_id); }}
		onCancel={() => { confirmDiscard = null; }}
	/>
{/if}

{#if confirmDiscardAll}
	<ConfirmModal
		bind:open={confirmDiscardAllOpen}
		title="Discard All DLQ Tasks"
		consequence="Permanently discard ALL {entries.length} dead letter queue entries. This cannot be undone."
		confirmText="DISCARD ALL"
		onConfirm={discardAll}
		onCancel={() => { confirmDiscardAll = false; }}
	/>
{/if}
