// Ref: japaconnect-engineering/sections/25-ad-placement.md (Amendment — conversion event).
// Returns the click-through URL with the campaign attribution param appended
// when the destination is same-origin. Internal promotions (house ads and
// org ads pointing at platform pages) fire a `conversion` ad event when the
// visitor lands — AdConversionTracker.svelte reads the param, posts the
// event, and strips it. External destinations are left untouched; their
// conversions are tracked advertiser-side.
export const AD_ATTRIBUTION_PARAM = 'jc_ad_cid';

export function trackedAdUrl(targetUrl: string, campaignId: string): string {
	try {
		const url = new URL(targetUrl, window.location.origin);
		// Only http(s) targets may be opened — javascript:/data: URIs parse
		// cleanly here and would execute in the opener's context on
		// window.open. Return '#' so the click is inert.
		if (url.protocol !== 'http:' && url.protocol !== 'https:') return '#';
		if (url.origin !== window.location.origin) return targetUrl;
		url.searchParams.set(AD_ATTRIBUTION_PARAM, campaignId);
		return url.pathname + url.search + url.hash;
	} catch {
		return '#';
	}
}
