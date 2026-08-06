<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { adsAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import { ArrowLeft, Pause, Play, StopCircle, BarChart3 } from 'lucide-svelte';

	// Ref: japaconnect-engineering/sections/25-ad-placement.md — backend
	// statuses are pending_review | active | paused | ended | rejected (plus
	// the legacy completed/draft aliases the UI previously assumed).
	type Campaign = {
		id: string;
		name: string;
		status: 'active' | 'paused' | 'draft' | 'completed' | 'ended' | 'pending_review' | 'rejected';
		budget?: number | null;
		spent?: number;
		priority?: number;
		frequency_cap_per_user?: number | null;
		start_date?: string | null;
		end_date?: string | null;
		target_url?: string | null;
		target_geo?: string[];
		target_space_ids?: string[];
		content?: { headline?: string; body?: string; cta?: string } | null;
		slot_type_id?: string;
		created_at?: string;
	};

	type CampaignStats = {
		campaign_id?: string;
		impressions: number;
		clicks: number;
		conversions: number;
		spend: number;
		ctr: number;
		cpc: number;
		daily_stats?: Array<{ date: string; impressions: number; clicks: number; spend: number }>;
	};

	// Ref: japaconnect-engineering/sections/03-rbac.md — enforced node is
	// `ad.campaign.manage` (seeded in migration 000109). `ad.stats.view` gates
	// the stats endpoint.
	const canManageAds = $derived(hasPermission('ad.campaign.manage'));
	const canViewStats = $derived(hasPermission('ad.stats.view'));

	const campaignId = $derived(page.params.id ?? '');

	let campaign = $state<Campaign | null>(null);
	let loading = $state(true);
	let errorMessage = $state('');
	let successMessage = $state('');

	// Edit form
	let editName = $state('');
	let editBudget = $state('');
	let saving = $state(false);

	// Stats
	let stats = $state<CampaignStats | null>(null);
	let statsLoading = $state(false);
	let showStatsSlideOver = $state(false);

	// End campaign confirm
	let showEndConfirm = $state(false);

	// Ref: japaconnect-engineering/conventions/svelte-component-patterns.md —
	// status badges use semantic token classes (success/warning/info/danger),
	// never raw hex or non-token shades.
	const statusBadgeClass = (value: string) => {
		const classes: Record<string, string> = {
			active: 'bg-success/15 text-success border-success',
			paused: 'bg-warning/15 text-warning border-warning',
			completed: 'bg-surface text-foreground border-border',
			draft: 'bg-info/15 text-info border-info',
			ended: 'bg-surface text-foreground border-border',
			pending_review: 'bg-warning/15 text-warning border-warning',
			rejected: 'bg-danger/15 text-danger border-danger'
		};
		return classes[value] ?? 'bg-surface text-foreground border-border';
	};

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await adsAdminApi.getCampaign(campaignId);
			if (result.ok) {
				campaign = result.data as Campaign;
				editName = campaign?.name ?? '';
				editBudget = campaign?.budget != null ? String(campaign.budget) : '';
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('ads.load_error', 'Failed to load campaign');
		} finally {
			loading = false;
		}
	}

	// Ref: AGENTS.md Rule 16 — after a successful mutation, re-fetch from the
	// server instead of patching local state, so the UI never diverges from
	// truth on a partial/failed write.
	async function toggleCampaignStatus() {
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

	async function endCampaign() {
		if (!campaign) return;
		try {
			const result = await adsAdminApi.endCampaign(campaign.id);
			if (result.ok) {
				showEndConfirm = false;
				await load();
			} else {
				errorMessage = result.error || t('ads.status_error');
			}
		} catch {
			errorMessage = t('ads.status_error');
		}
	}

	async function saveCampaign() {
		if (!campaign || !editName.trim()) return;
		saving = true;
		errorMessage = '';
		successMessage = '';
		try {
			const body: Record<string, unknown> = { name: editName.trim() };
			if (editBudget) body.budget = Number(editBudget);
			const result = await adsAdminApi.updateCampaign(campaign.id, body);
			if (result.ok) {
				successMessage = t('ads.updated', 'Campaign updated');
				await load();
			} else {
				errorMessage = result.error || t('ads.update_error');
			}
		} catch {
			errorMessage = t('ads.update_error', 'Failed to update campaign');
		} finally {
			saving = false;
		}
	}

	async function viewStats() {
		if (!campaign) return;
		statsLoading = true;
		showStatsSlideOver = true;
		stats = null;
		try {
			const result = await adsAdminApi.getCampaignStats(campaign.id);
			if (result.ok) {
				stats = result.data as CampaignStats;
			} else {
				errorMessage = result.error || t('ads.stats_error');
			}
		} catch {
			errorMessage = t('ads.stats_error', 'Failed to load statistics');
		} finally {
			statsLoading = false;
		}
	}

	function confirmEnd() {
		showEndConfirm = true;
	}

	$effect(() => {
		void campaignId;
		load();
	});
</script>

<svelte:head>
	<title>{campaign ? campaign.name : t('ads.details', 'Campaign Details')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={campaign ? campaign.name : t('ads.details', 'Campaign Details')}
	description={t('ads.details', 'Campaign Details')}
	actions={[
		{ label: t('ads.back', 'Back to Campaigns'), icon: ArrowLeft, href: '/ads' }
	]}
/>

{#if loading}
	<p class="py-8 text-center text-sm text-muted">{t('common.loading')}</p>
{:else if errorMessage}
	<div class="rounded-lg border border-danger/30 bg-danger/10 px-4 py-3 text-sm text-danger">
		{errorMessage}
	</div>
{:else if campaign}
	<div class="space-y-6">
		{#if successMessage}
			<div class="rounded-lg border border-success/30 bg-success/10 px-4 py-3 text-sm text-success">
				{successMessage}
			</div>
		{/if}

		<!-- Status & Actions -->
		<div class="flex flex-wrap items-center gap-3">
			<Badge class={statusBadgeClass(campaign.status)}>{campaign.status}</Badge>
			{#if canManageAds && campaign.status !== 'completed' && campaign.status !== 'ended'}
				<Button variant="outline" size="sm" onclick={toggleCampaignStatus}>
					{#if campaign.status === 'active'}
						<Pause class="h-3.5 w-3.5" />
						{t('ads.pause', 'Pause')}
					{:else}
						<Play class="h-3.5 w-3.5" />
						{t('ads.resume', 'Resume')}
					{/if}
				</Button>
				<Button variant="outline" size="sm" onclick={confirmEnd} style="color: var(--color-danger)">
					<StopCircle class="h-3.5 w-3.5" />
					{t('ads.end', 'End')}
				</Button>
			{/if}
			{#if canViewStats}
				<Button variant="outline" size="sm" onclick={viewStats}>
					<BarChart3 class="h-3.5 w-3.5" />
					{t('ads.stats', 'Statistics')}
				</Button>
			{/if}
		</div>

		<!-- Basic Info -->
		<Card>
			<CardContent class="space-y-4">
				<h2 class="text-sm font-semibold uppercase tracking-wider text-muted">{t('ads.basic_info', 'Basic Information')}</h2>
				<div class="grid grid-cols-2 gap-4">
					<div>
						<p class="text-xs text-muted">{t('ads.fields.name', 'Name')}</p>
						<p class="text-sm font-medium text-foreground">{campaign.name}</p>
					</div>
					<div>
						<p class="text-xs text-muted">{t('ads.status', 'Status')}</p>
						<p class="text-sm font-medium text-foreground">{campaign.status}</p>
					</div>
					<div>
						<p class="text-xs text-muted">{t('ads.priority', 'Priority')}</p>
						<p class="text-sm font-medium text-foreground">{campaign.priority ?? 0}</p>
					</div>
					<div>
						<p class="text-xs text-muted">{t('ads.frequency_cap', 'Frequency Cap / User')}</p>
						<p class="text-sm font-medium text-foreground">{campaign.frequency_cap_per_user ?? '—'}</p>
					</div>
					<div>
						<p class="text-xs text-muted">{t('ads.fields.start_date', 'Start Date')}</p>
						<p class="text-sm font-medium text-foreground">{campaign.start_date ?? '—'}</p>
					</div>
					<div>
						<p class="text-xs text-muted">{t('ads.fields.end_date', 'End Date')}</p>
						<p class="text-sm font-medium text-foreground">{campaign.end_date ?? '—'}</p>
					</div>
				</div>
			</CardContent>
		</Card>

		<!-- Ad Content -->
		{#if campaign.content}
			<Card>
				<CardContent class="space-y-4">
					<h2 class="text-sm font-semibold uppercase tracking-wider text-muted">{t('ads.content', 'Ad Content')}</h2>
					<div class="grid grid-cols-1 gap-3">
						{#if campaign.content.headline}
							<div>
								<p class="text-xs text-muted">{t('ads.headline', 'Headline')}</p>
								<p class="text-sm font-medium text-foreground">{campaign.content.headline}</p>
							</div>
						{/if}
						{#if campaign.content.body}
							<div>
								<p class="text-xs text-muted">{t('ads.body', 'Body Text')}</p>
								<p class="text-sm text-foreground">{campaign.content.body}</p>
							</div>
						{/if}
						{#if campaign.content.cta}
							<div>
								<p class="text-xs text-muted">{t('ads.cta', 'Call to Action')}</p>
								<p class="text-sm font-medium text-foreground">{campaign.content.cta}</p>
							</div>
						{/if}
					</div>
				</CardContent>
			</Card>
		{/if}

		<!-- Targeting -->
		<Card>
			<CardContent class="space-y-4">
				<h2 class="text-sm font-semibold uppercase tracking-wider text-muted">{t('ads.sections.targeting', 'Targeting')}</h2>
				<div class="grid grid-cols-1 gap-3">
					<div>
						<p class="text-xs text-muted">{t('ads.target_url', 'Target URL')}</p>
						<p class="text-sm font-medium text-foreground">{campaign.target_url ?? '—'}</p>
					</div>
					<div>
						<p class="text-xs text-muted">{t('ads.fields.countries', 'Target Countries')}</p>
						{#if campaign.target_geo && campaign.target_geo.length > 0}
							<div class="flex flex-wrap gap-1.5">
								{#each campaign.target_geo as code}
									<span class="inline-flex items-center rounded-full border border-border bg-surface px-2.5 py-0.5 text-xs font-medium text-foreground">{code}</span>
								{/each}
							</div>
						{:else}
							<p class="text-sm text-foreground">{t('ads.target_geo_all', 'Global (all countries)')}</p>
						{/if}
					</div>
					<div>
						<p class="text-xs text-muted">{t('ads.target_spaces', 'Target Spaces')}</p>
						{#if campaign.target_space_ids && campaign.target_space_ids.length > 0}
							<p class="text-sm text-foreground">{campaign.target_space_ids.length} space(s)</p>
						{:else}
							<p class="text-sm text-foreground">{t('ads.target_spaces_all', 'All spaces')}</p>
						{/if}
					</div>
				</div>
			</CardContent>
		</Card>

		<!-- Budget -->
		<Card>
			<CardContent class="space-y-4">
				<h2 class="text-sm font-semibold uppercase tracking-wider text-muted">{t('ads.sections.budget', 'Budget & Bidding')}</h2>
				<div class="grid grid-cols-2 gap-4">
					<div>
						<p class="text-xs text-muted">{t('ads.budget', 'Budget')}</p>
						<p class="text-sm font-medium text-foreground">{campaign.budget ?? '—'}</p>
					</div>
					<div>
						<p class="text-xs text-muted">{t('ads.budget_spent', 'Budget Spent')}</p>
						<p class="text-sm font-medium text-foreground">{campaign.spent ?? 0}</p>
					</div>
				</div>
			</CardContent>
		</Card>

		<!-- Edit (admin only) -->
		{#if canManageAds}
			<Card>
				<CardContent class="space-y-4">
					<h2 class="text-sm font-semibold uppercase tracking-wider text-muted">{t('ads.details', 'Campaign Details')}</h2>
					<div class="space-y-3">
						<div>
							<label for="edit-name" class="mb-1 block text-sm font-medium">{t('ads.campaign_name', 'Campaign Name')}</label>
							<input id="edit-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={editName} />
						</div>
						<div>
							<label for="edit-budget" class="mb-1 block text-sm font-medium">{t('ads.budget', 'Budget (credits)')}</label>
							<input id="edit-budget" type="number" min="0" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={editBudget} placeholder="1000" />
						</div>
						<div class="flex justify-end gap-2">
							<Button onclick={saveCampaign} disabled={saving || !editName.trim()}>
								{saving ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
							</Button>
						</div>
					</div>
				</CardContent>
			</Card>
		{/if}
	</div>
{/if}

<!-- Stats SlideOver (view-detail) -->
<SlideOver bind:open={showStatsSlideOver} title={campaign ? `${t('ads.stats', 'Statistics')}: ${campaign.name}` : t('ads.stats', 'Statistics')} size="lg">
	{#if statsLoading}
		<p class="py-8 text-center text-sm text-muted">{t('common.loading')}</p>
	{:else if stats}
		<div class="space-y-4">
			<div class="grid grid-cols-2 gap-3">
				<div class="rounded-lg border border-border bg-surface p-3">
					<p class="text-xs text-muted">{t('ads.impressions', 'Impressions')}</p>
					<p class="text-lg font-bold text-foreground">{(stats.impressions ?? 0).toLocaleString()}</p>
				</div>
				<div class="rounded-lg border border-border bg-surface p-3">
					<p class="text-xs text-muted">{t('ads.clicks', 'Clicks')}</p>
					<p class="text-lg font-bold text-foreground">{(stats.clicks ?? 0).toLocaleString()}</p>
				</div>
				<div class="rounded-lg border border-border bg-surface p-3">
					<p class="text-xs text-muted">{t('ads.conversions', 'Conversions')}</p>
					<p class="text-lg font-bold text-foreground">{(stats.conversions ?? 0).toLocaleString()}</p>
				</div>
				<div class="rounded-lg border border-border bg-surface p-3">
					<p class="text-xs text-muted">{t('ads.spend', 'Spend')}</p>
					<p class="text-lg font-bold text-foreground">{(stats.spend ?? 0).toLocaleString()}</p>
				</div>
				<div class="rounded-lg border border-border bg-surface p-3">
					<p class="text-xs text-muted">{t('ads.ctr', 'CTR')}</p>
					<p class="text-lg font-bold text-foreground">{(stats.ctr ?? 0).toFixed(2)}%</p>
				</div>
				<div class="rounded-lg border border-border bg-surface p-3">
					<p class="text-xs text-muted">{t('ads.cpc', 'Cost Per Click')}</p>
					<p class="text-lg font-bold text-foreground">{(stats.cpc ?? 0).toFixed(2)}</p>
				</div>
			</div>
			{#if stats.daily_stats?.length}
				<div>
					<h3 class="mb-2 text-sm font-medium text-foreground">{t('ads.daily_breakdown', 'Daily Breakdown')}</h3>
					<div class="max-h-64 overflow-y-auto rounded-lg border border-border">
						<table class="w-full text-xs">
							<thead>
								<tr class="border-b border-border bg-surface">
									<th class="px-3 py-2 text-left text-muted">{t('ads.date', 'Date')}</th>
									<th class="px-3 py-2 text-right text-muted">{t('ads.impressions', 'Impressions')}</th>
									<th class="px-3 py-2 text-right text-muted">{t('ads.clicks', 'Clicks')}</th>
									<th class="px-3 py-2 text-right text-muted">{t('ads.spend', 'Spend')}</th>
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
		<p class="py-8 text-center text-sm text-muted">{t('common.no_data', 'No data available')}</p>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end">
			<Button variant="outline" onclick={() => (showStatsSlideOver = false)}>{t('common.close', 'Close')}</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- End Campaign Confirm -->
<Modal bind:open={showEndConfirm} title={t('common.confirmDelete', 'Confirm End')}>
	{#if campaign}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('ads.end_confirm', 'End campaign')} <strong>{campaign.name}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">{t('common.cannotUndo', 'This action cannot be undone.')}</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showEndConfirm = false}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={endCampaign}>{t('ads.end', 'End')}</Button>
	</div>
</Modal>
