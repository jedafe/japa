<script lang="ts">
	import { adsAdminApi, api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { BarChart3, Zap, Pause, Play, StopCircle, Plus, Eye, Trash2 } from 'lucide-svelte';

	type Campaign = {
		id: string;
		name: string;
		// Ref: sections/25-ad-placement.md — backend statuses are
		// pending_review | active | paused | ended | rejected (plus the legacy
		// `completed`/`draft` aliases the UI previously assumed).
		status: 'active' | 'paused' | 'draft' | 'completed' | 'ended' | 'pending_review' | 'rejected';
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

	// Ref: japaconnect-engineering/sections/03-rbac.md — the enforced node is
	// `ad.campaign.manage` (seeded in migration 000109). The previous `ads.manage`
	// was an unseeded typo that only worked for super_admin via the `*` wildcard.
	const canManageAds = $derived(hasPermission('ad.campaign.manage'));

	// Stats modal
	let showStatsModal = $state(false);
	let statsCampaign = $state<Campaign | null>(null);
	let stats = $state<CampaignStats | null>(null);
	let statsLoading = $state(false);

	// End campaign confirm
	let showEndCampaignConfirm = $state(false);
	let endCampaignTarget = $state<Campaign | null>(null);

	// End boost confirm
	let showEndBoostConfirm = $state(false);
	let endBoostTarget = $state<ActiveBoost | null>(null);
	let searchQuery = $state("");

	let selectedCampaignIds = $state<Set<string>>(new Set());
	let selectedBoostIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	const tabs = $derived([
		{ value: 'campaigns', label: t('ads.tab_campaigns', 'Campaigns'), icon: BarChart3, count: campaigns.length },
		{ value: 'boosts', label: t('ads.tab_boosts', 'Boosts'), icon: Zap, count: activeBoosts.length }
	]);

	// Ref: japaconnect-engineering/conventions/svelte-component-patterns.md —
	// status badges use semantic token classes (success/warning/info), never
	// raw hex or non-token shades like `info-100`/`info-800`.
	const statusBadge = (value: string) => {
		const classes: Record<string, string> = {
			active: 'bg-success/15 text-success border-success',
			paused: 'bg-warning/15 text-warning border-warning',
			completed: 'bg-surface text-foreground border-border',
			draft: 'bg-info/15 text-info border-info',
			ended: 'bg-surface text-foreground border-border',
			pending_review: 'bg-warning/15 text-warning border-warning',
			rejected: 'bg-danger/15 text-danger border-danger'
		};
		return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes[value] || 'bg-surface text-foreground border-border'}">${value}</span>`;
	};

	const campaignColumns = $derived([
		{
			key: 'name',
			label: t('ads.col_campaign', 'Campaign'),
			sortable: true,
			render: (_: any, row: any) => `
				<div>
					<a href="/ads/${row.id}" class="font-medium text-foreground hover:underline">${row.name}</a>
					<p class="text-xs text-muted">${row.start_date ?? ''} &mdash; ${row.end_date ?? 'ongoing'}</p>
				</div>
			`
		},
		{
			key: 'status',
			label: t('ads.col_status', 'Status'),
			sortable: true,
			render: (value: string) => statusBadge(value)
		},
		{
			key: 'impressions',
			label: t('ads.col_impressions', 'Impressions'),
			sortable: true,
			render: (value: number) => `<span class="font-medium">${(value ?? 0).toLocaleString()}</span>`
		},
		{
			key: 'clicks',
			label: t('ads.col_clicks', 'Clicks'),
			sortable: true,
			render: (value: number) => `<span class="font-medium">${(value ?? 0).toLocaleString()}</span>`
		},
		{
			key: 'ctr',
			label: t('ads.col_ctr', 'CTR'),
			render: (_: any, row: any) => `<span class="text-sm">${row.impressions > 0 ? ((row.clicks / row.impressions) * 100).toFixed(1) : '0.0'}%</span>`
		},
		{
			key: 'budget',
			label: t('ads.col_budget', 'Budget'),
			sortable: true,
			render: (value: number) => `<span class="text-sm font-medium">${(value ?? 0).toLocaleString()}</span>`
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			componentSnippet: CampaignActionsCell
		}
	]);

	const boostColumns = $derived([
		{
			key: 'boost_type',
			label: t('ads.col_boost_type', 'Boost Type'),
			sortable: true,
			render: (value: string) => `<span class="text-xs font-medium">${value}</span>`
		},
		{
			key: 'space_id',
			label: t('ads.col_space', 'Space'),
			render: (value: string) => `<span class="font-mono text-xs text-muted">${(value ?? '').slice(0, 12)}</span>`
		},
		{
			key: 'organization_id',
			label: t('ads.col_organization', 'Organization'),
			render: (value: string) => `<span class="font-mono text-xs text-muted">${(value ?? '').slice(0, 12)}</span>`
		},
		{
			key: 'start_date',
			label: t('ads.col_period', 'Period'),
			render: (_: any, row: any) => `<span class="text-xs text-muted">${row.start_date ?? ''} — ${row.end_date ?? ''}</span>`
		},
		{
			key: 'status',
			label: t('ads.col_status', 'Status'),
			sortable: true,
			render: (value: string) => statusBadge(value)
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			componentSnippet: BoostActionsCell
		}
	]);

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			if (activeTab === 'campaigns') {
				const result = await adsAdminApi.listCampaigns();
				if (result.ok) {
					// §6.2: backend may historically return null for an empty list;
					// guard so a null `data` never throws on `.items` access.
					const data = (result.data ?? []) as { items?: Campaign[]; campaigns?: Campaign[] } | Campaign[];
					let list = Array.isArray(data) ? data : (data.items ?? data.campaigns ?? []);
					if (searchQuery) {
						const q = searchQuery.toLowerCase();
						list = list.filter((c: any) => c.name?.toLowerCase().includes(q) || c.status?.toLowerCase().includes(q));
					}
					campaigns = list;
				} else {
					errorMessage = result.error;
				}
			} else {
				const result = await adsAdminApi.listActiveBoosts();
				if (result.ok) {
					const data = (result.data ?? []) as { items?: ActiveBoost[]; boosts?: ActiveBoost[] } | ActiveBoost[];
					let list = Array.isArray(data) ? data : (data.items ?? data.boosts ?? []);
					if (searchQuery) {
						const q = searchQuery.toLowerCase();
						list = list.filter((b: any) => b.boost_type?.toLowerCase().includes(q) || b.status?.toLowerCase().includes(q));
					}
					activeBoosts = list;
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

	// Ref: AGENTS.md Rule 16 — after a successful mutation, re-fetch from the
	// server instead of patching local state, so the UI never diverges from
	// truth on a partial/failed write.
	async function toggleCampaign(campaignId: string) {
		const campaign = campaigns.find((c) => c.id === campaignId);
		if (!campaign) return;
		try {
			const result = campaign.status === 'active'
				? await adsAdminApi.pauseCampaign(campaign.id)
				: await adsAdminApi.activateCampaign(campaign.id);
			if (result.ok) {
				await load();
			} else {
				errorMessage = result.error || t('ads.status_error');
			}
		} catch {
			errorMessage = t('ads.status_error');
		}
	}

	async function endCampaign(campaignId: string) {
		try {
			const result = await adsAdminApi.endCampaign(campaignId);
			if (result.ok) {
				await load();
			} else {
				errorMessage = result.error || t('ads.status_error');
			}
		} catch {
			errorMessage = t('ads.status_error');
		}
	}

	async function endBoost(boostId: string) {
		try {
			const result = await adsAdminApi.endBoost(boostId);
			if (result.ok) {
				await load();
			} else {
				errorMessage = result.error || t('ads.status_error');
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
			} else {
				errorMessage = result.error || t('ads.stats_error');
			}
		} catch {
			errorMessage = t('ads.stats_error');
		} finally {
			statsLoading = false;
		}
	}

	function confirmEndCampaign(campaign: Campaign) {
		endCampaignTarget = campaign;
		showEndCampaignConfirm = true;
	}

	function confirmEndBoost(boost: ActiveBoost) {
		endBoostTarget = boost;
		showEndBoostConfirm = true;
	}

	async function executeEndCampaign() {
		if (!endCampaignTarget) return;
		await endCampaign(endCampaignTarget.id);
		showEndCampaignConfirm = false;
		endCampaignTarget = null;
	}

	async function executeEndBoost() {
		if (!endBoostTarget) return;
		await endBoost(endBoostTarget.id);
		showEndBoostConfirm = false;
		endBoostTarget = null;
	}

	async function bulkDeleteCampaigns() {
		if (selectedCampaignIds.size === 0) return;
		bulkDeleting = true;
		try {
			for (const id of selectedCampaignIds) {
				await api(`/admin/ads/campaigns/${id}`, { method: 'DELETE' });
			}
			selectedCampaignIds = new Set();
			await load();
		} catch {
			errorMessage = t('ads.load_error');
		}
		bulkDeleting = false;
	}

	async function bulkDeleteBoosts() {
		if (selectedBoostIds.size === 0) return;
		bulkDeleting = true;
		try {
			for (const id of selectedBoostIds) {
				await api(`/admin/ads/boosts/${id}`, { method: 'DELETE' });
			}
			selectedBoostIds = new Set();
			await load();
		} catch {
			errorMessage = t('ads.load_error');
		}
		bulkDeleting = false;
	}

	// The $effect tracks `activeTab` and `searchQuery` (both read inside
	// load), so tab switches and search-box changes automatically re-fetch.
	// The DataTable `onsearch` callback only sets `searchQuery` — it must NOT
	// also call load() or the effect fires a second fetch (double-load bug).
	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('ads.title')} — JapaConnect Admin</title>
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
		<div class="rounded-lg border border-danger/30 bg-danger/10 px-4 py-3 text-sm text-danger">
			{errorMessage}
		</div>
	{/if}

	<Tabs {tabs} bind:value={activeTab} />

	{#if loading}
		<p class="py-8 text-center text-sm text-muted">{t('common.loading')}</p>
	{:else if activeTab === 'campaigns'}
		<Card>
			<CardContent class="p-0">
			<DataTable
				columns={campaignColumns}
				data={campaigns}
				pageSize={20}
				emptyMessage={t('ads.empty')}
				searchable
				onsearch={(q) => { searchQuery = q; }}
				selectable
				bind:selectedIds={selectedCampaignIds}
			>
				{#snippet bulkActions()}
					<Button variant="destructive" size="sm" onclick={bulkDeleteCampaigns} disabled={bulkDeleting || selectedCampaignIds.size === 0}>
						<Trash2 class="h-3.5 w-3.5 mr-1" />
						{bulkDeleting ? t('ads.deleting', 'Deleting...') : t('ads.delete_selected', 'Delete Selected')} ({selectedCampaignIds.size})
					</Button>
				{/snippet}
			</DataTable>
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
				searchable
				onsearch={(q) => { searchQuery = q; }}
				selectable
				bind:selectedIds={selectedBoostIds}
			>
				{#snippet bulkActions()}
					<Button variant="destructive" size="sm" onclick={bulkDeleteBoosts} disabled={bulkDeleting || selectedBoostIds.size === 0}>
						<Trash2 class="h-3.5 w-3.5 mr-1" />
						{bulkDeleting ? t('ads.deleting', 'Deleting...') : t('ads.delete_selected', 'Delete Selected')} ({selectedBoostIds.size})
					</Button>
				{/snippet}
			</DataTable>
			</CardContent>
		</Card>
	{/if}
</div>

<!-- Stats Modal -->
<Modal bind:open={showStatsModal} title={statsCampaign ? `${t('ads.stats')}: ${statsCampaign.name}` : t('ads.stats')}>
	{#if statsLoading}
		<p class="py-8 text-center text-sm text-muted">{t('common.loading')}</p>
	{:else if stats}
		<div class="space-y-4">
			<div class="grid grid-cols-2 gap-3">
				<div class="rounded-lg border border-border bg-surface p-3">
					<p class="text-xs text-muted">{t('ads.impressions')}</p>
					<p class="text-lg font-bold text-foreground">{(stats.impressions ?? 0).toLocaleString()}</p>
				</div>
				<div class="rounded-lg border border-border bg-surface p-3">
					<p class="text-xs text-muted">{t('ads.clicks')}</p>
					<p class="text-lg font-bold text-foreground">{(stats.clicks ?? 0).toLocaleString()}</p>
				</div>
				<div class="rounded-lg border border-border bg-surface p-3">
					<p class="text-xs text-muted">{t('ads.conversions')}</p>
					<p class="text-lg font-bold text-foreground">{(stats.conversions ?? 0).toLocaleString()}</p>
				</div>
				<div class="rounded-lg border border-border bg-surface p-3">
					<p class="text-xs text-muted">{t('ads.spend')}</p>
					<p class="text-lg font-bold text-foreground">{(stats.spend ?? 0).toLocaleString()}</p>
				</div>
				<div class="rounded-lg border border-border bg-surface p-3">
					<p class="text-xs text-muted">{t('ads.ctr')}</p>
					<p class="text-lg font-bold text-foreground">{(stats.ctr ?? 0).toFixed(2)}%</p>
				</div>
				<div class="rounded-lg border border-border bg-surface p-3">
					<p class="text-xs text-muted">{t('ads.cpc')}</p>
					<p class="text-lg font-bold text-foreground">{(stats.cpc ?? 0).toFixed(2)}</p>
				</div>
			</div>
			{#if stats.daily_stats?.length}
				<div>
					<h3 class="mb-2 text-sm font-medium text-foreground">{t('ads.daily_breakdown')}</h3>
					<div class="max-h-48 overflow-y-auto rounded-lg border border-border">
						<table class="w-full text-xs">
							<thead>
								<tr class="border-b border-border bg-surface">
									<th class="px-3 py-2 text-left text-muted">{t('ads.date')}</th>
									<th class="px-3 py-2 text-right text-muted">{t('ads.impressions')}</th>
									<th class="px-3 py-2 text-right text-muted">{t('ads.clicks')}</th>
									<th class="px-3 py-2 text-right text-muted">{t('ads.spend')}</th>
								</tr>
							</thead>
							<tbody>
								{#each stats.daily_stats as day}
									<tr class="border-b border-border last:border-0">
										<td class="px-3 py-2">{day.date}</td>
										<td class="px-3 py-2 text-right">{(day.impressions ?? 0).toLocaleString()}</td>
										<td class="px-3 py-2 text-right">{(day.clicks ?? 0).toLocaleString()}</td>
										<td class="px-3 py-2 text-right">{(day.spend ?? 0).toFixed(2)}</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				</div>
			{/if}
		</div>
	{:else}
		<p class="py-8 text-center text-sm text-muted">{t('common.no_data')}</p>
	{/if}
	<div class="flex justify-end pt-4">
		<Button variant="outline" onclick={() => showStatsModal = false}>{t('common.close')}</Button>
	</div>
</Modal>

<!-- End Campaign Confirm -->
<Modal bind:open={showEndCampaignConfirm} title={t('common.confirmDelete', 'Confirm End')}>
	{#if endCampaignTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('ads.end_confirm', 'End campaign')} <strong>{endCampaignTarget.name}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">{t('common.cannotUndo', 'This action cannot be undone.')}</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showEndCampaignConfirm = false}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={executeEndCampaign}>{t('ads.end', 'End')}</Button>
	</div>
</Modal>

<!-- End Boost Confirm -->
<Modal bind:open={showEndBoostConfirm} title={t('common.confirmDelete', 'Confirm End')}>
	{#if endBoostTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('ads.end_boost_confirm', 'End boost')} <strong>{endBoostTarget.boost_type}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">{t('common.cannotUndo', 'This action cannot be undone.')}</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showEndBoostConfirm = false}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={executeEndBoost}>{t('ads.end', 'End')}</Button>
	</div>
</Modal>

<!-- CampaignActionsCell component -->
{#snippet CampaignActionsCell({ value, row }: { value: any; row: Campaign })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => viewStats(row.id)} title={t('ads.stats')}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canManageAds && row.status !== 'completed' && row.status !== 'ended'}
			<Button variant="ghost" size="sm" onclick={() => toggleCampaign(row.id)} style={row.status === 'active' ? 'color: var(--color-warning)' : 'color: var(--color-success)'}>
				{#if row.status === 'active'}<Pause class="h-3.5 w-3.5" />{:else}<Play class="h-3.5 w-3.5" />{/if}
			</Button>
			<Button variant="ghost" size="sm" onclick={() => confirmEndCampaign(row)} style="color: var(--color-danger)">
				<StopCircle class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}

<!-- BoostActionsCell component -->
{#snippet BoostActionsCell({ value, row }: { value: any; row: ActiveBoost })}
	<div class="flex items-center gap-1">
		{#if canManageAds && row.status === 'active'}
			<Button variant="ghost" size="sm" onclick={() => confirmEndBoost(row)} style="color: var(--color-danger)" title={t('ads.end', 'End')}>
				<StopCircle class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
