<!-- Ref: japaconnect-engineering/sections/61-self-serve-ads.md (ADR-079) -->
<!-- Self-serve ads dashboard: list campaigns, create new campaigns, view stats.
     Calls /org/{org_id}/ads/* endpoints (§61 API Routes). -->
<script lang="ts">
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import AppealButton from '$lib/components/appeals/AppealButton.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
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
	const statusFilter = $derived(data.status ?? '');

	const statusFilterOptions = $derived([
		{ value: '', label: t('org_ads.all_statuses', 'All statuses') },
		{ value: 'draft', label: t('org_ads.status_draft', 'Draft') },
		{ value: 'pending_review', label: t('org_ads.status_pending_review', 'Pending Review') },
		{ value: 'active', label: t('org_ads.status_active', 'Active') },
		{ value: 'paused', label: t('org_ads.status_paused', 'Paused') },
		{ value: 'rejected', label: t('org_ads.status_rejected', 'Rejected') },
		{ value: 'ended', label: t('org_ads.status_ended', 'Ended') }
	]);

	function changeStatus(v: string) {
		const url = new URL(window.location.href);
		if (v) url.searchParams.set('status', v);
		else url.searchParams.delete('status');
		goto(url.pathname + url.search);
	}

	// ── Create-campaign form state ───────────────────────────────────────────
	let showCreateForm = $state(false);
	let creating = $state(false);
	let errorKey = $state('');
	let successKey = $state('');

	import { page } from '$app/state';
	import { StopCircle } from 'lucide-svelte';

	let formName = $state('');
	let formSlotTypeId = $state('');
	let formHeadline = $state('');
	let formBody = $state('');
	let formCta = $state('');
	let formImageUrl = $state('');
	let formTargetUrl = $state('');
	let formDurationDays = $state(7);
	let formStartDate = $state('');
	let formTargetGeo = $state('');
	let formFreqCap = $state('');
	let errorDetail = $state('');

	// ads.min_campaign_days is a system setting — honour it, don't hardcode.
	const minDays = $derived(
		Math.max(1, Number(page.data.siteSettings?.['ads.min_campaign_days'] ?? 3) || 3)
	);
	// §61: only verified orgs may create campaigns — surface that up front.
	const orgVerified = $derived((org as { verified?: boolean } | null)?.verified !== false);

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';

	function resetForm() {
		formName = '';
		formSlotTypeId = '';
		formHeadline = '';
		formBody = '';
		formCta = '';
		formImageUrl = '';
		formTargetUrl = '';
		formDurationDays = 7;
		formStartDate = '';
		formTargetGeo = '';
		formFreqCap = '';
	}

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

	async function loadMore() {
		if (!org || !hasMore || loadingMore) return;
		loadingMore = true;
		try {
			const next = currentPage + 1;
			const statusParam = statusFilter ? `&status=${encodeURIComponent(statusFilter)}` : '';
			const res = await api.getEnvelope<AdCampaign[]>(
				`/org/${org.id}/ads/campaigns?page=${next}&limit=${pageSize}${statusParam}`
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
		if (!formName || !formSlotTypeId || !formHeadline) {
			errorKey = 'org_ads.errors.missing_fields';
			errorDetail = '';
			return;
		}
		creating = true;
		errorKey = '';
		errorDetail = '';
		successKey = '';
		try {
			// Structured creative — AdSlot/NativeAd read headline/body/cta/
			// image_url from content; a raw string would render a blank ad.
			const body: Record<string, unknown> = {
				name: formName,
				slot_type_id: formSlotTypeId,
				content: {
					headline: formHeadline,
					...(formBody ? { body: formBody } : {}),
					...(formCta ? { cta: formCta } : {}),
					...(formImageUrl ? { image_url: formImageUrl } : {})
				},
				duration_days: formDurationDays
			};
			if (formTargetUrl) body.target_url = formTargetUrl;
			if (formTargetGeo.trim()) {
				body.target_geo = formTargetGeo.split(',').map((c) => c.trim().toUpperCase()).filter(Boolean);
			}
			if (formFreqCap) body.frequency_cap_per_user = Number(formFreqCap);
			if (formStartDate) {
				// Send an RFC3339 timestamp so the Go backend's *time.Time field
				// unmarshals correctly (BUG: raw YYYY-MM-DD strings fail).
				body.start_date = `${formStartDate}T00:00:00Z`;
			}

			const res = await api.post(`/org/${org.id}/ads/campaigns`, body);
			if (res.error) {
				errorKey = 'org_ads.errors.create_failed';
				errorDetail = typeof res.error === 'string' ? res.error : '';
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

	async function endCampaign(campaignId: string) {
		if (!org) return;
		errorKey = '';
		try {
			const res = await api.post(`/org/${org.id}/ads/campaigns/${campaignId}/end`);
			if (res.error) {
				errorKey = 'org_ads.errors.end_failed';
				errorDetail = typeof res.error === 'string' ? res.error : '';
			} else {
				await invalidateAll();
			}
		} catch {
			errorKey = 'org_ads.errors.end_failed';
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
				<ArrowLeft class="h-5 w-5 rtl:-scale-x-100" />
			</a>
			<div>
				<h1 class="flex items-center gap-2 text-xl font-semibold text-[var(--color-text)]">
					<Megaphone class="h-5 w-5" />
					{t('org_ads.title', 'Ad Campaigns')}
				</h1>
				<p class="text-sm text-[var(--color-muted)]">
					{org?.name}
				</p>
			</div>
		</div>
		<button
			class="inline-flex items-center gap-2 rounded-lg bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
			disabled={!orgVerified}
			title={!orgVerified ? t('org_ads.not_verified', 'Only verified organizations can run ads') : ''}
			onclick={() => (showCreateForm = !showCreateForm)}
		>
			<Plus class="h-4 w-4" />
			{t('org_ads.create_campaign', 'Create Campaign')}
		</button>
	</div>

	{#if errorKey}
		<div class="rounded-lg bg-[var(--color-danger)]/10 p-3 text-sm text-[var(--color-danger)]">
			{t(errorKey, 'An error occurred')}{errorDetail ? ` — ${errorDetail}` : ''}
		</div>
	{/if}
	{#if successKey}
		<div class="rounded-lg bg-[var(--color-success)]/10 p-3 text-sm text-[var(--color-success)]">
			{t(successKey, 'Campaign created successfully')}
		</div>
	{/if}

	{#if !orgVerified}
		<div class="rounded-lg bg-[var(--color-warning)]/10 p-3 text-sm text-[var(--color-warning)]">
			{t('org_ads.not_verified', 'Only verified organizations can run ads')}
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
					<p class="mt-1 text-xs text-[var(--color-muted)]">{t('org_ads.name_hint', 'Internal name for this campaign — not shown to users.')}</p>
					<input id="ad-name" class={inputClass} type="text" bind:value={formName} required />
				</div>

				<div>
					<label for="ad-slot" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('org_ads.slot_type', 'Ad Slot')}
					</label>
					<p class="mt-1 text-xs text-[var(--color-muted)]">{t('org_ads.slot_hint', 'Where the ad appears — feed, sidebar, banner.')}</p>
					<AutocompleteSelect
						id="ad-slot"
						bind:value={formSlotTypeId}
						options={[
							{ value: '', label: t('org_ads.select_slot', 'Select a slot...') },
							...slots.map((slot) => ({
								value: slot.id,
								label: `${slot.slot_name} — ${slot.flat_rate} ${t('org_ads.credits_day', 'credits/day')}`
							}))
						]}
					/>
				</div>

				<div>
					<label for="ad-duration" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('org_ads.duration_days', 'Duration (days)')}
					</label>
					<p class="mt-1 text-xs text-[var(--color-muted)]">{t('org_ads.duration_hint', 'How many days the campaign runs.')}</p>
					<input
						id="ad-duration"
						class={inputClass}
						type="number"
						min={minDays}
						bind:value={formDurationDays}
						required
					/>
				</div>

				<div>
					<label for="ad-start" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('org_ads.start_date', 'Start Date')}
					</label>
					<p class="mt-1 text-xs text-[var(--color-muted)]">{t('org_ads.start_hint', 'When the campaign begins.')}</p>
					<input id="ad-start" class={inputClass} type="date" bind:value={formStartDate} />
				</div>

				<div class="md:col-span-2">
					<label for="ad-headline" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('org_ads.headline', 'Headline')} *
					</label>
					<p class="mt-1 text-xs text-[var(--color-muted)]">{t('org_ads.headline_hint', 'The main text users see.')}</p>
					<input id="ad-headline" class={inputClass} type="text" bind:value={formHeadline} required />
				</div>

				<div class="md:col-span-2">
					<label for="ad-body" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('org_ads.body', 'Body text')}
					</label>
					<p class="mt-1 text-xs text-[var(--color-muted)]">{t('org_ads.body_hint', 'Supporting copy under the headline.')}</p>
					<textarea id="ad-body" class={inputClass} rows="3" bind:value={formBody}></textarea>
				</div>

				<div>
					<label for="ad-cta" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('org_ads.cta', 'Call to action')}
					</label>
					<p class="mt-1 text-xs text-[var(--color-muted)]">{t('org_ads.cta_hint', 'The button text — e.g. Learn More.')}</p>
					<input id="ad-cta" class={inputClass} type="text" bind:value={formCta} placeholder={t('org_ads.cta_ph', 'Learn More')} />
				</div>

				<div>
					<label for="ad-image" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('org_ads.image_url', 'Image URL')}
					</label>
					<p class="mt-1 text-xs text-[var(--color-muted)]">{t('org_ads.image_hint', 'Direct image URL for the creative.')}</p>
					<input id="ad-image" class={inputClass} type="url" bind:value={formImageUrl} placeholder="https://..." />
				</div>

				<div>
					<label for="ad-geo" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('org_ads.target_geo', 'Target countries (ISO codes, comma-separated)')}
					</label>
					<p class="mt-1 text-xs text-[var(--color-muted)]">{t('org_ads.geo_hint', 'Country codes to target — e.g. NG, GB, US.')}</p>
					<input id="ad-geo" class={inputClass} type="text" bind:value={formTargetGeo} placeholder="NG, GB, US" />
				</div>

				<div>
					<label for="ad-freq" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('org_ads.freq_cap', 'Frequency cap / user / day')}
					</label>
					<p class="mt-1 text-xs text-[var(--color-muted)]">{t('org_ads.freq_hint', 'How often the ad shows per viewer.')}</p>
					<input id="ad-freq" class={inputClass} type="number" min="0" bind:value={formFreqCap} />
				</div>

				<div>
					<label for="ad-url" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('org_ads.target_url', 'Target URL')}
					</label>
					<p class="mt-1 text-xs text-[var(--color-muted)]">{t('org_ads.url_hint', 'Where clicks land.')}</p>
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
				<div class="rounded-lg bg-[var(--color-surface)] p-3 text-sm">
					<span class="text-[var(--color-muted)]">{t('org_ads.estimated_cost', 'Estimated cost')}:</span>
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
	<div class="flex justify-end">
		<AutocompleteSelect
			class="w-48"
			value={statusFilter}
			options={statusFilterOptions}
			ariaLabel={t('org_ads.filter_status', 'Filter by status')}
			onValueChange={changeStatus}
		/>
	</div>
	{#if campaigns.length === 0}
		<EmptyState
			icon={Megaphone}
			title={t('org_ads.no_campaigns', 'No campaigns yet')}
			body={t('org_ads.no_campaigns_desc', 'Create your first ad campaign to get started.')}
		/>
	{:else}
		<div class="overflow-x-auto rounded-lg border border-[var(--color-border)]">
			<table class="w-full text-sm">
				<thead class="bg-[var(--color-surface)]">
					<tr>
						<th class="px-4 py-3 text-start font-medium text-[var(--color-text)]">
							{t('org_ads.campaign_name', 'Campaign Name')}
						</th>
						<th class="px-4 py-3 text-start font-medium text-[var(--color-text)]">
							{t('common.status', 'Status')}
						</th>
						<th class="px-4 py-3 text-start font-medium text-[var(--color-text)]">
							{t('org_ads.budget', 'Budget')}
						</th>
						<th class="px-4 py-3 text-start font-medium text-[var(--color-text)]">
							{t('org_ads.created', 'Created')}
						</th>
						<th class="px-4 py-3 text-end font-medium text-[var(--color-text)]">
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
									<div class="text-xs text-[var(--color-muted)]">{campaign.target_url}</div>
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
									{#if campaign.status === 'active' || campaign.status === 'paused' || campaign.status === 'pending_review'}
										<button
											class="rounded-lg p-2 text-[var(--color-danger)] transition-colors hover:bg-[var(--color-surface)]"
											title={t('org_ads.end', 'End campaign')}
											onclick={() => endCampaign(campaign.id)}
										>
											<StopCircle class="h-4 w-4" />
										</button>
									{/if}
									{#if campaign.status === 'rejected'}
										<AppealButton entityType="ad_campaign" entityId={campaign.id} />
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
