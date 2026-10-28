<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { afterNavigate } from '$app/navigation';
	import {
		Plane,
		Lock,
		Hotel,
		ExternalLink,
		Search,
		Hourglass
	} from 'lucide-svelte';
	import {
		EmptyState,
		LoadingSkeleton,
		UrlTabs,
		PaginationControls,
		AutocompleteSelect
	} from '$lib/components/ui';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import type { Space } from '$lib/types';
	import { api } from '$lib/api/client';
	import { formatMoney } from '$lib/utils/currency';
	import type {
		TravelFlightResult,
		TravelHotelResult,
		TravelSearchResponse,
		TravelHotelDirectoryItem,
		TravelHotelDirectoryResponse,
		TravelAirlineItem,
		TravelAirlineDirectoryResponse
	} from '$lib/types';
	import type { PageData } from './$types';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('tools.travel', 'Travel Search — {site}'));
	const description = $derived(t('tools.travel_desc', 'Search flights and hotels for your move.'));
	// Section 26 — self-referencing canonical on the clean tool URL (never
	// the query-string form) so tab/filter hits consolidate.
	const canonical = $derived(`${page.url.origin}/tools/travel`);
	// WebApplication JSON-LD (Section 26 — interactive tool surface).
	const travelJsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'WebApplication',
			name: title,
			description,
			url: canonical,
			applicationCategory: 'TravelApplication'
		})
	);

	let disabled = $state(false);

	type Tab = 'flights' | 'hotels' | 'directory' | 'airlines';
	const TAB_VALUES: Tab[] = ['flights', 'hotels', 'directory', 'airlines'];
	const TABS = $derived<Array<{ value: Tab; label: string }>>([
		{ value: 'flights', label: t('travel.flights', 'Flights') },
		{ value: 'hotels', label: t('travel.hotels', 'Hotels') },
		{ value: 'directory', label: t('travel.directory', 'Hotel directory') },
		{ value: 'airlines', label: t('travel.airlines', 'Airlines') }
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

	const cabinOptions = $derived([
		{ value: '', label: t('travel.any_cabin', 'Any cabin') },
		...(data.cabinClasses ?? []).map((o) => ({ value: o.value, label: o.label }))
	]);

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

	// Today's date (local) for the date-input min attributes.
	const today = new Date(Date.now() - new Date().getTimezoneOffset() * 60_000)
		.toISOString()
		.slice(0, 10);

	// ── Flight form ─────────────────────────────────────────────────────────
	// Origin/destination autocomplete over the IATA airport catalog
	// (GET /travel/locations, §67) — real flight locations, not geo spaces.
	// Selecting an item commits its IATA code; freeText keeps raw code entry
	// working — the value is forwarded verbatim to the provider either way.
	interface TravelLocation {
		iata: string;
		name: string;
		city: string;
		country_code: string;
	}

	async function fetchAirportOptions(q: string): Promise<AutocompleteItem[]> {
		const res = await api.get<TravelLocation[]>(
			`/travel/locations?q=${encodeURIComponent(q.trim())}&limit=15`
		);
		return (res.data ?? []).map((a) => ({
			id: a.iata,
			label: `${a.city} (${a.iata})`,
			sublabel: a.name
		}));
	}

	// Hotel destination autocomplete over city-level geo spaces (Rule 16) —
	// hotel provider searches take city names, not airport codes.
	async function fetchCityOptions(q: string): Promise<AutocompleteItem[]> {
		const res = await api.get<Space[]>(
			`/spaces?level=city&q=${encodeURIComponent(q.trim())}&page_size=15`
		);
		return (res.data ?? []).map((c) => ({
			id: c.id,
			label: c.name,
			sublabel: c.geo_code ?? undefined
		}));
	}

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
	let flightQuotaExceeded = $state(false);
	let hotelQuotaExceeded = $state(false);
	let flightsSearched = $state(false);
	let hotelsSearched = $state(false);
	let flightMeta = $state<{ cache_hit: boolean; source?: string; search_time_ms: number; total?: number; page?: number; page_size?: number } | null>(null);
	let hotelMeta = $state<{ cache_hit: boolean; source?: string; search_time_ms: number; total?: number; page?: number; page_size?: number } | null>(null);
	let flights = $state<TravelFlightResult[]>([]);
	let hotels = $state<TravelHotelResult[]>([]);
	let flightTotal = $state<number | null>(null);
	let hotelTotal = $state<number | null>(null);

	function isQuotaError(code?: string): boolean {
		return code === 'travel_quota_exceeded' || code === 'rate_limit_exceeded';
	}

	// ── Flight result list state (sort / filter / page) ─────────────────────
	let flightSort = $state('');
	let flightAirline = $state('');
	let flightPageSize = $state('10');
	let flightPage = $state(1);

	// Airline filter groups by the canonical airline_key so "P4" and
	// "Air Peace" collapse into one option; the label prefers whichever raw
	// provider string reads as a name over a bare code. Offers without a
	// resolved key fall back to their raw value so filtering still works.
	const airlineIdentity = (f: TravelFlightResult) => f.airline_key || f.airline;

	const airlineOptions = $derived.by(() => {
		const labels = new Map<string, string>();
		for (const f of flights) {
			const key = airlineIdentity(f);
			const raw = f.airline || '';
			if (!key || !raw) continue;
			const prev = labels.get(key);
			if (prev === undefined || (isBareCode(prev) && !isBareCode(raw))) {
				labels.set(key, raw);
			}
		}
		return [
			{ value: '', label: t('travel.all_airlines', 'All airlines') },
			...[...labels.entries()]
				.sort((a, b) => a[1].localeCompare(b[1]))
				.map(([value, label]) => ({ value, label }))
		];
	});

	const filteredFlights = $derived(
		flightAirline ? flights.filter((f) => airlineIdentity(f) === flightAirline) : flights
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
		flightQuotaExceeded = false;
		flights = [];
		flightTotal = null;
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
		// Sort/filter/page ride on the request — the backend applies them
		// server-side over the coverage offer set.
		if (flightSort) params.set('sort', flightSort);
		if (flightAirline) params.set('airline', flightAirline);
		params.set('page', String(flightPage));
		params.set('page_size', flightPageSize);

		const res = await api.get<TravelSearchResponse>(
			`/travel/search/flights?${params.toString()}`
		);
		flightLoading = false;
		flightsSearched = true;
		if (res.error) {
			// feature_disabled is what the route gate returns when
			// travel.enabled is off; travel_disabled is the service-level
			// variant — both mean the whole surface is unavailable.
			if (res.error.code === 'travel_disabled' || res.error.code === 'feature_disabled') {
				disabled = true;
				return;
			}
			if (res.error.code === 'provider_not_configured') {
				flightProviderMissing = true;
				return;
			}
			if (isQuotaError(res.error.code)) {
				flightQuotaExceeded = true;
				return;
			}
			flightError = res.error.message;
			return;
		}
		flights = (res.data?.results as TravelFlightResult[] | undefined) ?? [];
		flightTotal = res.data?.total ?? flights.length;
		flightMeta = res.data
			? { cache_hit: res.data.cache_hit, source: res.data.source, search_time_ms: res.data.search_time_ms, total: res.data.total, page: res.data.page, page_size: res.data.page_size }
			: null;
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
		hotelQuotaExceeded = false;
		hotels = [];
		hotelTotal = null;
		hotelsSearched = false;
		hotelMeta = null;

		const params = new URLSearchParams({
			dest,
			checkin: hotelCheckIn,
			checkout: hotelCheckOut,
			guests: hotelGuests,
			rooms: hotelRooms
		});
		// Sort/page ride on the request — the backend applies them
		// server-side over the coverage offer set.
		if (hotelSort) params.set('sort', hotelSort);
		params.set('page', String(hotelPage));
		params.set('page_size', hotelPageSize);
		const res = await api.get<TravelSearchResponse>(
			`/travel/search/hotels?${params.toString()}`
		);
		hotelLoading = false;
		hotelsSearched = true;
		if (res.error) {
			if (res.error.code === 'travel_disabled' || res.error.code === 'feature_disabled') {
				disabled = true;
				return;
			}
			if (res.error.code === 'provider_not_configured') {
				hotelProviderMissing = true;
				return;
			}
			if (isQuotaError(res.error.code)) {
				hotelQuotaExceeded = true;
				return;
			}
			hotelError = res.error.message;
			return;
		}
		hotels = (res.data?.results as TravelHotelResult[] | undefined) ?? [];
		hotelTotal = res.data?.total ?? hotels.length;
		hotelMeta = res.data
			? { cache_hit: res.data.cache_hit, source: res.data.source, search_time_ms: res.data.search_time_ms, total: res.data.total, page: res.data.page, page_size: res.data.page_size }
			: null;
	}

	// ── Hotel directory (ADR-124) — server-paginated browse over
	// /travel/hotels; rows accumulate from searches and the provider feed
	// sync, so results are always local and free to browse.
	let dirCity = $state('');
	let dirQuery = $state('');
	let dirMinStars = $state('');
	let dirPageSize = $state('10');
	let dirPage = $state(1);
	let dirLoading = $state(false);
	let dirError = $state<string | null>(null);
	let dirLoaded = $state(false);
	let dirItems = $state<TravelHotelDirectoryItem[]>([]);
	let dirTotal = $state(0);

	const dirStarOptions = $derived([
		{ value: '', label: t('travel.any_stars', 'Any rating') },
		{ value: '3', label: t('travel.stars_3plus', '3+ stars') },
		{ value: '4', label: t('travel.stars_4plus', '4+ stars') },
		{ value: '5', label: t('travel.stars_5', '5 stars') }
	]);

	const dirTotalPages = $derived(Math.max(1, Math.ceil(dirTotal / (Number(dirPageSize) || 10))));

	async function loadDirectory(nextPage = dirPage) {
		dirLoading = true;
		dirError = null;

		const params = new URLSearchParams({
			page: String(nextPage),
			page_size: dirPageSize
		});
		if (dirCity.trim()) params.set('city', dirCity.trim());
		if (dirQuery.trim()) params.set('q', dirQuery.trim());
		if (dirMinStars) params.set('min_stars', dirMinStars);

		const res = await api.get<TravelHotelDirectoryResponse>(`/travel/hotels?${params.toString()}`);
		dirLoading = false;
		dirLoaded = true;

		if (res.error || !res.data) {
			if (res.error?.code === 'feature_disabled' || res.error?.code === 'travel_disabled') {
				disabled = true;
				return;
			}
			dirError = res.error?.message ?? t('travel.directory_error', 'Could not load the hotel directory.');
			return;
		}

		dirItems = res.data.results ?? [];
		dirTotal = res.data.total ?? 0;
		dirPage = res.data.page ?? nextPage;
	}

	function dirSearch() {
		dirPage = 1;
		loadDirectory(1);
	}

	// First visit to the directory tab loads the first page; filters re-query.
	$effect(() => {
		if (tab === 'directory' && !dirLoaded && !dirLoading) loadDirectory(1);
	});

	// ── Airline directory (ADR-124) — server-paginated browse over
	// /travel/airlines; q matches name, IATA/ICAO code and country. The
	// providers shown per row come from observed offer coverage, not
	// integration config.
	let alQuery = $state('');
	let alPageSize = $state('25');
	let alPage = $state(1);
	let alLoading = $state(false);
	let alError = $state<string | null>(null);
	let alLoaded = $state(false);
	let alItems = $state<TravelAirlineItem[]>([]);
	let alTotal = $state(0);

	const alTotalPages = $derived(Math.max(1, Math.ceil(alTotal / (Number(alPageSize) || 25))));

	async function loadAirlines(nextPage = alPage) {
		alLoading = true;
		alError = null;

		const params = new URLSearchParams({
			page: String(nextPage),
			page_size: alPageSize
		});
		if (alQuery.trim()) params.set('q', alQuery.trim());

		const res = await api.get<TravelAirlineDirectoryResponse>(`/travel/airlines?${params.toString()}`);
		alLoading = false;
		alLoaded = true;

		if (res.error || !res.data) {
			if (res.error?.code === 'feature_disabled' || res.error?.code === 'travel_disabled') {
				disabled = true;
				return;
			}
			alError = res.error?.message ?? t('travel.airlines_error', 'Could not load the airline directory.');
			return;
		}

		alItems = res.data.results ?? [];
		alTotal = res.data.total ?? 0;
		alPage = res.data.page ?? nextPage;
	}

	function alSearch() {
		alPage = 1;
		loadAirlines(1);
	}

	$effect(() => {
		if (tab === 'airlines' && !alLoaded && !alLoading) loadAirlines(1);
	});

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

	function formatDate(iso: string): string {
		return iso ? iso.slice(0, 10) : '';
	}

	// A 2–3 char all-caps code reads as an identifier, not a carrier name —
	// used to prefer "Air Peace" over "P4" as an airline-option label.
	function isBareCode(s: string): boolean {
		return /^[A-Z0-9]{2,3}$/.test(s);
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
	<link rel="canonical" href={page.url.href} />
	<meta property="og:url" content={canonical} />
	{@html safeJsonLdScript(travelJsonLd)}
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
			{#if tab === 'flights'}
				<form onsubmit={(e) => { e.preventDefault(); searchFlights(); }} class="mb-4 grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-4">
					<div>
						<label for="flight-origin" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('travel.origin', 'Origin (IATA)')}
						<HintIcon text={t('tools.travel.origin_hint', 'Departure city or airport code.')}  />
</label>
						<Autocomplete
							id="flight-origin"
							freeText
							placeholder={t('travel.origin_hint', 'City or airport code')}
							emptyMessage={t('common.no_results', 'No results found')}
							fetcher={fetchAirportOptions}
							selectedId={null}
							selectedLabel={flightOrigin}
							onSelect={(item) => (flightOrigin = item.id)}
							onFreeTextBlur={(q) => (flightOrigin = q)}
						/>
					</div>
					<div>
						<label for="flight-dest" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('travel.destination', 'Destination (IATA)')}
						<HintIcon text={t('tools.travel.dest_hint', 'Arrival city or airport code.')}  />
</label>
						<Autocomplete
							id="flight-dest"
							freeText
							placeholder={t('travel.destination_hint_flight', 'City or airport code')}
							emptyMessage={t('common.no_results', 'No results found')}
							fetcher={fetchAirportOptions}
							selectedId={null}
							selectedLabel={flightDest}
							onSelect={(item) => (flightDest = item.id)}
							onFreeTextBlur={(q) => (flightDest = q)}
						/>
					</div>
					<div>
						<label for="flight-date" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('travel.departure_date', 'Departure date')}
						<HintIcon text={t('tools.travel.date_hint', 'When you fly out.')}  />
</label>
						<input id="flight-date" type="date" required min={today} bind:value={flightDate} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]" />
					</div>
					<div>
						<label for="flight-return" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('travel.return_date', 'Return date (optional)')}
						<HintIcon text={t('tools.travel.return_hint', 'Optional — leave blank for one-way.')}  />
