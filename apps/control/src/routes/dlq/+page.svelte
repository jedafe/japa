<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import SearchInput from '$lib/components/ui/SearchInput.svelte';
	import AlertBanner from '$lib/components/ui/AlertBanner.svelte';
	import { Tabs, Tooltip } from 'bits-ui';
	import { api } from '$lib/api';
	import type { DlqEntry, DataTableColumn, DataTableRowAction, DataTableBulkAction } from '$lib/api/types';
	import { t, translations } from '$lib/stores/i18n';
	import { RefreshCw, RotateCw, Trash2 } from 'lucide-svelte';

	let entries = $state<DlqEntry[]>([]);
	let loading = $state(false);
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

	$effect(() => {
		const controller = new AbortController();
		load(controller.signal);
		return () => controller.abort();
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

	function escapeHtml(str: unknown): string {
		if (str == null) return '';
		return String(str)
			.replace(/&/g, '&amp;')
			.replace(/</g, '&lt;')
			.replace(/>/g, '&gt;')
			.replace(/"/g, '&quot;')
			.replace(/'/g, '&#39;');
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

	async function load(signal?: AbortSignal) {
		loading = true;
		const result = await api<DlqEntry[]>('/control/queue/dlq', { signal });
		if (result.ok) {
			entries = result.data;
			showSuccess(t('dlq.loaded'));
		} else {
			showError(t('dlq.load_error'));
		}
		loading = false;
	}

	async function retryTask(taskId: string) {
		retrying = taskId;
		const result = await api(`/control/queue/dlq/${taskId}/retry`, { method: 'POST' });
		retrying = null;
		confirmRetryEntry = null;
		if (result.ok) {
			showSuccess(t('dlq.retry_success'));
			await load();
		} else {
			showError(t('dlq.retry_error'));
		}
	}

	async function discardTask(taskId: string) {
		deleting = taskId;
		const result = await api(`/control/queue/dlq/${taskId}`, { method: 'DELETE' });
		deleting = null;
		confirmDiscardEntry = null;
		if (result.ok) {
			entries = entries.filter((e) => e.task_id !== taskId);
			showSuccess(t('dlq.discard_success'));
		} else {
			showError(t('dlq.discard_error'));
		}
	}

	async function discardAll() {
		deleting = '__all__';
		const result = await api('/control/queue/dlq/discard-all', { method: 'POST' });
		deleting = null;
		confirmDiscardAll = false;
		if (result.ok) {
			entries = [];
			selectedIds = new Set();
			showSuccess(t('dlq.discard_all_success'));
		} else {
			showError(t('dlq.discard_all_error'));
		}
	}

	function extractTaskId(rowId: string): string {
		return rowId.split('::')[0];
	}

	async function bulkDiscard(ids: string[]) {
		const taskIds = ids.map(extractTaskId);
		for (const taskId of taskIds) {
			await api(`/control/queue/dlq/${taskId}`, { method: 'DELETE' });
		}
		entries = entries.filter((e) => !taskIds.includes(e.task_id));
		selectedIds = new Set();
		showSuccess(t('dlq.discard_success'));
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
			error_message: e.error_message,
			retry_count: e.retry_count,
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
		const tr = $translations;
		return [
			{
				key: 'task_type',
				label: t('dlq.task_type', undefined, tr),
				sortable: true,
				render: (val) =>
					`<span class="bg-tertiary/12 text-tertiary rounded-5px px-1.5 py-0.5 text-xs">${escapeHtml(val)}</span>`
			},
			{
				key: 'error_message',
				label: t('dlq.error', undefined, tr),
				sortable: true,
				class: 'max-w-xs truncate',
				render: (val) =>
					`<span class="text-destructive text-xs">${escapeHtml(val)}</span>`
			},
			{
				key: 'retry_count',
				label: t('dlq.retries', undefined, tr),
				sortable: true,
				render: (val) => `<span class="text-xs">${escapeHtml(val)}</span>`
			},
			{
				key: 'failed_at',
				label: t('dlq.failed_at', undefined, tr),
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
			error_message: row.error_message as string,
			retry_count: row.retry_count as number,
			failed_at: row.failed_at as string,
			payload: {}
		};
	}

	let rowActions = $derived.by<DataTableRowAction[]>(() => {
		const tr = $translations;
		return [
			{
				label: t('dlq.retry', undefined, tr),
				icon: RotateCw,
				onClick: (row) => {
					confirmRetryEntry = rowToEntry(row as Record<string, unknown>);
					confirmRetryOpen = true;
				},
				disabled: (row) => retrying === (row as Record<string, unknown>).task_id || deleting === (row as Record<string, unknown>).task_id
			},
			{
				label: t('dlq.discard', undefined, tr),
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
		const tr = $translations;
		return [
			{
				label: t('dlq.discard_selected', undefined, tr),
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

<PageHeader title={t('dlq.title', undefined, $translations)} description={t('dlq.description', undefined, $translations)} />

{#if successMessage}
	<AlertBanner variant="success" message={successMessage} class="mb-4" />
{/if}

{#if errorMessage}
	<AlertBanner variant="error" message={errorMessage} class="mb-4" />
{/if}

<div class="mb-4 flex items-center gap-3">
	<SearchInput bind:value={filter} placeholder={t('dlq.filter_placeholder', undefined, $translations)} class="w-72" />
	<button
		class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-3 py-1.5 text-xs font-medium transition-colors"
		onclick={() => load()}
	>
		<RefreshCw class="size-3" />
		{t('common.refresh', undefined, $translations)}
	</button>
	{#if entries.length > 0}
		<Tooltip.Root>
			<Tooltip.Trigger>
				<button
					class="border-destructive/30 bg-destructive/10 text-destructive hover:bg-destructive/20 rounded-9px border px-3 py-1.5 text-xs font-medium transition-colors"
					onclick={() => { confirmDiscardAll = true; confirmDiscardAllOpen = true; }}
				>
					<Trash2 class="size-3" />
					{t('dlq.discard_all', undefined, $translations)}
				</button>
			</Tooltip.Trigger>
			<Tooltip.Portal>
				<Tooltip.Content side="top" sideOffset={4} class="border-dark-10 bg-background shadow-popover z-50 rounded-input border px-3 py-2 text-xs">
					{t('dlq.discard_all_tooltip', undefined, $translations).replace('{count}', String(entries.length))}
				</Tooltip.Content>
			</Tooltip.Portal>
		</Tooltip.Root>
	{/if}
</div>

<Tabs.Root bind:value={activeTab}>
	<Tabs.List class="bg-muted inline-flex gap-1 rounded-9px p-1">
		<Tabs.Trigger value="all" class="data-[state=active]:bg-background data-[state=active]:shadow-mini rounded-7px px-4 py-2 text-sm font-medium transition-all">
			{t('dlq.all', undefined, $translations)} ({entries.length})
		</Tabs.Trigger>
		<Tabs.Trigger value="by-type" class="data-[state=active]:bg-background data-[state=active]:shadow-mini rounded-7px px-4 py-2 text-sm font-medium transition-all">
			{t('dlq.by_type', undefined, $translations)} ({byTaskType.length})
		</Tabs.Trigger>
	</Tabs.List>

	<Tabs.Content value="all" class="mt-4">
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
			emptyMessage={t('dlq.no_entries', undefined, $translations)}
			getRowId={(row) => (row as Record<string, unknown>).id as string}
		/>
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
									<p class="text-muted-foreground text-[10px]">{t('dlq.retries', undefined, $translations)}: {entry.retry_count} | {formatDate(entry.failed_at)}</p>
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
	</Tabs.Content>
</Tabs.Root>

{#if confirmRetryEntry}
	<ConfirmModal
		bind:open={confirmRetryOpen}
		title={t('dlq.retry_title', undefined, $translations)}
		consequence={t('dlq.retry_confirm', undefined, $translations).replace('{task_type}', confirmRetryEntry.task_type)}
		confirmText={t('dlq.retry_confirm_text', undefined, $translations)}
		onConfirm={() => { if (confirmRetryEntry) retryTask(confirmRetryEntry.task_id); }}
		onCancel={() => { confirmRetryEntry = null; }}
	/>
{/if}

{#if confirmDiscardEntry}
	<ConfirmModal
		bind:open={confirmDiscardOpen}
		title={t('dlq.discard_title', undefined, $translations)}
		consequence={t('dlq.discard_confirm', undefined, $translations).replace('{task_type}', confirmDiscardEntry.task_type).replace('{task_id}', confirmDiscardEntry.task_id)}
		confirmText={t('dlq.discard_confirm_text', undefined, $translations)}
		onConfirm={() => { if (confirmDiscardEntry) discardTask(confirmDiscardEntry.task_id); }}
		onCancel={() => { confirmDiscardEntry = null; }}
	/>
{/if}

{#if confirmDiscardAll}
	<ConfirmModal
		bind:open={confirmDiscardAllOpen}
		title={t('dlq.discard_all_title', undefined, $translations)}
		consequence={t('dlq.discard_all_confirm', undefined, $translations).replace('{count}', String(entries.length))}
		confirmText={t('dlq.discard_all_confirm_text', undefined, $translations)}
		onConfirm={discardAll}
		onCancel={() => { confirmDiscardAll = false; }}
	/>
{/if}

{#if confirmBulkDiscardOpen}
	<ConfirmModal
		bind:open={confirmBulkDiscardOpen}
		title={t('dlq.discard_selected_title', undefined, $translations)}
		consequence={t('dlq.discard_selected_confirm', undefined, $translations).replace('{count}', String(pendingBulkIds.length))}
		confirmText={t('dlq.discard_selected_confirm_text', undefined, $translations)}
		onConfirm={handleBulkDiscardConfirm}
		onCancel={() => { confirmBulkDiscardOpen = false; pendingBulkIds = []; }}
	/>
{/if}
