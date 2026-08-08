<script lang="ts">
	/**
	 * Location settings — ADR-064a.
	 *
	 * Lets the user set their origin country, destination country, and current
	 * location (city-level via cascading picker or browser geolocation).
	 * Origin and destination countries are saved as custom field values via
	 * PATCH /me/profile. Current location is saved via POST /me/location.
	 *
	 * Ref: japaconnect-engineering/adr/ADR-064a-geo-awareness-extension.md
	 *      japaconnect-engineering/sections/06-custom-field-builder-form-builder.md
	 */
	import { Select } from 'bits-ui';
	import { onMount, untrack } from 'svelte';
	import SettingsSection from '$lib/components/hub/SettingsSection.svelte';
	import { patchProfile } from '$lib/api/hub';
	import { listCountries, listStates, listCities, setLocation, getMyLocation } from '$lib/api/geo';
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

	// Browser geolocation state.
	let geoLoading = $state(false);
	let geoError = $state<string | null>(null);

	// Load current location on mount.
	onMount(async () => {
		const loc = await getMyLocation();
		currentLocation = loc;
		loadingLocation = false;
	});

	async function saveOriginCountry(value: string) {
		originCountry = value;
		savingOrigin = true;
		originError = null;
		// Save via PATCH /me/profile — the backend stores origin_country
		// as a custom_field_values entry on the user_profile entity (ADR-064a).
		const res = await patchProfile({ origin_country: value });
		savingOrigin = false;
		if (res.error) originError = res.error.message;
	}

	async function saveDestCountry(value: string) {
		destCountry = value;
		savingDest = true;
		destError = null;
		const res = await patchProfile({ destination_country: value });
		savingDest = false;
		if (res.error) destError = res.error.message;
	}

	async function onCountrySelect(code: string) {
		selectedCountryCode = code;
		selectedStateId = '';
		states = [];
		cities = [];
		if (code) {
			loadingStates = true;
			states = await listStates(code);
			loadingStates = false;
		}
	}

	async function onStateSelect(id: string) {
		selectedStateId = id;
		cities = [];
		if (id) {
			loadingCities = true;
			cities = await listCities(id);
			loadingCities = false;
		}
	}

	async function onCitySelect(city: GeoCity) {
		savingLocation = true;
		locationError = null;
		const res = await setLocation({ space_id: city.id });
		savingLocation = false;
		if (res) {
			currentLocation = {
				space_id: res.space_id,
				city: city.name,
				country: countries.find((c) => c.geo_code === selectedCountryCode)?.name ?? null,
				country_code: selectedCountryCode || null
			};
		} else {
			locationError = t('settings.location.save_error', 'Could not save location');
		}
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
	<label class="mb-4 block">
		<span class="mb-1.5 flex items-center gap-1.5 text-sm text-[var(--color-muted)]">
			<Home size={14} /> {t('settings.location.origin_country', 'Origin country')}
		</span>
		<Select.Root
			type="single"
			value={originCountry}
			onValueChange={(v) => v !== undefined && saveOriginCountry(v ?? '')}
		>
			<Select.Trigger
				class="inline-flex w-full items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				aria-label={t('settings.location.origin_country', 'Origin country')}
			>
				{originCountry
					? countries.find((c) => c.geo_code === originCountry)?.name ?? originCountry
					: t('settings.location.not_set', 'Not set')}
				{#if savingOrigin}<Loader2 size={12} class="animate-spin" />{/if}
			</Select.Trigger>
			<Select.Portal>
				<Select.Content class="z-50 max-h-60 overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg">
					<Select.Viewport>
						<Select.Item
							value=""
							label={t('settings.location.not_set', 'Not set')}
							class="cursor-pointer rounded px-2 py-1.5 text-sm text-[var(--color-text)] data-[highlighted]:bg-[var(--color-surface-subtle)]"
						>
							{t('settings.location.not_set', 'Not set')}
						</Select.Item>
						{#each countries as c (c.id)}
							<Select.Item
								value={c.geo_code}
								label={c.name}
								class="flex cursor-pointer items-center justify-between rounded px-2 py-1.5 text-sm text-[var(--color-text)] data-[highlighted]:bg-[var(--color-surface-subtle)]"
							>
								<span>{c.name}</span>
								<span class="text-xs text-[var(--color-muted)]">{c.geo_code}</span>
							</Select.Item>
						{/each}
					</Select.Viewport>
				</Select.Content>
			</Select.Portal>
		</Select.Root>
		<p class="mt-1.5 text-xs text-[var(--color-muted)]">
			{t('settings.location.origin_hint', 'Where you are from. Used to personalize content for your origin community.')}
		</p>
		{#if originError}<p class="mt-1 text-xs text-[var(--color-danger,#dc2626)]">{originError}</p>{/if}
	</label>

	<!-- Destination Country -->
	<label class="mb-4 block">
		<span class="mb-1.5 flex items-center gap-1.5 text-sm text-[var(--color-muted)]">
			<Plane size={14} /> {t('settings.location.dest_country', 'Destination country')}
		</span>
		<Select.Root
			type="single"
			value={destCountry}
			onValueChange={(v) => v !== undefined && saveDestCountry(v ?? '')}
		>
			<Select.Trigger
				class="inline-flex w-full items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				aria-label={t('settings.location.dest_country', 'Destination country')}
			>
				{destCountry
					? countries.find((c) => c.geo_code === destCountry)?.name ?? destCountry
					: t('settings.location.not_set', 'Not set')}
				{#if savingDest}<Loader2 size={12} class="animate-spin" />{/if}
			</Select.Trigger>
			<Select.Portal>
				<Select.Content class="z-50 max-h-60 overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg">
					<Select.Viewport>
						<Select.Item
							value=""
							label={t('settings.location.not_set', 'Not set')}
							class="cursor-pointer rounded px-2 py-1.5 text-sm text-[var(--color-text)] data-[highlighted]:bg-[var(--color-surface-subtle)]"
						>
							{t('settings.location.not_set', 'Not set')}
						</Select.Item>
						{#each countries as c (c.id)}
							<Select.Item
								value={c.geo_code}
								label={c.name}
								class="flex cursor-pointer items-center justify-between rounded px-2 py-1.5 text-sm text-[var(--color-text)] data-[highlighted]:bg-[var(--color-surface-subtle)]"
							>
								<span>{c.name}</span>
								<span class="text-xs text-[var(--color-muted)]">{c.geo_code}</span>
							</Select.Item>
						{/each}
					</Select.Viewport>
				</Select.Content>
			</Select.Portal>
		</Select.Root>
		<p class="mt-1.5 text-xs text-[var(--color-muted)]">
			{t('settings.location.dest_hint', 'Where you are relocating to or planning to move. Used to surface relevant tools and guides.')}
		</p>
		{#if destError}<p class="mt-1 text-xs text-[var(--color-danger,#dc2626)]">{destError}</p>{/if}
	</label>

	<!-- Current Location -->
	<div class="mt-6 border-t border-[var(--color-border)] pt-4">
		<span class="mb-2 flex items-center gap-1.5 text-sm font-medium text-[var(--color-text)]">
			<MapPin size={14} /> {t('settings.location.current', 'Current location')}
		</span>

		{#if loadingLocation}
			<p class="text-sm text-[var(--color-muted)]"><Loader2 size={14} class="inline animate-spin" /> Loading…</p>
		{:else if currentLocation?.city}
			<p class="mb-3 text-sm text-[var(--color-text)]">
				{currentLocation.city}{#if currentLocation.country}, {currentLocation.country}{/if}
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

		<!-- Cascading picker: Country → State → City -->
		<p class="mb-2 text-xs text-[var(--color-muted)]">
			{t('settings.location.or_pick', 'Or pick your location manually:')}
		</p>
		<div class="space-y-2">
			<Select.Root
				type="single"
				value={selectedCountryCode}
				onValueChange={(v) => v && onCountrySelect(v)}
			>
				<Select.Trigger
					class="inline-flex w-full items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
					aria-label={t('settings.location.select_country', 'Select country')}
				>
					{selectedCountryCode
						? countries.find((c) => c.geo_code === selectedCountryCode)?.name ?? selectedCountryCode
						: t('settings.location.select_country', 'Select country')}
				</Select.Trigger>
				<Select.Portal>
					<Select.Content class="z-50 max-h-48 overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg">
						<Select.Viewport>
							{#each countries as c (c.id)}
								<Select.Item
									value={c.geo_code}
									label={c.name}
									class="cursor-pointer rounded px-2 py-1.5 text-sm text-[var(--color-text)] data-[highlighted]:bg-[var(--color-surface-subtle)]"
								>
									{c.name}
								</Select.Item>
							{/each}
						</Select.Viewport>
					</Select.Content>
				</Select.Portal>
			</Select.Root>

			{#if selectedCountryCode && states.length > 0}
				<Select.Root
					type="single"
					value={selectedStateId}
					onValueChange={(v) => v && onStateSelect(v)}
				>
					<Select.Trigger
						class="inline-flex w-full items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
						aria-label={t('settings.location.select_state', 'Select state/region')}
					>
						{selectedStateId
							? states.find((s) => s.id === selectedStateId)?.name ?? selectedStateId
							: t('settings.location.select_state', 'Select state/region')}
						{#if loadingStates}<Loader2 size={12} class="animate-spin" />{/if}
					</Select.Trigger>
					<Select.Portal>
						<Select.Content class="z-50 max-h-48 overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg">
							<Select.Viewport>
								{#each states as s (s.id)}
									<Select.Item
										value={s.id}
										label={s.name}
										class="cursor-pointer rounded px-2 py-1.5 text-sm text-[var(--color-text)] data-[highlighted]:bg-[var(--color-surface-subtle)]"
									>
										{s.name}
									</Select.Item>
								{/each}
							</Select.Viewport>
						</Select.Content>
					</Select.Portal>
				</Select.Root>
			{/if}

			{#if selectedStateId && cities.length > 0}
				<Select.Root
					type="single"
					value=""
					onValueChange={(v) => {
						const city = cities.find((c) => c.id === v);
						if (city) onCitySelect(city);
					}}
				>
					<Select.Trigger
						class="inline-flex w-full items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
						aria-label={t('settings.location.select_city', 'Select city')}
					>
						{t('settings.location.select_city', 'Select city')}
						{#if loadingCities}<Loader2 size={12} class="animate-spin" />{/if}
						{#if savingLocation}<Loader2 size={12} class="animate-spin" />{/if}
					</Select.Trigger>
					<Select.Portal>
						<Select.Content class="z-50 max-h-48 overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg">
							<Select.Viewport>
								{#each cities as c (c.id)}
									<Select.Item
										value={c.id}
										label={c.name}
										class="cursor-pointer rounded px-2 py-1.5 text-sm text-[var(--color-text)] data-[highlighted]:bg-[var(--color-surface-subtle)]"
									>
										{c.name}
									</Select.Item>
								{/each}
							</Select.Viewport>
						</Select.Content>
					</Select.Portal>
				</Select.Root>
			{/if}
		</div>
	</div>
</SettingsSection>
