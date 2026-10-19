<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import AlertBanner from '$lib/components/ui/AlertBanner.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import Tabs from '$lib/components/ui/Tabs.svelte';
	import TabsContent from '$lib/components/ui/TabsContent.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { escapeHtml } from '$lib/utils';
	import type { DataTableColumn } from '$lib/api/types';
	import { Database, RefreshCw } from 'lucide-svelte';

	interface CacheNamespace {
		prefix: string;
		key_count: number;
		memory_estimate_mb: number;
	}

	interface CacheStats {
		namespaces: CacheNamespace[];
		total_keys: number;
		total_memory_mb: number;
	}

	let flushing = $state(false);
	let message = $state('');
	let messageType = $state<'success' | 'error' | 'info'>('info');
	let showFlushConfirm = $state(false);
	let activeTab = $state('stats');
	let stats = $state<CacheStats | null>(null);
	let loadingStats = $state(false);
	let lastRefresh = $state('');

	// H1: Add real polling interval for live cache dashboard updates.
	// The previous $effect was a one-shot fetch with an empty cleanup return.
	$effect(() => {
		loadStats();
		// Poll quietly — re-rendering the whole stats view as "Loading..."
		// every 30s flashes the UI.
		const interval = setInterval(() => loadStats(true), 30_000);
		return () => clearInterval(interval);
	});

	async function loadStats(quiet = false) {
		if (!quiet) loadingStats = true;
		try {
			const result = await api<CacheStats | { message: string }>('/control/cache/stats');
			if (result.ok) {
				const raw = result.data;
				if ('message' in raw && raw.message) {
					stats = null;
				} else {
					stats = raw as CacheStats;
				}
				lastRefresh = new Date().toLocaleTimeString();
			}
		} catch {
			// Stats unavailable — leave stats as null
		} finally {
			loadingStats = false;
		}
	}

	async function flush() {
		flushing = true;
		message = '';
		messageType = 'info';
		try {
			const result = await api('/control/cache/flush', { method: 'POST' });
			if (result.ok) {
				message = t('cache.flushed');
				messageType = 'success';
				await loadStats();
			} else {
				message = result.error || t('cache.flush_error');
				messageType = 'error';
			}
		} catch {
			message = t('cache.flush_error');
			messageType = 'error';
		} finally {
			flushing = false;
		}
	}

	function formatMemory(mb: number): string {
		if (mb >= 1024) return (mb / 1024).toFixed(1) + ' GB';
		return mb.toFixed(1) + ' MB';
	}

	const namespaceColumns: DataTableColumn[] = [
		{
			key: 'prefix',
			label: t('cache.prefix', 'Namespace'),
			sortable: true,
			render: (val: unknown) => `<span class="font-mono text-xs">${escapeHtml(String(val))}</span>`
		},
		{
			key: 'key_count',
			label: t('cache.keys', 'Keys'),
			sortable: true,
			class: 'text-right',
			render: (val: unknown) => `<span class="font-mono text-xs text-right block">${Number(val).toLocaleString()}</span>`
		},
		{
			key: 'memory_estimate_mb',
			label: t('cache.memory', 'Memory'),
			sortable: true,
			class: 'text-right',
			render: (val: unknown) => `<span class="font-mono text-xs text-right block">${formatMemory(Number(val))}</span>`
		},
		{
			key: 'memory_estimate_mb',
			label: t('cache.usage', 'Usage'),
			render: (_val: unknown, row: Record<string, unknown>) => {
				const ns = row as unknown as CacheNamespace;
				const total = stats?.total_memory_mb ?? 1;
				const pct = total > 0 ? (ns.memory_estimate_mb / total) * 100 : 0;
				return `<div class="bg-muted h-2 w-full max-w-[120px] rounded-full"><div class="bg-foreground h-full rounded-full transition-all" style="width: ${pct.toFixed(1)}%"></div></div>`;
			}
		}
	];
</script>

<PageHeader title={t('cache.title')} description={t('cache.description')} />

