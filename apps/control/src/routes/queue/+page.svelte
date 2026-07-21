<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import MetricCard from '$lib/components/MetricCard.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { Tabs, Tooltip } from 'bits-ui';
	import { api } from '$lib/api';
	import type { QueueInfo, QueueStats, QueueTask, QueueTasksResponse } from '$lib/api/types';
	import { t } from '$lib/stores/i18n.svelte';
	import { escapeHtml } from '$lib/utils';
	import { ListTodo, Clock, RefreshCw, X, AlertTriangle, CheckCircle, Timer, RotateCw } from 'lucide-svelte';
	import { Dialog } from 'bits-ui';

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
	let taskTab = $state('pending');

	interface QueueResponse {
		queues?: QueueInfo[];
		total_pending: number;
		total_active: number;
		total_completed_24h: number;
		total_failed_24h: number;
		dlq_count: number;
	}

	async function refresh(signal?: AbortSignal) {
		refreshing = true;
		try {
			const result = await api<QueueResponse | { message: string }>('/control/queue/stats', { signal });
			if (signal?.aborted) return;
			if (result.ok) {
				const raw = result.data;
				if ('message' in raw && raw.message) {
					queueStats = { queues: [], total_pending: 0, total_active: 0, total_completed_24h: 0, total_failed_24h: 0, dlq_count: 0 };
				} else {
					const queueData = raw as QueueResponse;
					queueStats = { queues: queueData.queues ?? [], total_pending: queueData.total_pending, total_active: queueData.total_active, total_completed_24h: queueData.total_completed_24h, total_failed_24h: queueData.total_failed_24h, dlq_count: queueData.dlq_count ?? 0 };
				}
				apiAvailable = true;
				lastRefresh = new Date();
				secondsAgo = 0;
			} else {
				apiAvailable = false;
			}
		} catch {
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

	function formatNumber(n: number): string {
		if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
		if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K';
		return n.toLocaleString();
	}

	function formatLatency(ms: number): string {
		if (ms >= 1000) return (ms / 1000).toFixed(1) + 's';
		return ms + 'ms';
	}

	async function openQueueDetail(q: QueueInfo) {
		queueTasksAbort?.abort();
		const controller = new AbortController();
		queueTasksAbort = controller;
		selectedQueue = q;
		queueTasksLoading = true;
		queueTasks = null;
		taskTab = 'pending';
		try {
			const result = await api<QueueTasksResponse>(`/control/queue/${q.name}/tasks`, { signal: controller.signal });
			if (controller.signal.aborted) return;
			if (result.ok) {
				queueTasks = result.data;
			}
		} catch {
			// silent
		} finally {
			if (!controller.signal.aborted) {
				queueTasksLoading = false;
			}
		}
	}

	function taskCount(state: keyof QueueTasksResponse): number {
		return queueTasks?.[state]?.length ?? 0;
	}

	function formatTaskTime(ts: string | undefined): string {
		if (!ts) return '—';
		try {
			return new Date(ts).toLocaleString();
		} catch {
			return ts;
		}
	}

	function truncatePayload(payload: string, max = 80): string {
		if (!payload || payload === '{}') return '—';
		return payload.length > max ? payload.slice(0, max) + '…' : payload;
	}

	$effect(() => {
		const controller = new AbortController();
		const timeout = setTimeout(() => controller.abort(), 15000);
		refresh(controller.signal);

		const interval = setInterval(() => {
			if (document.visibilityState === 'visible') {
				refresh();
			}
		}, 30_000);

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
		};
	});
</script>

<PageHeader title={t('queue.title')} description={t('queue.description')} />

{#if !apiAvailable}
	<div class="border-tertiary bg-tertiary/10 text-tertiary mb-4 flex items-center gap-2 rounded-lg border px-4 py-3 text-sm">
		{t('queue.unavailable')}
	</div>
{/if}

<div class="mb-4 flex items-center justify-between">
	<div class="flex items-center gap-3">
		<p class="text-muted-foreground text-xs">
			{t('queue.last_updated').replace('{time}', lastRefresh.toLocaleTimeString())}
			<span class="ml-2">({t('queue.seconds_ago').replace('{seconds}', String(secondsAgo))})</span>
		</p>
		<span class="bg-accent text-accent-foreground rounded-full px-2 py-0.5 text-[10px] font-medium">
			{t('queue.auto_refresh')}
		</span>
	</div>
	<button
		class="border-border-input bg-background-alt hover:bg-muted text-foreground flex items-center gap-1.5 rounded-9px border px-3 py-1.5 text-xs font-medium transition-colors disabled:opacity-50"
		disabled={refreshing}
		onclick={() => refresh()}
	>
		<RefreshCw class="size-3 {refreshing ? 'animate-spin' : ''}" />
		{t('common.refresh')}
	</button>
</div>

<Tabs.Root bind:value={activeTab}>
	<Tabs.List class="bg-muted inline-flex gap-1 rounded-9px p-1">
		<Tabs.Trigger
			value="overview"
			class="data-[state=active]:bg-background data-[state=active]:shadow-mini rounded-7px px-4 py-2 text-sm font-medium transition-all"
		>
			{t('queue.tab_overview')}
		</Tabs.Trigger>
		<Tabs.Trigger
			value="queues"
			class="data-[state=active]:bg-background data-[state=active]:shadow-mini rounded-7px px-4 py-2 text-sm font-medium transition-all"
		>
			{t('queue.tab_queues').replace('{count}', String(queueStats?.queues?.length ?? 0))}
		</Tabs.Trigger>
	</Tabs.List>

	<Tabs.Content value="overview" class="mt-4">
		{#if loading}
			<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
				{#each Array(5) as _}
					<div class="bg-muted h-24 animate-pulse rounded-lg"></div>
				{/each}
			</div>
		{:else}
			<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
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
			</div>
		{/if}
	</Tabs.Content>

	<Tabs.Content value="queues" class="mt-4">
		<div class="border-border-card bg-background-alt shadow-card overflow-hidden rounded-xl">
			<div class="overflow-x-auto">
				<table class="w-full text-sm">
					<thead>
						<tr class="border-border-card border-b">
							<th class="text-muted-foreground px-4 py-3 text-left font-medium">{t('queue.name')}</th>
							<th class="text-muted-foreground px-4 py-3 text-right font-medium">{t('queue.pending_count')}</th>
							<th class="text-muted-foreground px-4 py-3 text-right font-medium">{t('queue.active_count')}</th>
							<th class="text-muted-foreground px-4 py-3 text-right font-medium">{t('queue.completed_count')}</th>
							<th class="text-muted-foreground px-4 py-3 text-right font-medium">{t('queue.failed_count')}</th>
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
									onkeydown={(e) => e.key === 'Enter' && openQueueDetail(q)}
									role="button"
									tabindex="0"
									aria-label="View details for {q.name}"
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
										<StatusBadge status={queueStatus(q)} />
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
	</Tabs.Content>
</Tabs.Root>

<Dialog.Root bind:open={() => selectedQueue !== null, (v) => { if (!v) selectedQueue = null; }}>
	<Dialog.Portal>
		<Dialog.Overlay class="bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out fixed inset-0 z-50" />
		<Dialog.Content
			class="border-border-card bg-background shadow-popover fixed left-1/2 top-1/2 z-50 w-full max-w-2xl -translate-x-1/2 -translate-y-1/2 rounded-xl border p-0"
			onOpenAutoFocus={(e) => e.preventDefault()}
		>
			<div class="border-border-card flex items-center justify-between border-b px-6 py-4">
				<div>
					<Dialog.Title class="text-foreground text-lg font-semibold">{selectedQueue?.name}</Dialog.Title>
					<p class="text-muted-foreground text-xs">{t('queue.recent_jobs')}</p>
				</div>
				<Dialog.Close class="hover:bg-dark-04 rounded-9px p-1.5 transition-colors" aria-label={t('common.close')}>
					<span class="sr-only">{t('common.close')}</span>
					<X class="size-4" />
				</Dialog.Close>
			</div>
			<div class="px-6 py-4">
				<div class="mb-4 grid grid-cols-4 gap-3">
					<div class="border-border-card rounded-9px border p-3">
						<p class="text-muted-foreground text-[10px] font-medium uppercase">{t('queue.pending')}</p>
						<p class="text-foreground text-lg font-bold">{formatNumber(selectedQueue?.pending ?? 0)}</p>
					</div>
					<div class="border-border-card rounded-9px border p-3">
						<p class="text-muted-foreground text-[10px] font-medium uppercase">{t('queue.active')}</p>
						<p class="text-foreground text-lg font-bold">{formatNumber(selectedQueue?.active ?? 0)}</p>
					</div>
					<div class="border-border-card rounded-9px border p-3">
						<p class="text-muted-foreground text-[10px] font-medium uppercase">{t('queue.completed')}</p>
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
						{#each [
							{ key: 'pending', icon: Timer, count: taskCount('pending') },
							{ key: 'active', icon: RotateCw, count: taskCount('active') },
							{ key: 'scheduled', icon: Clock, count: taskCount('scheduled') },
							{ key: 'retry', icon: AlertTriangle, count: taskCount('retry') },
							{ key: 'archived', icon: AlertTriangle, count: taskCount('archived') },
							{ key: 'completed', icon: CheckCircle, count: taskCount('completed') }
						] as tab}
							<button
								class="flex items-center gap-1 whitespace-nowrap rounded-7px px-3 py-1.5 text-xs font-medium transition-colors {taskTab === tab.key ? 'bg-accent text-accent-foreground' : 'text-muted-foreground hover:bg-muted'}"
								onclick={() => { taskTab = tab.key; }}
							>
								<tab.icon class="size-3" />
								{tab.key}
								<span class="rounded-full bg-background/50 px-1.5 py-0.5 text-[10px]">{tab.count}</span>
							</button>
						{/each}
					</div>

					<!-- Task list -->
					{#if queueTasks[taskTab as keyof QueueTasksResponse]?.length}
						<div class="max-h-64 overflow-y-auto rounded-lg border border-border-card">
							<table class="w-full text-xs">
								<thead>
									<tr class="border-b border-border-card">
										<th class="text-muted-foreground px-3 py-2 text-left font-medium">Type</th>
										<th class="text-muted-foreground px-3 py-2 text-left font-medium">ID</th>
										{#if taskTab === 'retry' || taskTab === 'archived'}
											<th class="text-muted-foreground px-3 py-2 text-left font-medium">Error</th>
										{/if}
										{#if taskTab === 'scheduled'}
											<th class="text-muted-foreground px-3 py-2 text-left font-medium">Next Run</th>
										{/if}
										{#if taskTab === 'retry' || taskTab === 'archived'}
											<th class="text-muted-foreground px-3 py-2 text-right font-medium">Retries</th>
										{/if}
									</tr>
								</thead>
								<tbody>
									{#each queueTasks[taskTab as keyof QueueTasksResponse] ?? [] as task (task.id)}
										<tr class="border-t border-border-card">
											<td class="px-3 py-2 font-mono">{task.type}</td>
											<td class="text-muted-foreground max-w-[120px] truncate px-3 py-2 font-mono">{task.id}</td>
											{#if taskTab === 'retry' || taskTab === 'archived'}
												<td class="max-w-[200px] truncate px-3 py-2 text-destructive">{escapeHtml(task.last_err) || '—'}</td>
											{/if}
											{#if taskTab === 'scheduled'}
												<td class="text-muted-foreground px-3 py-2">{formatTaskTime(task.next_process_at)}</td>
											{/if}
											{#if taskTab === 'retry' || taskTab === 'archived'}
												<td class="text-muted-foreground px-3 py-2 text-right">{task.retry_count}/{task.max_retry}</td>
											{/if}
										</tr>
									{/each}
								</tbody>
							</table>
						</div>
					{:else}
						<div class="py-6 text-center">
							<p class="text-muted-foreground text-sm">{t('queue.no_tasks_in_state', 'No tasks in this state')}</p>
						</div>
					{/if}
				{:else}
					<div class="py-6 text-center">
						<p class="text-muted-foreground text-sm">{t('queue.failed_to_load_tasks', 'Failed to load tasks')}</p>
					</div>
				{/if}
			</div>
			<div class="border-border-card flex justify-end border-t px-6 py-3">
				<Dialog.Close class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-4 py-1.5 text-sm font-medium transition-colors">
					{t('common.close')}
				</Dialog.Close>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
