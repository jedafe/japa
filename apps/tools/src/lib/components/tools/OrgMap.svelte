<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import type { CommunityOrgWithAi } from '$lib/types';

	/**
	 * OrgMap — Leaflet map showing community organisation markers.
	 * Uses OpenStreetMap tiles (no Google Maps dependency, per ADR-109).
	 *
	 * Ref: japaconnect-engineering/sections/101-diaspora-community-locator.md
	 *      japaconnect-engineering/adr/ADR-109-community-locator.md
	 */

	let {
		orgs,
		height = '300px'
	}: { orgs: CommunityOrgWithAi[]; height?: string } = $props();

	let mapEl: HTMLDivElement;
	let map: any = null;

	onMount(async () => {
		const L = await import('leaflet');

		// Filter orgs with valid coordinates.
		const geocoded = orgs.filter(
			(o) => o.latitude != null && o.longitude != null
		);
		if (geocoded.length === 0) return;

		map = L.map(mapEl).setView([geocoded[0].latitude!, geocoded[0].longitude!], 12);

		L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
			attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
			maxZoom: 19
		}).addTo(map);

		for (const org of geocoded) {
			const marker = L.marker([org.latitude!, org.longitude!]).addTo(map);
			const popupHtml = `<strong>${org.name}</strong><br/>${org.org_type}${org.origin_community ? ' · ' + org.origin_community : ''}`;
			marker.bindPopup(popupHtml);
		}

		// Fit bounds to show all markers.
		if (geocoded.length > 1) {
			const bounds = L.latLngBounds(geocoded.map((o) => [o.latitude!, o.longitude!] as [number, number]));
			map.fitBounds(bounds, { padding: [40, 40] });
		}
	});

	onDestroy(() => {
		if (map) {
			map.remove();
			map = null;
		}
	});
</script>

<div bind:this={mapEl} style="height: {height}; width: 100%; border-radius: var(--radius, 0.5rem); overflow: hidden; z-index: 0;"></div>
