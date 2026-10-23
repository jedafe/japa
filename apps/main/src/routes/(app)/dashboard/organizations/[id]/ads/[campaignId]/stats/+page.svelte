<!-- Ref: japaconnect-engineering/sections/61-self-serve-ads.md (ADR-079) -->
<!-- Campaign stats page: impressions, clicks, CTR, spend, budget.
     Calls GET /org/{org_id}/ads/campaigns/{id}/stats (§61 API Routes). -->
<script lang="ts">
	import { t } from '$lib/i18n';
	import { ArrowLeft, BarChart3, MousePointer, Eye, TrendingUp, Wallet } from 'lucide-svelte';
	import type { CampaignStats } from './+page.server';

	let { data } = $props();

	const campaign = $derived(data.campaign);
	const stats = $derived(data.stats as CampaignStats | null);

	const ctr = $derived(stats ? stats.ctr.toFixed(2) : '0.00');
	const impressions = $derived(stats?.impressions ?? 0);
	const clicks = $derived(stats?.clicks ?? 0);
	const spent = $derived(stats?.spent ?? 0);
	const budgetCredits = $derived(stats?.budget_credits ?? campaign?.budget_credits ?? 0);
	const remainingCredits = $derived(budgetCredits - spent);

	function statusBadgeClass(status: string): string {
		switch (status) {
			case 'active':
				return 'bg-[var(--color-success)]/10 text-[var(--color-success)]';
			case 'paused':
				return 'bg-[var(--color-warning)]/10 text-[var(--color-warning)]';
			case 'pending_review':
				return 'bg-[var(--color-info)]/10 text-[var(--color-info)]';
			case 'rejected':
				return 'bg-[var(--color-danger)]/10 text-[var(--color-danger)]';
			case 'ended':
				return 'bg-[var(--color-muted)]/10 text-[var(--color-muted)]';
			default:
				return 'bg-[var(--color-muted)]/10 text-[var(--color-muted)]';
		}
	}

	function statusLabel(status: string): string {
		switch (status) {
			case 'active':
				return t('org_ads.status_active', 'Active');
			case 'paused':
				return t('org_ads.status_paused', 'Paused');
			case 'pending_review':
				return t('org_ads.status_pending_review', 'Pending Review');
			case 'rejected':
				return t('org_ads.status_rejected', 'Rejected');
			case 'ended':
				return t('org_ads.status_ended', 'Ended');
			default:
				return status;
		}
	}

	const statCards = $derived([
		{
			icon: Eye,
			label: t('org_ads.stats_impressions', 'Impressions'),
			value: impressions.toLocaleString(),
			color: 'var(--color-info)'
		},
		{
			icon: MousePointer,
			label: t('org_ads.stats_clicks', 'Clicks'),
			value: clicks.toLocaleString(),
			color: 'var(--color-primary)'
		},
		{
			icon: TrendingUp,
			label: t('org_ads.stats_ctr', 'CTR'),
			value: `${ctr}%`,
			color: 'var(--color-success)'
		},
		{
			icon: Wallet,
			label: t('org_ads.stats_spent', 'Spent'),
			value: `${spent} ${t('org_ads.credits', 'credits')}`,
			color: 'var(--color-warning)'
		}
	]);
</script>

<svelte:head>
	<title>{t('org_ads.stats_title', 'Campaign Stats')} — {campaign?.name ?? ''}</title>
</svelte:head>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center gap-3">
		<a
			href="/dashboard/organizations/{data.orgId}/ads"
			class="rounded-lg p-2 transition-colors hover:bg-[var(--color-surface)]"
			title={t('common.back', 'Back')}
		>
			<ArrowLeft class="h-5 w-5 rtl:-scale-x-100" />
		</a>
		<div>
			<h1 class="flex items-center gap-2 text-xl font-semibold text-[var(--color-text)]">
				<BarChart3 class="h-5 w-5" />
				{t('org_ads.stats_title', 'Campaign Stats')}
			</h1>
			<p class="text-sm text-[var(--color-muted)]">
				{campaign?.name}
			</p>
		</div>
	</div>

	<!-- Campaign summary -->
	{#if campaign}
		<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<div class="flex items-center justify-between">
				<div>
					<h2 class="text-lg font-semibold text-[var(--color-text)]">{campaign.name}</h2>
					{#if campaign.target_url}
						<a
							href={campaign.target_url}
							target="_blank"
							rel="noopener noreferrer"
							class="text-xs text-[var(--color-primary)] hover:underline"
						>
							{campaign.target_url}
						</a>
					{/if}
				</div>
				<span
					class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {statusBadgeClass(campaign.status)}"
				>
					{statusLabel(campaign.status)}
				</span>
			</div>
		</div>
	{/if}

	<!-- Stats cards -->
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
		{#each statCards as card (card.label)}
			<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<div class="flex items-center gap-2" style="color: {card.color}">
					<card.icon class="h-5 w-5" />
					<span class="text-sm font-medium">{card.label}</span>
				</div>
				<p class="mt-2 text-2xl font-bold text-[var(--color-text)]">{card.value}</p>
			</div>
		{/each}
	</div>

	<!-- Budget breakdown -->
	<div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h3 class="mb-3 text-sm font-medium text-[var(--color-text)]">
			{t('org_ads.budget_breakdown', 'Budget Breakdown')}
		</h3>
		<div class="space-y-2 text-sm">
			<div class="flex justify-between">
				<span class="text-[var(--color-muted)]">{t('org_ads.total_budget', 'Total Budget')}</span>
				<span class="font-medium text-[var(--color-text)]">
					{budgetCredits} {t('org_ads.credits', 'credits')}
				</span>
			</div>
			<div class="flex justify-between">
				<span class="text-[var(--color-muted)]">{t('org_ads.stats_spent', 'Spent')}</span>
				<span class="font-medium text-[var(--color-text)]">
					{spent} {t('org_ads.credits', 'credits')}
				</span>
			</div>
			<div class="flex justify-between border-t border-[var(--color-border)] pt-2">
				<span class="text-[var(--color-muted)]">{t('org_ads.remaining', 'Remaining')}</span>
				<span class="font-semibold text-[var(--color-text)]">
					{remainingCredits} {t('org_ads.credits', 'credits')}
				</span>
			</div>
		</div>
		{#if budgetCredits > 0}
			<div class="mt-3 h-2 overflow-hidden rounded-full bg-[var(--color-border)]">
				<div
					class="h-full rounded-full bg-[var(--color-primary)] transition-all"
					style="width: {Math.min(100, (spent / budgetCredits) * 100)}%"
				></div>
			</div>
		{/if}
	</div>
</div>
