<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import SearchInput from '$lib/components/ui/SearchInput.svelte';
	import AlertBanner from '$lib/components/ui/AlertBanner.svelte';
	import Tabs from '$lib/components/ui/Tabs.svelte';
	import TabsContent from '$lib/components/ui/TabsContent.svelte';
	import Tooltip from '$lib/components/ui/Tooltip.svelte';
	import { api } from '$lib/api';
	import type { DlqEntry, DataTableColumn, DataTableRowAction, DataTableBulkAction } from '$lib/api/types';
	import { t, translations } from '$lib/stores/i18n.svelte';
	import { escapeHtml } from '$lib/utils';
	import { RefreshCw, RotateCw, Trash2 } from 'lucide-svelte';

	let entries = $state<DlqEntry[]>([]);
	let loading = $state(true);
	let filter = $state('');
	let retrying = $state<string | null>(null);
	let deleting = $state<string | null>(null);
	let activeTab = $state('all');

	let page = $state(0);
	let pageSize = $state(10);
	let sortKey = $state('');
	let sortDirection = $state<'asc' | 'desc'>('asc');
	let selectedIds = $state(new Set<string>());

	let successMessage = $state('');
	let errorMessage = $state('');

	// H2: Add real polling interval for live DLQ monitoring.
	// The previous $effect was a one-shot fetch with only an abort timeout.
	$effect(() => {
		const controller = new AbortController();
		const timeout = setTimeout(() => controller.abort(), 15000);
		load(controller.signal);
		// Poll every 15s for live queue updates — quiet so the loading
		// skeleton doesn't flash on every tick.
		const interval = setInterval(() => {
			const c = new AbortController();
			const t = setTimeout(() => c.abort(), 15000);
			load(c.signal, true).finally(() => clearTimeout(t));
		}, 15_000);
		return () => {
			clearTimeout(timeout);
			clearInterval(interval);
			controller.abort();
		};
	});

	let confirmRetryOpen = $state(false);
	let confirmRetryEntry = $state<DlqEntry | null>(null);
	let confirmDiscardOpen = $state(false);
	let confirmDiscardEntry = $state<DlqEntry | null>(null);
	let confirmDiscardAllOpen = $state(false);
	let confirmDiscardAll = $state(false);
	let confirmBulkDiscardOpen = $state(false);
	let pendingBulkIds = $state<string[]>([]);

	function formatDate(iso: string | null | undefined): string {
		if (!iso) return '\u2014';
		try {
			const d = new Date(iso);
			if (isNaN(d.getTime())) return '\u2014';
			return d.toLocaleString();
		} catch {
			return '\u2014';
		}
	}

	function showSuccess(msg: string) {
		successMessage = msg;
		errorMessage = '';
		setTimeout(() => { successMessage = ''; }, 3000);
	}

	function showError(msg: string) {
		errorMessage = msg;
		successMessage = '';
		setTimeout(() => { errorMessage = ''; }, 5000);
	}

	async function load(signal?: AbortSignal, quiet = false) {
		if (!quiet) loading = true;
		try {
			const result = await api<DlqEntry[]>('/control/queue/dlq', { signal });
			if (signal?.aborted) return;
			if (result.ok) {
				entries = result.data ?? [];
			} else {
				showError(t('dlq.load_error', 'Failed to load DLQ entries.'));
			}
		} catch {
			if (!signal?.aborted) showError(t('dlq.load_error', 'Failed to load DLQ entries.'));
		} finally {
			loading = false;
		}
	}

	async function retryTask(taskId: string) {
		retrying = taskId;
		try {
			const result = await api(`/control/queue/dlq/${taskId}/retry`, { method: 'POST' });
			if (result.ok) {
				showSuccess(t('dlq.retry_success'));
				await load();
			} else {
				showError(result.error || t('dlq.retry_error'));
			}
		} catch {
			showError(t('dlq.retry_error'));
		} finally {
			retrying = null;
			confirmRetryEntry = null;
		}
	}

	async function discardTask(taskId: string) {
		deleting = taskId;
		try {
			const result = await api(`/control/queue/dlq/${taskId}`, { method: 'DELETE' });
			if (result.ok) {
				showSuccess(t('dlq.discard_success'));
				await load();
			} else {
				showError(result.error || t('dlq.discard_error'));
			}
		} catch {
			showError(t('dlq.discard_error'));
		} finally {
			deleting = null;
			confirmDiscardEntry = null;
		}
	}

	async function discardAll() {
		deleting = '__all__';
		try {
			const result = await api<{ discarded?: number; errors?: string[] }>(
				'/control/queue/dlq/discard-all',
				{
					method: 'POST',
					body: { confirm: true }
				}
			);
			if (result.ok) {
				selectedIds = new Set();
				// Re-fetch rather than clearing locally — a per-queue partial
				// failure (data.errors) leaves tasks server-side that must
				// stay visible in the list.
				await load();
				const errs = result.data?.errors ?? [];
				if (errs.length > 0) {
					showError(errs.join('; '));
				} else {
					showSuccess(t('dlq.discard_all_success'));
				}
			} else {
				showError(result.error || t('dlq.discard_all_error'));
			}
		} catch {
			showError(t('dlq.discard_all_error'));
		} finally {
			deleting = null;
			confirmDiscardAll = false;
		}
	}

	function extractTaskId(rowId: string): string {
		// Row ids are `${task_id}::${pageIndex}` — strip the trailing index
		// suffix rather than splitting on '::' (task ids may contain '::').
		const idx = rowId.lastIndexOf('::');
		return idx === -1 ? rowId : rowId.slice(0, idx);
	}

	async function bulkDiscard(ids: string[]) {
		const taskIds = ids.map(extractTaskId);
		const deleted = new Set<string>();
		for (const taskId of taskIds) {
			const result = await api(`/control/queue/dlq/${taskId}`, { method: 'DELETE' });
			if (result.ok) deleted.add(taskId);
		}
		// Remove only confirmed deletions — failed ids stay in the list.
		entries = entries.filter((e) => !deleted.has(e.task_id));
		selectedIds = new Set();
		const failed = taskIds.length - deleted.size;
		if (failed > 0) {
			showError(t('dlq.discard_partial_error', '{count} tasks could not be discarded.').replace('{count}', String(failed)));
		} else {
			showSuccess(t('dlq.discard_success'));
		}
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

	let sorted = $derived.by(() => {
		if (!sortKey) return filtered;
		const sortedArr = [...filtered];
		sortedArr.sort((a, b) => {
			const av = a[sortKey as keyof DlqEntry];
			const bv = b[sortKey as keyof DlqEntry];
			if (av == null && bv == null) return 0;
			if (av == null) return 1;
			if (bv == null) return -1;
			const cmp = String(av).localeCompare(String(bv));
			return sortDirection === 'asc' ? cmp : -cmp;
		});
		return sortedArr;
	});

	let paginatedEntries = $derived.by(() => {
		const start = page * pageSize;
		return sorted.slice(start, start + pageSize);
	});

	let rows = $derived(
		paginatedEntries.map((e, i) => ({
			id: `${e.task_id}::${page * pageSize + i}`,
			task_id: e.task_id,
			task_type: e.task_type,
			queue: e.queue,
			error_message: e.error_message,
			retry_count: e.retry_count,
			max_retries: e.max_retries,
			failed_at: e.failed_at
		}))
	);

	let byTaskType = $derived(
		Object.entries(
			entries.reduce((acc, e) => {
				(acc[e.task_type] ??= []).push(e);
				return acc;
			}, {} as Record<string, DlqEntry[]>)
		).sort(([, a], [, b]) => b.length - a.length)
	);

	let columns = $derived.by<DataTableColumn[]>(() => {
		const tr = translations();
		return [
			{
				key: 'task_type',
				label: t('dlq.task_type'),
				sortable: true,
				render: (val) =>
					`<span class="bg-tertiary/12 text-tertiary rounded-5px px-1.5 py-0.5 text-xs">${escapeHtml(val)}</span>`
			},
			{
				key: 'queue',
				label: t('dlq.queue', 'Queue'),
				sortable: true,
				render: (val) =>
					`<span class="bg-muted text-muted-foreground rounded-5px px-1.5 py-0.5 text-xs">${escapeHtml(val)}</span>`
			},
			{
				key: 'error_message',
				label: t('dlq.error'),
				sortable: true,
				class: 'max-w-xs truncate',
				render: (val) =>
					`<span class="text-destructive text-xs">${escapeHtml(val)}</span>`
			},
			{
				key: 'retry_count',
				label: t('dlq.retries'),
				sortable: true,
				render: (val) => `<span class="text-xs">${escapeHtml(val)}</span>`
			},
			{
				key: 'failed_at',
				label: t('dlq.failed_at'),
				sortable: true,
				render: (val) =>
					`<span class="text-muted-foreground text-xs">${escapeHtml(formatDate(val as string))}</span>`
			}
		];
	});

	function rowToEntry(row: Record<string, unknown>): DlqEntry {
		return {
			task_id: row.task_id as string,
			task_type: row.task_type as string,
			queue: (row.queue as string) ?? '',
			error_message: row.error_message as string,
			retry_count: row.retry_count as number,
			max_retries: (row.max_retries as number) ?? 0,
			failed_at: row.failed_at as string,
			payload: {}
		};
	}

	let rowActions = $derived.by<DataTableRowAction[]>(() => {
		const tr = translations();
		return [
			{
				label: t('dlq.retry'),
				icon: RotateCw,
				onClick: (row) => {
					confirmRetryEntry = rowToEntry(row as Record<string, unknown>);
					confirmRetryOpen = true;
				},
				disabled: (row) => retrying === (row as Record<string, unknown>).task_id || deleting === (row as Record<string, unknown>).task_id
			},
			{
				label: t('dlq.discard'),
				icon: Trash2,
				variant: 'destructive',
				onClick: (row) => {
					confirmDiscardEntry = rowToEntry(row as Record<string, unknown>);
					confirmDiscardOpen = true;
				},
				disabled: (row) => retrying === (row as Record<string, unknown>).task_id || deleting === (row as Record<string, unknown>).task_id
			}
		];
	});

	let bulkActions = $derived.by<DataTableBulkAction[]>(() => {
		const tr = translations();
		return [
			{
				label: t('dlq.discard_selected'),
				icon: Trash2,
				variant: 'destructive',
				onClick: (ids) => {
					pendingBulkIds = ids;
					confirmBulkDiscardOpen = true;
				}
			}
		];
	});

	function handleBulkDiscardConfirm() {
		bulkDiscard(pendingBulkIds);
		confirmBulkDiscardOpen = false;
		pendingBulkIds = [];
	}
</script>

<PageHeader title={t('dlq.title')} description={t('dlq.description')} />

{#if successMessage}
	<AlertBanner variant="success" message={successMessage} class="mb-4" />
{/if}

{#if errorMessage}
	<AlertBanner variant="error" message={errorMessage} class="mb-4" />
{/if}

<div class="mb-4 flex items-center gap-3">
	<SearchInput bind:value={filter} placeholder={t('dlq.filter_placeholder')} class="w-72" />
	<button
		class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-3 py-1.5 text-xs font-medium transition-colors"
		onclick={() => load()}
	>
		<RefreshCw class="size-3" />
		{t('common.refresh')}
	</button>
	{#if entries.length > 0}
		<Tooltip content={t('dlq.discard_all_tooltip').replace('{count}', String(entries.length))} sideOffset={4}>
			<button
				class="border-destructive/30 bg-destructive/10 text-destructive hover:bg-destructive/20 rounded-9px border px-3 py-1.5 text-xs font-medium transition-colors"
				onclick={() => { confirmDiscardAll = true; confirmDiscardAllOpen = true; }}
			>
				<Trash2 class="size-3" />
				{t('dlq.discard_all')}
			</button>
		</Tooltip>
	{/if}
</div>

<Tabs
	bind:value={activeTab}
	tabs={[
		{ value: 'all', label: `${t('dlq.all')} (${entries.length})` },
		{ value: 'by-type', label: `${t('dlq.by_type')} (${byTaskType.length})` }
	]}
>
	<TabsContent value="all">
		<DataTable
			{columns}
			{rows}
			totalRows={filtered.length}
			bind:page
			bind:pageSize
			bind:sortKey
			bind:sortDirection
			bind:selectedIds
			{rowActions}
			{bulkActions}
			{loading}
			emptyMessage={t('dlq.no_entries')}
			getRowId={(row) => (row as Record<string, unknown>).id as string}
		/>
	</TabsContent>

	<TabsContent value="by-type">
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
									<p class="text-muted-foreground text-[10px]">{t('dlq.retries')}: {entry.retry_count} | {formatDate(entry.failed_at)}</p>
								</div>
								<div class="ml-4 flex items-center gap-1">
									<button
										class="text-accent-foreground hover:bg-accent rounded-5px p-1 transition-colors"
										disabled={retrying === entry.task_id}
										onclick={() => { confirmRetryEntry = entry; confirmRetryOpen = true; }}
									>
										<RotateCw class="size-3" />
									</button>
									<button
										class="text-destructive hover:bg-destructive/10 rounded-5px p-1 transition-colors"
										disabled={deleting === entry.task_id}
										onclick={() => { confirmDiscardEntry = entry; confirmDiscardOpen = true; }}
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
	</TabsContent>
</Tabs>

{#if confirmRetryEntry}
	<ConfirmModal
		bind:open={confirmRetryOpen}
		title={t('dlq.retry_title')}
		consequence={t('dlq.retry_confirm').replace('{task_type}', confirmRetryEntry.task_type)}
		confirmText={t('dlq.retry_confirm_text')}
		onConfirm={() => { if (confirmRetryEntry) retryTask(confirmRetryEntry.task_id); }}
		onCancel={() => { confirmRetryEntry = null; }}
	/>
{/if}

{#if confirmDiscardEntry}
	<ConfirmModal
		bind:open={confirmDiscardOpen}
		title={t('dlq.discard_title')}
		consequence={t('dlq.discard_confirm').replace('{task_type}', confirmDiscardEntry.task_type).replace('{task_id}', confirmDiscardEntry.task_id)}
		confirmText={t('dlq.discard_confirm_text')}
		onConfirm={() => { if (confirmDiscardEntry) discardTask(confirmDiscardEntry.task_id); }}
		onCancel={() => { confirmDiscardEntry = null; }}
	/>
{/if}

{#if confirmDiscardAll}
	<ConfirmModal
		bind:open={confirmDiscardAllOpen}
		title={t('dlq.discard_all_title')}
		consequence={t('dlq.discard_all_confirm').replace('{count}', String(entries.length))}
		confirmText={t('dlq.discard_all_confirm_text')}
		onConfirm={discardAll}
		onCancel={() => { confirmDiscardAll = false; }}
	/>
{/if}

{#if confirmBulkDiscardOpen}
	<ConfirmModal
		bind:open={confirmBulkDiscardOpen}
		title={t('dlq.discard_selected_title')}
		consequence={t('dlq.discard_selected_confirm').replace('{count}', String(pendingBulkIds.length))}
		confirmText={t('dlq.discard_selected_confirm_text')}
		onConfirm={handleBulkDiscardConfirm}
		onCancel={() => { confirmBulkDiscardOpen = false; pendingBulkIds = []; }}
	/>
{/if}
