<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md + §16 explore_map
	// Mounts the shared World Explorer block scoped to this wiki's geo space:
	// the map opens zoomed to the space's context and the space is pinned.
	//   country → fit the continent region, pin at country centroid
	//   state   → fit the country bounds, pin at the admin-1 centroid
	//   city    → fit the admin-1 (state) bounds, pin best-effort
	import { ExploreMapBlock, WORLD_COUNTRIES, ADMIN1_CENTROIDS, COUNTRY_BOUNDS } from '@japaconnect/landing-blocks';
	import type { ExploreMapProps, FetchContext } from '@japaconnect/landing-blocks';
	import type { Space } from '$lib/types';

	let {
		space,
		fetchCtx,
		countryCode: countryCodeProp,
		stateCode: stateCodeProp
	}: {
		space: Space;
		fetchCtx?: FetchContext;
		/** Alpha-2 of the parent country — resolved server-side via the
		 *  space's ancestor chain (state/city geo_codes are local, not
		 *  ISO-3166-2). Required for subnational spaces. */
		countryCode?: string | null;
		/** The parent state's local geo_code (city spaces) — used for the
		 *  admin-1 zoom target and approximate pin. */
		stateCode?: string | null;
	} = $props();

	// Country spaces carry an alpha-2 geo_code ("NG"); subnational spaces
	// carry a local code ("AD", "30") — the parent country's alpha-2 is
	// resolved server-side via the ancestor chain. If geo_code is already
	// full ISO-3166-2 ("NG-LA"), its prefix is a usable fallback.
	const geoCode = $derived((space.geo_code ?? '').toUpperCase());
	const countryCode = $derived(
		space.level === 'country'
			? geoCode.split('-')[0] || ''
			: (countryCodeProp ?? (geoCode.includes('-') ? geoCode.split('-')[0] : ''))
	);
	const meta = $derived(WORLD_COUNTRIES.find((c) => c.code === countryCode));

	// Candidate ISO-3166-2 keys for the space's own admin-1 unit:
	// "{country}-{geo_code}" ("NG-AD"), then the raw geo_code trimmed
	// progressively ("NG-LA-LOS" → "NG-LA").
	const ownAdmin1 = $derived.by(() => {
		const candidates: string[] = [];
		if (geoCode && countryCode) candidates.push(`${countryCode}-${geoCode}`);
		let probe = geoCode;
		while (probe.includes('-')) {
			candidates.push(probe);
			probe = probe.slice(0, probe.lastIndexOf('-'));
		}
		for (const key of candidates) {
			if (ADMIN1_CENTROIDS[key]) return key;
		}
		return '';
	});
	// Parent state key for city spaces.
	const parentAdmin1 = $derived(
		countryCode && stateCodeProp ? `${countryCode}-${stateCodeProp}` : ''
	);

	// Pin position: own admin-1 centroid → parent state's centroid
	// (approximate for cities) → country centroid fallback.
	const pinPos = $derived.by<[number, number] | undefined>(() => {
		const own = ownAdmin1 ? ADMIN1_CENTROIDS[ownAdmin1] : undefined;
		if (own) return [own.x, own.y];
		const parent = parentAdmin1 ? ADMIN1_CENTROIDS[parentAdmin1] : undefined;
		if (parent) return [parent.x, parent.y];
		return undefined;
	});

	// Zoom target by level: country → continent region; state → its country;
	// city → its state (admin-1) bounds, else the country.
	const initialBounds = $derived.by<[number, number, number, number] | undefined>(() => {
		if (space.level === 'country') return undefined;
		if (space.level === 'state') return countryCode ? COUNTRY_BOUNDS[countryCode] : undefined;
		// city or deeper: prefer the parent state's bounds, then own, then country
		const b =
			(parentAdmin1 ? ADMIN1_CENTROIDS[parentAdmin1]?.b : undefined) ??
			(ownAdmin1 ? ADMIN1_CENTROIDS[ownAdmin1].b : undefined) ??
			(countryCode ? COUNTRY_BOUNDS[countryCode] : undefined);
		return b || undefined;
	});

	// geo_source 'manual' — on a destination wiki the space pin is the subject.
	// (api mode ignores `pins` entirely and would show top-countries instead.)
	const mapProps = $derived<ExploreMapProps>({
		headline: '',
		subtitle: '',
		show_legend: false,
		show_stats: false,
		show_region_filter: false,
		geo_source: 'manual',
		pin_limit: 10,
		initial_region: meta?.region,
		initial_bounds: initialBounds,
		pins: countryCode
			? [{
					country: countryCode,
					label: space.name,
					// Subnational spaces pin on their admin-1 unit — the card
					// names the actual space and says which country it's in.
					tagline: space.level === 'country' ? 'This destination' : `in ${meta?.name ?? countryCode}`,
					href: `/spaces/${space.slug}`,
					...(pinPos ? { x: pinPos[0], y: pinPos[1] } : {}),
				}]
			: [],
	});
</script>

{#if meta}
	<div class="mt-6">
		<ExploreMapBlock props={mapProps} {fetchCtx} geoCountry={countryCode} />
	</div>
{/if}
