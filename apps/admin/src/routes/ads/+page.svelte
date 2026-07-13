<script lang="ts">
	import { onMount } from 'svelte';
	import { adsAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { BarChart3, Zap, Pause, Play, StopCircle, Eye } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';

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
	let activeTab = $state<'campaigns' | 'boosts'>('campaigns');

	// Stats modal
	let showStatsModal = $state(false);
	let statsCampaign = $state<Campaign | null>(null);
	let stats = $state<CampaignStats | null>(null);
	let statsLoading = $state(false);

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

	async function toggleCampaign(campaign: Campaign) {
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

	async function endCampaign(campaign: Campaign) {
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

	async function endBoost(boost: ActiveBoost) {
		try {
			const result = await adsAdminApi.endBoost(boost.id);
			if (result.ok) {
				load();
			}
		} catch {
			errorMessage = t('ads.status_error');
		}
	}

	async function viewStats(campaign: Campaign) {
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

	function statusColor(status: string) {
		switch (status) {
			case 'active': return 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400';
			case 'paused': return 'bg-amber-500/15 text-amber-700 dark:text-amber-400';
			case 'completed': return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
			default: return 'bg-blue-500/15 text-blue-700 dark:text-blue-400';
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>Advertising — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('ads.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('ads.description')}</p>

<!-- Tabs -->
<div class="mb-6 flex gap-1 rounded-lg border p-1" style="border-color: var(--color-border)">
	<button
		class="flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'campaigns' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'campaigns'; load(); }}
	>
		<BarChart3 class="h-4 w-4" />
		{t('ads.tab_campaigns')}
	</button>
	<button
		class="flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'boosts' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'boosts'; load(); }}
	>
		<Zap class="h-4 w-4" />
		{t('ads.tab_boosts')}
	</button>
</div>

<div class="mb-4 flex justify-end">
	<a
		href="/ads/new"
		class="inline-block rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
		style="background-color: var(--color-primary)"
	>
		{t('ads.create')}
	</a>
</div>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else if activeTab === 'campaigns'}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ads.campaign')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ads.status')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ads.impressions')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ads.clicks')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ads.ctr')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ads.budget')}</th>
					<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each campaigns as campaign}
					<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
						<td class="px-4 py-3">
							<a href="/ads/{campaign.id}" class="font-medium hover:underline">{campaign.name}</a>
							<p class="text-xs" style="color: var(--color-muted)">
								{campaign.start_date} &mdash; {campaign.end_date ?? 'ongoing'}
							</p>
						</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {statusColor(campaign.status)}">
								{campaign.status}
							</span>
						</td>
						<td class="px-4 py-3 font-medium">{campaign.impressions.toLocaleString()}</td>
						<td class="px-4 py-3 font-medium">{campaign.clicks.toLocaleString()}</td>
						<td class="px-4 py-3 text-xs">
							{campaign.impressions > 0 ? ((campaign.clicks / campaign.impressions) * 100).toFixed(1) : '0.0'}%
						</td>
						<td class="px-4 py-3 text-xs font-medium">
							${campaign.budget.toLocaleString()}
						</td>
						<td class="px-4 py-3">
							<div class="flex items-center gap-1">
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-info)"
									onclick={() => viewStats(campaign)}
									title={t('ads.stats')}
								>
									<BarChart3 class="h-3 w-3" />
								</button>
								{#if campaign.status !== 'completed'}
									<button
										class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
										style="color: {campaign.status === 'active' ? 'var(--color-warning)' : 'var(--color-success)'}"
										onclick={() => toggleCampaign(campaign)}
										title={campaign.status === 'active' ? t('ads.pause') : t('ads.resume')}
									>
										{#if campaign.status === 'active'}
											<Pause class="h-3 w-3" />
										{:else}
											<Play class="h-3 w-3" />
										{/if}
									</button>
									<button
										class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
										style="color: var(--color-danger)"
										onclick={() => endCampaign(campaign)}
										title={t('ads.end')}
									>
										<StopCircle class="h-3 w-3" />
									</button>
								{/if}
								<a href="/ads/{campaign.id}" class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" style="color: var(--color-info)" title={t('ads.view')}>
									<Eye class="h-3 w-3" />
								</a>
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="7" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('ads.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{:else if activeTab === 'boosts'}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ads.boost_type')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ads.space_id')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ads.org_id')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ads.period')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ads.status')}</th>
					<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each activeBoosts as boost (boost.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 text-xs font-medium">{boost.boost_type}</td>
						<td class="px-4 py-3 text-xs font-mono" style="color: var(--color-muted)">{boost.space_id.slice(0, 12)}</td>
						<td class="px-4 py-3 text-xs font-mono" style="color: var(--color-muted)">{boost.organization_id.slice(0, 12)}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{boost.start_date} — {boost.end_date}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {statusColor(boost.status)}">
								{boost.status}
							</span>
						</td>
						<td class="px-4 py-3 text-right">
							{#if boost.status === 'active'}
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-danger)"
									onclick={() => endBoost(boost)}
									title={t('ads.end_boost')}
								>
									<StopCircle class="h-3 w-3" />
								</button>
							{/if}
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="6" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('ads.no_boosts')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<!-- Stats Modal -->
<Modal bind:open={showStatsModal} title={statsCampaign ? `${t('ads.stats')}: ${statsCampaign.name}` : t('ads.stats')}>
	{#if statsLoading}
		<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
	{:else if stats}
		<div class="space-y-4">
			<div class="grid grid-cols-2 gap-3">
				<div class="rounded-md border p-3" style="border-color: var(--color-border)">
					<p class="text-xs" style="color: var(--color-muted)">{t('ads.impressions')}</p>
					<p class="text-lg font-bold">{stats.impressions.toLocaleString()}</p>
				</div>
				<div class="rounded-md border p-3" style="border-color: var(--color-border)">
					<p class="text-xs" style="color: var(--color-muted)">{t('ads.clicks')}</p>
					<p class="text-lg font-bold">{stats.clicks.toLocaleString()}</p>
				</div>
				<div class="rounded-md border p-3" style="border-color: var(--color-border)">
					<p class="text-xs" style="color: var(--color-muted)">{t('ads.conversions')}</p>
					<p class="text-lg font-bold">{stats.conversions.toLocaleString()}</p>
				</div>
				<div class="rounded-md border p-3" style="border-color: var(--color-border)">
					<p class="text-xs" style="color: var(--color-muted)">{t('ads.spend')}</p>
					<p class="text-lg font-bold">${stats.spend.toLocaleString()}</p>
				</div>
				<div class="rounded-md border p-3" style="border-color: var(--color-border)">
					<p class="text-xs" style="color: var(--color-muted)">{t('ads.ctr')}</p>
					<p class="text-lg font-bold">{stats.ctr.toFixed(2)}%</p>
				</div>
				<div class="rounded-md border p-3" style="border-color: var(--color-border)">
					<p class="text-xs" style="color: var(--color-muted)">{t('ads.cpc')}</p>
					<p class="text-lg font-bold">${stats.cpc.toFixed(2)}</p>
				</div>
			</div>
			{#if stats.daily_stats?.length}
				<div>
					<h3 class="mb-2 text-sm font-medium">{t('ads.daily_breakdown')}</h3>
					<div class="max-h-48 overflow-y-auto rounded-md border" style="border-color: var(--color-border)">
						<table class="w-full text-xs">
							<thead>
								<tr style="border-bottom: 1px solid var(--color-border)">
									<th class="px-3 py-2 text-left" style="color: var(--color-muted)">{t('ads.date')}</th>
									<th class="px-3 py-2 text-right" style="color: var(--color-muted)">{t('ads.impressions')}</th>
									<th class="px-3 py-2 text-right" style="color: var(--color-muted)">{t('ads.clicks')}</th>
									<th class="px-3 py-2 text-right" style="color: var(--color-muted)">{t('ads.spend')}</th>
								</tr>
							</thead>
							<tbody>
								{#each stats.daily_stats as day}
									<tr class="border-t" style="border-color: var(--color-border)">
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
		<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.no_data')}</p>
	{/if}
</Modal>
