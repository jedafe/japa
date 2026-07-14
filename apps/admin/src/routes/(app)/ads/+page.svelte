<script lang="ts">
	import { onMount } from 'svelte';
	import { adsAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { BarChart3, Zap, Pause, Play, StopCircle, Plus, Eye } from 'lucide-svelte';

	type Campaign = {
		id: string;
		name: string;
		status: 'active' | 'paused' | 'draft' | 'completed';
		impressions: number;
		clicks: number;
		budget: number;
		start_date: string;
		end_date: string;
		created_at: string;
	};

	type CampaignStats = {
		campaign_id: string;
		impressions: number;
		clicks: number;
		conversions: number;
		spend: number;
		ctr: number;
		cpc: number;
		daily_stats: Array<{ date: string; impressions: number; clicks: number; spend: number }>;
	};

	type ActiveBoost = {
		id: string;
		organization_id: string;
		space_id: string;
		boost_type: string;
		start_date: string;
		end_date: string;
		status: string;
	};

	let campaigns = $state<Campaign[]>([]);
	let activeBoosts = $state<ActiveBoost[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let activeTab = $state('campaigns');

	// Stats modal
	let showStatsModal = $state(false);
	let statsCampaign = $state<Campaign | null>(null);
	let stats = $state<CampaignStats | null>(null);
	let statsLoading = $state(false);

	const tabs = $derived([
		{ value: 'campaigns', label: 'Campaigns', icon: BarChart3, count: campaigns.length },
		{ value: 'boosts', label: 'Boosts', icon: Zap, count: activeBoosts.length }
	]);

	const campaignColumns = [
		{
			key: 'name',
			label: 'Campaign',
			sortable: true,
			render: (_: any, row: any) => `
				<div>
					<a href="/ads/${row.id}" class="font-medium text-gray-900 hover:underline">${row.name}</a>
					<p class="text-xs text-gray-500">${row.start_date} &mdash; ${row.end_date ?? 'ongoing'}</p>
				</div>
			`
		},
		{
			key: 'status',
			label: 'Status',
			sortable: true,
			render: (value: string) => {
				const classes: Record<string, string> = {
					active: 'bg-success-100 text-success-800 border-success-200',
					paused: 'bg-warning-100 text-warning-800 border-warning-200',
					completed: 'bg-gray-100 text-gray-800 border-gray-200',
					draft: 'bg-info-100 text-info-800 border-info-200'
				};
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes[value] || 'bg-gray-100 text-gray-800 border-gray-200'}">${value}</span>`;
			}
		},
		{
			key: 'impressions',
			label: 'Impressions',
			sortable: true,
			render: (value: number) => `<span class="font-medium">${(value ?? 0).toLocaleString()}</span>`
		},
		{
			key: 'clicks',
			label: 'Clicks',
			sortable: true,
			render: (value: number) => `<span class="font-medium">${(value ?? 0).toLocaleString()}</span>`
		},
		{
			key: 'impressions',
			label: 'CTR',
			render: (_: any, row: any) => `<span class="text-sm">${row.impressions > 0 ? ((row.clicks / row.impressions) * 100).toFixed(1) : '0.0'}%</span>`
		},
		{
			key: 'budget',
			label: 'Budget',
			sortable: true,
			render: (value: number) => `<span class="text-sm font-medium">$${(value ?? 0).toLocaleString()}</span>`
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			render: (_: any, row: any) => {
				let actions = `<button onclick="window.__viewStats('${row.id}')" class="inline-flex items-center justify-center rounded-md px-2 py-1.5 text-sm font-medium text-info-700 hover:bg-info-50" title="Stats"><svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"></path></svg></button>`;
				if (row.status !== 'completed') {
					actions += `<button onclick="window.__toggleCampaign('${row.id}')" class="inline-flex items-center justify-center rounded-md px-2 py-1.5 text-sm font-medium ${row.status === 'active' ? 'text-warning-700 hover:bg-warning-50' : 'text-success-700 hover:bg-success-50'}" title="${row.status === 'active' ? 'Pause' : 'Resume'}">${row.status === 'active' ? '<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 9v6m4-6v6m7-3a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>' : '<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z"></path><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>'}</button>`;
					actions += `<button onclick="window.__endCampaign('${row.id}')" class="inline-flex items-center justify-center rounded-md px-2 py-1.5 text-sm font-medium text-danger-700 hover:bg-danger-50" title="End"><svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 10a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1h-4a1 1 0 01-1-1v-4z"></path></svg></button>`;
				}
				actions += `<a href="/ads/${row.id}" class="inline-flex items-center justify-center rounded-md px-2 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100" title="View"><svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"></path></svg></a>`;
				return `<div class="flex items-center gap-1">${actions}</div>`;
			}
		}
	];

	const boostColumns = [
		{
			key: 'boost_type',
			label: 'Boost Type',
			sortable: true,
			render: (value: string) => `<span class="text-xs font-medium">${value}</span>`
		},
		{
			key: 'space_id',
			label: 'Space',
			render: (value: string) => `<span class="font-mono text-xs text-gray-500">${(value ?? '').slice(0, 12)}</span>`
		},
		{
			key: 'organization_id',
			label: 'Organization',
			render: (value: string) => `<span class="font-mono text-xs text-gray-500">${(value ?? '').slice(0, 12)}</span>`
		},
		{
			key: 'start_date',
			label: 'Period',
			render: (_: any, row: any) => `<span class="text-xs text-gray-500">${row.start_date} — ${row.end_date}</span>`
		},
		{
			key: 'status',
			label: 'Status',
			sortable: true,
			render: (value: string) => {
				const classes: Record<string, string> = {
					active: 'bg-success-100 text-success-800 border-success-200',
					completed: 'bg-gray-100 text-gray-800 border-gray-200'
				};
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes[value] || 'bg-gray-100 text-gray-800 border-gray-200'}">${value}</span>`;
			}
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			render: (_: any, row: any) => {
				if (row.status === 'active') {
					return `<button onclick="window.__endBoost('${row.id}')" class="inline-flex items-center justify-center rounded-md px-2 py-1.5 text-sm font-medium text-danger-700 hover:bg-danger-50" title="End Boost"><svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 10a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1h-4a1 1 0 01-1-1v-4z"></path></svg></button>`;
				}
				return '';
			}
		}
	];

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			if (activeTab === 'campaigns') {
				const result = await adsAdminApi.listCampaigns();
				if (result.ok) {
					const data = result.data as { items?: Campaign[]; campaigns?: Campaign[] } | Campaign[];
					campaigns = Array.isArray(data) ? data : (data.items ?? data.campaigns ?? []);
				} else {
					errorMessage = result.error;
				}
			} else {
				const result = await adsAdminApi.listActiveBoosts();
				if (result.ok) {
					const data = result.data as { items?: ActiveBoost[]; boosts?: ActiveBoost[] } | ActiveBoost[];
					activeBoosts = Array.isArray(data) ? data : (data.items ?? data.boosts ?? []);
				} else {
					errorMessage = result.error;
				}
			}
		} catch {
			errorMessage = t('ads.load_error');
		} finally {
			loading = false;
		}
	}

	async function toggleCampaign(campaignId: string) {
		const campaign = campaigns.find((c) => c.id === campaignId);
		if (!campaign) return;
		try {
			const result = campaign.status === 'active'
				? await adsAdminApi.pauseCampaign(campaign.id)
				: await adsAdminApi.activateCampaign(campaign.id);
			if (result.ok) {
				campaign.status = campaign.status === 'active' ? 'paused' : 'active';
				campaigns = campaigns;
			}
		} catch {
			errorMessage = t('ads.status_error');
		}
	}

	async function endCampaign(campaignId: string) {
		const campaign = campaigns.find((c) => c.id === campaignId);
		if (!campaign) return;
		try {
			const result = await adsAdminApi.endCampaign(campaign.id);
			if (result.ok) {
				campaign.status = 'completed';
				campaigns = campaigns;
			}
		} catch {
			errorMessage = t('ads.status_error');
		}
	}

	async function endBoost(boostId: string) {
		try {
			const result = await adsAdminApi.endBoost(boostId);
			if (result.ok) {
				load();
			}
		} catch {
			errorMessage = t('ads.status_error');
		}
	}

	async function viewStats(campaignId: string) {
		const campaign = campaigns.find((c) => c.id === campaignId);
		if (!campaign) return;
		statsCampaign = campaign;
		statsLoading = true;
		showStatsModal = true;
		stats = null;
		try {
			const result = await adsAdminApi.getCampaignStats(campaign.id);
			if (result.ok) {
				stats = result.data as CampaignStats;
			}
		} catch {
			errorMessage = t('ads.stats_error');
		} finally {
			statsLoading = false;
		}
	}

	// Expose functions for DataTable inline actions
	if (typeof window !== 'undefined') {
		(window as any).__toggleCampaign = toggleCampaign;
		(window as any).__endCampaign = endCampaign;
		(window as any).__endBoost = endBoost;
		(window as any).__viewStats = viewStats;
	}

	onMount(load);
