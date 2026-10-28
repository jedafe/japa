<script lang="ts">
	// Ref: japaconnect-engineering/sections/25-ad-placement.md (Amendment — conversion event).
	// Mounted once in the (app) layout. When a navigation lands with the
	// ad-attribution param (jc_ad_cid — appended by trackedAdUrl on same-origin
	// ad clicks), posts a `conversion` event to /ads/events, then strips the
	// param so a refresh or shared URL does not double-count.
	import { page } from '$app/state';
	import { api } from '$lib/api/client';
	import { AD_ATTRIBUTION_PARAM } from '$lib/ads/trackedUrl';

	let handled = $state('');

	$effect(() => {
		const cid = page.url.searchParams.get(AD_ATTRIBUTION_PARAM);
		if (!cid || cid === handled) return;
		// Only count landings that actually came from a page on this site —
		// a crafted/shared URL with the param must not mint a conversion.
		if (!document.referrer.startsWith(window.location.origin)) return;
		handled = cid;

		const url = new URL(page.url);
		url.searchParams.delete(AD_ATTRIBUTION_PARAM);
		history.replaceState(history.state, '', url.pathname + url.search + url.hash);

		api.post('/ads/events', {
			events: [{ campaign_id: cid, event_type: 'conversion' }]
		}).catch(() => {
			// Fire-and-forget; conversion failures are non-critical.
		});
	});
</script>