</label>
						<input id="flight-return" type="date" min={flightDate || today} bind:value={flightReturn} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]" />
					</div>
					<div>
						<label for="flight-adults" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('travel.adults', 'Adults')}
						<HintIcon text={t('tools.travel.adults_hint', 'Travelers 12 and over.')}  />
</label>
						<AutocompleteSelect id="flight-adults" options={numberOptions(1, 9)} bind:value={flightAdults} />
					</div>
					<div>
						<label for="flight-children" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('travel.children', 'Children')}
						<HintIcon text={t('tools.travel.children_hint', 'Travelers under 12.')}  />
</label>
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
				{:else if flightQuotaExceeded}
					<EmptyState
						icon={Hourglass}
						title={t('travel.quota_exceeded', 'Search quota reached')}
						body={t('travel.quota_exceeded_body', 'Too many provider requests right now. Wait a few minutes and try again.')}
						action={{ label: t('common.retry', 'Try again'), onclick: () => searchFlights() }}
					/>
				{:else if flights.length > 0}
					<div class="mb-3 flex flex-wrap items-center gap-2">
						<p class="me-auto text-xs text-[var(--color-muted)]">
							{t('travel.results', '{count} results', { count: sortedFlights.length })}
							{#if flightMeta?.source === 'stale'}· {t('travel.stale_results', 'results may be outdated')}{:else if flightMeta?.cache_hit}· {t('travel.cached', 'cached')}{/if}
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
											{t('travel.book_now', 'Book now')} <ExternalLink class="rtl:-scale-x-100" size={12} />
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
				<form onsubmit={(e) => { e.preventDefault(); searchHotels(); }} class="mb-4 grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
					<div class="sm:col-span-2 lg:col-span-1">
						<label for="hotel-dest" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('travel.destination', 'Destination')}</label>
						<Autocomplete
							id="hotel-dest"
							freeText
							placeholder={t('travel.destination_hint', 'City or area')}
							emptyMessage={t('common.no_results', 'No results found')}
							fetcher={fetchCityOptions}
							selectedId={null}
							selectedLabel={hotelDest}
							onSelect={(item) => (hotelDest = item.label)}
							onFreeTextBlur={(q) => (hotelDest = q)}
						/>
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
				{:else if hotelQuotaExceeded}
					<EmptyState
						icon={Hourglass}
						title={t('travel.quota_exceeded', 'Search quota reached')}
						body={t('travel.quota_exceeded_body', 'Too many provider requests right now. Wait a few minutes and try again.')}
						action={{ label: t('common.retry', 'Try again'), onclick: () => searchHotels() }}
					/>
				{:else if hotels.length > 0}
					<div class="mb-3 flex flex-wrap items-center gap-2">
						<p class="me-auto text-xs text-[var(--color-muted)]">
							{t('travel.results', '{count} results', { count: sortedHotels.length })}
							{#if hotelMeta?.source === 'stale'}· {t('travel.stale_results', 'results may be outdated')}{:else if hotelMeta?.cache_hit}· {t('travel.cached', 'cached')}{/if}
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
											{t('travel.book_now', 'Book now')} <ExternalLink class="rtl:-scale-x-100" size={12} />
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
			{:else if tab === 'directory'}
				<!-- Ref: .jeos/sections/67-travel-search.md (hotel directory, ADR-124) -->
				<form onsubmit={(e) => { e.preventDefault(); dirSearch(); }} class="mb-4 grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-4">
					<div class="sm:col-span-2 lg:col-span-1">
						<label for="dir-city" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('travel.destination', 'Destination')}</label>
						<Autocomplete
							id="dir-city"
							freeText
							placeholder={t('travel.directory_city_hint', 'City (optional)')}
							emptyMessage={t('common.no_results', 'No results found')}
							fetcher={fetchCityOptions}
							selectedId={null}
							selectedLabel={dirCity}
							onSelect={(item) => { dirCity = item.label; dirSearch(); }}
							onFreeTextBlur={(q) => (dirCity = q)}
						/>
					</div>
					<div>
						<label for="dir-q" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('travel.hotel_name', 'Hotel name')}</label>
						<input id="dir-q" type="search" bind:value={dirQuery} placeholder={t('travel.directory_name_hint', 'Search names…')} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]" />
					</div>
					<div>
						<label for="dir-stars" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('travel.min_stars', 'Minimum rating')}</label>
						<AutocompleteSelect id="dir-stars" options={dirStarOptions} bind:value={dirMinStars} onValueChange={() => dirSearch()} />
					</div>
					<div class="flex items-end">
						<button type="submit" disabled={dirLoading} class="w-full rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-60">
							{dirLoading ? t('common.searching', 'Searching…') : t('travel.browse', 'Browse')}
						</button>
					</div>
				</form>

				{#if dirError}
					<div class="mb-4 rounded-[var(--radius)] bg-red-500/10 p-3 text-sm text-red-500" role="alert">
						{dirError}
					</div>
				{/if}

				{#if dirLoading && dirItems.length === 0}
					<LoadingSkeleton variant="lines" />
				{:else if dirItems.length > 0}
					<div class="mb-3 flex flex-wrap items-center gap-2">
						<p class="me-auto text-xs text-[var(--color-muted)]">
							{t('travel.directory_count', '{count} hotels in the directory', { count: dirTotal })}
						</p>
						<div class="w-24"><AutocompleteSelect options={PAGE_SIZE_OPTIONS} bind:value={dirPageSize} onValueChange={() => dirSearch()} ariaLabel={t('travel.per_page', 'Results per page')} /></div>
					</div>
					<div class="space-y-2">
						{#each dirItems as h, i (i)}
							<div class="flex items-center justify-between gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
								{#if h.image_url}
									<img src={h.image_url} alt="" loading="lazy" class="hidden h-14 w-20 shrink-0 rounded-[var(--radius)] object-cover sm:block" />
								{/if}
								<div class="min-w-0 flex-1">
									<span class="block text-sm font-medium text-[var(--color-text)]">{h.name}</span>
									<span class="mt-0.5 block text-xs text-[var(--color-muted)]">
										{#if (h.star_rating ?? 0) > 0}{'★'.repeat(Math.min(5, Math.floor(h.star_rating ?? 0)))} · {/if}
										{#if h.address}{h.address} · {/if}
										{h.city}
									</span>
								</div>
								{#if h.last_price}
									<span class="text-xs text-[var(--color-muted)]">
										{t('travel.last_seen', 'recent')} · <span class="font-medium text-[var(--color-text)]">{formatMoney(h.last_price, h.last_currency ?? 'USD', data.currencies)}</span>
									</span>
								{/if}
							</div>
						{/each}
					</div>
					{#if dirTotalPages > 1}
						<div class="mt-4 flex justify-center">
							<PaginationControls page={dirPage} totalPages={dirTotalPages} onPageChange={(p) => loadDirectory(p)} />
						</div>
					{/if}
				{:else if dirLoaded && !dirError}
					<EmptyState
						icon={Hotel}
						title={t('travel.no_directory', 'No hotels in the directory yet')}
						body={t('travel.no_directory_body', 'The directory grows as hotel searches run — try a hotel search for your destination first.')}
					/>
				{/if}
			{:else if tab === 'airlines'}
				<!-- Ref: .jeos/sections/67-travel-search.md (airline directory, ADR-124) -->
				<form onsubmit={(e) => { e.preventDefault(); alSearch(); }} class="mb-4 grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-4">
					<div class="sm:col-span-2 lg:col-span-3">
						<label for="al-q" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('travel.airline_name', 'Airline')}</label>
						<input id="al-q" type="search" bind:value={alQuery} placeholder={t('travel.airline_hint', 'Name, IATA code or country…')} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]" />
					</div>
					<div class="flex items-end">
						<button type="submit" disabled={alLoading} class="w-full rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-60">
							{alLoading ? t('common.searching', 'Searching…') : t('travel.browse', 'Browse')}
						</button>
					</div>
				</form>

				{#if alError}
					<div class="mb-4 rounded-[var(--radius)] bg-red-500/10 p-3 text-sm text-red-500" role="alert">
						{alError}
					</div>
				{/if}

				{#if alLoading && alItems.length === 0}
					<LoadingSkeleton variant="lines" />
				{:else if alItems.length > 0}
					<div class="mb-3 flex flex-wrap items-center gap-2">
						<p class="me-auto text-xs text-[var(--color-muted)]">
							{t('travel.airlines_count', '{count} airlines in the directory', { count: alTotal })}
						</p>
						<div class="w-24"><AutocompleteSelect options={PAGE_SIZE_OPTIONS} bind:value={alPageSize} onValueChange={() => alSearch()} ariaLabel={t('travel.per_page', 'Results per page')} /></div>
					</div>
					<div class="space-y-2">
						{#each alItems as a (a.airline_key)}
							<div class="flex items-center justify-between gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
								<div class="min-w-0 flex-1">
									<span class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
										{#if a.iata_code}
											<span class="rounded-[var(--radius)] bg-[var(--color-primary)]/10 px-1.5 py-0.5 font-mono text-xs font-semibold text-[var(--color-primary)]">{a.iata_code}</span>
										{/if}
										{a.name}
									</span>
									<span class="mt-0.5 block text-xs text-[var(--color-muted)]">
										{a.country}
										{#if a.providers.length > 0}
											· {t('travel.bookable_via', 'via')} {a.providers.join(', ')}
										{:else}
											· {t('travel.catalog_only', 'catalog only')}
										{/if}
									</span>
								</div>
								{#if a.observed}
									<span class="shrink-0 rounded-[var(--radius)] bg-green-500/10 px-2 py-1 text-xs font-medium text-green-600 dark:text-green-400">
										{t('travel.bookable', 'bookable')}
									</span>
								{/if}
							</div>
						{/each}
					</div>
					{#if alTotalPages > 1}
						<div class="mt-4 flex justify-center">
							<PaginationControls page={alPage} totalPages={alTotalPages} onPageChange={(p) => loadAirlines(p)} />
						</div>
					{/if}
				{:else if alLoaded && !alError}
					<EmptyState
						icon={Plane}
						title={t('travel.no_airlines', 'No airlines match')}
						body={t('travel.no_airlines_body', 'Try a different name, code or country.')}
					/>
				{/if}
			{/if}
		</div>
	{/if}
</div>