</script>

<svelte:head>
	<title>Advertising — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('ads.title')}
	description={t('ads.description')}
	actions={[
		{ label: t('ads.create'), icon: Plus, href: '/ads/new' }
	]}
/>

<div class="space-y-6">
	{#if errorMessage}
		<div class="rounded-lg border border-danger-200 bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{errorMessage}
		</div>
	{/if}

	<Tabs {tabs} bind:value={activeTab} />

	{#if loading}
		<p class="py-8 text-center text-sm text-gray-500">{t('common.loading')}</p>
	{:else if activeTab === 'campaigns'}
		<Card>
			<CardContent class="p-0">
				<DataTable
					columns={campaignColumns}
					data={campaigns}
					pageSize={20}
					emptyMessage={t('ads.empty')}
				/>
			</CardContent>
		</Card>
	{:else if activeTab === 'boosts'}
		<Card>
			<CardContent class="p-0">
				<DataTable
					columns={boostColumns}
					data={activeBoosts}
					pageSize={20}
					emptyMessage={t('ads.no_boosts')}
				/>
			</CardContent>
		</Card>
	{/if}
</div>

<!-- Stats Modal -->
<Modal bind:open={showStatsModal} title={statsCampaign ? `${t('ads.stats')}: ${statsCampaign.name}` : t('ads.stats')}>
	{#if statsLoading}
		<p class="py-8 text-center text-sm text-gray-500">{t('common.loading')}</p>
	{:else if stats}
		<div class="space-y-4">
			<div class="grid grid-cols-2 gap-3">
				<div class="rounded-lg border border-gray-200 bg-gray-50 p-3">
					<p class="text-xs text-gray-500">{t('ads.impressions')}</p>
					<p class="text-lg font-bold text-gray-900">{stats.impressions.toLocaleString()}</p>
				</div>
				<div class="rounded-lg border border-gray-200 bg-gray-50 p-3">
					<p class="text-xs text-gray-500">{t('ads.clicks')}</p>
					<p class="text-lg font-bold text-gray-900">{stats.clicks.toLocaleString()}</p>
				</div>
				<div class="rounded-lg border border-gray-200 bg-gray-50 p-3">
					<p class="text-xs text-gray-500">{t('ads.conversions')}</p>
					<p class="text-lg font-bold text-gray-900">{stats.conversions.toLocaleString()}</p>
				</div>
				<div class="rounded-lg border border-gray-200 bg-gray-50 p-3">
					<p class="text-xs text-gray-500">{t('ads.spend')}</p>
					<p class="text-lg font-bold text-gray-900">${stats.spend.toLocaleString()}</p>
				</div>
				<div class="rounded-lg border border-gray-200 bg-gray-50 p-3">
					<p class="text-xs text-gray-500">{t('ads.ctr')}</p>
					<p class="text-lg font-bold text-gray-900">{stats.ctr.toFixed(2)}%</p>
				</div>
				<div class="rounded-lg border border-gray-200 bg-gray-50 p-3">
					<p class="text-xs text-gray-500">{t('ads.cpc')}</p>
					<p class="text-lg font-bold text-gray-900">${stats.cpc.toFixed(2)}</p>
				</div>
			</div>
			{#if stats.daily_stats?.length}
				<div>
					<h3 class="mb-2 text-sm font-medium text-gray-900">{t('ads.daily_breakdown')}</h3>
					<div class="max-h-48 overflow-y-auto rounded-lg border border-gray-200">
						<table class="w-full text-xs">
							<thead>
								<tr class="border-b border-gray-200 bg-gray-50">
									<th class="px-3 py-2 text-left text-gray-500">{t('ads.date')}</th>
									<th class="px-3 py-2 text-right text-gray-500">{t('ads.impressions')}</th>
									<th class="px-3 py-2 text-right text-gray-500">{t('ads.clicks')}</th>
									<th class="px-3 py-2 text-right text-gray-500">{t('ads.spend')}</th>
								</tr>
							</thead>
							<tbody>
								{#each stats.daily_stats as day}
									<tr class="border-b border-gray-200 last:border-0">
										<td class="px-3 py-2">{day.date}</td>
										<td class="px-3 py-2 text-right">{day.impressions.toLocaleString()}</td>
										<td class="px-3 py-2 text-right">{day.clicks.toLocaleString()}</td>
										<td class="px-3 py-2 text-right">${day.spend.toFixed(2)}</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				</div>
			{/if}
		</div>
	{:else}
		<p class="py-8 text-center text-sm text-gray-500">{t('common.no_data')}</p>
	{/if}
	<div class="flex justify-end pt-4">
		<Button variant="outline" onclick={() => showStatsModal = false}>{t('common.close')}</Button>
	</div>
</Modal>
