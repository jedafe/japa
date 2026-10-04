<script lang="ts">
	import { onMount, onDestroy, untrack } from 'svelte';
	import type { CommunityOrgWithAi } from '$lib/types';

	/**
	 * OrgMap — Leaflet map showing community organisation markers.
	 * Uses OpenStreetMap tiles (no Google Maps dependency, per ADR-109).
	 * Markers sync reactively when the `orgs` prop changes (destination or
	 * filter changes) — the map is initialised once and a layer group is
	 * cleared/refilled on each update.
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
	let markerLayer: any = null;
	let L: any = null;
	let ready = $state(false);

	function syncMarkers(list: CommunityOrgWithAi[]) {
		if (!map || !markerLayer || !L) return;
		markerLayer.clearLayers();
		const geocoded = list.filter((o) => o.latitude != null && o.longitude != null);
		if (geocoded.length === 0) return;
		for (const org of geocoded) {
			const marker = L.marker([org.latitude!, org.longitude!]).addTo(markerLayer);
			const typeLabel = org.org_type.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
			const popupHtml = `<strong>${org.name}</strong><br/>${typeLabel}${org.origin_community ? ' · ' + org.origin_community : ''}`;
			marker.bindPopup(popupHtml);
		}
		if (geocoded.length > 1) {
			const bounds = L.latLngBounds(
				geocoded.map((o) => [o.latitude!, o.longitude!] as [number, number])
			);
			map.fitBounds(bounds, { padding: [40, 40] });
		} else {
			map.setView([geocoded[0].latitude!, geocoded[0].longitude!], 12);
		}
	}

	onMount(async () => {
		L = await import('leaflet');
		const geocoded = orgs.filter((o) => o.latitude != null && o.longitude != null);
		if (geocoded.length === 0) return;

		map = L.map(mapEl).setView([geocoded[0].latitude!, geocoded[0].longitude!], 12);
		L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
			attribution:
				'&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
			maxZoom: 19
		}).addTo(map);
		markerLayer = L.layerGroup().addTo(map);
		ready = true;
		syncMarkers(untrack(() => orgs));
	});

	// Re-sync markers whenever the org list changes after init.
	$effect(() => {
		if (!ready) return;
		syncMarkers(orgs);
	});

	onDestroy(() => {
		if (map) {
			map.remove();
			map = null;
		}
	});
</script>

<div
	bind:this={mapEl}
	style="height: {height}; width: 100%; border-radius: var(--radius, 0.5rem); overflow: hidden; z-index: 0;"
></div>
