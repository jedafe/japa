<script lang="ts">
	/**
	 * Location settings — ADR-064a.
	 *
	 * Lets the user set their origin country, destination country, and current
	 * location (city-level via cascading picker or browser geolocation).
	 * Origin and destination countries are saved as custom field values via
	 * PATCH /me/profile. Current location is saved via POST /me/location.
	 *
	 * All five location fields use the debounced Autocomplete.svelte component
	 * for live search — the fetchers filter the already-loaded geo arrays
	 * client-side (no extra API round-trip per keystroke).
	 *
	 * Ref: japaconnect-engineering/adr/ADR-064a-geo-awareness-extension.md
	 *      japaconnect-engineering/sections/06-custom-field-builder-form-builder.md
	 */
	import { onMount, untrack } from 'svelte';
	import SettingsSection from '$lib/components/hub/SettingsSection.svelte';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { patchProfile } from '$lib/api/hub';
	import { listStates, listCities, setLocation, getMyLocation } from '$lib/api/geo';
	import { t } from '$lib/i18n';
	import { MapPin, Navigation, Loader2, Home, Plane } from 'lucide-svelte';
	import type { GeoCountry, GeoState, GeoCity } from '$lib/types';

	interface LocationData {
		profile: {
			origin_country?: string | null;
			destination_country?: string | null;
		};
		countries?: GeoCountry[];
	}

	let { data }: { data: unknown } = $props();
	const payload = $derived(data as LocationData | null);
	const countries = $derived(payload?.countries ?? []);

	// Origin and destination country values (ISO 3166-1 alpha-2).
	let originCountry = $state(untrack(() => payload?.profile?.origin_country ?? ''));
	let destCountry = $state(untrack(() => payload?.profile?.destination_country ?? ''));
	let savingOrigin = $state(false);
	let savingDest = $state(false);
	let originError = $state<string | null>(null);
	let destError = $state<string | null>(null);

	// Resolved labels for the currently-selected origin/destination (avoids
	// re-fetching on every render — the Autocomplete shows this as the chip).
	const originLabel = $derived(
		originCountry ? countries.find((c) => c.geo_code === originCountry)?.name ?? originCountry : null
	);
	const destLabel = $derived(
		destCountry ? countries.find((c) => c.geo_code === destCountry)?.name ?? destCountry : null
	);

	// Current location (city-level Space selection).
	let currentLocation = $state<{
		space_id: string | null;
		city: string | null;
		country: string | null;
		country_code: string | null;
	} | null>(null);
	let loadingLocation = $state(true);
	let savingLocation = $state(false);
	let locationError = $state<string | null>(null);

	// Cascading picker state for current location.
	let selectedCountryCode = $state('');
	let selectedStateId = $state('');
	let states = $state<GeoState[]>([]);
	let cities = $state<GeoCity[]>([]);
	let loadingStates = $state(false);
	let loadingCities = $state(false);

	// Resolved labels for the cascading picker selections.
	const cascadeCountryLabel = $derived(
		selectedCountryCode
			? countries.find((c) => c.geo_code === selectedCountryCode)?.name ?? selectedCountryCode
			: null
	);
	const cascadeStateLabel = $derived(
		selectedStateId ? states.find((s) => s.id === selectedStateId)?.name ?? selectedStateId : null
	);

	// Browser geolocation state.
	let geoLoading = $state(false);
	let geoError = $state<string | null>(null);

	// Load current location on mount.
	onMount(async () => {
		const loc = await getMyLocation();
		currentLocation = loc;
		loadingLocation = false;
	});

	// ---- Fetchers: filter the already-loaded geo arrays client-side ----
	// No API call per keystroke — the loader already fetched all 250
	// countries, and states/cities are loaded on parent selection.

	function countryFetcher(q: string): Promise<AutocompleteItem[]> {
		const needle = q.toLowerCase().trim();
		const filtered = needle
			? countries.filter(
					(c) => c.name.toLowerCase().includes(needle) || c.geo_code.toLowerCase().includes(needle)
				)
			: countries;
		return Promise.resolve(
			filtered.slice(0, 50).map((c) => ({ id: c.geo_code, label: c.name, sublabel: c.geo_code }))
		);
	}

	function countryInitialFetcher(): Promise<AutocompleteItem[]> {
		return Promise.resolve(
			countries.slice(0, 8).map((c) => ({ id: c.geo_code, label: c.name, sublabel: c.geo_code }))
		);
	}

	function stateFetcher(q: string): Promise<AutocompleteItem[]> {
		const needle = q.toLowerCase().trim();
		const filtered = needle
			? states.filter(
					(s) => s.name.toLowerCase().includes(needle) || (s.geo_code ?? '').toLowerCase().includes(needle)
				)
			: states;
		return Promise.resolve(filtered.slice(0, 50).map((s) => ({ id: s.id, label: s.name })));
	}

	function stateInitialFetcher(): Promise<AutocompleteItem[]> {
		return Promise.resolve(states.slice(0, 8).map((s) => ({ id: s.id, label: s.name })));
	}

	function cityFetcher(q: string): Promise<AutocompleteItem[]> {
		const needle = q.toLowerCase().trim();
		const filtered = needle
			? cities.filter(
					(c) => c.name.toLowerCase().includes(needle) || (c.geo_code ?? '').toLowerCase().includes(needle)
				)
			: cities;
		return Promise.resolve(filtered.slice(0, 50).map((c) => ({ id: c.id, label: c.name })));
	}

	function cityInitialFetcher(): Promise<AutocompleteItem[]> {
		return Promise.resolve(cities.slice(0, 8).map((c) => ({ id: c.id, label: c.name })));
	}

	// ---- Save handlers ----

	async function saveOriginCountry(item: AutocompleteItem) {
		originCountry = item.id;
		savingOrigin = true;
		originError = null;
		// Save via PATCH /me/profile — the backend stores origin_country
		// as a custom_field_values entry on the user_profile entity (ADR-064a).
		const res = await patchProfile({ origin_country: item.id });
		savingOrigin = false;
		if (res.error) originError = res.error.message;
	}

	async function clearOriginCountry() {
		originCountry = '';
		savingOrigin = true;
		originError = null;
		const res = await patchProfile({ origin_country: '' });
		savingOrigin = false;
		if (res.error) originError = res.error.message;
	}

	async function saveDestCountry(item: AutocompleteItem) {
		destCountry = item.id;
		savingDest = true;
		destError = null;
		const res = await patchProfile({ destination_country: item.id });
		savingDest = false;
		if (res.error) destError = res.error.message;
	}

	async function clearDestCountry() {
		destCountry = '';
		savingDest = true;
		destError = null;
		const res = await patchProfile({ destination_country: '' });
		savingDest = false;
		if (res.error) destError = res.error.message;
	}

	async function onCountrySelect(item: AutocompleteItem) {
		selectedCountryCode = item.id;
		selectedStateId = '';
		states = [];
		cities = [];
		if (item.id) {
			loadingStates = true;
			states = await listStates(item.id);
			loadingStates = false;
		}
	}

	function onCountryClear() {
		selectedCountryCode = '';
		selectedStateId = '';
		states = [];
		cities = [];
	}

	async function onStateSelect(item: AutocompleteItem) {
		selectedStateId = item.id;
		cities = [];
		if (item.id) {
			loadingCities = true;
			cities = await listCities(item.id);
			loadingCities = false;
		}
	}

	function onStateClear() {
		selectedStateId = '';
		cities = [];
	}

	async function onCitySelect(item: AutocompleteItem) {
		await saveLocation(item.id, item.label);
	}

	/**
	 * Save the selected geo space as the user's current location. Works at
	 * any level (country, state, or city) — the backend validates that the
	 * space_id is a geo space. This lets users save their location even
	 * when no city-level spaces are seeded for their state (BUG-005).
	 */
	async function saveLocation(spaceId: string, label: string) {
		savingLocation = true;
		locationError = null;
		const res = await setLocation({ space_id: spaceId });
		savingLocation = false;
		if (res) {
			currentLocation = {
				space_id: res.space_id,
				city: label,
				country: countries.find((c) => c.geo_code === selectedCountryCode)?.name ?? null,
				country_code: selectedCountryCode || null
			};
		} else {
			locationError = t('settings.location.save_error', 'Could not save location');
		}
	}

	/** Save the currently-selected country as the user's location. */
	async function saveCountryAsLocation() {
		const country = countries.find((c) => c.geo_code === selectedCountryCode);
		if (country) await saveLocation(country.id, country.name);
	}

	/** Save the currently-selected state as the user's location. */
	async function saveStateAsLocation() {
		const state = states.find((s) => s.id === selectedStateId);
		if (state) await saveLocation(state.id, state.name);
	}

	async function useBrowserGeolocation() {
		geoLoading = true;
		geoError = null;
		try {
			const pos = await new Promise<GeolocationPosition>((resolve, reject) => {
				navigator.geolocation.getCurrentPosition(resolve, reject, {
					enableHighAccuracy: false,
					timeout: 10000,
					maximumAge: 300000
				});
			});
			savingLocation = true;
			const res = await setLocation({
				latitude: pos.coords.latitude,
				longitude: pos.coords.longitude
			});
			savingLocation = false;
			if (res) {
				// Refresh the resolved location from the backend.
				currentLocation = await getMyLocation();
			} else {
				geoError = t('settings.location.geocode_error', 'Could not resolve your location to a known city');
			}
		} catch (err) {
			const geolocErr = err as GeolocationPositionError;
			if (geolocErr.code === geolocErr.PERMISSION_DENIED) {
				geoError = t('settings.location.permission_denied', 'Location permission denied. Using IP-based detection instead.');
			} else {
				geoError = t('settings.location.unavailable', 'Could not determine your location.');
			}
		} finally {
			geoLoading = false;
			savingLocation = false;
		}
	}
