<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';

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

	let campaigns = $state<Campaign[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<{ items: Campaign[] }>(`/admin/advertising/campaigns`);
			if (result.ok) {
				campaigns = result.data.items ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = 'Failed to load campaigns';
		} finally {
			loading = false;
		}
	}

	async function toggleCampaign(campaign: Campaign) {
		const endpoint = campaign.status === 'active'
			? `/admin/advertising/campaigns/${campaign.id}/pause`
			: `/admin/advertising/campaigns/${campaign.id}/activate`;
		try {
			const result = await api(endpoint, { method: 'POST' });
			if (result.ok) {
				campaign.status = campaign.status === 'active' ? 'paused' : 'active';
				campaigns = campaigns;
			}
		} catch {
			errorMessage = 'Failed to update campaign status';
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

<h1 class="mb-1 text-2xl font-bold tracking-tight">Ad Campaigns</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">Manage advertising campaigns, targeting, and placements</p>

<div class="mb-4 flex justify-end">
	<a
		href="/ads/new"
		class="inline-block rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
		style="background-color: var(--color-primary)"
	>
		Create Campaign
	</a>
</div>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Campaign</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Status</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Impressions</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Clicks</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">CTR</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Budget</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
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
							<div class="flex items-center gap-2">
								{#if campaign.status !== 'completed'}
									<button
										class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
										style="color: {campaign.status === 'active' ? 'var(--color-warning)' : 'var(--color-success)'}"
										onclick={() => toggleCampaign(campaign)}
									>
										{campaign.status === 'active' ? 'Pause' : 'Resume'}
									</button>
								{/if}
								<a href="/ads/{campaign.id}" class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" style="color: var(--color-info)">
									View
								</a>
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="7" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							No campaigns created yet
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
