<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { Tabs } from 'bits-ui';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n';
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
	let messageType = $state<'success' | 'error' | ''>('');
	let showFlushConfirm = $state(false);
	let activeTab = $state('stats');
	let stats = $state<CacheStats | null>(null);
	let loadingStats = $state(false);

	$effect(() => {
		if (!stats) {
			loadStats();
		}
	});

	async function loadStats() {
		loadingStats = true;
		const result = await api<CacheStats | { message: string }>('/control/cache/stats');
		if (result.ok) {
			const raw = result.data;
			if ('message' in raw && raw.message) {
				stats = null;
			} else {
				stats = raw as CacheStats;
			}
		}
		loadingStats = false;
	}

	async function flush() {
		flushing = true;
		message = '';
		messageType = '';
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
</script>

<PageHeader title={t('cache.title')} description={t('cache.description')} />

<div class="mb-4 flex items-center justify-end">
	<button
		class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px flex items-center gap-1.5 border px-3 py-1.5 text-xs font-medium transition-colors"
		onclick={loadStats}
		disabled={loadingStats}
	>
		<RefreshCw class="size-3 {loadingStats ? 'animate-spin' : ''}" />
		{t('common.refresh')}
	</button>
</div>

<Tabs.Root bind:value={activeTab}>
	<Tabs.List class="bg-muted inline-flex gap-1 rounded-9px p-1">
		<Tabs.Trigger value="stats" class="data-[state=active]:bg-background data-[state=active]:shadow-mini rounded-7px px-4 py-2 text-sm font-medium transition-all">
			{t('cache.stats_tab')}
		</Tabs.Trigger>
		<Tabs.Trigger value="flush" class="data-[state=active]:bg-background data-[state=active]:shadow-mini rounded-7px px-4 py-2 text-sm font-medium transition-all">
			{t('cache.flush_tab')}
		</Tabs.Trigger>
	</Tabs.List>

	<Tabs.Content value="stats" class="mt-4">
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
					<table class="w-full text-sm">
						<thead>
							<tr class="border-border-card border-b">
								<th class="text-muted-foreground px-4 py-3 text-left font-medium">{t('cache.prefix')}</th>
								<th class="text-muted-foreground px-4 py-3 text-right font-medium">{t('cache.keys')}</th>
								<th class="text-muted-foreground px-4 py-3 text-right font-medium">{t('cache.memory')}</th>
								<th class="text-muted-foreground px-4 py-3 text-left font-medium">{t('cache.usage')}</th>
							</tr>
						</thead>
						<tbody>
							{#each stats.namespaces as ns}
								<tr class="border-border-card border-t">
									<td class="px-4 py-3 font-mono text-xs">{ns.prefix}</td>
									<td class="px-4 py-3 text-right font-mono text-xs">{ns.key_count.toLocaleString()}</td>
									<td class="px-4 py-3 text-right font-mono text-xs">{formatMemory(ns.memory_estimate_mb)}</td>
									<td class="px-4 py-3">
										<div class="bg-muted h-2 w-full max-w-[120px] rounded-full">
											<div
												class="bg-foreground h-full rounded-full transition-all"
												style="width: {(stats.total_memory_mb ?? 0) > 0 ? (ns.memory_estimate_mb / (stats.total_memory_mb ?? 1)) * 100 : 0}%"
											></div>
										</div>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		{:else}
			<div class="border-border-card bg-background-alt rounded-9px border p-8 text-center">
				<Database class="text-muted-foreground mx-auto size-8 opacity-40" />
				<p class="text-muted-foreground mt-3 text-sm">{t('cache.no_stats')}</p>
			</div>
		{/if}
	</Tabs.Content>

	<Tabs.Content value="flush" class="mt-4">
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
				{#if message}
					<span
						class="text-xs font-medium"
						class:text-emerald-500={messageType === 'success'}
						class:text-destructive={messageType === 'error'}
					>
						{message}
					</span>
				{/if}
			</div>
		</div>
	</Tabs.Content>
</Tabs.Root>

<ConfirmModal
	bind:open={showFlushConfirm}
	title={t('cache.flush_all')}
	consequence={t('cache.flush_description')}
	confirmText={t('cache.flush_confirm')}
	onConfirm={flush}
	onCancel={() => {}}
/>
