<script lang="ts">
	import { onMount } from 'svelte';
	import { analyticsAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Users, FileText, DollarSign, Search, Building, Shield, Trash2, BarChart3 } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';

	type AnalyticsOverview = {
		total_users: number;
		active_users: number;
		total_content: number;
		total_spaces: number;
		total_revenue: number;
		recent_signups: number;
	};

	type SecurityScan = {
		id: string;
		scan_type: string;
		status: string;
		issues_found: number;
		created_at: string;
	};

	type DomainData = Record<string, unknown>;

	let overview = $state<AnalyticsOverview | null>(null);
	let securityScans = $state<SecurityScan[]>([]);
	let domainData = $state<DomainData | null>(null);
	let loading = $state(true);
	let error = $state('');
	let activeTab = $state<'overview' | 'content' | 'revenue' | 'search' | 'spaces' | 'users' | 'security'>('overview');

	// Cache flush
	let showFlushModal = $state(false);
	let flushTargets = $state('all');
	let flushing = $state(false);

	async function load() {
		loading = true;
		error = '';
		domainData = null;
		overview = null;
		securityScans = [];
		try {
			switch (activeTab) {
				case 'overview': {
					const result = await analyticsAdminApi.getOverview();
					if (result.ok) {
						overview = result.data as AnalyticsOverview;
					} else {
						error = result.error;
					}
					break;
				}
				case 'content': {
					const result = await analyticsAdminApi.getContent();
					if (result.ok) {
						domainData = result.data as DomainData;
					} else {
						error = result.error;
					}
					break;
				}
				case 'revenue': {
					const result = await analyticsAdminApi.getRevenue();
					if (result.ok) {
						domainData = result.data as DomainData;
					} else {
						error = result.error;
					}
					break;
				}
				case 'search': {
					const result = await analyticsAdminApi.getSearch();
					if (result.ok) {
						domainData = result.data as DomainData;
					} else {
						error = result.error;
					}
					break;
				}
				case 'spaces': {
					const result = await analyticsAdminApi.getSpaces();
					if (result.ok) {
						domainData = result.data as DomainData;
					} else {
						error = result.error;
					}
					break;
				}
				case 'users': {
					const result = await analyticsAdminApi.getUsers();
					if (result.ok) {
						domainData = result.data as DomainData;
					} else {
						error = result.error;
					}
					break;
				}
				case 'security': {
					const result = await analyticsAdminApi.getSecurity();
					if (result.ok) {
						securityScans = (result.data as SecurityScan[]) ?? [];
					} else {
						error = result.error;
					}
					break;
				}
			}
		} catch {
			error = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function runSecurityScan() {
		try {
			await analyticsAdminApi.runSecurityScan({});
			load();
		} catch {
			error = t('common.error');
		}
	}

	async function resolveSecurityIssue(id: string) {
		try {
			await analyticsAdminApi.resolveSecurityItem(id, {});
			load();
		} catch {
			error = t('common.error');
		}
	}

	async function flushCache() {
		flushing = true;
		try {
			await analyticsAdminApi.flushCache({ targets: flushTargets });
			showFlushModal = false;
			load();
		} catch {
			error = t('common.error');
		} finally {
			flushing = false;
		}
	}

	function renderDomainValue(key: string, value: unknown): string {
		if (typeof value === 'number') return value.toLocaleString();
		if (typeof value === 'string') return value;
		if (typeof value === 'boolean') return value ? 'Yes' : 'No';
		if (Array.isArray(value)) return `${value.length} items`;
		if (typeof value === 'object' && value !== null) return JSON.stringify(value);
		return '—';
	}

	function statusColor(status: string) {
		switch (status) {
			case 'pass': return 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400';
			case 'fail': return 'bg-red-500/15 text-red-700 dark:text-red-400';
			case 'warning': return 'bg-amber-500/15 text-amber-700 dark:text-amber-400';
			default: return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
		}
	}

	const tabs = [
		{ key: 'overview' as const, icon: BarChart3, labelKey: 'analytics.tab_overview' },
		{ key: 'content' as const, icon: FileText, labelKey: 'analytics.tab_content' },
		{ key: 'revenue' as const, icon: DollarSign, labelKey: 'analytics.tab_revenue' },
		{ key: 'search' as const, icon: Search, labelKey: 'analytics.tab_search' },
		{ key: 'spaces' as const, icon: Building, labelKey: 'analytics.tab_spaces' },
		{ key: 'users' as const, icon: Users, labelKey: 'analytics.tab_users' },
		{ key: 'security' as const, icon: Shield, labelKey: 'analytics.tab_security' },
	];

	onMount(load);
</script>

<svelte:head>
	<title>{t('analytics.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('analytics.title')}</h1>
<p class="mb-6 text-sm" style="color: var(--color-muted)">{t('analytics.description')}</p>

<!-- Tabs -->
<div class="mb-6 flex flex-wrap gap-1 rounded-lg border p-1" style="border-color: var(--color-border)">
	{#each tabs as tab}
		<button
			class="flex items-center gap-1.5 rounded-md px-3 py-2 text-xs font-medium transition-colors"
			style={activeTab === tab.key ? 'background-color: var(--color-primary); color: white' : ''}
			onclick={() => { activeTab = tab.key; load(); }}
		>
			<tab.icon class="h-3.5 w-3.5" />
			{t(tab.labelKey)}
		</button>
	{/each}
</div>

{#if error}
	<div class="mb-4 rounded-md border p-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{error}
	</div>
{/if}

<div class="mb-4 flex justify-end gap-2">
	{#if activeTab !== 'security'}
		<button
			class="flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
			style="border-color: var(--color-border)"
			onclick={() => { showFlushModal = true; }}
		>
			<Trash2 class="h-3 w-3" />
			{t('analytics.flush_cache')}
		</button>
	{/if}
	{#if activeTab === 'security'}
		<button
			class="flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium text-white transition-colors"
			style="background-color: var(--color-primary)"
			onclick={runSecurityScan}
		>
			<Shield class="h-3 w-3" />
			{t('analytics.run_scan')}
		</button>
	{/if}
	<button class="flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10" style="border-color: var(--color-border)" onclick={load}>
		<RefreshCw class="h-3 w-3" />
		{t('common.refresh')}
	</button>
</div>

{#if loading}
	<p class="text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else if activeTab === 'overview' && overview}
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
		<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
			<p class="text-sm" style="color: var(--color-muted)">{t('analytics.total_users')}</p>
			<p class="mt-1 text-2xl font-bold">{overview.total_users.toLocaleString()}</p>
		</div>
		<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
			<p class="text-sm" style="color: var(--color-muted)">{t('analytics.active_users')}</p>
			<p class="mt-1 text-2xl font-bold">{overview.active_users.toLocaleString()}</p>
		</div>
		<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
			<p class="text-sm" style="color: var(--color-muted)">{t('analytics.total_content')}</p>
			<p class="mt-1 text-2xl font-bold">{overview.total_content.toLocaleString()}</p>
		</div>
		<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
			<p class="text-sm" style="color: var(--color-muted)">{t('analytics.total_spaces')}</p>
			<p class="mt-1 text-2xl font-bold">{overview.total_spaces.toLocaleString()}</p>
		</div>
		<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
			<p class="text-sm" style="color: var(--color-muted)">{t('analytics.total_revenue')}</p>
			<p class="mt-1 text-2xl font-bold">${overview.total_revenue.toLocaleString()}</p>
		</div>
		<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
			<p class="text-sm" style="color: var(--color-muted)">{t('analytics.recent_signups')}</p>
			<p class="mt-1 text-2xl font-bold">{overview.recent_signups.toLocaleString()}</p>
		</div>
	</div>
{:else if activeTab === 'security'}
	<div class="rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium">{t('analytics.scan_type')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('analytics.status')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('analytics.issues')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('analytics.created_at')}</th>
					<th class="px-4 py-3 text-right font-medium">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each securityScans as scan (scan.id)}
					<tr style="border-bottom: 1px solid var(--color-border)">
						<td class="px-4 py-3">{scan.scan_type}</td>
						<td class="px-4 py-3">
							<span class="rounded-full px-2 py-0.5 text-xs font-medium {statusColor(scan.status)}">
								{scan.status}
							</span>
						</td>
						<td class="px-4 py-3">{scan.issues_found}</td>
						<td class="px-4 py-3">{new Date(scan.created_at).toLocaleDateString()}</td>
						<td class="px-4 py-3 text-right">
							{#if scan.status === 'fail' || scan.status === 'warning'}
								<button
									class="text-xs text-emerald-500 hover:text-emerald-700"
									onclick={() => resolveSecurityIssue(scan.id)}
								>
									{t('analytics.resolve')}
								</button>
							{/if}
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('analytics.no_scans')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{:else if domainData}
	<div class="rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('analytics.metric')}</th>
					<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('analytics.value')}</th>
				</tr>
			</thead>
			<tbody>
				{#each Object.entries(domainData) as [key, value] (key)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 text-xs font-medium">{key.replace(/_/g, ' ')}</td>
						<td class="px-4 py-3 text-xs text-right">{renderDomainValue(key, value)}</td>
					</tr>
				{:else}
					<tr>
						<td colspan="2" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('common.no_data')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<!-- Cache Flush Modal -->
<Modal bind:open={showFlushModal} title={t('analytics.flush_cache_title')} onConfirm={flushCache} confirmLabel={flushing ? t('common.saving') : t('analytics.flush')}>
	<div class="space-y-4">
		<div>
			<label for="flush-targets" class="mb-1 block text-sm font-medium">{t('analytics.flush_targets')}</label>
			<select id="flush-targets" bind:value={flushTargets} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)">
				<option value="all">{t('analytics.flush_all')}</option>
				<option value="analytics">{t('analytics.flush_analytics')}</option>
				<option value="search">{t('analytics.flush_search')}</option>
				<option value="cdn">{t('analytics.flush_cdn')}</option>
			</select>
		</div>
	</div>
</Modal>