<div class="mb-4 flex items-center justify-between">
	<p class="text-muted-foreground text-xs font-medium">
		{#if lastRefresh}
			{t('cache.last_updated', 'Last updated: {time}').replace('{time}', lastRefresh)}
		{:else}
			{t('common.loading', 'Loading...')}
		{/if}
	</p>
	<button
		class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px flex items-center gap-1.5 border px-3 py-1.5 text-xs font-medium transition-colors"
		onclick={() => loadStats()}
		disabled={loadingStats}
	>
		<RefreshCw class="size-3 {loadingStats ? 'animate-spin' : ''}" />
		{t('common.refresh')}
	</button>
</div>

<Tabs
	bind:value={activeTab}
	tabs={[
		{ value: 'stats', label: t('cache.stats_tab') },
		{ value: 'flush', label: t('cache.flush_tab') }
	]}
>
	<TabsContent value="stats">
		{#if loadingStats}
			<p class="text-muted-foreground py-8 text-center text-sm">{t('common.loading')}</p>
		{:else if stats?.total_keys !== undefined}
			<div class="mb-6 grid gap-3 sm:grid-cols-3">
				<div class="border-border-card bg-background-alt shadow-card rounded-card p-4">
					<p class="text-muted-foreground text-xs font-medium uppercase tracking-wider">{t('cache.total_keys')}</p>
					<p class="text-foreground mt-2 text-2xl font-bold">{(stats.total_keys ?? 0).toLocaleString()}</p>
				</div>
				<div class="border-border-card bg-background-alt shadow-card rounded-card p-4">
					<p class="text-muted-foreground text-xs font-medium uppercase tracking-wider">{t('cache.total_memory')}</p>
					<p class="text-foreground mt-2 text-2xl font-bold">{formatMemory(stats.total_memory_mb ?? 0)}</p>
				</div>
				<div class="border-border-card bg-background-alt shadow-card rounded-card p-4">
					<p class="text-muted-foreground text-xs font-medium uppercase tracking-wider">{t('cache.namespaces')}</p>
					<p class="text-foreground mt-2 text-2xl font-bold">{stats?.namespaces?.length ?? 0}</p>
				</div>
			</div>

			{#if stats?.namespaces && stats.namespaces.length > 0}
				<div class="border-border-card bg-background-alt shadow-card overflow-hidden rounded-xl">
					<div class="border-border-card border-b px-4 py-3">
						<h3 class="text-foreground text-sm font-semibold">{t('cache.namespaces')}</h3>
					</div>
					<DataTable
						columns={namespaceColumns}
						rows={stats.namespaces}
						totalRows={stats.namespaces.length}
						loading={loadingStats}
						selectable={false}
						getRowId={(row) => (row as CacheNamespace).prefix}
						emptyMessage={t('cache.no_stats', 'No cache statistics available')}
					/>
				</div>
			{/if}
		{:else}
			<div class="border-border-card bg-background-alt rounded-9px border p-8 text-center">
				<Database class="text-muted-foreground mx-auto size-8 opacity-40" />
				<p class="text-muted-foreground mt-3 text-sm">{t('cache.no_stats')}</p>
			</div>
		{/if}
	</TabsContent>

	<TabsContent value="flush">
		<div class="border-border-card bg-background-alt shadow-card rounded-xl p-6">
			<h3 class="text-foreground text-sm font-semibold">{t('cache.flush_all')}</h3>
			<p class="text-muted-foreground mt-1 text-xs">{t('cache.flush_description')}</p>
			<div class="mt-4 flex items-center gap-3">
				<button
					class="border-destructive/30 bg-destructive/10 text-destructive hover:bg-destructive/20 rounded-9px border px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50"
					disabled={flushing}
					onclick={() => (showFlushConfirm = true)}
				>
					{flushing ? t('cache.flushing') : t('cache.flush_all')}
				</button>
			</div>
			{#if message}
				<div class="mt-4">
					<AlertBanner variant={messageType} {message} />
				</div>
			{/if}
		</div>
	</TabsContent>
</Tabs>

<ConfirmModal
	bind:open={showFlushConfirm}
	title={t('cache.flush_all')}
	consequence={t('cache.flush_description')}
	confirmText={t('cache.flush_confirm')}
	onConfirm={flush}
	onCancel={() => {}}
/>
