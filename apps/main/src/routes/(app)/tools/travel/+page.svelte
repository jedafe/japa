<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { afterNavigate } from '$app/navigation';
	import { enhance } from '$app/forms';
	import {
		Plane,
		Lock,
		Hotel,
		ExternalLink,
		Loader2,
		GitCompare,
		Building2,
		Search
	} from 'lucide-svelte';
	import {
		EmptyState,
		LoadingSkeleton,
		UrlTabs,
		PaginationControls,
		AutocompleteSelect
	} from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import { formatMoney } from '$lib/utils/currency';
	import type {
		TravelFlightResult,
		TravelHotelResult,
		TravelSearchResponse,
		TravelProvider
	} from '$lib/types';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form?: ActionData } = $props();

	const title = $derived(t('tools.travel', 'Travel Search — JapaConnect'));
	const description = $derived(t('tools.travel_desc', 'Search flights and hotels for your move.'));

	const authed = $derived(data.authed);
	const providers = $derived<TravelProvider[]>(data.providers ?? []);
	const providersError = $derived(data.providersError ?? false);
	let disabled = $state(false);

	type Tab = 'flights' | 'hotels' | 'providers';
	const TAB_VALUES: Tab[] = ['flights', 'hotels', 'providers'];
	const TABS = $derived<Array<{ value: Tab; label: string }>>([
		{ value: 'flights', label: t('travel.flights', 'Flights') },
		{ value: 'hotels', label: t('travel.hotels', 'Hotels') },
		{ value: 'providers', label: t('travel.providers', 'Providers') }
	]);
	const initialTabParam = page.url.searchParams.get('tab') as Tab;
	let tab = $state<Tab>(
		TAB_VALUES.includes(initialTabParam) ? initialTabParam : 'flights'
	);

	// Keep the active tab in sync with ?tab= on back/forward and same-page
	// link/goto navigations. UrlTabs writes the param via replaceState for
	// clicks — nav.to is already the destination URL by then, so those
	// firings are no-ops. Using afterNavigate (rather than an $effect on
	// page.url) avoids racing the async page-state update after
	// replaceState, which previously reset the tab to the stale param.
	afterNavigate((nav) => {
		const param = nav.to?.url.searchParams.get('tab') as Tab | null;
		const next = param && TAB_VALUES.includes(param) ? param : 'flights';
		if (next !== tab) tab = next;
	});

	// ── Shared list controls ────────────────────────────────────────────────
	// Numeric pickers are input constraints (not domain data); enum options
	// come from the backend /catalogs endpoint via the server load (Rule 16).
	const PAGE_SIZE_OPTIONS = [10, 25, 50].map((n) => ({ value: String(n), label: String(n) }));
	const numberOptions = (from: number, to: number) =>
		Array.from({ length: to - from + 1 }, (_, i) => {
			const n = from + i;
			return { value: String(n), label: String(n) };
		});

	const MAX_COMPARE = 4;

	const cabinOptions = $derived([
		{ value: '', label: t('travel.any_cabin', 'Any cabin') },
		...(data.cabinClasses ?? []).map((o) => ({ value: o.value, label: o.label }))
	]);

	const providerTypeOptions = $derived([
		{ value: '', label: t('travel.all_types', 'All types') },
		...(data.providerTypes ?? []).map((o) => ({ value: o.value, label: o.label }))
	]);
	const providerTypeLabel = $derived(
		new Map((data.providerTypes ?? []).map((o) => [o.value, o.label]))
	);

	const flightSortOptions = $derived([
		{ value: '', label: t('travel.sort_relevance', 'Best match') },
		{ value: 'price_asc', label: t('travel.sort_price_asc', 'Price: low to high') },
		{ value: 'price_desc', label: t('travel.sort_price_desc', 'Price: high to low') },
		{ value: 'duration', label: t('travel.sort_duration', 'Shortest duration') },
		{ value: 'stops', label: t('travel.sort_stops', 'Fewest stops') },
		{ value: 'departure', label: t('travel.sort_departure', 'Earliest departure') }
	]);
	const hotelSortOptions = $derived([
		{ value: '', label: t('travel.sort_relevance', 'Best match') },
		{ value: 'price_asc', label: t('travel.sort_price_asc', 'Price: low to high') },
		{ value: 'price_desc', label: t('travel.sort_price_desc', 'Price: high to low') },
		{ value: 'rating', label: t('travel.sort_rating', 'Highest rated') }
	]);
	const providerSortOptions = $derived([
		{ value: 'name_asc', label: t('travel.sort_name', 'Name (A–Z)') },
		{ value: 'name_desc', label: t('travel.sort_name_desc', 'Name (Z–A)') },
		{ value: 'newest', label: t('travel.sort_newest', 'Newest first') },
		{ value: 'oldest', label: t('travel.sort_oldest', 'Oldest first') }
	]);

	// Today's date (local) for the date-input min attributes.
	const today = new Date(Date.now() - new Date().getTimezoneOffset() * 60_000)
		.toISOString()
		.slice(0, 10);

	// ── Flight form ─────────────────────────────────────────────────────────
	let flightOrigin = $state('');
	let flightDest = $state('');
	let flightDate = $state('');
	let flightReturn = $state('');
	let flightAdults = $state('1');
	let flightChildren = $state('0');
	let flightCabin = $state('');

	// ── Hotel form ──────────────────────────────────────────────────────────
	let hotelDest = $state('');
	let hotelCheckIn = $state('');
	let hotelCheckOut = $state('');
	let hotelGuests = $state('1');
	let hotelRooms = $state('1');

	// ── Per-tab request state ───────────────────────────────────────────────
	let flightLoading = $state(false);
	let hotelLoading = $state(false);
	let flightError = $state<string | null>(null);
	let hotelError = $state<string | null>(null);
	let flightProviderMissing = $state(false);
	let hotelProviderMissing = $state(false);
	let flightsSearched = $state(false);
	let hotelsSearched = $state(false);
	let flightMeta = $state<{ cache_hit: boolean; search_time_ms: number } | null>(null);
	let hotelMeta = $state<{ cache_hit: boolean; search_time_ms: number } | null>(null);
	let flights = $state<TravelFlightResult[]>([]);
	let hotels = $state<TravelHotelResult[]>([]);

	// ── Flight result list state (sort / filter / page) ─────────────────────
	let flightSort = $state('');
	let flightAirline = $state('');
	let flightPageSize = $state('10');
	let flightPage = $state(1);

	const airlineOptions = $derived([
		{ value: '', label: t('travel.all_airlines', 'All airlines') },
		...[
			...new Set(flights.map((f) => f.airline).filter((a) => !!a))
		]
			.sort((a, b) => a.localeCompare(b))
			.map((a) => ({ value: a, label: a }))
	]);

	const filteredFlights = $derived(
		flightAirline ? flights.filter((f) => f.airline === flightAirline) : flights
	);

	const sortedFlights = $derived.by(() => {
		const arr = [...filteredFlights];
		switch (flightSort) {
			case 'price_asc':
				return arr.sort((a, b) => a.price - b.price);
			case 'price_desc':
				return arr.sort((a, b) => b.price - a.price);
			case 'duration':
				return arr.sort((a, b) => durationMinutes(a.duration) - durationMinutes(b.duration));
			case 'stops':
				return arr.sort((a, b) => a.stops - b.stops);
			case 'departure':
				return arr.sort((a, b) => a.departure_time.localeCompare(b.departure_time));
			default:
				return arr;
		}
	});

	const flightPageSizeNum = $derived(Number(flightPageSize) || 10);
	const flightTotalPages = $derived(
		Math.max(1, Math.ceil(sortedFlights.length / flightPageSizeNum))
	);
	const pagedFlights = $derived(
		sortedFlights.slice(
			(flightPage - 1) * flightPageSizeNum,
			flightPage * flightPageSizeNum
		)
	);

	// ── Hotel result list state ─────────────────────────────────────────────
	let hotelSort = $state('');
	let hotelPageSize = $state('10');
	let hotelPage = $state(1);

	const sortedHotels = $derived.by(() => {
		const arr = [...hotels];
		switch (hotelSort) {
			case 'price_asc':
				return arr.sort((a, b) => a.price - b.price);
			case 'price_desc':
				return arr.sort((a, b) => b.price - a.price);
			case 'rating':
				return arr.sort((a, b) => b.star_rating - a.star_rating);
			default:
				return arr;
		}
	});

	const hotelPageSizeNum = $derived(Number(hotelPageSize) || 10);
	const hotelTotalPages = $derived(Math.max(1, Math.ceil(sortedHotels.length / hotelPageSizeNum)));
	const pagedHotels = $derived(
		sortedHotels.slice((hotelPage - 1) * hotelPageSizeNum, hotelPage * hotelPageSizeNum)
	);

	// ── Providers tab state (search / filter / sort / page / selection) ─────
	let providerQuery = $state('');
	let providerType = $state('');
	let providerSort = $state('name_asc');
	let providerPageSize = $state('10');
	let providerPage = $state(1);
	let savingJCE = $state(false);
	let selectedProviderIds = $state<Set<string>>(new Set());

	const canCompareProviders = $derived(
		selectedProviderIds.size >= 2 && selectedProviderIds.size <= MAX_COMPARE
	);

	const filteredProviders = $derived.by(() => {
		const q = providerQuery.trim().toLowerCase();
		let list = providers;
		if (providerType) list = list.filter((p) => p.provider_type === providerType);
		if (q) {
			list = list.filter((p) =>
				[p.name, p.slug, p.provider_type].some((v) => v.toLowerCase().includes(q))
			);
		}
		return [...list].sort((a, b) => {
			switch (providerSort) {
				case 'name_desc':
					return b.name.localeCompare(a.name);
				case 'newest':
					return b.created_at.localeCompare(a.created_at);
				case 'oldest':
					return a.created_at.localeCompare(b.created_at);
				default:
					return a.name.localeCompare(b.name);
			}
		});
	});

	const providerPageSizeNum = $derived(Number(providerPageSize) || 10);
	const providerTotalPages = $derived(
		Math.max(1, Math.ceil(filteredProviders.length / providerPageSizeNum))
	);
	const pagedProviders = $derived(
		filteredProviders.slice(
			(providerPage - 1) * providerPageSizeNum,
			providerPage * providerPageSizeNum
		)
	);

	function toggleProviderSelection(id: string) {
		const next = new Set(selectedProviderIds);
		if (next.has(id)) {
			next.delete(id);
		} else {
			if (next.size >= MAX_COMPARE) return;
			next.add(id);
		}
		selectedProviderIds = next;
	}

	// ── Search actions ──────────────────────────────────────────────────────
	async function searchFlights() {
		const origin = flightOrigin.trim().toUpperCase();
		const dest = flightDest.trim().toUpperCase();
		if (!origin || !dest || !flightDate) {
			flightError = t('travel.missing_fields', 'Fill in origin, destination and departure date.');
			return;
		}
		if (origin === dest) {
			flightError = t('travel.origin_dest_same', 'Origin and destination must differ.');
			return;
		}
		if (flightReturn && flightReturn < flightDate) {
			flightError = t(
				'travel.return_after_departure',
				'Return date must be on or after the departure date.'
			);
			return;
		}

		flightLoading = true;
		flightError = null;
		flightProviderMissing = false;
		flights = [];
		flightsSearched = false;
		flightMeta = null;

		const params = new URLSearchParams({
			origin,
			dest,
			date: flightDate,
			adults: flightAdults,
			children: flightChildren
		});
		if (flightReturn) params.set('return_date', flightReturn);
		if (flightCabin) params.set('cabin_class', flightCabin);

		const res = await api.get<TravelSearchResponse>(
			`/travel/search/flights?${params.toString()}`
		);
		flightLoading = false;
		flightsSearched = true;
		if (res.error) {
			if (res.error.code === 'travel_disabled') {
				disabled = true;
				return;
			}
			if (res.error.code === 'provider_not_configured') {
				flightProviderMissing = true;
				return;
			}
			flightError = res.error.message;
			return;
		}
		flights = (res.data?.results as TravelFlightResult[] | undefined) ?? [];
		flightMeta = res.data
			? { cache_hit: res.data.cache_hit, search_time_ms: res.data.search_time_ms }
			: null;
		flightPage = 1;
		flightAirline = '';
		flightSort = '';
	}

	async function searchHotels() {
		const dest = hotelDest.trim();
		if (!dest || !hotelCheckIn || !hotelCheckOut) {
			hotelError = t('travel.missing_fields_hotels', 'Fill in destination, check-in and check-out.');
			return;
		}
		if (hotelCheckOut <= hotelCheckIn) {
			hotelError = t('travel.checkout_after_checkin', 'Check-out must be after check-in.');
			return;
		}

		hotelLoading = true;
		hotelError = null;
		hotelProviderMissing = false;
		hotels = [];
		hotelsSearched = false;
		hotelMeta = null;

		const params = new URLSearchParams({
			dest,
			checkin: hotelCheckIn,
			checkout: hotelCheckOut,
			guests: hotelGuests,
			rooms: hotelRooms
		});
		const res = await api.get<TravelSearchResponse>(
			`/travel/search/hotels?${params.toString()}`
		);
		hotelLoading = false;
		hotelsSearched = true;
		if (res.error) {
			if (res.error.code === 'travel_disabled') {
				disabled = true;
				return;
			}
			if (res.error.code === 'provider_not_configured') {
				hotelProviderMissing = true;
				return;
			}
			hotelError = res.error.message;
			return;
		}
		hotels = (res.data?.results as TravelHotelResult[] | undefined) ?? [];
		hotelMeta = res.data
			? { cache_hit: res.data.cache_hit, search_time_ms: res.data.search_time_ms }
			: null;
		hotelPage = 1;
		hotelSort = '';
	}

	// ── Formatting helpers ──────────────────────────────────────────────────
	function durationMinutes(s: string): number {
		if (!s) return Number.MAX_SAFE_INTEGER;
		const iso = /^PT(?:(\d+)H)?(?:(\d+)M)?/i.exec(s);
		if (iso && (iso[1] || iso[2])) return Number(iso[1] ?? 0) * 60 + Number(iso[2] ?? 0);
		const hm = /(\d+)\s*h(?:ours?)?\s*(\d+)?\s*m?/i.exec(s);
		if (hm) return Number(hm[1]) * 60 + Number(hm[2] ?? 0);
		const colon = /(\d+):(\d+)/.exec(s);
		if (colon) return Number(colon[1]) * 60 + Number(colon[2]);
		const mins = /(\d+)\s*m(?:in(?:ute)?s?)?\b/i.exec(s);
		if (mins) return Number(mins[1]);
		return Number.MAX_SAFE_INTEGER;
	}

	function formatProviderType(type: string): string {
		return providerTypeLabel.get(type) ?? type.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
	}

	function formatDate(iso: string): string {
		return iso ? iso.slice(0, 10) : '';
	}

	// Provider names falling back to raw endpoint URLs are a debug aid from
	// the backend, not displayable copy — hide URL-shaped values.
	function displayProvider(p: string): string {
		return p && !p.startsWith('http') ? p : '';
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-4xl">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Plane size={20} /> {t('tools.travel', 'Travel Search')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">{description}</p>
	</div>

	{#if disabled}
		<div class="p-4">
			<EmptyState
				icon={Lock}
				title={t('tools.disabled_title', 'This tool is currently unavailable')}
				body={t('tools.disabled_body', 'Check back soon or contact support.')}
			/>
		</div>
	{:else}
		<!-- Tab switcher -->
		<div class="border-b border-[var(--color-border)] px-4">
			<UrlTabs tabs={TABS} bind:value={tab} urlParam="tab" defaultTab="flights" />
		</div>

		<div class="p-4">
			{#if form?.error}
				<div class="mb-4 rounded-[var(--radius)] bg-red-500/10 p-3 text-sm text-red-500" role="alert">
					{form.message || t('travel.jce_error', 'Could not create comparison')}
				</div>
			{/if}

			{#if tab === 'flights'}
				<form onsubmit={(e) => { e.preventDefault(); searchFlights(); }} class="mb-4 grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
					<div>
						<label for="flight-origin" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('travel.origin', 'Origin (IATA)')}</label>
						<input id="flight-origin" type="text" required maxlength="5" placeholder="LOS" bind:value={flightOrigin} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm uppercase text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]" />
					</div>
					<div>
						<label for="flight-dest" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('travel.destination', 'Destination (IATA)')}</label>
						<input id="flight-dest" type="text" required maxlength="5" placeholder="LHR" bind:value={flightDest} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm uppercase text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]" />
					</div>
					<div>
						<label for="flight-date" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('travel.departure_date', 'Departure date')}</label>
						<input id="flight-date" type="date" required min={today} bind:value={flightDate} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]" />
					</div>
					<div>
						<label for="flight-return" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('travel.return_date', 'Return date (optional)')}</label>
						<input id="flight-return" type="date" min={flightDate || today} bind:value={flightReturn} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]" />
					</div>
					<div>
						<label for="flight-adults" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('travel.adults', 'Adults')}</label>
						<AutocompleteSelect id="flight-adults" options={numberOptions(1, 9)} bind:value={flightAdults} />
					</div>
					<div>
						<label for="flight-children" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('travel.children', 'Children')}</label>
						<AutocompleteSelect id="flight-children" options={numberOptions(0, 8)} bind:value={flightChildren} />
					</div>
					{#if cabinOptions.length > 1}
						<div>
							<label for="flight-cabin" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('travel.cabin_class', 'Cabin class')}</label>
							<AutocompleteSelect id="flight-cabin" options={cabinOptions} bind:value={flightCabin} />
						</div>
					{/if}
					<div class="flex items-end">
						<button type="submit" disabled={flightLoading} class="w-full rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-60">
							{flightLoading ? t('common.searching', 'Searching…') : t('travel.search', 'Search')}
						</button>
					</div>
				</form>

				{#if flightError}
					<div class="mb-4 rounded-[var(--radius)] bg-red-500/10 p-3 text-sm text-red-500" role="alert">
						{flightError}
					</div>
				{/if}

				{#if flightLoading}
					<LoadingSkeleton variant="lines" />
				{:else if flightProviderMissing}
					<EmptyState
						icon={Plane}
						title={t('travel.provider_unavailable', 'No provider configured yet')}
						body={t('travel.provider_unavailable_body', 'Flight search will be available once a provider is configured.')}
					/>
				{:else if sortedFlights.length > 0}
					<div class="mb-3 flex flex-wrap items-center gap-2">
						<p class="mr-auto text-xs text-[var(--color-muted)]">
							{t('travel.results', '{count} results', { count: sortedFlights.length })}
							{#if flightMeta?.cache_hit}· {t('travel.cached', 'cached')}{/if}
						</p>
						<div class="w-36"><AutocompleteSelect options={airlineOptions} bind:value={flightAirline} onValueChange={() => (flightPage = 1)} ariaLabel={t('travel.filter_airline', 'Filter by airline')} placeholder={t('travel.filter_airline', 'Filter by airline')} /></div>
						<div class="w-44"><AutocompleteSelect options={flightSortOptions} bind:value={flightSort} onValueChange={() => (flightPage = 1)} ariaLabel={t('travel.sort_by', 'Sort results')} /></div>
						<div class="w-24"><AutocompleteSelect options={PAGE_SIZE_OPTIONS} bind:value={flightPageSize} onValueChange={() => (flightPage = 1)} ariaLabel={t('travel.per_page', 'Results per page')} /></div>
					</div>
					<div class="space-y-2">
						{#each pagedFlights as f, i (i)}
							<div class="flex items-center justify-between gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
								<div class="min-w-0 flex-1">
									<span class="block text-sm font-medium text-[var(--color-text)]">{f.airline} {f.flight_number}</span>
									<span class="mt-0.5 block text-xs text-[var(--color-muted)]">
										{f.departure_time} → {f.arrival_time} ·
										{#if f.stops === 0}{t('travel.nonstop', 'Nonstop')}{:else}{f.stops} {t('travel.stops', 'stops')}{/if} · {f.duration}
										{#if displayProvider(f.provider)} · {displayProvider(f.provider)}{/if}
									</span>
								</div>
								<div class="flex items-center gap-3">
									<span class="text-sm font-medium text-[var(--color-text)]">{formatMoney(f.price, f.currency, data.currencies)}</span>
									{#if f.booking_url}
										<a href={f.booking_url} target="_blank" rel="noopener sponsored" class="flex items-center gap-1 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-white transition-colors hover:opacity-90">
											{t('travel.book_now', 'Book now')} <ExternalLink size={12} />
										</a>
									{/if}
								</div>
							</div>
						{/each}
					</div>
					{#if flightTotalPages > 1}
						<div class="mt-4 flex justify-center">
							<PaginationControls page={flightPage} totalPages={flightTotalPages} onPageChange={(p) => (flightPage = p)} />
						</div>
					{/if}
				{:else if flightsSearched && !flightError}
					<EmptyState
						icon={Plane}
						title={t('travel.no_flights', 'No flights found')}
						body={t('travel.no_flights_body', 'Try different dates or nearby airports.')}
					/>
				{/if}
			{:else if tab === 'hotels'}
				<form onsubmit={(e) => { e.preventDefault(); searchHotels(); }} class="mb-4 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
					<div class="sm:col-span-2 lg:col-span-1">
						<label for="hotel-dest" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('travel.destination', 'Destination')}</label>
						<input id="hotel-dest" type="text" required placeholder={t('travel.destination_hint', 'City or area')} bind:value={hotelDest} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]" />
					</div>
					<div>
						<label for="hotel-checkin" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('travel.check_in', 'Check-in date')}</label>
						<input id="hotel-checkin" type="date" required min={today} bind:value={hotelCheckIn} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]" />
					</div>
					<div>
						<label for="hotel-checkout" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('travel.check_out', 'Check-out date')}</label>
						<input id="hotel-checkout" type="date" required min={hotelCheckIn || today} bind:value={hotelCheckOut} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]" />
					</div>
					<div>
						<label for="hotel-guests" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('travel.guests', 'Guests')}</label>
						<AutocompleteSelect id="hotel-guests" options={numberOptions(1, 8)} bind:value={hotelGuests} />
					</div>
					<div>
						<label for="hotel-rooms" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('travel.rooms', 'Rooms')}</label>
						<AutocompleteSelect id="hotel-rooms" options={numberOptions(1, 4)} bind:value={hotelRooms} />
					</div>
					<div class="flex items-end">
						<button type="submit" disabled={hotelLoading} class="w-full rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-60">
							{hotelLoading ? t('common.searching', 'Searching…') : t('travel.search', 'Search')}
						</button>
					</div>
				</form>

				{#if hotelError}
					<div class="mb-4 rounded-[var(--radius)] bg-red-500/10 p-3 text-sm text-red-500" role="alert">
						{hotelError}
					</div>
				{/if}

				{#if hotelLoading}
					<LoadingSkeleton variant="lines" />
				{:else if hotelProviderMissing}
					<EmptyState
						icon={Hotel}
						title={t('travel.provider_unavailable', 'No provider configured yet')}
						body={t('travel.provider_unavailable_hotels_body', 'Hotel search will be available once a provider is configured.')}
					/>
				{:else if sortedHotels.length > 0}
					<div class="mb-3 flex flex-wrap items-center gap-2">
						<p class="mr-auto text-xs text-[var(--color-muted)]">
							{t('travel.results', '{count} results', { count: sortedHotels.length })}
							{#if hotelMeta?.cache_hit}· {t('travel.cached', 'cached')}{/if}
						</p>
						<div class="w-44"><AutocompleteSelect options={hotelSortOptions} bind:value={hotelSort} onValueChange={() => (hotelPage = 1)} ariaLabel={t('travel.sort_by', 'Sort results')} /></div>
						<div class="w-24"><AutocompleteSelect options={PAGE_SIZE_OPTIONS} bind:value={hotelPageSize} onValueChange={() => (hotelPage = 1)} ariaLabel={t('travel.per_page', 'Results per page')} /></div>
					</div>
					<div class="space-y-2">
						{#each pagedHotels as h, i (i)}
							<div class="flex items-center justify-between gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
								{#if h.image_url}
									<img src={h.image_url} alt="" loading="lazy" class="hidden h-14 w-20 shrink-0 rounded-[var(--radius)] object-cover sm:block" />
								{/if}
								<div class="min-w-0 flex-1">
									<span class="block text-sm font-medium text-[var(--color-text)]">{h.name}</span>
									<span class="mt-0.5 block text-xs text-[var(--color-muted)]">
										{#if h.star_rating > 0}{'★'.repeat(Math.min(5, Math.floor(h.star_rating)))} · {/if}{h.address}
										{#if h.check_in && h.check_out} · {formatDate(h.check_in)} → {formatDate(h.check_out)}{/if}
										{#if displayProvider(h.provider)} · {displayProvider(h.provider)}{/if}
									</span>
								</div>
								<div class="flex items-center gap-3">
									<span class="text-sm font-medium text-[var(--color-text)]">{formatMoney(h.price, h.currency, data.currencies)}</span>
									{#if h.booking_url}
										<a href={h.booking_url} target="_blank" rel="noopener sponsored" class="flex items-center gap-1 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-white transition-colors hover:opacity-90">
											{t('travel.book_now', 'Book now')} <ExternalLink size={12} />
										</a>
									{/if}
								</div>
							</div>
						{/each}
					</div>
					{#if hotelTotalPages > 1}
						<div class="mt-4 flex justify-center">
							<PaginationControls page={hotelPage} totalPages={hotelTotalPages} onPageChange={(p) => (hotelPage = p)} />
						</div>
					{/if}
				{:else if hotelsSearched && !hotelError}
					<EmptyState
						icon={Hotel}
						title={t('travel.no_hotels', 'No hotels found')}
						body={t('travel.no_hotels_body', 'Try different dates or a nearby destination.')}
					/>
				{/if}
			{:else}
				{#if providersError}
					<EmptyState
						icon={Building2}
						title={t('travel.providers_error', 'Could not load providers')}
						body={t('travel.providers_error_body', 'Refresh the page to try again.')}
					/>
				{:else if providers.length === 0}
					<EmptyState
						icon={Building2}
						title={t('travel.no_providers', 'No travel providers')}
						body={t('travel.no_providers_body', 'Providers will appear here once an admin configures them.')}
					/>
				{:else}
					<!-- Providers toolbar: client-side search, type filter, sort, page size -->
					<div class="mb-3 flex flex-wrap items-center gap-2">
						<div class="relative min-w-40 flex-1">
							<Search size={14} class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" />
							<input
								type="search"
								bind:value={providerQuery}
								oninput={() => (providerPage = 1)}
								placeholder={t('travel.search_providers', 'Search providers…')}
								aria-label={t('travel.search_providers', 'Search providers…')}
								class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-2 pl-9 pr-3 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
							/>
						</div>
						{#if providerTypeOptions.length > 1}
							<div class="w-40"><AutocompleteSelect options={providerTypeOptions} bind:value={providerType} onValueChange={() => (providerPage = 1)} ariaLabel={t('travel.filter_type', 'Filter by type')} /></div>
						{/if}
						<div class="w-40"><AutocompleteSelect options={providerSortOptions} bind:value={providerSort} onValueChange={() => (providerPage = 1)} ariaLabel={t('travel.sort_by', 'Sort results')} /></div>
						<div class="w-24"><AutocompleteSelect options={PAGE_SIZE_OPTIONS} bind:value={providerPageSize} onValueChange={() => (providerPage = 1)} ariaLabel={t('travel.per_page', 'Results per page')} /></div>
					</div>

					{#if filteredProviders.length === 0}
						<EmptyState
							icon={Building2}
							title={t('travel.no_providers_filtered', 'No providers match your filters')}
							body={t('travel.no_providers_filtered_body', 'Clear the search or pick another type.')}
						/>
					{:else}
						<form
							method="POST"
							use:enhance={() => {
								savingJCE = true;
								return async ({ update }) => {
									savingJCE = false;
									await update();
								};
							}}
							class="space-y-3"
						>
							<div class="mb-4 flex items-center justify-between flex-wrap gap-3">
								<p class="text-sm text-[var(--color-muted)]">
									{t('travel.selected', 'Selected')}: {selectedProviderIds.size} / {MAX_COMPARE}
									{#if selectedProviderIds.size > 0}
										<button type="button" onclick={() => (selectedProviderIds = new Set())} class="ml-2 text-xs text-[var(--color-primary)] underline hover:opacity-80">
											{t('travel.clear', 'Clear')}
										</button>
									{/if}
								</p>
								<button
									type="submit"
									disabled={!canCompareProviders || !authed || savingJCE}
									title={authed
										? t('travel.compare_in_jce_hint', 'Open in JCE comparison')
										: t('travel.sign_in_to_compare', 'Sign in to compare')}
									class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed"
								>
									{#if savingJCE}
										<Loader2 size={16} class="animate-spin" />
									{:else}
										<GitCompare size={16} />
									{/if}
									{t('travel.compare_in_jce', 'Compare in JCE')}
								</button>
							</div>

							<!-- Hidden inputs for every selected provider (not just the
							     current page's rows) so filtering/paging can't drop ids. -->
							{#each [...selectedProviderIds] as id (id)}
								<input type="hidden" name="provider_id" value={id} />
							{/each}

							{#each pagedProviders as p (p.id)}
								<label class="flex cursor-pointer items-start gap-3 rounded-[var(--radius-card)] border bg-[var(--color-surface)] p-4 transition-colors {selectedProviderIds.has(p.id) ? 'border-[var(--color-primary)]' : 'border-[var(--color-border)] hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)]'}">
									<input
										type="checkbox"
										checked={selectedProviderIds.has(p.id)}
										onchange={() => toggleProviderSelection(p.id)}
										disabled={selectedProviderIds.size >= MAX_COMPARE && !selectedProviderIds.has(p.id)}
										aria-label={p.name}
										class="mt-1 accent-[var(--color-primary)]"
									/>
									<div class="min-w-0 flex-1">
										<span class="block text-sm font-medium text-[var(--color-text)]">{p.name}</span>
										<span class="mt-0.5 block text-xs text-[var(--color-muted)]">
											{formatProviderType(p.provider_type)} · {p.slug}{#if p.created_at} · {t('travel.since', 'since')} {formatDate(p.created_at)}{/if}
										</span>
									</div>
								</label>
							{/each}
						</form>

						{#if providerTotalPages > 1}
							<div class="mt-4 flex justify-center">
								<PaginationControls page={providerPage} totalPages={providerTotalPages} onPageChange={(p) => (providerPage = p)} />
							</div>
						{/if}
					{/if}
				{/if}
			{/if}
		</div>
	{/if}
</div>
