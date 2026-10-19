<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import MetricCard from '$lib/components/MetricCard.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import Modal from '$lib/components/ui/Modal.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import AlertBanner from '$lib/components/ui/AlertBanner.svelte';
	import Tabs from '$lib/components/ui/Tabs.svelte';
	import TabsContent from '$lib/components/ui/TabsContent.svelte';
	import { api } from '$lib/api';
	import type { QueueInfo, QueueStats, QueueTask, QueueTaskState, QueueTasksResponse } from '$lib/api/types';
	import { t } from '$lib/stores/i18n.svelte';
	import { ListTodo, Clock, RefreshCw, AlertTriangle, CheckCircle, Timer, RotateCw, RotateCcw, Archive, Trash2, Play } from 'lucide-svelte';

	let {} = $props();

	let queueStats = $state<QueueStats | null>(null);
	let loading = $state(true);
	let apiAvailable = $state(true);
	let lastRefresh = $state(new Date());
	let selectedQueue = $state<QueueInfo | null>(null);
	let queueTasks = $state<QueueTasksResponse | null>(null);
	let queueTasksLoading = $state(false);
	let queueTasksAbort = $state<AbortController | null>(null);
	let refreshing = $state(false);
	let secondsAgo = $state(0);
	let activeTab = $state('overview');
	let taskTab = $state<QueueTaskState>('pending');
	let taskBusy = $state('');
	let pendingTaskDelete = $state<QueueTask | null>(null);
	let taskDeleteOpen = $state(false);
	let successMessage = $state('');

	interface QueueResponse {
		queues?: QueueInfo[];
		total_pending: number;
		total_active: number;
		total_completed_24h: number;
		total_failed_24h: number;
		dlq_count: number;
	}

	function showSuccess(msg: string) {
		successMessage = msg;
		setTimeout(() => { successMessage = ''; }, 4000);
	}

	async function refresh(signal?: AbortSignal) {
		refreshing = true;
		try {
			const result = await api<QueueResponse | { message: string }>('/control/queue/stats', { signal });
			if (signal?.aborted) return;
			if (result.ok) {
				const raw = result.data;
				if (raw && 'message' in raw && raw.message) {
					queueStats = { queues: [], total_pending: 0, total_active: 0, total_completed_24h: 0, total_failed_24h: 0, dlq_count: 0 };
				} else {
					const queueData = (raw ?? {}) as QueueResponse;
					queueStats = { queues: queueData.queues ?? [], total_pending: queueData.total_pending ?? 0, total_active: queueData.total_active ?? 0, total_completed_24h: queueData.total_completed_24h ?? 0, total_failed_24h: queueData.total_failed_24h ?? 0, dlq_count: queueData.dlq_count ?? 0 };
				}
				apiAvailable = true;
				lastRefresh = new Date();
				secondsAgo = 0;
				// Keep the open detail modal in sync with the poll — a
				// monitor that reports fresh stats but stale task rows is
				// worse than no auto-refresh at all.
				if (selectedQueue) {
					const fresh = queueStats.queues.find((q) => q.name === selectedQueue!.name);
					if (fresh) selectedQueue = fresh;
					void loadQueueTasks(selectedQueue.name, { quiet: true });
				}
			} else {
				apiAvailable = false;
			}
		} catch (err) {
			// Don't mark the API as unavailable when the request was
			// aborted (e.g. initial timeout or page backgrounding).
			if (err instanceof DOMException && err.name === 'AbortError') return;
			apiAvailable = false;
		} finally {
			loading = false;
			refreshing = false;
		}
	}

	function queueStatus(q: QueueInfo): 'healthy' | 'warning' | 'critical' {
		const failRate = q.completed_24h + q.failed_24h > 0
			? q.failed_24h / (q.completed_24h + q.failed_24h)
			: 0;
		if (failRate > 0.1 || q.pending > 1000) return 'critical';
		if (failRate > 0.03 || q.pending > 200 || q.latency_ms > 5000) return 'warning';
		return 'healthy';
	}

	function queueStatusLabel(q: QueueInfo): string {
		const s = queueStatus(q);
		return t(`queue.${s === 'warning' ? 'elevated' : s}`, s);
	}

	function formatNumber(n: number): string {
		if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
		if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K';
		return n.toLocaleString();
	}

	function formatLatency(ms: number): string {
		const rounded = Math.round(ms);
		if (rounded >= 1000) return (ms / 1000).toFixed(1) + 's';
		return rounded + 'ms';
	}

	// loadQueueTasks fetches the per-state task preview for a queue.
	// quiet skips the loading skeleton (poll-driven reloads keep showing
	// the previous data until the new payload lands); resetTab returns
	// the task-state tab to pending (used on first open only).
	async function loadQueueTasks(queueName: string, opts: { quiet?: boolean; resetTab?: boolean } = {}) {
		queueTasksAbort?.abort();
		const controller = new AbortController();
		queueTasksAbort = controller;
		if (!opts.quiet) {
			queueTasksLoading = true;
			queueTasks = null;
		}
		if (opts.resetTab) taskTab = 'pending';
		try {
			const result = await api<QueueTasksResponse>(`/control/queue/${encodeURIComponent(queueName)}/tasks`, { signal: controller.signal });
			if (controller.signal.aborted) return;
			if (result.ok) {
				queueTasks = result.data;
			} else if (!opts.quiet) {
				queueTasks = null;
			}
		} catch {
			// silent — aborted or network blip; quiet polls keep last data
		} finally {
			if (!controller.signal.aborted) {
				queueTasksLoading = false;
			}
		}
	}

	async function openQueueDetail(q: QueueInfo) {
		selectedQueue = q;
		await loadQueueTasks(q.name, { resetTab: true });
	}

	function closeQueueDetail() {
		queueTasksAbort?.abort();
		selectedQueue = null;
		queueTasks = null;
	}

	function taskCount(state: QueueTaskState): number {
		return queueTasks?.counts?.[state] ?? queueTasks?.[state]?.length ?? 0;
	}

	function currentTasks(): QueueTask[] {
		return queueTasks?.[taskTab] ?? [];
	}

	// Per-state task actions surfaced in the modal:
	//   retry  → RunTask — retry/archived/scheduled back to pending
	//   archive → ArchiveTask — pending/scheduled/retry into the DLQ
	//   delete → DeleteTask — permanent removal of any non-active task
	const taskSuccessKeys = { retry: 'queue.retry_success', archive: 'queue.task_archived', delete: 'queue.task_deleted' } as const;

	function taskActions(state: QueueTaskState): ('retry' | 'archive' | 'delete')[] {
		switch (state) {
			case 'pending': return ['archive'];
			case 'scheduled': return ['retry', 'archive'];
			case 'retry': return ['retry', 'archive', 'delete'];
			case 'archived': return ['retry', 'delete'];
			case 'completed': return ['delete'];
			default: return [];
		}
	}

	function taskActionLabel(action: 'retry' | 'archive' | 'delete', state: QueueTaskState): string {
		if (action === 'retry') return state === 'scheduled' ? t('queue.run_now', 'Run now') : t('queue.retry', 'Retry');
		if (action === 'archive') return t('queue.archive_task', 'Archive');
		return t('queue.delete_task', 'Remove');
	}

	async function runTaskAction(task: QueueTask, action: 'retry' | 'archive' | 'delete') {
		if (!selectedQueue || !task.id || taskBusy) return;
		const q = encodeURIComponent(selectedQueue.name);
		const id = encodeURIComponent(task.id);
		taskBusy = `${task.id}:${action}`;
		try {
			const result = action === 'delete'
				? await api(`/control/queue/${q}/tasks/${id}`, { method: 'DELETE' })
				: await api(`/control/queue/${q}/tasks/${id}/${action}`, { method: 'POST' });
			if (result.ok) {
				showSuccess(t(taskSuccessKeys[action], 'Done'));
				await loadQueueTasks(selectedQueue.name, { quiet: true });
				await refresh();
			} else {
				bulkError = t('queue.retry_failed', 'Task action failed: {error}').replace('{error}', result.error);
			}
		} finally {
			taskBusy = '';
		}
	}

	function taskAction(task: QueueTask, action: 'retry' | 'archive' | 'delete') {
		if (action === 'delete') {
			pendingTaskDelete = task;
			taskDeleteOpen = true;
			return;
		}
		void runTaskAction(task, action);
	}

	async function confirmTaskDelete() {
		if (pendingTaskDelete) await runTaskAction(pendingTaskDelete, 'delete');
		pendingTaskDelete = null;
	}

	// Bulk queue ops: archive (clear → DLQ, recoverable), requeue (run all
	// back to pending), purge (permanently delete archived). queueName='all'
	// hits every managed queue; otherwise one queue. Destructive ops
	// confirm via ConfirmModal — never window.confirm.
	let bulkBusy = $state('');
	let bulkError = $state('');
	let pendingBulk = $state<{ queue: string; action: 'archive' | 'requeue' | 'purge' } | null>(null);
	let bulkConfirmOpen = $state(false);

	const bulkConfirmDefaults: Record<string, string> = {
		archive: 'Archive all pending, scheduled, and retry tasks in {queue}? They move to the dead-letter queue and can be requeued.',
		requeue: 'Requeue all scheduled, retry, and archived tasks in {queue} for immediate processing?',
		purge: 'Permanently delete all archived tasks in {queue}? This cannot be undone.'
	};

	function queueBulkAction(queueName: string, action: 'archive' | 'requeue' | 'purge') {
		if (bulkBusy) return;
		pendingBulk = { queue: queueName, action };
		bulkConfirmOpen = true;
	}

	async function runBulkAction() {
		if (!pendingBulk) return;
		const { queue, action } = pendingBulk;
		bulkBusy = `${queue}:${action}`;
		bulkError = '';
		try {
			// Purge is irreversible — the API requires an explicit confirm flag
			// in the body, on top of this dialog.
			const result = await api<{ archived?: number; requeued?: number; purged?: number; errors?: string[] }>(`/control/queue/${encodeURIComponent(queue)}/${action}`, {
				method: 'POST',
				...(action === 'purge' ? { body: { confirm: true } } : {})
			});
			if (result.ok) {
				// Partial failures ride the 200 payload — destructive ops must
				// never report clean success while a queue errored.
				const partial = result.data?.errors;
				if (Array.isArray(partial) && partial.length > 0) {
					bulkError = t('queue.bulk_failed', 'Queue operation failed: {error}').replace(
						'{error}',
						partial.join('; ')
					);
				} else {
					const count = result.data?.[action === 'archive' ? 'archived' : action === 'requeue' ? 'requeued' : 'purged'] ?? 0;
					showSuccess(
						t(`queue.${action === 'archive' ? 'archived' : action === 'requeue' ? 'requeued' : 'purged'}_success`, '{count} tasks affected')
							.replace('{count}', formatNumber(count))
					);
				}
				await refresh();
				if (selectedQueue) await loadQueueTasks(selectedQueue.name, { quiet: true });
			} else {
				bulkError = t('queue.bulk_failed', 'Queue operation failed: {error}').replace(
					'{error}',
					result.error
				);
			}
		} catch (e) {
			bulkError = t('queue.bulk_failed', 'Queue operation failed: {error}').replace(
				'{error}',
				e instanceof Error ? e.message : String(e)
			);
		} finally {
			bulkBusy = '';
		}
	}

	function formatTaskTime(ts: string | undefined): string {
		if (!ts) return '—';
		try {
			return new Date(ts).toLocaleString();
		} catch {
			return ts;
		}
	}

	const taskTabs: { key: QueueTaskState; icon: typeof Timer }[] = [
		{ key: 'pending', icon: Timer },
		{ key: 'active', icon: RotateCw },
		{ key: 'scheduled', icon: Clock },
		{ key: 'retry', icon: AlertTriangle },
		{ key: 'archived', icon: Archive },
		{ key: 'completed', icon: CheckCircle }
	];

	$effect(() => {
		const controller = new AbortController();
		const timeout = setTimeout(() => controller.abort(), 15000);
		refresh(controller.signal);

		const interval = setInterval(() => {
			if (document.visibilityState !== 'visible') return;
			// Each tick gets its own abort + timeout — a hung request must
			// not stack or stall the poll loop.
			const c = new AbortController();
			const t = setTimeout(() => c.abort(), 15000);
			refresh(c.signal).finally(() => clearTimeout(t));
		}, 15_000);

		const tickInterval = setInterval(() => {
			if (document.visibilityState === 'visible') {
				secondsAgo = Math.floor((Date.now() - lastRefresh.getTime()) / 1000);
			}
		}, 1000);

		const onVisible = () => {
			if (document.visibilityState === 'visible') refresh();
		};
		document.addEventListener('visibilitychange', onVisible);

		return () => {
			clearTimeout(timeout);
			controller.abort();
			clearInterval(interval);
			clearInterval(tickInterval);
			document.removeEventListener('visibilitychange', onVisible);
			queueTasksAbort?.abort();
		};
	});
