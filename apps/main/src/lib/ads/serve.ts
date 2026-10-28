// Ref: japaconnect-engineering/sections/25-ad-placement.md
// Shared /ads/serve fetch layer for AdSlot, NativeAd and StickyAnchorAd.
// Multiple units for the same slot on one page share a single request —
// concurrent callers join the in-flight promise and results are cached
// briefly so a blog page doesn't fan out N identical serve calls.
import { api } from '$lib/api/client';

export interface AdCampaignContent {
	headline?: string;
	body?: string;
	cta?: string;
	html?: string;
	markdown?: string;
	image_url?: string;
	image_media_id?: string;
}

export interface AdCampaign {
	id: string;
	name: string;
	content: AdCampaignContent;
	target_url?: string;
	priority: number;
	is_house?: boolean;
}

const TTL_MS = 30_000;
const cache = new Map<string, { at: number; data: AdCampaign[] }>();
const inflight = new Map<string, Promise<AdCampaign[]>>();

export async function fetchAdCampaigns(slot: string, spaceId = ''): Promise<AdCampaign[]> {
	const key = `${slot}|${spaceId}`;
	const hit = cache.get(key);
	if (hit && Date.now() - hit.at < TTL_MS) return hit.data;

	const pending = inflight.get(key);
	if (pending) return pending;

	const qs = spaceId ? `&space_id=${encodeURIComponent(spaceId)}` : '';
	const p = api
		.get<AdCampaign[]>(`/ads/serve?slot=${encodeURIComponent(slot)}${qs}`)
		.then((result) => {
			const campaigns = result.data ?? [];
			campaigns.sort((a, b) => b.priority - a.priority);
			cache.set(key, { at: Date.now(), data: campaigns });
			return campaigns;
		})
		.catch(() => {
			cache.set(key, { at: Date.now(), data: [] });
			return [] as AdCampaign[];
		})
		.finally(() => inflight.delete(key));

	inflight.set(key, p);
	return p;
}
