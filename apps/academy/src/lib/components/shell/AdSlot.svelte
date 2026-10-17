<script lang="ts">
	// AdSlot — serves a real ad campaign via GET /ads/serve (Section 25, §8).
	// Ported from the Main app's AdSlot so admin-managed ad_slot widgets on
	// Academy renders an actual campaign instead of a placeholder box. The slot
	// is suppressed on the lesson player by RightSidebar (ADR-060: no ads).
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { sanitizeContentHtml } from '$lib/utils/safeHtml';
	import { page } from '$app/state';

	interface Props {
		/** Ad slot identifier, surfaced as the label. */
		slot: string;
		format?: 'banner' | 'square' | 'inline' | 'skyscraper';
	}

	let { slot, format = 'banner' }: Props = $props();

	interface AdContent {
		headline?: string;
		body?: string;
		cta?: string;
		html?: string;
		markdown?: string;
		image_url?: string;
		image_media_id?: string;
	}

	interface AdCampaign {
		id: string;
		name: string;
		content: AdContent;
		target_url?: string;
		priority: number;
	}

	let campaign = $state<AdCampaign | null>(null);
	let loading = $state(true);
	let impressionTracked = $state(false);

	let containerEl = $state<HTMLDivElement | null>(null);

	// /ads/serve is gated behind the feature.ads flag — skip the request
	// entirely when ads are disabled so no 404 noise is generated.
	const adsEnabled = $derived(
		(page.data?.features as Record<string, boolean> | undefined)?.['feature.ads'] !== false
	);

	const dimensions = {
		banner: 'h-[90px] w-full',
		square: 'h-[250px] w-[300px]',
		inline: 'h-[60px] w-full',
		// Standard IAB wide skyscraper — used by the left rail (Section 59).
		skyscraper: 'h-[600px] w-[160px]'
	} as const;

	let sizeClass = $derived(dimensions[format]);

	async function fetchAds(): Promise<void> {
		if (!adsEnabled) { loading = false; return; }
		try {
			const result = await api.get<AdCampaign[]>(`/ads/serve?slot=${encodeURIComponent(slot)}`);
			const campaigns = result.data;
			if (!campaigns || campaigns.length === 0) {
				campaign = null;
				return;
			}
			// Pick the highest-priority ad (descending); first in array wins ties.
			campaigns.sort((a, b) => b.priority - a.priority);
			campaign = campaigns[0];
		} catch {
			campaign = null;
		} finally {
			loading = false;
		}
	}

	async function trackImpression(): Promise<void> {
		if (!campaign || impressionTracked) return;
		impressionTracked = true;
		try {
			// POST /ads/events expects {events: [{campaign_id, event_type}]} —
			// a bare object is rejected with 400 empty_events.
			await api.post('/ads/events', {
				events: [{ campaign_id: campaign.id, event_type: 'impression' }]
			});
		} catch {
			// Fire-and-forget; impression failures are non-critical.
		}
	}

	async function handleClick(e: MouseEvent): Promise<void> {
		if (!campaign?.target_url) return;
		e.preventDefault();
		api.post('/ads/events', {
			events: [{ campaign_id: campaign.id, event_type: 'click' }]
		});
		// Same-origin destinations carry the attribution param so the landing
		// page fires the conversion event (§25 — mirrors main's trackedAdUrl).
		try {
			const url = new URL(campaign.target_url, window.location.origin);
			if (url.protocol !== 'http:' && url.protocol !== 'https:') return;
			const same = url.origin === window.location.origin;
			if (same) url.searchParams.set('jc_ad_cid', campaign.id);
			window.open(same ? url.pathname + url.search + url.hash : campaign.target_url, '_blank', 'noopener,noreferrer');
		} catch {
			// Unparseable target — drop the click rather than open an unknown scheme.
		}
	}

	// Non-reactive fetch guard — the effect re-runs when containerEl flips
	// null→element post-hydration, which would issue /ads/serve twice.
	let fetchedSlot = '';

	$effect(() => {
		if (fetchedSlot !== slot) {
			fetchedSlot = slot;
			void fetchAds();
		}

		const observer = new IntersectionObserver(
			(entries) => {
				for (const entry of entries) {
					if (entry.isIntersecting) {
						trackImpression();
						break;
					}
				}
			},
			{ threshold: 0.5 }
		);

		if (containerEl) {
			observer.observe(containerEl);
		}

		return () => observer.disconnect();
	});
</script>

<!-- The container stays mounted (bind:this + IntersectionObserver depend on
     it) but is visually hidden until a campaign is served — an empty dashed
     box must not occupy sidebar space when no ad is returned. -->
<div
	bind:this={containerEl}
	class="grid place-items-center overflow-hidden rounded-[var(--radius)] border border-dashed border-[var(--color-border)] bg-[var(--color-surface-subtle)] text-xs text-[var(--color-muted)] {sizeClass}"
	class:hidden={!loading && !campaign}
	data-ad-slot={slot}
	aria-label={t('ads.label', 'Advertisement')}
>
	{#if loading}
		<!-- Reserve the slot while the serve call resolves so a served ad
		     swaps in place instead of popping in and shifting widgets below;
		     a no-fill collapses the box on resolve. -->
		<div class="h-full w-full animate-pulse"></div>
	{:else if campaign}
		<a
			href={campaign.target_url ?? '#'}
			onclick={handleClick}
			class="flex h-full w-full items-center justify-center overflow-hidden"
			rel="noopener noreferrer"
			target="_blank"
		>
			{#if campaign.content?.image_url}
				<img
					src={campaign.content.image_url}
					alt={campaign.content.headline ?? campaign.name}
					class="h-full w-full object-cover"
					loading="lazy"
				/>
			{:else if campaign.content?.html}
				{@html sanitizeContentHtml(campaign.content.html)}
			{:else}
				<div class="flex flex-col items-center gap-1 px-2 text-center">
					{#if campaign.content?.headline}
						<span class="font-semibold text-foreground">{campaign.content.headline}</span>
					{/if}
					{#if campaign.content?.body}
						<span class="text-muted">{campaign.content.body}</span>
					{/if}
					{#if campaign.content?.cta}
						<span class="mt-1 rounded bg-[var(--color-primary)] px-2 py-0.5 text-[10px] font-medium text-[var(--color-primary-foreground)]">
							{campaign.content.cta}
						</span>
					{/if}
				</div>
			{/if}
		</a>
	{/if}
</div>
