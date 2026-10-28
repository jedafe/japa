<script lang="ts">
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { page } from '$app/state';
	import { sanitizeContentHtml } from '$lib/utils/safeHtml';
	import { trackedAdUrl } from '$lib/ads/trackedUrl';
	import { fetchAdCampaigns, type AdCampaign } from '$lib/ads/serve';

	interface Props {
		/** Ad slot identifier, surfaced as the label. */
		slot: string;
		format?: 'banner' | 'square' | 'inline' | 'skyscraper';
		/** Space context for space-targeted campaigns (§25 targeting). */
		spaceId?: string;
		/** When several units share one slot on a page, each passes a
		 *  different index so they show different creatives — the server
		 *  returns up to max_display_per_page campaigns in priority order. */
		pickIndex?: number;
	}

	let { slot, format = 'banner', spaceId = '', pickIndex = 0 }: Props = $props();

	let campaign = $state<AdCampaign | null>(null);
	let loading = $state(true);
	let impressionTracked = $state(false);

	let containerEl = $state<HTMLDivElement | null>(null);
	// Non-reactive fetch guard — the effect below re-runs when containerEl
	// flips null→element post-hydration, which would issue /ads/serve twice.
	let fetchedSlot = '';

	const dimensions = {
		banner: 'h-[90px] w-full',
		square: 'h-[250px] w-[300px]',
		inline: 'h-[60px] w-full',
		// Standard IAB wide skyscraper — used by the left rail (Section 59).
		skyscraper: 'h-[600px] w-[160px]'
	} as const;

	let sizeClass = $derived(dimensions[format]);

	// /ads/serve is gated behind the feature.ads flag — skip the request
	// entirely when ads are disabled (mirrors the docs/academy AdSlot).
	const adsEnabled = $derived(
		(page.data?.features as Record<string, boolean> | undefined)?.['feature.ads'] !== false
	);

	async function fetchAds(): Promise<void> {
		if (!adsEnabled) { loading = false; return; }
		loading = true;
		const campaigns = await fetchAdCampaigns(slot, spaceId);
		// pickIndex beyond the served set renders nothing rather than
		// repeating the same creative in two units on one page.
		campaign = pickIndex < campaigns.length ? campaigns[pickIndex] : null;
		loading = false;
	}

	async function trackImpression(): Promise<void> {
		if (!campaign || impressionTracked) return;
		impressionTracked = true;
		try {
			// Ref: japaconnect-engineering/sections/25-ad-placement.md
			// POST /ads/events expects {events: [{campaign_id, event_type}]}.
			// A bare object is rejected with 400 empty_events and the
			// impression is never recorded.
			await api.post('/ads/events', {
				events: [{ campaign_id: campaign.id, event_type: 'impression' }]
			});
		} catch {
			// Fire-and-forget; impression failures are non-critical.
		}
	}

	async function handleClick(e: MouseEvent): Promise<void> {
		e.preventDefault();
		if (!campaign?.target_url) return;
		// Track click event (fire-and-forget). Same {events: [...]} shape.
		api.post('/ads/events', {
			events: [{ campaign_id: campaign.id, event_type: 'click' }]
		});
		// Navigate to the target URL — same-origin destinations carry the
		// attribution param so the landing page fires the conversion event.
		window.open(trackedAdUrl(campaign.target_url, campaign.id), '_blank', 'noopener,noreferrer');
	}

	// Ref: japaconnect-engineering/sections/23-main-site-frontend.md (Svelte 5 runes)
	// $effect replaces onMount for Svelte 5 — it runs after the DOM is mounted
	// and re-runs when its tracked dependencies (slot, containerEl) change.
	// The returned cleanup function disconnects the IntersectionObserver when
	// the component unmounts or the effect re-runs.
	$effect(() => {
		const fetchKey = `${slot}|${spaceId}`;
		if (fetchedSlot !== fetchKey) {
			fetchedSlot = fetchKey;
			void fetchAds();
		}

		// IntersectionObserver to track impression when the ad enters the viewport.
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
	class="grid place-items-center rounded-[var(--radius)] border border-[var(--color-border)] text-xs text-[var(--color-muted)] overflow-hidden {sizeClass}"
	class:hidden={!loading && !campaign}
	data-ad-slot={slot}
	aria-label={t('ads.label', 'Advertisement')}
>
	{#if loading}
		<!-- Reserve the slot while the serve call resolves so a served ad
		     swaps in place instead of popping in and shifting the widgets
		     below it; a no-fill collapses the box on resolve. -->
		<div class="h-full w-full animate-pulse bg-[var(--color-surface-subtle)]"></div>
	{:else if campaign}
		<a
			href={campaign.target_url ?? '#'}
			onclick={handleClick}
			class="relative flex h-full w-full items-center justify-center overflow-hidden"
			rel="noopener sponsored"
			target="_blank"
		>
			<span class="absolute end-1 top-1 rounded bg-black/55 px-1 py-px text-[9px] font-semibold uppercase tracking-wide text-white">
				{t('ads.label', 'Ad')}
			</span>
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