</script>

<PageHeader title={t('queue.title')} description={t('queue.description')} />

{#if !apiAvailable}
	<AlertBanner variant="warning" message={t('queue.unavailable')} class="mb-4" />
{/if}

{#if successMessage}
	<AlertBanner variant="success" message={successMessage} class="mb-4" />
{/if}

{#if bulkError}
	<AlertBanner variant="error" message={bulkError} class="mb-4" />
{/if}

<div class="mb-4 flex flex-wrap items-center justify-between gap-3">
	<div class="flex items-center gap-3">
		<p class="text-muted-foreground text-xs">
			{t('queue.last_updated').replace('{time}', lastRefresh.toLocaleTimeString())}
			<span class="ml-2">({t('queue.seconds_ago').replace('{seconds}', String(secondsAgo))})</span>
		</p>
		<span class="bg-accent text-accent-foreground rounded-full px-2 py-0.5 text-[10px] font-medium whitespace-nowrap">
			{t('queue.auto_refresh')}
		</span>
	</div>
	<div class="flex flex-wrap items-center gap-2">
		<button
			class="border-border-input bg-background-alt hover:bg-muted text-foreground flex items-center gap-1.5 rounded-9px border px-3 py-1.5 text-xs font-medium whitespace-nowrap transition-colors disabled:opacity-50"
			disabled={bulkBusy !== ''}
			onclick={() => queueBulkAction('all', 'requeue')}
		>
			<RotateCcw class="size-3" />
			{t('queue.requeue_all', 'Requeue all')}
		</button>
		<button
			class="border-border-input bg-background-alt hover:bg-muted text-foreground flex items-center gap-1.5 rounded-9px border px-3 py-1.5 text-xs font-medium whitespace-nowrap transition-colors disabled:opacity-50"
			disabled={bulkBusy !== ''}
			onclick={() => queueBulkAction('all', 'archive')}
		>
			<Archive class="size-3" />
			{t('queue.archive_all', 'Archive all')}
		</button>
		<button
			class="border-border-input bg-background-alt hover:bg-muted text-destructive flex items-center gap-1.5 rounded-9px border px-3 py-1.5 text-xs font-medium whitespace-nowrap transition-colors disabled:opacity-50"
			disabled={bulkBusy !== ''}
			onclick={() => queueBulkAction('all', 'purge')}
		>
			<Trash2 class="size-3" />
			{t('queue.purge_archived', 'Purge archived')}
		</button>
		<button
			class="border-border-input bg-background-alt hover:bg-muted text-foreground flex items-center gap-1.5 rounded-9px border px-3 py-1.5 text-xs font-medium whitespace-nowrap transition-colors disabled:opacity-50"
			disabled={refreshing}
			onclick={() => refresh()}
		>
			<RefreshCw class="size-3 {refreshing ? 'animate-spin' : ''}" />
			{t('common.refresh')}
		</button>
	</div>
</div>

<Tabs
	bind:value={activeTab}
	tabs={[
		{ value: 'overview', label: t('queue.tab_overview') },
		{ value: 'queues', label: t('queue.tab_queues').replace('{count}', String(queueStats?.queues?.length ?? 0)) }
	]}
>
	<TabsContent value="overview">
		{#if loading}
			<div class="mb-6 grid gap-3 sm:grid-cols-3 lg:grid-cols-6">
				{#each Array(6) as _}
					<div class="bg-muted h-24 animate-pulse rounded-lg"></div>
				{/each}
			</div>
		{:else}
			<div class="mb-6 grid gap-3 sm:grid-cols-3 lg:grid-cols-6">
				<MetricCard
					label={t('queue.total_queues')}
					value={queueStats?.queues?.length ?? 0}
					subtitle={t('queue.subtitle_queues')}
				/>
				<MetricCard
					label={t('queue.pending')}
					value={formatNumber(queueStats?.total_pending ?? 0)}
					subtitle={t('queue.subtitle_waiting')}
				/>
				<MetricCard
					label={t('queue.active')}
					value={formatNumber(queueStats?.total_active ?? 0)}
					subtitle={t('queue.subtitle_processing')}
				/>
				<MetricCard
					label={t('queue.completed_24h')}
					value={formatNumber(queueStats?.total_completed_24h ?? 0)}
					subtitle={t('queue.subtitle_last_24h')}
					trend="up"
				/>
				<MetricCard
					label={t('queue.failed_24h')}
					value={formatNumber(queueStats?.total_failed_24h ?? 0)}
					subtitle={t('queue.subtitle_last_24h')}
					trend={(queueStats?.total_failed_24h ?? 0) > 0 ? 'down' : 'neutral'}
				/>
				<a href="/dlq" class="block">
					<MetricCard
						label={t('queue.dlq_card', 'Dead Letter')}
						value={formatNumber(queueStats?.dlq_count ?? 0)}
						subtitle={t('queue.dlq_subtitle', 'archived tasks')}
						trend={(queueStats?.dlq_count ?? 0) > 0 ? 'down' : 'neutral'}
					/>
				</a>
			</div>
		{/if}
	</TabsContent>

	<TabsContent value="queues">
		<div class="border-border-card bg-background-alt shadow-card overflow-hidden rounded-xl">
			<div class="overflow-x-auto">
				<table class="w-full text-sm">
					<thead>
						<tr class="border-border-card border-b">
							<th class="text-muted-foreground px-4 py-3 text-left font-medium">{t('queue.name')}</th>
							<th class="text-muted-foreground px-4 py-3 text-right font-medium">{t('queue.pending_count')}</th>
							<th class="text-muted-foreground px-4 py-3 text-right font-medium">{t('queue.active_count')}</th>
							<th class="text-muted-foreground px-4 py-3 text-right font-medium">{t('queue.completed_24h')}</th>
							<th class="text-muted-foreground px-4 py-3 text-right font-medium">{t('queue.failed_24h')}</th>
							<th class="text-muted-foreground px-4 py-3 text-right font-medium">{t('queue.latency')}</th>
							<th class="text-muted-foreground px-4 py-3 text-center font-medium">{t('queue.status')}</th>
						</tr>
					</thead>
					<tbody>
						{#if queueStats?.queues && queueStats.queues.length > 0}
							{#each queueStats.queues as q (q.name)}
								<tr
									class="border-border-card hover:bg-dark-04 cursor-pointer border-t transition-colors"
									onclick={() => openQueueDetail(q)}
									onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); openQueueDetail(q); } }}
									role="button"
									tabindex="0"
									aria-label={t('queue.view_details', 'View details for {queue}').replace('{queue}', q.name)}
								>
									<td class="px-4 py-3 font-medium">
										<div class="flex items-center gap-2">
											<ListTodo class="text-muted-foreground size-3.5" />
											{q.name}
										</div>
									</td>
									<td class="px-4 py-3 text-right font-mono text-xs">
										<span class:text-tertiary={q.pending > 200} class:text-destructive={q.pending > 1000}>
											{formatNumber(q.pending)}
										</span>
									</td>
									<td class="px-4 py-3 text-right font-mono text-xs">{formatNumber(q.active)}</td>
									<td class="px-4 py-3 text-right font-mono text-xs">{formatNumber(q.completed_24h)}</td>
									<td class="px-4 py-3 text-right font-mono text-xs">
										<span class:text-destructive={q.failed_24h > 0}>
											{formatNumber(q.failed_24h)}
										</span>
									</td>
									<td class="px-4 py-3 text-right font-mono text-xs">
										<span class:text-tertiary={q.latency_ms > 5000}>
											{formatLatency(q.latency_ms)}
										</span>
									</td>
									<td class="px-4 py-3 text-center">
										<StatusBadge status={queueStatus(q)}>{queueStatusLabel(q)}</StatusBadge>
									</td>
								</tr>
							{/each}
						{:else}
							<tr>
								<td colspan="7" class="text-muted-foreground px-4 py-12 text-center">
									<ListTodo class="mx-auto size-8 opacity-40" />
									<p class="mt-3 text-sm">{t('queue.no_data')}</p>
								</td>
							</tr>
						{/if}
					</tbody>
				</table>
			</div>
		</div>
	</TabsContent>
</Tabs>

<Modal
	bind:open={() => selectedQueue !== null, (v) => { if (!v) closeQueueDetail(); }}
	title={selectedQueue?.name ?? t('queue.queue_details', 'Queue details')}
	description={t('queue.recent_jobs')}
	size="max-w-2xl"
>
	{#snippet children(close)}
			<div>
				<div class="mb-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
					<div class="border-border-card rounded-9px border p-3">
						<p class="text-muted-foreground text-[10px] font-medium uppercase">{t('queue.pending_count')}</p>
						<p class="text-foreground text-lg font-bold">{formatNumber(selectedQueue?.pending ?? 0)}</p>
					</div>
					<div class="border-border-card rounded-9px border p-3">
						<p class="text-muted-foreground text-[10px] font-medium uppercase">{t('queue.active_count')}</p>
						<p class="text-foreground text-lg font-bold">{formatNumber(selectedQueue?.active ?? 0)}</p>
					</div>
					<div class="border-border-card rounded-9px border p-3">
						<p class="text-muted-foreground text-[10px] font-medium uppercase">{t('queue.completed_24h')}</p>
						<p class="text-foreground text-lg font-bold">{formatNumber(selectedQueue?.completed_24h ?? 0)}</p>
					</div>
					<div class="border-border-card rounded-9px border p-3">
						<p class="text-muted-foreground text-[10px] font-medium uppercase">{t('queue.failed_24h')}</p>
						<p class="text-destructive text-lg font-bold">{formatNumber(selectedQueue?.failed_24h ?? 0)}</p>
					</div>
				</div>

				{#if queueTasksLoading}
					<div class="space-y-2">
						{#each Array(3) as _}
							<div class="bg-muted h-12 animate-pulse rounded-lg"></div>
						{/each}
					</div>
				{:else if queueTasks}
					<!-- Task state tabs -->
					<div class="mb-3 flex gap-1 overflow-x-auto">
						{#each taskTabs as tab (tab.key)}
							<button
								class="flex shrink-0 items-center gap-1 whitespace-nowrap rounded-7px px-2 py-1.5 text-xs font-medium transition-colors {taskTab === tab.key ? 'bg-accent text-accent-foreground' : 'text-muted-foreground hover:bg-muted'}"
								onclick={() => { taskTab = tab.key; }}
							>
								<tab.icon class="size-3" />
								{t(`queue.state_${tab.key}`, tab.key)}
								<span class="rounded-full bg-background/50 px-1.5 py-0.5 text-[10px]">{taskCount(tab.key)}</span>
							</button>
						{/each}
					</div>

					<!-- Task list -->
					{#if currentTasks().length}
						<div class="max-h-64 overflow-y-auto rounded-lg border border-border-card">
							<table class="w-full text-xs">
								<thead>
									<tr class="border-b border-border-card">
										<th class="text-muted-foreground px-3 py-2 text-left font-medium">{t('queue.task_type', 'Type')}</th>
										<th class="text-muted-foreground px-3 py-2 text-left font-medium">{t('queue.task_id', 'ID')}</th>
										<th class="text-muted-foreground px-3 py-2 text-left font-medium">{t('queue.payload', 'Payload')}</th>
										{#if taskTab === 'retry' || taskTab === 'archived'}
											<th class="text-muted-foreground px-3 py-2 text-left font-medium">{t('queue.error', 'Error')}</th>
											<th class="text-muted-foreground px-3 py-2 text-left font-medium">{t('queue.last_failed', 'Last Failed')}</th>
										{/if}
										{#if taskTab === 'scheduled'}
											<th class="text-muted-foreground px-3 py-2 text-left font-medium">{t('queue.next_run', 'Next Run')}</th>
										{/if}
										{#if taskTab === 'retry' || taskTab === 'archived'}
											<th class="text-muted-foreground px-3 py-2 text-right font-medium">{t('queue.retries', 'Retries')}</th>
										{/if}
										{#if taskActions(taskTab).length}
											<th class="text-muted-foreground px-3 py-2 text-right font-medium">{t('queue.action', 'Action')}</th>
										{/if}
									</tr>
								</thead>
								<tbody>
									{#each currentTasks() as task, i (task.id || `${task.type}-${i}`)}
										<tr class="border-t border-border-card">
											<td class="max-w-[140px] truncate px-3 py-2 font-mono">
												{task.type || '—'}
												{#if task.is_orphaned}
													<span class="bg-tertiary/12 text-tertiary rounded-5px ml-1 px-1.5 py-0.5 text-[10px] font-medium">{t('queue.orphaned', 'Orphaned')}</span>
												{/if}
											</td>
											<td class="text-muted-foreground max-w-[120px] truncate px-3 py-2 font-mono">{task.id || '—'}</td>
											<td class="text-muted-foreground max-w-[140px] truncate px-3 py-2 font-mono" title={task.payload || undefined}>{task.payload || '—'}</td>
											{#if taskTab === 'retry' || taskTab === 'archived'}
												<td class="max-w-[200px] truncate px-3 py-2 text-destructive">{task.last_err || '—'}</td>
												<td class="text-muted-foreground px-3 py-2 whitespace-nowrap">{formatTaskTime(task.last_failed_at)}</td>
											{/if}
											{#if taskTab === 'scheduled'}
												<td class="text-muted-foreground px-3 py-2 whitespace-nowrap">{formatTaskTime(task.next_process_at)}</td>
											{/if}
											{#if taskTab === 'retry' || taskTab === 'archived'}
												<td class="text-muted-foreground px-3 py-2 text-right whitespace-nowrap">{task.retry_count}/{task.max_retry}</td>
											{/if}
											{#if taskActions(taskTab).length}
												<td class="px-3 py-2 text-right">
													<div class="flex items-center justify-end gap-1">
														{#each taskActions(taskTab) as action (action)}
															<button
																class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium whitespace-nowrap transition-colors disabled:opacity-50 {action === 'delete' ? 'text-destructive hover:bg-destructive/10' : 'text-accent-foreground hover:bg-accent'}"
																disabled={!task.id || taskBusy === `${task.id}:${action}`}
																onclick={() => taskAction(task, action)}
																aria-label={taskActionLabel(action, taskTab)}
															>
																{#if action === 'delete'}
																	<Trash2 class="size-3" />
																{:else if action === 'archive'}
																	<Archive class="size-3" />
																{:else if taskTab === 'scheduled'}
																	<Play class="size-3" />
																{:else}
																	<RotateCcw class="size-3" />
																{/if}
																{taskActionLabel(action, taskTab)}
															</button>
														{/each}
													</div>
												</td>
											{/if}
										</tr>
									{/each}
								</tbody>
							</table>
						</div>
						{#if (queueTasks?.counts?.[taskTab] ?? 0) > currentTasks().length}
							<p class="text-muted-foreground mt-2 text-[11px]">
								{t('queue.preview_note', 'Showing the first {shown} of {total} tasks')
									.replace('{shown}', String(currentTasks().length))
									.replace('{total}', formatNumber(queueTasks?.counts?.[taskTab] ?? 0))}
							</p>
						{/if}
					{:else}
						<div class="py-6 text-center">
							<p class="text-muted-foreground text-sm">{t('queue.no_tasks_in_state', 'No tasks in this state')}</p>
							{#if taskTab === 'completed'}
								<p class="text-muted-foreground mx-auto mt-1 max-w-sm text-xs">{t('queue.completed_note', 'Completed tasks are not retained — only the 24h counter tracks them.')}</p>
							{:else if taskTab === 'retry' || taskTab === 'archived'}
								<p class="text-muted-foreground mx-auto mt-1 max-w-sm text-xs">{t('queue.failed_note', 'Failed tasks appear here while retrying or once exhausted. The 24h counter tracks failure events, not live tasks.')}</p>
							{/if}
						</div>
					{/if}
				{:else}
					<div class="py-6 text-center">
						<p class="text-muted-foreground text-sm">{t('queue.failed_to_load_tasks', 'Failed to load tasks')}</p>
					</div>
				{/if}
			</div>
			<div class="border-border-card flex flex-wrap items-center justify-between gap-2 border-t px-6 py-3">
				{#if selectedQueue}
					<div class="flex flex-wrap items-center gap-2">
						<button
							class="border-border-input bg-background-alt hover:bg-muted text-foreground flex items-center gap-1.5 rounded-9px border px-3 py-1.5 text-xs font-medium whitespace-nowrap transition-colors disabled:opacity-50"
							disabled={bulkBusy !== ''}
							onclick={() => queueBulkAction(selectedQueue!.name, 'requeue')}
						>
							<RotateCcw class="size-3" />
							{t('queue.requeue_all', 'Requeue all')}
						</button>
						<button
							class="border-border-input bg-background-alt hover:bg-muted text-foreground flex items-center gap-1.5 rounded-9px border px-3 py-1.5 text-xs font-medium whitespace-nowrap transition-colors disabled:opacity-50"
							disabled={bulkBusy !== ''}
							onclick={() => queueBulkAction(selectedQueue!.name, 'archive')}
						>
							<Archive class="size-3" />
							{t('queue.archive_all', 'Archive all')}
						</button>
						<button
							class="border-border-input bg-background-alt hover:bg-muted text-destructive flex items-center gap-1.5 rounded-9px border px-3 py-1.5 text-xs font-medium whitespace-nowrap transition-colors disabled:opacity-50"
							disabled={bulkBusy !== ''}
							onclick={() => queueBulkAction(selectedQueue!.name, 'purge')}
						>
							<Trash2 class="size-3" />
							{t('queue.purge_archived', 'Purge archived')}
						</button>
					</div>
				{/if}
				<button
					type="button"
					class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-4 py-1.5 text-sm font-medium whitespace-nowrap transition-colors"
					onclick={close}
				>
					{t('common.close')}
				</button>
			</div>
	{/snippet}
</Modal>

{#if pendingBulk}
	<ConfirmModal
		bind:open={bulkConfirmOpen}
		title={t(`queue.confirm_${pendingBulk.action}_title`, 'Confirm queue operation')}
		consequence={t(`queue.confirm_${pendingBulk.action}`, bulkConfirmDefaults[pendingBulk.action]).replace('{queue}', pendingBulk.queue)}
		onConfirm={runBulkAction}
		onCancel={() => { pendingBulk = null; }}
	/>
{/if}

{#if pendingTaskDelete}
	<ConfirmModal
		bind:open={taskDeleteOpen}
		title={t('queue.delete_task_title', 'Delete task')}
		consequence={t('queue.delete_confirm', 'Permanently delete task {id}? This cannot be undone.').replace('{id}', pendingTaskDelete.id)}
		onConfirm={confirmTaskDelete}
		onCancel={() => { pendingTaskDelete = null; }}
	/>
{/if}
