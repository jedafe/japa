<script lang="ts">
	// Ref: japaconnect-engineering/sections/25-ad-placement.md (Amendment — native placements).
	// Renders a campaign as a feed-card-shaped unit so it blends into FeedList
	// streams. Same fetch/track contract as AdSlot.svelte: /ads/serve picks the
	// campaign, IntersectionObserver fires the impression, clicks go through
	// /ads/events. Hidden entirely when nothing is served — native slots must
	// never leave a "Sponsored" shell with no ad inside.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { trackedAdUrl } from '$lib/ads/trackedUrl';

	interface Props {
		/** Ad slot name (native_feed, native_discover, native_space_feed, native_forum). */
		slot: string;
		/** Space context for space-targeted campaigns. */
		spaceId?: string;
	}

	let { slot, spaceId = '' }: Props = $props();

	interface AdContent {
		headline?: string;
		body?: string;
		cta?: string;
		image_url?: string;
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
	let containerEl = $state<HTMLElement | null>(null);
	let fetchedKey = '';

	async function fetchAds(): Promise<void> {
		loading = true;
		try {
			const qs = spaceId ? `&space_id=${encodeURIComponent(spaceId)}` : '';
			const result = await api.get<AdCampaign[]>(`/ads/serve?slot=${encodeURIComponent(slot)}${qs}`);
			const campaigns = result.data;
			if (!campaigns || campaigns.length === 0) {
				campaign = null;
				return;
			}
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
		api.post('/ads/events', {
			events: [{ campaign_id: campaign.id, event_type: 'click' }]
		});
		window.open(trackedAdUrl(campaign.target_url, campaign.id), '_blank', 'noopener,noreferrer');
	}

	$effect(() => {
		const fetchKey = `${slot}|${spaceId}`;
		if (fetchedKey !== fetchKey) {
			fetchedKey = fetchKey;
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

{#if !loading && campaign}
	<article
		bind:this={containerEl}
		class="flex flex-col overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]"
		data-ad-slot={slot}
		aria-label={t('ads.sponsored', 'Sponsored')}
	>
		<a
			href={campaign.target_url ?? '#'}
			onclick={handleClick}
			class="flex flex-col hover:bg-[color-mix(in_srgb,var(--color-surface-subtle)_45%,transparent)]"
			rel="noopener sponsored"
			target="_blank"
		>
			{#if campaign.content?.image_url}
				<img
					src={campaign.content.image_url}
					alt={campaign.content.headline ?? campaign.name}
					class="aspect-video w-full object-cover"
					loading="lazy"
				/>
			{/if}
			<div class="flex flex-1 flex-col gap-2 p-3">
				<div class="flex items-center justify-between">
					<h2 class="line-clamp-2 text-[18px] font-semibold leading-snug tracking-tight text-[var(--color-text)]">
						{campaign.content?.headline ?? campaign.name}
					</h2>
					<span class="inline-block shrink-0 rounded border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10.5px] font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]">
						{t('ads.sponsored', 'Sponsored')}
					</span>
				</div>
				{#if campaign.content?.body}
					<p class="line-clamp-2 text-sm text-[var(--color-text-subtle)]">{campaign.content.body}</p>
				{/if}
				{#if campaign.content?.cta}
					<div class="mt-auto">
						<span class="inline-block rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1 text-xs font-medium text-[var(--color-primary-contrast)]">
							{campaign.content.cta}
						</span>
					</div>
				{/if}
			</div>
		</a>
	</article>
{:else}
	<!-- Keep bind target mounted for the observer while hidden. -->
	<div bind:this={containerEl} class="hidden" aria-hidden="true"></div>
{/if}