</script>

<SettingsSection titleKey="settings.nav.location" descKey="settings.location.desc">
	<!-- Origin Country -->
	<div class="mb-4 block">
		<span class="mb-1.5 flex items-center gap-1.5 text-sm text-[var(--color-muted)]">
			<Home size={14} /> {t('settings.location.origin_country', 'Origin country')}
		</span>
		<div class="relative">
			<Autocomplete
				placeholder={t('settings.location.search_origin', 'Search origin country…')}
				emptyMessage={t('settings.location.no_countries', 'No countries found')}
				fetcher={countryFetcher}
				initialFetcher={countryInitialFetcher}
				selectedId={originCountry || null}
				selectedLabel={originLabel}
				onSelect={saveOriginCountry}
				onClear={clearOriginCountry}
			/>
			{#if savingOrigin}
				<span class="absolute right-3 top-1/2 -translate-y-1/2"><Loader2 size={12} class="animate-spin" /></span>
			{/if}
		</div>
		<p class="mt-1.5 text-xs text-[var(--color-muted)]">
			{t('settings.location.origin_hint', 'Where you are from. Used to personalize content for your origin community.')}
		</p>
		{#if originError}<p class="mt-1 text-xs text-[var(--color-danger,#dc2626)]">{originError}</p>{/if}
	</div>

	<!-- Destination Country -->
	<div class="mb-4 block">
		<span class="mb-1.5 flex items-center gap-1.5 text-sm text-[var(--color-muted)]">
			<Plane size={14} /> {t('settings.location.dest_country', 'Destination country')}
		</span>
		<div class="relative">
			<Autocomplete
				placeholder={t('settings.location.search_dest', 'Search destination country…')}
				emptyMessage={t('settings.location.no_countries', 'No countries found')}
				fetcher={countryFetcher}
				initialFetcher={countryInitialFetcher}
				selectedId={destCountry || null}
				selectedLabel={destLabel}
				onSelect={saveDestCountry}
				onClear={clearDestCountry}
			/>
			{#if savingDest}
				<span class="absolute right-3 top-1/2 -translate-y-1/2"><Loader2 size={12} class="animate-spin" /></span>
			{/if}
		</div>
		<p class="mt-1.5 text-xs text-[var(--color-muted)]">
			{t('settings.location.dest_hint', 'Where you are relocating to or planning to move. Used to surface relevant tools and guides.')}
		</p>
		{#if destError}<p class="mt-1 text-xs text-[var(--color-danger,#dc2626)]">{destError}</p>{/if}
	</div>

	<!-- Current Location -->
	<div class="mt-6 border-t border-[var(--color-border)] pt-4">
		<span class="mb-2 flex items-center gap-1.5 text-sm font-medium text-[var(--color-text)]">
			<MapPin size={14} /> {t('settings.location.current', 'Current location')}
		</span>

		{#if loadingLocation}
			<p class="text-sm text-[var(--color-muted)]"><Loader2 size={14} class="inline animate-spin" /> Loading…</p>
		{:else if currentLocation?.city}
			<p class="mb-3 text-sm text-[var(--color-text)]">
				{currentLocation.city}{#if currentLocation.country && currentLocation.country !== currentLocation.city}, {currentLocation.country}{/if}
			</p>
		{:else}
			<p class="mb-3 text-sm text-[var(--color-muted)]">
				{t('settings.location.not_set_current', 'Not set — using IP-based detection')}
			</p>
		{/if}

		<!-- Browser geolocation button -->
		<button
			type="button"
			disabled={geoLoading || savingLocation}
			onclick={useBrowserGeolocation}
			class="mb-3 inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs text-white hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
		>
			{#if geoLoading}<Loader2 size={12} class="animate-spin" />{:else}<Navigation size={12} />{/if}
			{t('settings.location.use_gps', 'Use my precise location')}
		</button>
		{#if geoError}<p class="mb-2 text-xs text-[var(--color-danger,#dc2626)]">{geoError}</p>{/if}
		{#if locationError}<p class="mb-2 text-xs text-[var(--color-danger,#dc2626)]">{locationError}</p>{/if}

		<!-- Cascading picker: Country → State → City (debounced Autocomplete) -->
		<p class="mb-2 text-xs text-[var(--color-muted)]">
			{t('settings.location.or_pick', 'Or pick your location manually:')}
		</p>
		<div class="space-y-2">
			<!-- Country -->
			<Autocomplete
				placeholder={t('settings.location.search_country', 'Search country…')}
				emptyMessage={t('settings.location.no_countries', 'No countries found')}
				fetcher={countryFetcher}
				initialFetcher={countryInitialFetcher}
				selectedId={selectedCountryCode || null}
				selectedLabel={cascadeCountryLabel}
				onSelect={onCountrySelect}
				onClear={onCountryClear}
			/>

			<!-- "Use this country" button — lets the user save at country
			     level without drilling into state/city (BUG-005). -->
			{#if selectedCountryCode && !selectedStateId}
				<button
					type="button"
					disabled={savingLocation}
					onclick={saveCountryAsLocation}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
				>
					{#if savingLocation}<Loader2 size={12} class="animate-spin" />{:else}<MapPin size={12} />{/if}
					{t('settings.location.use_country', 'Set {country} as my location').replace('{country}', cascadeCountryLabel ?? selectedCountryCode)}
				</button>
			{/if}

			<!-- State (only after country is selected and states are loaded) -->
			{#if selectedCountryCode && states.length > 0}
				<div class="relative">
					<Autocomplete
						placeholder={t('settings.location.search_state', 'Search state/region…')}
						emptyMessage={t('settings.location.no_states', 'No states/regions found')}
						fetcher={stateFetcher}
						initialFetcher={stateInitialFetcher}
						selectedId={selectedStateId || null}
						selectedLabel={cascadeStateLabel}
						onSelect={onStateSelect}
						onClear={onStateClear}
					/>
					{#if loadingStates}
						<span class="absolute right-3 top-1/2 -translate-y-1/2"><Loader2 size={12} class="animate-spin" /></span>
					{/if}
				</div>
			{/if}

			<!-- "Use this state" button — lets the user save at state level
			     when no cities are seeded for the state (BUG-005). -->
			{#if selectedStateId && cities.length === 0 && !loadingCities}
				<button
					type="button"
					disabled={savingLocation}
					onclick={saveStateAsLocation}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
				>
					{#if savingLocation}<Loader2 size={12} class="animate-spin" />{:else}<MapPin size={12} />{/if}
					{t('settings.location.use_state', 'Set {state} as my location').replace('{state}', cascadeStateLabel ?? selectedStateId)}
				</button>
			{/if}

			<!-- City (only after state is selected and cities are loaded) -->
			{#if selectedStateId && cities.length > 0}
				<div class="relative">
					<Autocomplete
						placeholder={t('settings.location.search_city', 'Search city…')}
						emptyMessage={t('settings.location.no_cities', 'No cities found')}
						fetcher={cityFetcher}
						initialFetcher={cityInitialFetcher}
						selectedId={null}
						onSelect={onCitySelect}
					/>
					{#if loadingCities}<span class="absolute right-3 top-1/2 -translate-y-1/2"><Loader2 size={12} class="animate-spin" /></span>{/if}
					{#if savingLocation}<span class="absolute right-3 top-1/2 -translate-y-1/2"><Loader2 size={12} class="animate-spin" /></span>{/if}
				</div>
			{/if}
		</div>
	</div>
</SettingsSection>
