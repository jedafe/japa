<!-- Ref: japaconnect-engineering/sections/61-self-serve-ads.md (ADR-079) -->
<!-- Self-serve ads dashboard: list campaigns, create new campaigns, view stats.
     Calls /org/{org_id}/ads/* endpoints (§61 API Routes). -->
<script lang="ts">
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { Megaphone, Plus, Pause, Play, BarChart3, ArrowLeft, Check, X } from 'lucide-svelte';
	import { invalidateAll } from '$app/navigation';
	import { goto } from '$app/navigation';
	import type { AdSlotType, AdCampaign } from './+page.server';

	let { data } = $props();
	const org = $derived(data.org);
	const slots = $derived(data.slots as AdSlotType[]);
	const initialCampaigns = $derived(data.campaigns as AdCampaign[]);

	// Client-side pagination state. Start from the SSR first page and allow
	// appending additional pages without a full reload. Sync from the prop
	// when the server data changes (e.g. after invalidateAll), but preserve
	// the locally loaded pages on subsequent client renders.
	let localCampaigns = $state([] as AdCampaign[]);
	let currentPage = $state(1);
	let totalCampaigns = $state(0);
	let pageSize = $state(20);
	let loadingMore = $state(false);
	const hasMore = $derived(totalCampaigns > localCampaigns.length);

	$effect.pre(() => {
		localCampaigns = [...initialCampaigns];
		currentPage = data.page ?? 1;
		totalCampaigns = data.total ?? 0;
		pageSize = data.limit ?? 20;
	});

	const campaigns = $derived(localCampaigns);

	// ── Create-campaign form state ───────────────────────────────────────────
	let showCreateForm = $state(false);
	let creating = $state(false);
	let errorKey = $state('');
	let successKey = $state('');

	let formName = $state('');
	let formSlotTypeId = $state('');
	let formContent = $state('');
	let formTargetUrl = $state('');
	let formDurationDays = $state(7);
	let formStartDate = $state('');

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';

	function resetForm() {
		formName = '';
		formSlotTypeId = '';
		formContent = '';
		formTargetUrl = '';
		formDurationDays = 7;
		formStartDate = '';
	}

	function statusBadgeClass(status: string): string {
		switch (status) {
			case 'active':
				return 'bg-success/15 text-success dark:text-success';
			case 'paused':
				return 'bg-warning/15 text-warning dark:text-warning';
			case 'pending_review':
				return 'bg-info/15 text-info dark:text-info';
			case 'rejected':
				return 'bg-danger/15 text-danger dark:text-danger';
			case 'ended':
				return 'bg-muted/15 text-muted dark:text-muted';
			default:
				return 'bg-muted/15 text-muted dark:text-muted';
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

	async function loadMore() {
		if (!org || !hasMore || loadingMore) return;
		loadingMore = true;
		try {
			const next = currentPage + 1;
			const res = await api.getEnvelope<AdCampaign[]>(
				`/org/${org.id}/ads/campaigns?page=${next}&limit=${pageSize}`
			);
			if (res.data && res.data.length > 0) {
				localCampaigns = [...localCampaigns, ...res.data];
				currentPage = next;
			}
			if (res.envelope && typeof res.envelope.total === 'number') {
				totalCampaigns = res.envelope.total;
			}
		} catch {
			errorKey = 'org_ads.errors.load_more_failed';
		} finally {
			loadingMore = false;
		}
	}

	async function createCampaign(e: SubmitEvent) {
		e.preventDefault();
		if (!org) return;
		if (!formName || !formSlotTypeId) {
			errorKey = 'org_ads.errors.missing_fields';
			return;
		}
		creating = true;
		errorKey = '';
		successKey = '';
		try {
			const body: Record<string, unknown> = {
				name: formName,
				slot_type_id: formSlotTypeId,
				content: formContent,
				duration_days: formDurationDays
			};
			if (formTargetUrl) body.target_url = formTargetUrl;
			if (formStartDate) {
				// Send an RFC3339 timestamp so the Go backend's *time.Time field
				// unmarshals correctly (BUG: raw YYYY-MM-DD strings fail).
				body.start_date = `${formStartDate}T00:00:00Z`;
			}

			const res = await api.post(`/org/${org.id}/ads/campaigns`, body);
			if (res.error) {
				errorKey = 'org_ads.errors.create_failed';
			} else {
				successKey = 'org_ads.created_success';
				resetForm();
				showCreateForm = false;
				await invalidateAll();
			}
		} catch {
			errorKey = 'org_ads.errors.create_failed';
		} finally {
			creating = false;
		}
	}

	async function pauseCampaign(campaignId: string) {
		if (!org) return;
		errorKey = '';
		try {
			const res = await api.post(`/org/${org.id}/ads/campaigns/${campaignId}/pause`);
			if (res.error) {
				errorKey = 'org_ads.errors.pause_failed';
			} else {
				await invalidateAll();
			}
		} catch {
			errorKey = 'org_ads.errors.pause_failed';
		}
	}

	async function resumeCampaign(campaignId: string) {
		if (!org) return;
		errorKey = '';
		try {
			const res = await api.post(`/org/${org.id}/ads/campaigns/${campaignId}/resume`);
			if (res.error) {
				errorKey = 'org_ads.errors.resume_failed';
			} else {
				await invalidateAll();
			}
		} catch {
			errorKey = 'org_ads.errors.resume_failed';
		}
	}

	const selectedSlot = $derived(slots.find((s) => s.id === formSlotTypeId));
	const estimatedCost = $derived(selectedSlot ? selectedSlot.flat_rate * formDurationDays : 0);
</script>

<svelte:head>
	<title>{t('org_ads.title', 'Ad Campaigns')} — {org?.name ?? ''}</title>
</svelte:head>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<div class="flex items-center gap-3">
			<a
				href="/dashboard/organizations/{org?.id}"
				class="rounded-lg p-2 transition-colors hover:bg-[var(--color-surface)]"
				title={t('common.back', 'Back')}
			>
				<ArrowLeft class="h-5 w-5" />
			</a>
			<div>
				<h1 class="flex items-center gap-2 text-xl font-semibold text-[var(--color-text)]">
					<Megaphone class="h-5 w-5" />
					{t('org_ads.title', 'Ad Campaigns')}
				</h1>
				<p class="text-sm" style="color: var(--color-muted)">
					{org?.name}
				</p>
			</div>
		</div>
		<button
			class="inline-flex items-center gap-2 rounded-lg bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
			onclick={() => (showCreateForm = !showCreateForm)}
		>
			<Plus class="h-4 w-4" />
			{t('org_ads.create_campaign', 'Create Campaign')}
		</button>
	</div>

	{#if errorKey}
		<div
			class="rounded-lg p-3 text-sm"
			style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
		>
			{t(errorKey, 'An error occurred')}
		</div>
	{/if}
	{#if successKey}
		<div
			class="rounded-lg p-3 text-sm"
			style="background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)"
		>
			{t(successKey, 'Campaign created successfully')}
		</div>
	{/if}

	<!-- Create campaign form -->
	{#if showCreateForm}
		<form
			class="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-6 space-y-4"
			onsubmit={createCampaign}
		>
			<h2 class="text-lg font-semibold text-[var(--color-text)]">
				{t('org_ads.create_campaign', 'Create Campaign')}
			</h2>

			<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
				<div>
					<label for="ad-name" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('org_ads.campaign_name', 'Campaign Name')}
					</label>
					<input id="ad-name" class={inputClass} type="text" bind:value={formName} required />
				</div>

				<div>
					<label for="ad-slot" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('org_ads.slot_type', 'Ad Slot')}
					</label>
					<select id="ad-slot" class={inputClass} bind:value={formSlotTypeId} required>
						<option value="">{t('org_ads.select_slot', 'Select a slot...')}</option>
						{#each slots as slot (slot.id)}
							<option value={slot.id}>
								{slot.slot_name} — {slot.flat_rate} {t('org_ads.credits_day', 'credits/day')}
							</option>
						{/each}
					</select>
				</div>

				<div>
					<label for="ad-duration" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('org_ads.duration_days', 'Duration (days)')}
					</label>
					<input
						id="ad-duration"
						class={inputClass}
						type="number"
						min="3"
						bind:value={formDurationDays}
						required
					/>
				</div>

				<div>
					<label for="ad-start" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('org_ads.start_date', 'Start Date')}
					</label>
					<input id="ad-start" class={inputClass} type="date" bind:value={formStartDate} />
				</div>

				<div class="md:col-span-2">
					<label for="ad-content" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('org_ads.content', 'Ad Content / Creative')}
					</label>
					<textarea
						id="ad-content"
						class={inputClass}
						rows="3"
						bind:value={formContent}
						placeholder={t('org_ads.content_placeholder', 'Ad copy or creative JSON...')}
					></textarea>
				</div>

				<div>
					<label for="ad-url" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('org_ads.target_url', 'Target URL')}
					</label>
					<input
						id="ad-url"
						class={inputClass}
						type="url"
						bind:value={formTargetUrl}
						placeholder="https://..."
					/>
				</div>
			</div>

			{#if estimatedCost > 0}
				<div class="rounded-lg p-3 text-sm" style="background: var(--color-surface)">
					<span style="color: var(--color-muted)">{t('org_ads.estimated_cost', 'Estimated cost')}:</span>
					<span class="font-semibold text-[var(--color-text)]">{estimatedCost} {t('org_ads.credits', 'credits')}</span>
				</div>
			{/if}

			<div class="flex gap-2">
				<button
					type="submit"
					disabled={creating}
					class="inline-flex items-center gap-2 rounded-lg bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
				>
					{#if creating}
						{t('org_ads.creating', 'Creating...')}
					{:else}
						<Check class="h-4 w-4" />
						{t('org_ads.create', 'Create')}
					{/if}
				</button>
				<button
					type="button"
					class="inline-flex items-center gap-2 rounded-lg border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface)]"
					onclick={() => {
						showCreateForm = false;
						resetForm();
					}}
				>
					<X class="h-4 w-4" />
					{t('common.cancel', 'Cancel')}
				</button>
			</div>
		</form>
	{/if}

	<!-- Campaigns list -->
	{#if campaigns.length === 0}
		<EmptyState
			icon={Megaphone}
			title={t('org_ads.no_campaigns', 'No campaigns yet')}
			body={t('org_ads.no_campaigns_desc', 'Create your first ad campaign to get started.')}
		/>
	{:else}
		<div class="overflow-hidden rounded-lg border border-[var(--color-border)]">
			<table class="w-full text-sm">
				<thead style="background: var(--color-surface)">
					<tr>
						<th class="px-4 py-3 text-left font-medium text-[var(--color-text)]">
							{t('org_ads.campaign_name', 'Campaign Name')}
						</th>
						<th class="px-4 py-3 text-left font-medium text-[var(--color-text)]">
							{t('common.status', 'Status')}
						</th>
						<th class="px-4 py-3 text-left font-medium text-[var(--color-text)]">
							{t('org_ads.budget', 'Budget')}
						</th>
						<th class="px-4 py-3 text-left font-medium text-[var(--color-text)]">
							{t('org_ads.created', 'Created')}
						</th>
						<th class="px-4 py-3 text-right font-medium text-[var(--color-text)]">
							{t('common.actions', 'Actions')}
						</th>
					</tr>
				</thead>
				<tbody>
					{#each campaigns as campaign (campaign.id)}
						<tr class="border-t border-[var(--color-border)]">
							<td class="px-4 py-3 text-[var(--color-text)]">
								<div class="font-medium">{campaign.name}</div>
								{#if campaign.target_url}
									<div class="text-xs" style="color: var(--color-muted)">{campaign.target_url}</div>
								{/if}
							</td>
							<td class="px-4 py-3">
								<span
									class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {statusBadgeClass(
										campaign.status
									)}"
								>
									{statusLabel(campaign.status)}
								</span>
							</td>
							<td class="px-4 py-3 text-[var(--color-text)]">
								{campaign.budget_credits} {t('org_ads.credits', 'credits')}
							</td>
							<td class="px-4 py-3 text-[var(--color-text)]">
								{new Date(campaign.created_at).toLocaleDateString()}
							</td>
							<td class="px-4 py-3">
								<div class="flex justify-end gap-1">
									{#if campaign.status === 'active'}
										<button
											class="rounded-lg p-2 transition-colors hover:bg-[var(--color-surface)]"
											title={t('org_ads.pause', 'Pause')}
											onclick={() => pauseCampaign(campaign.id)}
										>
											<Pause class="h-4 w-4" />
										</button>
									{/if}
									{#if campaign.status === 'paused'}
										<button
											class="rounded-lg p-2 transition-colors hover:bg-[var(--color-surface)]"
											title={t('org_ads.resume', 'Resume')}
											onclick={() => resumeCampaign(campaign.id)}
										>
											<Play class="h-4 w-4" />
										</button>
									{/if}
									<a
										href="/dashboard/organizations/{org?.id}/ads/{campaign.id}/stats"
										class="rounded-lg p-2 transition-colors hover:bg-[var(--color-surface)]"
										title={t('org_ads.stats', 'Stats')}
									>
										<BarChart3 class="h-4 w-4" />
									</a>
								</div>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>

		{#if hasMore}
			<div class="flex justify-center">
				<button
					type="button"
					disabled={loadingMore}
					class="inline-flex items-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					onclick={loadMore}
				>
					{#if loadingMore}
						{t('org_ads.loading_more', 'Loading...')}
					{:else}
						{t('org_ads.load_more', 'Load more')}
					{/if}
				</button>
			</div>
		{/if}
	{/if}
</div>
