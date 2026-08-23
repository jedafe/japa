<script lang="ts">
	/**
	 * Location settings — ADR-064a.
	 *
	 * Lets the user set their origin location, destination country, and current
	 * location (city-level via cascading picker or browser geolocation).
	 * Origin location (country/state/city) and destination country are saved as
	 * custom field values via PATCH /me/profile. Current location is saved via
	 * POST /me/location.
	 *
	 * All location fields use the debounced Autocomplete.svelte component for
	 * live search — the fetchers filter the already-loaded geo arrays
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
	import { listStates, listCities, setLocation, getMyLocation, createCity } from '$lib/api/geo';
	import { t } from '$lib/i18n';
	import { MapPin, Navigation, Loader2, Home, Plane } from 'lucide-svelte';
	import type { GeoCountry, GeoState, GeoCity } from '$lib/types';

	interface LocationData {
		profile: {
			origin_country?: string | null;
			origin_state_id?: string | null;
			origin_city_id?: string | null;
			destination_country?: string | null;
		};
		countries?: GeoCountry[];
	}

	let { data }: { data: unknown } = $props();
	const payload = $derived(data as LocationData | null);
	const countries = $derived(payload?.countries ?? []);

	// Origin location values. Country is an ISO 3166-1 alpha-2 code; state and
	// city are geo Space UUIDs.
	let originCountry = $state(untrack(() => payload?.profile?.origin_country ?? ''));
	let originStateId = $state(untrack(() => payload?.profile?.origin_state_id ?? ''));
	let originCityId = $state(untrack(() => payload?.profile?.origin_city_id ?? ''));
	let savingOrigin = $state(false);
	let originError = $state<string | null>(null);

	// Cascading picker data for origin location.
	let originStates = $state<GeoState[]>([]);
	let originCities = $state<GeoCity[]>([]);
	let originLoadingStates = $state(false);
	let originLoadingCities = $state(false);

	// Resolved labels for the currently-selected origin values.
	const originCountryLabel = $derived(
		originCountry ? countries.find((c) => c.geo_code === originCountry)?.name ?? originCountry : null
	);
	const originStateLabel = $derived(
		originStateId ? originStates.find((s) => s.id === originStateId)?.name ?? originStateId : null
	);
	const originCityLabel = $derived(
		originCityId ? originCities.find((c) => c.id === originCityId)?.name ?? originCityId : null
	);

	// Destination country value (ISO 3166-1 alpha-2).
	let destCountry = $state(untrack(() => payload?.profile?.destination_country ?? ''));
	let savingDest = $state(false);
	let destError = $state<string | null>(null);
	const destLabel = $derived(
		destCountry ? countries.find((c) => c.geo_code === destCountry)?.name ?? destCountry : null
	);

	// Current location (city-level Space selection).
	let currentLocation = $state<{
		space_id: string | null;
		name: string | null;
		city: string | null;
		city_id: string | null;
		state: string | null;
		state_id: string | null;
		country: string | null;
		country_code: string | null;
	} | null>(null);
	let loadingLocation = $state(true);
	let savingLocation = $state(false);
	let locationError = $state<string | null>(null);

	// Cascading picker state for current location.
	let selectedCountryCode = $state('');
	let selectedStateId = $state('');
	let currentCityId = $state('');
	let states = $state<GeoState[]>([]);
	let cities = $state<GeoCity[]>([]);
	let loadingStates = $state(false);
	let loadingCities = $state(false);

	// Resolved labels for the current location cascading picker selections.
	const cascadeCountryLabel = $derived(
		selectedCountryCode
			? countries.find((c) => c.geo_code === selectedCountryCode)?.name ?? selectedCountryCode
			: null
	);
	const cascadeStateLabel = $derived(
		selectedStateId ? states.find((s) => s.id === selectedStateId)?.name ?? selectedStateId : null
	);
	const currentCityLabel = $derived(
		currentCityId ? cities.find((c) => c.id === currentCityId)?.name ?? currentCityId : null
	);
	const currentLocationDisplay = $derived(
		currentLocation
			? [currentLocation.city, currentLocation.state, currentLocation.country]
					.filter((v): v is string => Boolean(v))
					.join(', ')
			: ''
	);

	// Browser geolocation state.
	let geoLoading = $state(false);
	let geoError = $state<string | null>(null);

	// Load current location and pre-populate the origin cascade on mount.
	onMount(async () => {
		const [loc] = await Promise.all([getMyLocation(), initOriginCascades()]);
		currentLocation = loc;
		loadingLocation = false;
		if (loc) {
			await syncCurrentPickersFromLocation(loc);
		}
	});

	async function initOriginCascades() {
		if (!originCountry) return;
		originLoadingStates = true;
		try {
			originStates = await listStates(originCountry);
		} finally {
			originLoadingStates = false;
		}
		if (originStateId) {
			originLoadingCities = true;
			try {
				originCities = await listCities(originStateId);
			} finally {
				originLoadingCities = false;
			}
		}
	}

	async function syncCurrentPickersFromLocation(loc: NonNullable<typeof currentLocation>) {
		if (loc.country_code) {
			selectedCountryCode = loc.country_code;
			loadingStates = true;
			try {
				states = await listStates(loc.country_code);
			} finally {
				loadingStates = false;
			}
		}
		if (loc.state_id) {
			selectedStateId = loc.state_id;
			loadingCities = true;
			try {
				cities = await listCities(loc.state_id);
			} finally {
				loadingCities = false;
			}
		} else {
			selectedStateId = '';
			cities = [];
		}
		if (loc.city_id) {
			currentCityId = loc.city_id;
		} else {
			currentCityId = '';
		}
	}

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

	function createStateFetcher(stateList: GeoState[]) {
		return (q: string): Promise<AutocompleteItem[]> => {
			const needle = q.toLowerCase().trim();
			const filtered = needle
				? stateList.filter(
						(s) =>
							s.name.toLowerCase().includes(needle) ||
							(s.geo_code ?? '').toLowerCase().includes(needle)
					)
				: stateList;
			return Promise.resolve(filtered.slice(0, 50).map((s) => ({ id: s.id, label: s.name })));
		};
	}

	function createStateInitialFetcher(stateList: GeoState[]) {
		return (): Promise<AutocompleteItem[]> => {
			return Promise.resolve(stateList.slice(0, 8).map((s) => ({ id: s.id, label: s.name })));
		};
	}

	function createCityFetcher(cityList: GeoCity[]) {
		return (q: string): Promise<AutocompleteItem[]> => {
			const needle = q.toLowerCase().trim();
			const filtered = needle
				? cityList.filter(
						(c) =>
							c.name.toLowerCase().includes(needle) ||
							(c.geo_code ?? '').toLowerCase().includes(needle)
					)
				: cityList;
			return Promise.resolve(filtered.slice(0, 50).map((c) => ({ id: c.id, label: c.name })));
		};
	}

	function createCityInitialFetcher(cityList: GeoCity[]) {
		return (): Promise<AutocompleteItem[]> => {
			return Promise.resolve(cityList.slice(0, 8).map((c) => ({ id: c.id, label: c.name })));
		};
	}

	// ---- Origin location save handlers ----

	async function saveOrigin(
		patch: {
			origin_country?: string;
			origin_state_id?: string;
			origin_city_id?: string;
		} = {}
	) {
		savingOrigin = true;
		originError = null;
		const res = await patchProfile(patch);
		savingOrigin = false;
		if (res.error) originError = res.error.message;
	}

	async function onOriginCountrySelect(item: AutocompleteItem) {
		originCountry = item.id;
		originStateId = '';
		originCityId = '';
		originStates = [];
		originCities = [];
		await saveOrigin({
			origin_country: item.id,
			origin_state_id: '',
			origin_city_id: ''
		});
		if (item.id) {
			originLoadingStates = true;
			try {
				originStates = await listStates(item.id);
			} finally {
				originLoadingStates = false;
			}
		}
	}

	async function onOriginCountryClear() {
		originCountry = '';
		originStateId = '';
		originCityId = '';
		originStates = [];
		originCities = [];
		await saveOrigin({
			origin_country: '',
			origin_state_id: '',
			origin_city_id: ''
		});
	}

	async function onOriginStateSelect(item: AutocompleteItem) {
		originStateId = item.id;
		originCityId = '';
		originCities = [];
		await saveOrigin({
			origin_state_id: item.id,
			origin_city_id: ''
		});
		if (item.id) {
			originLoadingCities = true;
			try {
				originCities = await listCities(item.id);
			} finally {
				originLoadingCities = false;
			}
		}
	}

	async function onOriginStateClear() {
		originStateId = '';
		originCityId = '';
		originCities = [];
		await saveOrigin({
			origin_state_id: '',
			origin_city_id: ''
		});
	}

	async function onOriginCitySelect(item: AutocompleteItem) {
		originCityId = item.id;
		await saveOrigin({
			origin_city_id: item.id
		});
	}

	async function onOriginCityClear() {
		originCityId = '';
		await saveOrigin({
			origin_city_id: ''
		});
	}

	async function addOriginCity(name: string): Promise<AutocompleteItem | null> {
		if (!originStateId) {
			originError = t('settings.location.search_state', 'Select a state first');
			return null;
		}
		const city = await createCity(originStateId, name);
		if (!city) {
			originError = t('settings.location.save_error', 'Could not add city');
			return null;
		}
		originCities = [...originCities, city];
		return { id: city.id, label: city.name };
	}

	// ---- Destination country save handlers ----

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

	// ---- Current location save handlers ----

	async function onCountrySelect(item: AutocompleteItem) {
		selectedCountryCode = item.id;
		selectedStateId = '';
		currentCityId = '';
		states = [];
		cities = [];
		if (item.id) {
			loadingStates = true;
			try {
				states = await listStates(item.id);
			} finally {
				loadingStates = false;
			}
		}
	}

	function onCountryClear() {
		selectedCountryCode = '';
		selectedStateId = '';
		currentCityId = '';
		states = [];
		cities = [];
	}

	async function onStateSelect(item: AutocompleteItem) {
		selectedStateId = item.id;
		currentCityId = '';
		cities = [];
		if (item.id) {
			loadingCities = true;
			try {
				cities = await listCities(item.id);
			} finally {
				loadingCities = false;
			}
		}
	}

	function onStateClear() {
		selectedStateId = '';
		currentCityId = '';
		cities = [];
	}

	async function onCitySelect(item: AutocompleteItem) {
		currentCityId = item.id;
		await saveLocation(item.id);
	}

	async function onCityClear() {
		currentCityId = '';
		const state = states.find((s) => s.id === selectedStateId);
		if (state) {
			await saveLocation(state.id);
		} else {
			const country = countries.find((c) => c.geo_code === selectedCountryCode);
			if (country) {
				await saveLocation(country.id);
			}
		}
	}

	async function addCurrentCity(name: string): Promise<AutocompleteItem | null> {
		if (!selectedStateId) {
			locationError = t('settings.location.search_state', 'Select a state first');
			return null;
		}
		const city = await createCity(selectedStateId, name);
		if (!city) {
			locationError = t('settings.location.save_error', 'Could not add city');
			return null;
		}
		cities = [...cities, city];
		return { id: city.id, label: city.name };
	}

	/**
	 * Save the selected geo space as the user's current location, then refresh
	 * the resolved location and the manual pickers so the UI stays in sync.
	 */
	async function saveLocation(spaceId: string) {
		savingLocation = true;
		locationError = null;
		const res = await setLocation({ space_id: spaceId });
		if (res) {
			const loc = await getMyLocation();
			currentLocation = loc;
			if (loc) {
				await syncCurrentPickersFromLocation(loc);
			}
		} else {
			locationError = t('settings.location.save_error', 'Could not save location');
		}
		savingLocation = false;
	}

	/** Save the currently-selected country as the user's location. */
	async function saveCountryAsLocation() {
		const country = countries.find((c) => c.geo_code === selectedCountryCode);
		if (country) await saveLocation(country.id);
	}

	/** Save the currently-selected state as the user's location. */
	async function saveStateAsLocation() {
		const state = states.find((s) => s.id === selectedStateId);
		if (state) await saveLocation(state.id);
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
				// Refresh the resolved location from the backend and sync the
				// manual pickers so the detected country/state/city are visible.
				const loc = await getMyLocation();
				currentLocation = loc;
				if (loc) {
					await syncCurrentPickersFromLocation(loc);
				}
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
	<!-- Origin Location -->
	<div class="mb-4 block">
		<span class="mb-1.5 flex items-center gap-1.5 text-sm text-[var(--color-muted)]">
			<Home size={14} /> {t('settings.location.origin_country', 'Origin location')}
		</span>

		<div class="grid grid-cols-1 gap-2 md:grid-cols-2">
			<!-- Country -->
			<div class="relative md:col-span-2">
				<Autocomplete
					placeholder={t('settings.location.search_origin', 'Search origin country…')}
					emptyMessage={t('settings.location.no_countries', 'No countries found')}
					fetcher={countryFetcher}
					initialFetcher={countryInitialFetcher}
					selectedId={originCountry || null}
					selectedLabel={originCountryLabel}
					onSelect={onOriginCountrySelect}
					onClear={onOriginCountryClear}
				/>
				{#if savingOrigin}
					<span class="absolute right-3 top-1/2 -translate-y-1/2"><Loader2 size={12} class="animate-spin" /></span>
				{/if}
			</div>

			<!-- State -->
			{#if originCountry}
				<div class="relative">
					<Autocomplete
						placeholder={t('settings.location.search_origin_state', 'Search origin state/region…')}
						emptyMessage={t('settings.location.no_states', 'No states/regions found')}
						fetcher={createStateFetcher(originStates)}
						initialFetcher={createStateInitialFetcher(originStates)}
						selectedId={originStateId || null}
						selectedLabel={originStateLabel}
						onSelect={onOriginStateSelect}
						onClear={onOriginStateClear}
					/>
					{#if originLoadingStates}
						<span class="absolute right-3 top-1/2 -translate-y-1/2"><Loader2 size={12} class="animate-spin" /></span>
					{/if}
				</div>
			{/if}

			<!-- City -->
			{#if originStateId}
				<div class="relative">
					<Autocomplete
						placeholder={t('settings.location.search_origin_city', 'Search origin city…')}
						emptyMessage={t('settings.location.no_cities', 'No cities found')}
						fetcher={createCityFetcher(originCities)}
						initialFetcher={createCityInitialFetcher(originCities)}
						selectedId={originCityId || null}
						selectedLabel={originCityLabel}
						onSelect={onOriginCitySelect}
						onClear={onOriginCityClear}
						onAdd={addOriginCity}
						addActionLabel={t('settings.location.add_city', 'Add "{query}"')}
					/>
					{#if originLoadingCities}
						<span class="absolute right-3 top-1/2 -translate-y-1/2"><Loader2 size={12} class="animate-spin" /></span>
					{/if}
				</div>
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
		{:else if currentLocationDisplay}
			<p class="mb-3 text-sm text-[var(--color-text)]">{currentLocationDisplay}</p>
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
		<div class="grid grid-cols-1 gap-2 md:grid-cols-2">
			<!-- Country -->
			<div class="md:col-span-2">
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
				{#if selectedCountryCode && !selectedStateId && !currentCityId}
					<button
						type="button"
						disabled={savingLocation}
						onclick={saveCountryAsLocation}
						class="mt-2 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>
						{#if savingLocation}<Loader2 size={12} class="animate-spin" />{:else}<MapPin size={12} />{/if}
						{t('settings.location.use_country', 'Set {country} as my location').replace('{country}', cascadeCountryLabel ?? selectedCountryCode)}
					</button>
				{/if}
			</div>

			<!-- State (only after country is selected and states are loaded) -->
			{#if selectedCountryCode && states.length > 0}
				<div>
					<div class="relative">
						<Autocomplete
							placeholder={t('settings.location.search_state', 'Search state/region…')}
							emptyMessage={t('settings.location.no_states', 'No states/regions found')}
							fetcher={createStateFetcher(states)}
							initialFetcher={createStateInitialFetcher(states)}
							selectedId={selectedStateId || null}
							selectedLabel={cascadeStateLabel}
							onSelect={onStateSelect}
							onClear={onStateClear}
						/>
						{#if loadingStates}
							<span class="absolute right-3 top-1/2 -translate-y-1/2"><Loader2 size={12} class="animate-spin" /></span>
						{/if}
					</div>

					<!-- "Use this state" button — lets the user save at state level
					     when no city is selected (BUG-005). -->
					{#if selectedStateId && !currentCityId}
						<button
							type="button"
							disabled={savingLocation}
							onclick={saveStateAsLocation}
							class="mt-2 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
						>
							{#if savingLocation}<Loader2 size={12} class="animate-spin" />{:else}<MapPin size={12} />{/if}
							{t('settings.location.use_state', 'Set {state} as my location').replace('{state}', cascadeStateLabel ?? selectedStateId)}
						</button>
					{/if}
				</div>
			{/if}

			<!-- City (only after state is selected and cities are loaded) -->
			{#if selectedStateId}
				<div class="relative">
					<Autocomplete
						placeholder={t('settings.location.search_city', 'Search city…')}
						emptyMessage={t('settings.location.no_cities', 'No cities found')}
						fetcher={createCityFetcher(cities)}
						initialFetcher={createCityInitialFetcher(cities)}
						selectedId={currentCityId || null}
						selectedLabel={currentCityLabel}
						onSelect={onCitySelect}
						onClear={onCityClear}
						onAdd={addCurrentCity}
						addActionLabel={t('settings.location.add_city', 'Add "{query}"')}
					/>
					{#if loadingCities}<span class="absolute right-3 top-1/2 -translate-y-1/2"><Loader2 size={12} class="animate-spin" /></span>{/if}
					{#if savingLocation}<span class="absolute right-3 top-1/2 -translate-y-1/2"><Loader2 size={12} class="animate-spin" /></span>{/if}
				</div>
			{/if}
		</div>
	</div>
</SettingsSection>
