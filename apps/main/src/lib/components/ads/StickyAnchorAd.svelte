<script lang="ts">
	// Ref: japaconnect-engineering/sections/25-ad-placement.md (Amendment — sticky/floating).
	// Dismissible anchor banner fixed to the viewport bottom (sticky_anchor slot).
	// Dismissal persists for the browser session (sessionStorage). Sits above
	// the mobile tab bar on small screens. Same fetch/track contract as
	// AdSlot.svelte.
	import { browser } from '$app/environment';
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { trackedAdUrl } from '$lib/ads/trackedUrl';
	import { fetchAdCampaigns, type AdCampaign } from '$lib/ads/serve';
	import { X } from 'lucide-svelte';

	const DISMISS_KEY = 'jc.ad.anchor.dismissed';

	interface Props {
		slot?: string;
		spaceId?: string;
	}

	let { slot = 'sticky_anchor', spaceId = '' }: Props = $props();

	let campaign = $state<AdCampaign | null>(null);
	// Dismissal is per-campaign — hiding one ad must not suppress a
	// different campaign that fills the slot later in the session.
	let dismissed = $derived(browser && campaign ? sessionStorage.getItem(`${DISMISS_KEY}.${campaign.id}`) === '1' : true);
	let impressionTracked = $state(false);
	let containerEl = $state<HTMLElement | null>(null);
	let fetchedKey = '';

	const adsEnabled = $derived(
		(page.data?.features as Record<string, boolean> | undefined)?.['feature.ads'] !== false
	);

	async function fetchAds(): Promise<void> {
		if (!adsEnabled) return;
		const campaigns = await fetchAdCampaigns(slot, spaceId);
		campaign = campaigns[0] ?? null;
	}

	async function trackImpression(): Promise<void> {
		if (!campaign || impressionTracked) return;
		impressionTracked = true;
		try {
			await api.post('/ads/events', {
				events: [{ campaign_id: campaign.id, event_type: 'impression' }]
			});
		} catch {
			// Fire-and-forget.
		}
	}

	function handleClick(e: MouseEvent): void {
		e.preventDefault();
		if (!campaign?.target_url) return;
		api.post('/ads/events', {
			events: [{ campaign_id: campaign.id, event_type: 'click' }]
		});
		window.open(trackedAdUrl(campaign.target_url, campaign.id), '_blank', 'noopener,noreferrer');
	}

	let dismissedNow = $state(false);
	function dismiss(): void {
		dismissedNow = true;
		if (browser && campaign) sessionStorage.setItem(`${DISMISS_KEY}.${campaign.id}`, '1');
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

{#if !dismissed && !dismissedNow && campaign}
	<div
		bind:this={containerEl}
		class="fixed inset-x-0 bottom-16 z-40 flex justify-center px-3 sm:bottom-0 motion-reduce:transition-none"
		role="complementary"
		aria-label={t('ads.label', 'Advertisement')}
		data-ad-slot={slot}
	>
		<div class="relative flex w-full max-w-3xl items-center gap-3 overflow-hidden rounded-t-[var(--radius-card)] border border-b-0 border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2.5 shadow-lg sm:rounded-[var(--radius-card)] sm:border-b">
			{#if campaign.content?.image_url}
				<img
					src={campaign.content.image_url}
					alt=""
					class="hidden h-12 w-12 shrink-0 rounded object-cover sm:block"
					loading="lazy"
				/>
			{/if}
			<a
				href={campaign.target_url ?? '#'}
				onclick={handleClick}
				class="flex min-w-0 flex-1 items-center gap-3"
				rel="noopener sponsored"
				target="_blank"
			>
				<span class="min-w-0 flex-1">
					<span class="block truncate text-sm font-semibold text-[var(--color-text)]">
						{campaign.content?.headline ?? campaign.name}
					</span>
					{#if campaign.content?.body}
						<span class="block truncate text-xs text-[var(--color-muted)]">{campaign.content.body}</span>
					{/if}
				</span>
				{#if campaign.content?.cta}
					<span class="shrink-0 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1 text-xs font-medium text-[var(--color-primary-contrast)]">
						{campaign.content.cta}
					</span>
				{/if}
			</a>
			<span class="text-[10px] font-medium uppercase tracking-wide text-[var(--color-muted)]">
				{t('ads.sponsored', 'Sponsored')}
			</span>
			<button
				type="button"
				onclick={dismiss}
				class="grid h-6 w-6 shrink-0 place-items-center rounded-full text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
				aria-label={t('ads.dismiss', 'Dismiss ad')}
			>
				<X size={14} />
			</button>
		</div>
	</div>
{:else}
	<div bind:this={containerEl} class="hidden" aria-hidden="true"></div>
{/if}
