<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

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

	let overview = $state<AnalyticsOverview | null>(null);
	let securityScans = $state<SecurityScan[]>([]);
	let loading = $state(true);
	let error = $state('');
	let activeTab = $state<'overview' | 'security'>('overview');

	onMount(load);

	async function load() {
		loading = true;
		error = '';
		try {
			if (activeTab === 'overview') {
				const result = await api<AnalyticsOverview>('/admin/analytics/overview');
				if (result.ok) {
					overview = result.data;
				} else {
					error = result.error;
				}
			} else {
				const result = await api<{ data: SecurityScan[] }>('/admin/analytics/security');
				if (result.ok) {
					securityScans = result.data.data ?? [];
				} else {
					error = result.error;
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
			await api('/admin/analytics/security/scan', { method: 'POST' });
			load();
		} catch {
			error = t('common.error');
		}
	}

	async function resolveSecurityIssue(id: string) {
		try {
			await api(`/admin/analytics/security/${id}/resolve`, { method: 'POST' });
			load();
		} catch {
			error = t('common.error');
		}
	}

	function statusColor(status: string) {
		switch (status) {
			case 'pass': return 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400';
			case 'fail': return 'bg-red-500/15 text-red-700 dark:text-red-400';
			case 'warning': return 'bg-amber-500/15 text-amber-700 dark:text-amber-400';
			default: return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
		}
	}
</script>

<svelte:head>
	<title>{t('analytics.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('analytics.title')}</h1>
<p class="mb-6 text-sm" style="color: var(--color-muted)">{t('analytics.description')}</p>

<!-- Tabs -->
<div class="mb-6 flex gap-1 rounded-lg border p-1" style="border-color: var(--color-border)">
	<button
		class="rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'overview' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'overview'; load(); }}
	>
		{t('analytics.overview')}
	</button>
	<button
		class="rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'security' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'security'; load(); }}
	>
		{t('analytics.security')}
	</button>
</div>

{#if error}
	<div class="mb-4 rounded-md border p-3 text-sm" style="border-color: var(--color-danger); background-color: var(--color-danger-bg)">
		{error}
	</div>
{/if}

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
	<div class="mb-4 flex justify-end">
		<button
			class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
			style="background-color: var(--color-primary)"
			onclick={runSecurityScan}
		>
			{t('analytics.run_scan')}
		</button>
	</div>

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
{/if}
