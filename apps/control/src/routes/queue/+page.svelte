<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import MetricCard from '$lib/components/MetricCard.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { api } from '$lib/api';
	import type { QueueInfo, QueueStats } from '$lib/api/types';
	import { t } from '$lib/stores/i18n.svelte';
	import { ListTodo, Clock, Play, CheckCircle, XCircle, RefreshCw, X, Loader2 } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data = {} as PageData } = $props();

	// svelte-ignore state_referenced_locally
	let queueStats = $state<QueueStats | null>(data.queueStats ?? null);
	let loading = $state(false);
	// svelte-ignore state_referenced_locally
	let apiAvailable = $state(data.queueStats !== null || data.error === null);
	let lastRefresh = $state(new Date());
	let selectedQueue = $state<QueueInfo | null>(null);
	let refreshing = $state(false);
	let secondsAgo = $state(0);

	interface QueueResponse {
		queues?: QueueInfo[];
		total_pending: number;
		total_active: number;
		total_completed_24h: number;
		total_failed_24h: number;
	}

	async function refresh() {
		refreshing = true;
		try {
			const result = await api<QueueResponse>('/control/queue/stats');
			if (result.ok) {
				queueStats = { queues: result.data.queues ?? [], ...result.data } as QueueStats;
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

	function formatBytes(bytes: number): string {
		if (bytes >= 1_073_741_824) return (bytes / 1_073_741_824).toFixed(1) + ' GB';
		if (bytes >= 1_048_576) return (bytes / 1_048_576).toFixed(1) + ' MB';
		if (bytes >= 1024) return (bytes / 1024).toFixed(1) + ' KB';
		return bytes + ' B';
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

	function openQueueDetail(q: QueueInfo) {
		selectedQueue = q;
	}

	function closeQueueDetail() {
		selectedQueue = null;
	}

	onMount(() => {
		const interval = setInterval(() => {
			if (document.visibilityState === 'visible') {
				refresh();
			}
		}, 30_000);

		const tickInterval = setInterval(() => {
			secondsAgo = Math.floor((Date.now() - lastRefresh.getTime()) / 1000);
		}, 1000);

		return () => {
			clearInterval(interval);
			clearInterval(tickInterval);
		};
	});
</script>

<PageHeader title={t('queue.title')} description={t('queue.description')} />

{#if !apiAvailable}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-warning); background-color: color-mix(in srgb, var(--color-warning) 8%, transparent); color: var(--color-warning)">
		{t('queue.unavailable')}
	</div>
{/if}

<!-- Refresh indicator -->
<div class="mb-4 flex items-center justify-between">
	<div class="flex items-center gap-3">
		<p class="text-xs" style="color: var(--color-muted)">
			{t('queue.last_updated').replace('{time}', lastRefresh.toLocaleTimeString())}
			<span class="ml-2">({secondsAgo}s ago)</span>
		</p>
		<span class="rounded-full px-2 py-0.5 text-[10px] font-medium" style="background-color: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)">
			{t('queue.auto_refresh')}
		</span>
	</div>
	<button
		class="flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-50"
		style="border-color: var(--color-border)"
		disabled={refreshing}
		onclick={refresh}
	>
		<RefreshCw class="h-3 w-3 {refreshing ? 'animate-spin' : ''}" />
		{t('common.refresh')}
	</button>
</div>

{#if loading}
	<!-- Loading skeletons -->
	<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
		{#each Array(5) as _}
			<div class="h-24 animate-pulse rounded-lg border" style="border-color: var(--color-border); background-color: var(--color-surface)"></div>
		{/each}
	</div>
{:else}
	<!-- Overview cards -->
	<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
		<MetricCard
			label={t('queue.total_queues')}
			value={queueStats?.queues?.length ?? 0}
			subtitle="queues"
		/>
		<MetricCard
			label={t('queue.pending')}
			value={formatNumber(queueStats?.total_pending ?? 0)}
			subtitle="waiting"
		/>
		<MetricCard
			label={t('queue.active')}
			value={formatNumber(queueStats?.total_active ?? 0)}
			subtitle="processing"
		/>
		<MetricCard
			label={t('queue.completed_24h')}
			value={formatNumber(queueStats?.total_completed_24h ?? 0)}
			subtitle="last 24h"
			trend="up"
		/>
		<MetricCard
			label={t('queue.failed_24h')}
			value={formatNumber(queueStats?.total_failed_24h ?? 0)}
			subtitle="last 24h"
			trend={(queueStats?.total_failed_24h ?? 0) > 0 ? 'down' : 'neutral'}
		/>
	</div>

	<!-- Queue list table -->
	<div class="overflow-hidden rounded-xl border" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<div class="overflow-x-auto">
			<table class="w-full text-sm">
				<thead>
					<tr style="border-bottom: 1px solid var(--color-border)">
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted)">{t('queue.name')}</th>
						<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted)">{t('queue.pending_count')}</th>
						<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted)">{t('queue.active_count')}</th>
						<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted)">{t('queue.completed_count')}</th>
						<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted)">{t('queue.failed_count')}</th>
						<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted)">{t('queue.latency')}</th>
						<th class="px-4 py-3 text-center font-medium" style="color: var(--color-muted)">Status</th>
					</tr>
				</thead>
				<tbody>
					{#if queueStats?.queues && queueStats.queues.length > 0}
						{#each queueStats.queues as q (q.name)}
							<tr
								class="cursor-pointer border-t transition-colors hover:bg-black/5 dark:hover:bg-white/5"
								style="border-color: var(--color-border)"
								onclick={() => openQueueDetail(q)}
								onkeydown={(e) => e.key === 'Enter' && openQueueDetail(q)}
								role="button"
								tabindex="0"
							>
								<td class="px-4 py-3 font-medium">
									<div class="flex items-center gap-2">
										<ListTodo class="h-3.5 w-3.5" style="color: var(--color-muted)" />
										{q.name}
									</div>
								</td>
								<td class="px-4 py-3 text-right font-mono text-xs">
									<span style:color={q.pending > 200 ? 'var(--color-warning)' : q.pending > 1000 ? 'var(--color-danger)' : undefined}>
										{formatNumber(q.pending)}
									</span>
								</td>
								<td class="px-4 py-3 text-right font-mono text-xs">{formatNumber(q.active)}</td>
								<td class="px-4 py-3 text-right font-mono text-xs">{formatNumber(q.completed_24h)}</td>
								<td class="px-4 py-3 text-right font-mono text-xs">
									<span style:color={q.failed_24h > 0 ? 'var(--color-danger)' : undefined}>
										{formatNumber(q.failed_24h)}
									</span>
								</td>
								<td class="px-4 py-3 text-right font-mono text-xs">
									<span style:color={q.latency_ms > 5000 ? 'var(--color-warning)' : undefined}>
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
							<td colspan="7" class="px-4 py-12 text-center" style="color: var(--color-muted)">
								<ListTodo class="mx-auto h-8 w-8 opacity-40" />
								<p class="mt-3 text-sm">No queue data available</p>
							</td>
						</tr>
					{/if}
				</tbody>
			</table>
		</div>
	</div>
{/if}

<!-- Queue detail modal -->
{#if selectedQueue}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="fixed inset-0 z-50 flex items-center justify-center p-4" style="background-color: rgba(0,0,0,0.5)">
		<div class="w-full max-w-2xl rounded-xl border shadow-xl" style="border-color: var(--color-border); background-color: var(--color-background)">
			<!-- Modal header -->
			<div class="flex items-center justify-between border-b px-6 py-4" style="border-color: var(--color-border)">
				<div>
					<h2 class="text-lg font-semibold">{selectedQueue.name}</h2>
					<p class="text-xs" style="color: var(--color-muted)">{t('queue.recent_jobs')}</p>
				</div>
				<button
					class="rounded-md p-1.5 transition-colors hover:bg-black/5 dark:hover:bg-white/10"
					onclick={closeQueueDetail}
				>
					<X class="h-4 w-4" />
				</button>
			</div>

			<!-- Modal body -->
			<div class="px-6 py-4">
				<!-- Queue stats summary -->
				<div class="mb-4 grid grid-cols-4 gap-3">
					<div class="rounded-lg border p-3" style="border-color: var(--color-border)">
						<p class="text-[10px] font-medium uppercase" style="color: var(--color-muted)">{t('queue.pending')}</p>
						<p class="text-lg font-bold">{formatNumber(selectedQueue.pending)}</p>
					</div>
					<div class="rounded-lg border p-3" style="border-color: var(--color-border)">
						<p class="text-[10px] font-medium uppercase" style="color: var(--color-muted)">{t('queue.active')}</p>
						<p class="text-lg font-bold">{formatNumber(selectedQueue.active)}</p>
					</div>
					<div class="rounded-lg border p-3" style="border-color: var(--color-border)">
						<p class="text-[10px] font-medium uppercase" style="color: var(--color-muted)">{t('queue.completed')}</p>
						<p class="text-lg font-bold">{formatNumber(selectedQueue.completed_24h)}</p>
					</div>
					<div class="rounded-lg border p-3" style="border-color: var(--color-border)">
						<p class="text-[10px] font-medium uppercase" style="color: var(--color-muted)">{t('queue.failed_24h')}</p>
						<p class="text-lg font-bold" style:color={selectedQueue.failed_24h > 0 ? 'var(--color-danger)' : undefined}>{formatNumber(selectedQueue.failed_24h)}</p>
					</div>
				</div>

				<!-- Placeholder for recent jobs list -->
				<div class="rounded-lg border p-6 text-center" style="border-color: var(--color-border); background-color: var(--color-surface)">
					<Clock class="mx-auto h-6 w-6 opacity-40" style="color: var(--color-muted)" />
					<p class="mt-2 text-sm" style="color: var(--color-muted)">{t('queue.no_jobs')}</p>
					<p class="mt-1 text-xs" style="color: var(--color-muted)">Job-level detail requires the backend queue inspection endpoint.</p>
				</div>
			</div>

			<!-- Modal footer -->
			<div class="flex justify-end border-t px-6 py-3" style="border-color: var(--color-border)">
				<button
					class="rounded-md border px-4 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
					style="border-color: var(--color-border)"
					onclick={closeQueueDetail}
				>
					Close
				</button>
			</div>
		</div>
	</div>
{/if}
