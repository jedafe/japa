<script lang="ts">
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { sanitizeContentHtml } from '$lib/utils/safeHtml';

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

	const dimensions = {
		banner: 'h-[90px] w-full',
		square: 'h-[250px] w-[300px]',
		inline: 'h-[60px] w-full',
		// Standard IAB wide skyscraper — used by the left rail (Section 59).
		skyscraper: 'h-[600px] w-[160px]'
	} as const;

	let sizeClass = $derived(dimensions[format]);

	async function fetchAds(): Promise<void> {
		loading = true;
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
		if (!campaign?.target_url) return;
		e.preventDefault();
		// Track click event (fire-and-forget). Same {events: [...]} shape.
		api.post('/ads/events', {
			events: [{ campaign_id: campaign.id, event_type: 'click' }]
		});
		// Navigate to the target URL.
		window.open(campaign.target_url, '_blank', 'noopener,noreferrer');
	}

	// Ref: japaconnect-engineering/sections/23-main-site-frontend.md (Svelte 5 runes)
	// $effect replaces onMount for Svelte 5 — it runs after the DOM is mounted
	// and re-runs when its tracked dependencies (slot, containerEl) change.
	// The returned cleanup function disconnects the IntersectionObserver when
	// the component unmounts or the effect re-runs.
	$effect(() => {
		fetchAds();

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
	class="grid place-items-center rounded-[var(--radius)] border border-dashed border-[var(--color-border)] bg-[var(--color-surface-subtle)] text-xs text-[var(--color-muted)] overflow-hidden {sizeClass}"
	class:hidden={!campaign}
	data-ad-slot={slot}
	aria-label={t('ads.label', 'Advertisement')}
>
	{#if loading}
		<!-- Empty while loading — no placeholder flash -->
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
