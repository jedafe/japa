<script lang="ts">
	// Fires a `conversion` ad event when a navigation lands with the
	// ad-attribution param (jc_ad_cid — appended on same-origin ad clicks),
	// then strips it. Mirrors apps/main's AdConversionTracker — requires a
	// same-origin referrer so crafted/shared URLs don't mint conversions.
	import { page } from '$app/state';
	import { apiPost } from '$lib/api/client';

	let handled = $state('');

	$effect(() => {
		const cid = page.url.searchParams.get('jc_ad_cid');
		if (!cid || cid === handled) return;
		if (!document.referrer.startsWith(window.location.origin)) return;
		handled = cid;

		const url = new URL(page.url);
		url.searchParams.delete('jc_ad_cid');
		history.replaceState(history.state, '', url.pathname + url.search + url.hash);

		apiPost('/ads/events', {
			events: [{ campaign_id: cid, event_type: 'conversion' }]
		}).catch(() => {
			// Fire-and-forget; conversion failures are non-critical.
		});
	});
</script>
