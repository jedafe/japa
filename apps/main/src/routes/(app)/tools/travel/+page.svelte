<script lang="ts">
	// Ref: japaconnect-engineering/sections/67-travel-search.md (ADR-085, ADR-124)
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { t } from '#lib/i18n/index.js';
	import { page } from '$app/state';
	import { afterNavigate, goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { Popover } from 'bits-ui';
	import {
		Plane,
		Lock,
		Hotel,
		Luggage,
		ExternalLink,
		Search,
		Hourglass,
		MapPin,
		History,
		ArrowLeftRight,
		ArrowRight,
		ChevronRight,
		Users,
		GraduationCap,
		Briefcase,
		Globe,
		BadgeCheck,
		ShieldCheck,
		Headphones,
		Percent,
		Check,
		Send,
		Star,
		Ticket,
		Building2,
		Gem
	} from 'lucide-svelte';

	import {
		EmptyState,
		LoadingSkeleton,
		PaginationControls,
		AutocompleteSelect
	} from '#lib/components/ui/index.js';
	import Autocomplete from '#lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '#lib/components/create/Autocomplete.svelte';
	import ThemeLogo from '#lib/components/branding/ThemeLogo.svelte';
	import { routeFeatureGateVisible } from '#lib/components/shell/routeFeatureFlags.js';
	import type { Space, MenuItem } from '#lib/types/index.js';
	import { api } from '#lib/api/client.js';
	import { formatMoney } from '#lib/utils/currency.js';
	import type {
		TravelFlightResult,
		TravelSearchResponse,
		TravelHotelDirectoryItem,
		TravelHotelDirectoryResponse,
		TravelAirlineItem,
		TravelAirlineDirectoryResponse
	} from '#lib/types/index.js';
	import type { PageData } from './$types';

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

	// main_footer items for the portal footer — guest-visible + feature-gated,
	// same contract GuestFooter applies to the shell rail footer.
	const footerItems = $derived(
		(data.footerMenu?.items ?? []).filter(
			(item: MenuItem) =>
				item.visibility !== 'authenticated' &&
				routeFeatureGateVisible(item.url, data.features ?? {})
		)
	);

	type Tab = 'flights' | 'hotels';
	const TAB_VALUES: Tab[] = ['flights', 'hotels'];
	// Retired tab values map onto where their content lives now — keeps
	// shared ?tab= links landing somewhere useful.
	const LEGACY_TABS: Record<string, Tab> = {
		bundle: 'flights',
		airlines: 'flights',
		directory: 'hotels'
	};
	const TABS = $derived<Array<{ value: Tab; label: string; icon: typeof Plane }>>([
		{ value: 'flights', label: t('travel.flights', 'Flights'), icon: Plane },
		{ value: 'hotels', label: t('travel.hotels', 'Hotels'), icon: Hotel }
	]);
	function resolveTab(param: string | null): Tab {
		if (param && (TAB_VALUES as string[]).includes(param)) return param as Tab;
		return LEGACY_TABS[param ?? ''] ?? 'flights';
	}
	let tab = $state<Tab>(resolveTab(page.url.searchParams.get('tab')));

	const tabLabel = $derived(TABS.find((x) => x.value === tab)?.label ?? '');

	// Tab strip inside the search card — same ?tab= contract UrlTabs kept
	// (replaceState, no navigation) so deep links and back/forward hold.
	function switchTab(next: Tab) {
		if (next === tab) return;
		tab = next;
		const url = new URL(page.url.href);
		if (next === 'flights') url.searchParams.delete('tab');
		else url.searchParams.set('tab', next);
		goto(`${url.pathname}${url.search}`, { shallow: true, replace: true });
	}

	// Roving tabindex + arrow-key navigation for the tablist (ARIA tabs
	// pattern): the active tab is the only one in the tab order; arrows
	// cycle and move focus, Home/End jump to the ends.
	function onTabKeydown(e: KeyboardEvent, current: Tab) {
		const i = TAB_VALUES.indexOf(current);
		let next: Tab | undefined;
		if (e.key === 'ArrowRight' || e.key === 'ArrowDown') next = TAB_VALUES[(i + 1) % TAB_VALUES.length];
		else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') next = TAB_VALUES[(i - 1 + TAB_VALUES.length) % TAB_VALUES.length];
		else if (e.key === 'Home') next = TAB_VALUES[0];
		else if (e.key === 'End') next = TAB_VALUES[TAB_VALUES.length - 1];
		if (!next || next === current) return;
		e.preventDefault();
		switchTab(next);
		document.getElementById(`travel-tab-${next}`)?.focus();
	}

	// Keep the active tab in sync with ?tab= on back/forward and same-page
	// link/goto navigations. Using afterNavigate (rather than an $effect on
	// page.url) avoids racing the async page-state update after
	// replaceState, which previously reset the tab to the stale param.
	afterNavigate((nav) => {
		if (nav.shallow && nav.type === 'goto') return;

		const next = resolveTab(nav.to?.url.searchParams.get('tab') ?? null);
		if (next !== tab) tab = next;
	});

	// ── Hero copy per tab (§67 mockups) ───────────────────────────────────
	const HERO_IMAGES = {
		flights: '/brand/kigali-airplane-window.jpg',
		hotels: '/brand/kigali-skyline.jpg'
	} as const;

	const hero = $derived.by(() => {
		if (tab === 'hotels') {
			return {
				image: HERO_IMAGES.hotels,
				title: t('travel.hero_hotels_title', 'Find the perfect place to stay.'),
				sub: t('travel.hero_hotels_sub', 'Hotels, apartments, and unique stays for every trip and budget.')
			};
		}
		return {
			image: HERO_IMAGES.flights,
			title: t('travel.hero_flights_title', 'Your journey starts here.'),
			sub: t('travel.hero_flights_sub', 'Search flights, hotels and travel options that take you closer to your next big move.')
		};
	});

	// ── Shared list controls ────────────────────────────────────────────────
	// Page-size is an input constraint (not domain data) — the only
	// hardcoded list allowed by Rule 16.
	const PAGE_SIZE_OPTIONS = [10, 25, 50].map((n) => ({ value: String(n), label: String(n) }));

	const flightSortOptions = $derived([
		{ value: '', label: t('travel.sort_relevance', 'Best match') },
		{ value: 'price_asc', label: t('travel.sort_price_asc', 'Price: low to high') },
		{ value: 'price_desc', label: t('travel.sort_price_desc', 'Price: high to low') },
		{ value: 'duration', label: t('travel.sort_duration', 'Shortest duration') },
		{ value: 'stops', label: t('travel.sort_stops', 'Fewest stops') },
		{ value: 'departure', label: t('travel.sort_departure', 'Earliest departure') }
	]);
	// ── Sample itinerary dates ────────────────────────────────────────────
	// The tool compares provider offers; it doesn't book (§67). Provider
	// searches still require a travel window, so a representative one is
	// applied behind the scenes and disclosed alongside the results rather
	// than collected from the user.
	function offsetDate(days: number): string {
		const d = new Date(Date.now() - new Date().getTimezoneOffset() * 60_000);
		d.setDate(d.getDate() + days);
		return d.toISOString().slice(0, 10);
	}

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
	// Set per search — the auto-picked departure date, shown in the
	// results meta line so the sample window stays honest.
	let flightDate = $state('');

	// ── Per-tab request state ───────────────────────────────────────────────
	let flightSeq = 0; // stale-response guard: last search wins
	let flightLoading = $state(false);
	let flightError = $state<string | null>(null);
	let flightProviderMissing = $state(false);
	let flightQuotaExceeded = $state(false);
	let flightsSearched = $state(false);
	let flightMeta = $state<{ cache_hit: boolean; source?: string } | null>(null);
	let flights = $state<TravelFlightResult[]>([]);

	function isQuotaError(code?: string): boolean {
		return code === 'travel_quota_exceeded' || code === 'rate_limit_exceeded';
	}

	// ── Flight result list state (sort / filter / page — all server-side,
	//    ADR-124) ─────────────────────────────────────────────────────────
	let flightSort = $state('');
	let flightAirline = $state('');
	let flightPageSize = $state('10');
	let flightPage = $state(1);
	// Filtered-set size from the response (`total` is pre-pagination).
	let flightTotal = $state(0);
	// The committed route the displayed offers belong to — page/filter
	// loads replay against it, not the (possibly edited) form fields.
	let flightRoute = $state<{ origin: string; dest: string } | null>(null);

	// Airline filter options are seeded from the search's full unfiltered
	// set — paginated responses carry no facets, so they can't be derived
	// from displayed pages. Rows group by the canonical airline_key so
	// "P4" and "Air Peace" collapse into one option; the label prefers
	// whichever raw provider string reads as a name over a bare code, and
	// it's the label that's sent as the `airline` param (the service
	// resolves names to canonical keys — a raw "iata:BA" key wouldn't
	// match). Offers without a resolved key fall back to their raw value.
	const airlineIdentity = (f: TravelFlightResult) => f.airline_key || f.airline;
	let flightAirlineRows = $state<Array<{ value: string; label: string }>>([]);

	function buildAirlineRows(offers: TravelFlightResult[]) {
		const labels = new Map<string, string>();
		for (const f of offers) {
			const key = airlineIdentity(f);
			const raw = f.airline || '';
			if (!key || !raw) continue;
			const prev = labels.get(key);
			if (prev === undefined || isBareCode(prev) && !isBareCode(raw)) {
				labels.set(key, raw);
			}
		}
		return [...labels.entries()]
			.sort((a, b) => a[1].localeCompare(b[1]))
			.map(([value, label]) => ({ value, label }));
	}

	const airlineOptions = $derived([
		{ value: '', label: t('travel.all_airlines', 'All airlines') },
		...flightAirlineRows
	]);

	const flightPageSizeNum = $derived(Number(flightPageSize) || 10);
	const flightTotalPages = $derived(
		Math.max(1, Math.ceil(flightTotal / flightPageSizeNum))
	);

	// Scroll results into view once a search lands (mockup keeps the
	// marketing rails below — the user shouldn't have to hunt for results).
	let resultsEl = $state<HTMLElement | null>(null);
	let airlineDirEl = $state<HTMLElement | null>(null);
	function scrollToEl(el: HTMLElement | null) {
		requestAnimationFrame(() => el?.scrollIntoView({ behavior: 'smooth', block: 'start' }));
	}
	function scrollToResults() {
		scrollToEl(resultsEl);
	}

	// ── Search actions ──────────────────────────────────────────────────────
	async function searchFlights() {
		const origin = flightOrigin.trim().toUpperCase();
		const dest = flightDest.trim().toUpperCase();
		if (!origin || !dest) {
			flightError = t('travel.missing_fields', 'Fill in origin and destination.');
			return;
		}
		if (origin === dest) {
			flightError = t('travel.origin_dest_same', 'Origin and destination must differ.');
			return;
		}

		// No booking inputs — the provider query runs on a representative
		// departure ~4 weeks out, disclosed in the results meta line.
		flightDate = offsetDate(30);
		flightRoute = { origin, dest };

		flightLoading = true;
		flightError = null;
		flightProviderMissing = false;
		flightQuotaExceeded = false;
		flights = [];
		flightsSearched = false;
		flightMeta = null;
		flightPage = 1;
		flightTotal = 0;
		flightAirlineRows = [];
		// A stale pick from a previous route would silently filter the
		// new offers away.
		flightAirline = '';

		const params = new URLSearchParams({ origin, dest, date: flightDate });
		const seq = ++flightSeq;
		// The full unfiltered set (page_size 0 contract) is fetched once
		// to seed the airline-filter options — paginated responses carry
		// no facets. Display pages then run server-side via loadFlightPage;
		// every presentation shape serves from the same stored coverage,
		// so those requests never hit the provider again.

		const res = await api.get<TravelSearchResponse>(
			`/travel/search/flights?${params.toString()}`
		);
		if (seq !== flightSeq) return; // a newer search owns the state
		flightsSearched = true;
		if (res.error) {
			flightLoading = false;
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
		const all = (res.data?.results as TravelFlightResult[] | undefined) ?? [];
		flightAirlineRows = buildAirlineRows(all);
		flightMeta = res.data
			? { cache_hit: res.data.cache_hit, source: res.data.source }
			: null;
		if (flightSort) {
			// A sort carried over from a previous search needs the
			// server's ordering over the full set — page 1 comes back
			// through the pager.
			await loadFlightPage(1);
		} else {
			// Unsorted/unfiltered: the response's first page-size slice is
			// exactly what the server returns for page 1 — no second
			// round trip.
			flightTotal = res.data?.total ?? all.length;
			flights = all.slice(0, flightPageSizeNum);
			flightPage = 1;
			flightLoading = false;
		}
		recordSearch('flight', `${origin} → ${dest}`, {
			flightOrigin: origin,
			flightDest: dest
		});
		scrollToResults();
	}

	// Server-side presentation (ADR-124): sort, airline filter and page
	// are request params over the committed route's stored coverage.
	async function loadFlightPage(page: number) {
		const route = flightRoute;
		if (!route) return;
		flightLoading = true;

		const params = new URLSearchParams({
			origin: route.origin,
			dest: route.dest,
			date: flightDate,
			page: String(page),
			page_size: String(flightPageSizeNum)
		});
		if (flightSort) params.set('sort', flightSort);
		if (flightAirline) {
			// Send the option's label (an airline name) — the canonical
			// key used as the select value wouldn't resolve server-side.
			const param =
				flightAirlineRows.find((o) => o.value === flightAirline)?.label ?? flightAirline;
			params.set('airline', param);
		}

		const seq = ++flightSeq;
		const res = await api.get<TravelSearchResponse>(
			`/travel/search/flights?${params.toString()}`
		);
		if (seq !== flightSeq) return; // a newer request owns the state
		flightLoading = false;
		if (res.error) {
			flightError = res.error.message;
			return;
		}
		flightError = null;
		flights = (res.data?.results as TravelFlightResult[] | undefined) ?? [];
		flightTotal = res.data?.total ?? 0;
		flightPage = res.data?.page ?? page;
		flightMeta = res.data
			? { cache_hit: res.data.cache_hit, source: res.data.source }
			: flightMeta;
	}

	// ── Hotel directory (ADR-124) — the Hotels tab searches this curated
	// inventory (server-paginated browse over /travel/hotels; rows
	// accumulate from provider searches and the feed sync, so results are
	// always local, free to browse, and need no booking dates).
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
	let dirSeq = 0; // stale-response guard

	const dirStarOptions = $derived([
		{ value: '', label: t('travel.any_stars', 'Any rating') },
		{ value: '3', label: t('travel.stars_3plus', '3+ stars') },
		{ value: '4', label: t('travel.stars_4plus', '4+ stars') },
		{ value: '5', label: t('travel.stars_5', '5 stars') }
	]);

	const dirTotalPages = $derived(Math.max(1, Math.ceil(dirTotal / (Number(dirPageSize) || 10))));

	async function loadDirectory(nextPage = dirPage) {
		const seq = ++dirSeq;
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
		if (seq !== dirSeq) return; // a newer request owns the state
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

	// recordRecent → store the search only once it succeeds — a failed
	// directory call shouldn't pollute My searches.
	async function dirSearch(recordRecent = false) {
		dirPage = 1;
		await loadDirectory(1);
		if (recordRecent && !dirError) {
			recordSearch(
				'hotel',
				dirCity.trim() || dirQuery.trim() || t('travel.all_stays', 'All stays'),
				{
					dirCity: dirCity.trim(),
					dirQuery: dirQuery.trim(),
					dirMinStars
				}
			);
		}
		scrollToResults();
	}

	// First visit to the hotels tab loads the first directory page;
	// filters re-query from there.
	$effect(() => {
		if (tab === 'hotels' && !dirLoaded && !dirLoading) loadDirectory(1);
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
	let alSeq = 0;

	const alTotalPages = $derived(Math.max(1, Math.ceil(alTotal / (Number(alPageSize) || 25))));

	async function loadAirlines(nextPage = alPage) {
		const seq = ++alSeq;
		alLoading = true;
		alError = null;

		const params = new URLSearchParams({
			page: String(nextPage),
			page_size: alPageSize
		});
		if (alQuery.trim()) params.set('q', alQuery.trim());

		const res = await api.get<TravelAirlineDirectoryResponse>(`/travel/airlines?${params.toString()}`);
		if (seq !== alSeq) return;
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
		// The popular-airlines row and compare fallback snapshot the first
		// (observed-first) page once — otherwise directory paging/searching
		// would silently swap what "popular" means.
		if (showcaseItems.length === 0) showcaseItems = alItems.slice(0, 12);
	}

	function alSearch() {
		alPage = 1;
		loadAirlines(1);
	}

	// The airline catalog feeds the flights tab's compare rails — load on
	// first visit (the flights tab is the default landing).
	$effect(() => {
		if (tab === 'flights' && !alLoaded && !alLoading) loadAirlines(1);
	});

	// ── Showcase data (§67 mockups) — the discovery rails under the search
	// card are all derived from the local directories, never hardcoded
	// (Rule 16): destinations + stays come from GET /travel/hotels, airline
	// cards + the side-by-side table from GET /travel/airlines.
	interface Destination {
		key: string;
		city: string;
		image: string | null;
		fromPrice: number | null;
		currency: string;
		stayCount: number | null;
	}

	let showcaseLoaded = $state(false);
	let showcaseLoading = $state(false);
	let destinations = $state<Destination[]>([]);
	let topStays = $state<TravelHotelDirectoryItem[]>([]);
	let cityStayCounts = $state<Map<string, number>>(new Map());

	// Popular-airlines showcase — a stable snapshot of the catalog's first
	// page (the directory's default observed-first ordering), seeded by the
	// first successful load and independent of later paging.
	let showcaseItems = $state<TravelAirlineItem[]>([]);

	// Compare tray — up to four carriers rendered side-by-side over real
	// directory fields (no ratings/baggage fabrication: the catalog carries
	// codes, country and provider coverage, nothing else). Picked items are
	// held, not keys, so paging the directory doesn't lose selections.
	let comparePicked = $state<TravelAirlineItem[]>([]);
	const compareAirlines = $derived(
		comparePicked.length > 0 ? comparePicked : showcaseItems.slice(0, 4)
	);

	function toggleCompare(a: TravelAirlineItem) {
		if (comparePicked.some((x) => x.airline_key === a.airline_key)) {
			comparePicked = comparePicked.filter((x) => x.airline_key !== a.airline_key);
		} else if (comparePicked.length < 4) {
			comparePicked = [...comparePicked, a];
		}
	}

	const titleCase = (s: string) =>
		s.replace(/\w\S*/g, (w) => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase());

	async function loadShowcase() {
		if (showcaseLoaded || showcaseLoading) return;
		showcaseLoading = true;
		const res = await api.get<TravelHotelDirectoryResponse>('/travel/hotels?page_size=60');
		showcaseLoading = false;
		showcaseLoaded = true;
		if (res.error || !res.data) return;

		const items = res.data.results ?? [];
		// Recommended stays: highest-rated properties with a photo.
		topStays = items
			.filter((h) => (h.star_rating ?? 0) >= 4 && !!h.image_url)
			.slice(0, 6);

		// One destination card per city — first photo found, lowest last-seen
		// rate so the "from" price is honest.
		const byCity = new Map<string, Destination>();
		for (const h of items) {
			const key = (h.city ?? '').trim().toLowerCase();
			if (!key) continue;
			const cur =
				byCity.get(key) ??
				({ key, city: titleCase(h.city.trim()), image: null, fromPrice: null, currency: h.last_currency ?? 'USD', stayCount: null } as Destination);
			if (!cur.image && h.image_url) cur.image = h.image_url;
			if (h.last_price != null && (cur.fromPrice == null || h.last_price < cur.fromPrice)) {
				cur.fromPrice = h.last_price;
				cur.currency = h.last_currency ?? 'USD';
			}
			byCity.set(key, cur);
		}
		destinations = [...byCity.values()].slice(0, 8);
	}

	// "Browse by destination" rail — real per-city stay counts from the
	// directory's own totals (page_size=1 keeps the payload tiny). Only
	// the hotels tab renders this rail, so it loads lazily on tab visit.
	// Failed requests are dropped rather than stored as 0 — a transient
	// error must not pin a bogus "0 stays" label on a city for the
	// session, and the per-city guard lets the next visit retry just the
	// missing ones.
	let cityCountsLoading = false;
	async function loadCityStayCounts() {
		if (cityCountsLoading || destinations.length === 0) return;
		const cities = destinations.slice(0, 6).map((d) => d.key);
		if (cities.every((c) => cityStayCounts.has(c))) return;
		cityCountsLoading = true;
		const rows = await Promise.all(
			cities.map(async (city) => {
				const r = await api.get<TravelHotelDirectoryResponse>(
					`/travel/hotels?city=${encodeURIComponent(city)}&page_size=1`
				);
				return [city, r.error || !r.data ? null : (r.data.total ?? 0)] as const;
			})
		);
		cityCountsLoading = false;
		const next = new Map(cityStayCounts);
		for (const [city, count] of rows) {
			if (count != null) next.set(city, count);
		}
		cityStayCounts = next;
	}

	$effect(() => {
		if (tab === 'hotels' && destinations.length > 0) loadCityStayCounts();
	});

	// The showcase feeds every tab's discovery rails — load once on mount.
	onMount(() => {
		loadShowcase();
		loadRecentSearches();
	});

	// ── Popular-searches chips (§67 mockup chip row) — destination cities
	// from the showcase. Clicking one prefills the active tab's destination;
	// the flight variant resolves the city to its primary IATA first so the
	// provider call stays code-shaped.
	const chips = $derived(destinations.slice(0, 7));

	async function pickChip(city: string) {
		if (tab === 'hotels') {
			dirCity = city;
			dirSearch(true);
			return;
		}
		// Flights — resolve the city to its airport code.
		const res = await api.get<TravelLocation[]>(
			`/travel/locations?q=${encodeURIComponent(city)}&limit=1`
		);
		const first = res.data?.[0];
		flightDest = first?.iata ?? city;
	}

	// Prefill helpers used by the discovery cards — they park the value in
	// the right form and hop to the tab that uses it. dirSearch also
	// scrolls the directory section (below the rails) into view.
	function openDestinationInHotels(city: string) {
		dirCity = city;
		switchTab('hotels');
		dirLoaded = true;
		dirSearch(true);
	}

	// "View all" rail links — clear the filters and browse the whole
	// directory on the hotels tab.
	function browseAllStays() {
		dirCity = '';
		dirQuery = '';
		dirMinStars = '';
		switchTab('hotels');
		dirLoaded = true;
		dirSearch();
	}

	// Recommended-stay cards — deep-link into the directory filtered by
	// the property's name so the rail lands on the stay instead of being
	// a dead end.
	function openStayInDirectory(name: string) {
		dirCity = '';
		dirQuery = name;
		dirMinStars = '';
		switchTab('hotels');
		dirLoaded = true;
		dirSearch(true);
	}

	// ── My searches (§67 mockup "My searches" chip) — the last few executed
	// searches kept client-side; picking one restores the form and reruns it.
	interface RecentSearch {
		kind: 'flight' | 'hotel';
		label: string;
		at: number;
		fields: Record<string, string>;
	}
	const RECENT_KEY = 'jc.travel.recent';
	let recentSearches = $state<RecentSearch[]>([]);

	function loadRecentSearches() {
		try {
			const v: unknown = JSON.parse(localStorage.getItem(RECENT_KEY) ?? '[]');
			// A hand-edited/corrupt value must not break the {#each}.
			recentSearches = Array.isArray(v) ? v : [];
		} catch {
			recentSearches = [];
		}
	}

	function recordSearch(kind: RecentSearch['kind'], label: string, fields: Record<string, string>) {
		const next: RecentSearch = { kind, label, at: Date.now(), fields };
		recentSearches = [next, ...recentSearches.filter((r) => r.label !== label)].slice(0, 6);
		try {
			localStorage.setItem(RECENT_KEY, JSON.stringify(recentSearches));
		} catch {
			// storage unavailable — recents stay session-only
		}
	}

	function runRecent(r: RecentSearch) {
		const fields = r.fields ?? {};
		if (r.kind === 'flight') {
			flightOrigin = fields.flightOrigin ?? '';
			flightDest = fields.flightDest ?? '';
			switchTab('flights');
			searchFlights();
		} else {
			// hotelDest: field name used by recents stored before the
			// directory-browse model — keep them replayable.
			dirCity = fields.dirCity ?? fields.hotelDest ?? '';
			dirQuery = fields.dirQuery ?? '';
			dirMinStars = fields.dirMinStars ?? '';
			switchTab('hotels');
			dirLoaded = true;
			dirSearch();
		}
	}

	// ── Newsletter (§62 / §67 mockup band) — feature-flagged subscribe.
	let newsletterEmail = $state('');
	let newsletterStatus = $state<'idle' | 'loading' | 'success' | 'error'>('idle');
	let newsletterMessage = $state('');

	async function subscribeNewsletter(e: Event) {
		e.preventDefault();
		const email = newsletterEmail.trim();
		if (!email) return;
		newsletterStatus = 'loading';
		const res = await api.post<{ message?: string }>('/newsletter/subscribe', { email });
		if (res.error) {
			newsletterStatus = 'error';
			newsletterMessage = res.error.message ?? t('travel.newsletter_error', 'Something went wrong. Please try again.');
			return;
		}
		newsletterStatus = 'success';
		newsletterMessage = res.data?.message ?? t('travel.newsletter_success', "You're on the list — watch your inbox.");
		newsletterEmail = '';
	}

	// ── Formatting helpers ──────────────────────────────────────────────────
	function formatDate(iso: string): string {
		return iso ? iso.slice(0, 10) : '';
	}

	// A 2–3 char all-caps code reads as an identifier, not a carrier name —
	// used to prefer "Air Peace" over "P4" as an airline-option label.
	function isBareCode(s: string): boolean {
		return (/^[A-Z0-9]{2,3}$/).test(s);
	}

	// Provider names falling back to raw endpoint URLs are a debug aid from
	// the backend, not displayable copy — hide URL-shaped values.
	function displayProvider(p: string): string {
		return p && !p.startsWith('http') ? p : '';
	}

	// "View flights" in the compare table — apply the airline filter and
	// land on the results (page top when nothing has been searched yet,
	// so the form is what the user sees next). When the carrier is in the
	// current option rows we select its canonical-key value; otherwise the
	// raw name is kept — it's what the `airline` param resolves against.
	function viewAirlineFlights(a: TravelAirlineItem) {
		const key = a.airline_key || a.name;
		flightAirline = flightAirlineRows.some((o) => o.value === key) ? key : a.name;
		if (flightsSearched) {
			loadFlightPage(1);
			scrollToResults();
		} else {
			window.scrollTo({ top: 0, behavior: 'smooth' });
		}
	}

	function swapFlightEnds() {
		const o = flightOrigin;
		flightOrigin = flightDest;
		flightDest = o;
	}
</script>

<SeoHead title={title} description={description} ogTitle={title} canonical={canonical} jsonLd={travelJsonLd} />

<div class="travel-page">
	{#if disabled}
		<div class="mx-auto w-full max-w-4xl p-4">
			<EmptyState
				icon={Lock}
				title={t('tools.disabled_title', 'This tool is currently unavailable')}
				body={t('tools.disabled_body', 'Check back soon or contact support.')}
			/>
		</div>
	{:else}
		<!-- ═══ Hero (§67 mockups — full-bleed image, breadcrumb, headline) ═══ -->
		<section class="travel-hero">
			<img src={hero.image} alt="" class="travel-hero-img" />
			<div class="travel-hero-overlay"></div>
			<div class="travel-hero-inner">
				<div class="travel-hero-copy">
					<nav class="travel-crumbs" aria-label={t('a11y.breadcrumb', 'Breadcrumb')}>
						<a href="/">{t('nav.home', 'Home')}</a>
						<ChevronRight size={12} aria-hidden="true" />
						<a href="/tools">{t('nav.tools', 'Tools')}</a>
						<ChevronRight size={12} aria-hidden="true" />
						<a href="/tools/travel">{t('tools.travel', 'Travel')}</a>
						{#if tab !== 'flights'}
							<ChevronRight size={12} aria-hidden="true" />
							<span aria-current="page">{tabLabel}</span>
						{/if}
					</nav>
					<h1 class="travel-hero-title">{hero.title}</h1>
					<p class="travel-hero-sub">{hero.sub}</p>
				</div>
			</div>
		</section>

		<!-- ═══ Search card — overlaps the hero, owns the ?tab= strip ═══ -->
		<div class="travel-search-wrap">
			<div class="travel-search-card">
				<div class="travel-search-tabs" role="tablist" aria-label={t('travel.tabs_label', 'Travel search type')} aria-orientation="horizontal">
					{#each TABS as tb (tb.value)}
						{@const Icon = tb.icon}
						<button
							type="button"
							role="tab"
							id="travel-tab-{tb.value}"
							aria-selected={tab === tb.value}
							aria-controls="travel-panel-form travel-panel-results travel-panel-rails"
							tabindex={tab === tb.value ? 0 : -1}
							onkeydown={(e) => onTabKeydown(e, tb.value)}
							onclick={() => switchTab(tb.value)}
							class="travel-tab"
							class:travel-tab-active={tab === tb.value}
						>
							<Icon size={16} />
							<span>{tb.label}</span>
						</button>
					{/each}
					<div class="travel-search-tools">
						<Popover.Root>
							<Popover.Trigger
								class="travel-tool-chip"
								aria-label={t('travel.my_searches', 'My searches')}
							>
								<History size={14} />
								<span class="max-sm:hidden">{t('travel.my_searches', 'My searches')}</span>
							</Popover.Trigger>
							<Popover.Portal>
								<Popover.Content
									class="z-50 w-72 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2 shadow-lg"
									sideOffset={6}
								>
									<p class="px-2 pb-1.5 pt-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
										{t('travel.recent_searches', 'Recent searches')}
									</p>
									{#if recentSearches.length === 0}
										<p class="px-2 py-3 text-sm text-[var(--color-muted)]">
											{t('travel.no_recent', 'No recent searches yet — run one and it will show up here.')}
										</p>
									{:else}
										{#each recentSearches as r (r.label)}
											<button
												type="button"
												onclick={() => runRecent(r)}
												class="flex w-full items-center gap-2 rounded-[var(--radius)] px-2 py-2 text-start text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
											>
												{#if r.kind === 'flight'}<Plane size={14} class="shrink-0 text-[var(--color-muted)]" />{:else}<Hotel size={14} class="shrink-0 text-[var(--color-muted)]" />{/if}
												<span class="truncate">{r.label}</span>
											</button>
										{/each}
									{/if}
								</Popover.Content>
							</Popover.Portal>
						</Popover.Root>
					</div>
				</div>

				<div class="travel-search-body" id="travel-panel-form" role="tabpanel" aria-labelledby="travel-tab-{tab}">
					{#if tab === 'flights'}
						<form onsubmit={(e) => { e.preventDefault(); searchFlights(); }} class="travel-form">
							<div class="travel-field travel-field-wide">
								<label for="flight-origin">{t('travel.origin', 'From')}</label>
								<div class="travel-input">
									<Autocomplete
										id="flight-origin"
										freeText
										placeholder={t('travel.origin_hint', 'City or airport code')}
										emptyMessage={t('common.no_results', 'No results found')}
										fetcher={fetchAirportOptions}
										selectedId={null}
										selectedLabel={flightOrigin}
										onSelect={(item) => flightOrigin = item.id}
										onFreeTextBlur={(q) => flightOrigin = q}
									/>
									<MapPin size={15} class="travel-input-icon" />
								</div>
							</div>
							<button type="button" onclick={swapFlightEnds} class="travel-swap" aria-label={t('travel.swap', 'Swap origin and destination')}>
								<ArrowLeftRight size={15} />
							</button>
							<div class="travel-field travel-field-wide">
								<label for="flight-dest">{t('travel.destination', 'To')}</label>
								<div class="travel-input">
									<Autocomplete
										id="flight-dest"
										freeText
										placeholder={t('travel.destination_hint_flight', 'Where to?')}
										emptyMessage={t('common.no_results', 'No results found')}
										fetcher={fetchAirportOptions}
										selectedId={null}
										selectedLabel={flightDest}
										onSelect={(item) => flightDest = item.id}
										onFreeTextBlur={(q) => flightDest = q}
									/>
									<MapPin size={15} class="travel-input-icon" />
								</div>
							</div>
							<button type="submit" disabled={flightLoading} class="travel-search-btn">
								<Search size={15} />
								{flightLoading ? t('common.searching', 'Searching…') : t('travel.search', 'Search')}
							</button>
						</form>
					{:else}
						<!-- Ref: .jeos/sections/67-travel-search.md (hotel directory, ADR-124) — browse the curated stay inventory; no booking dates needed. -->
						<form onsubmit={(e) => { e.preventDefault(); dirSearch(true); }} class="travel-form">
							<div class="travel-field travel-field-wide">
								<label for="dir-city">{t('travel.where_going', 'Where are you going?')}</label>
								<div class="travel-input">
									<Autocomplete
										id="dir-city"
										freeText
										placeholder={t('travel.destination_hint', 'City or area')}
										emptyMessage={t('common.no_results', 'No results found')}
										fetcher={fetchCityOptions}
										selectedId={null}
										selectedLabel={dirCity}
										onSelect={(item) => {
											dirCity = item.label;
											dirSearch(true);
										}}
										onFreeTextBlur={(q) => dirCity = q}
									/>
									<MapPin size={15} class="travel-input-icon" />
								</div>
							</div>
							<div class="travel-field">
								<label for="dir-q">{t('travel.hotel_name', 'Hotel name')}</label>
								<div class="travel-input">
									<input id="dir-q" type="search" bind:value={dirQuery} placeholder={t('travel.directory_name_hint', 'Search names…')} />
								</div>
							</div>
							<div class="travel-field">
								<label for="dir-stars">{t('travel.min_stars', 'Minimum rating')}</label>
								<AutocompleteSelect id="dir-stars" options={dirStarOptions} bind:value={dirMinStars} onValueChange={() => dirSearch(true)} />
							</div>
							<button type="submit" disabled={dirLoading} class="travel-search-btn">
								<Search size={15} />
								{dirLoading ? t('common.searching', 'Searching…') : t('travel.search', 'Search')}
							</button>
						</form>
					{/if}

					{#if chips.length > 0}
						<div class="travel-chips">
							<span class="travel-chips-label">{t('travel.popular_searches', 'Popular searches:')}</span>
							{#each chips as c (c.key)}
								<button type="button" class="travel-chip" onclick={() => pickChip(c.city)}>{c.city}</button>
							{/each}
						</div>
					{/if}
				</div>
			</div>
		</div>

		<!-- ═══ Results zone — flights only. The hotels directory sits
		     below the discovery rails on that tab instead. ═══ -->
		{#if tab === 'flights'}
			<div class="travel-content" id="travel-panel-results" role="tabpanel" aria-labelledby="travel-tab-{tab}" bind:this={resultsEl}>
				{#if flightError}
					<div class="travel-alert" role="alert">
						{flightError}
						<button type="button" class="travel-alert-retry" onclick={() => searchFlights()}>{t('common.retry', 'Try again')}</button>
					</div>
				{/if}

				{#if flightLoading && flights.length === 0}
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
				{:else if flightsSearched && (flights.length > 0 || flightAirline)}
					<div class="mb-3 flex flex-wrap items-center gap-2">
						<p class="me-auto text-xs text-[var(--color-muted)]">
							{t('travel.results', '{count} results', { count: flightTotal })}
							· {t('travel.fares_for', 'fares for {date}', { date: formatDate(flightDate) })}
							{#if flightMeta?.source === 'stale'}· {t('travel.stale_results', 'results may be outdated')}{:else if flightMeta?.cache_hit}· {t('travel.cached', 'cached')}{/if}
						</p>

						<div class="w-36">
							<AutocompleteSelect
								options={airlineOptions}
								bind:value={flightAirline}
								onValueChange={() => loadFlightPage(1)}
								ariaLabel={t('travel.filter_airline', 'Filter by airline')}
								placeholder={t('travel.filter_airline', 'Filter by airline')}
							/>
						</div>

						<div class="w-44">
							<AutocompleteSelect
								options={flightSortOptions}
								bind:value={flightSort}
								onValueChange={() => loadFlightPage(1)}
								ariaLabel={t('travel.sort_by', 'Sort results')}
							/>
						</div>

						<div class="w-24">
							<AutocompleteSelect
								options={PAGE_SIZE_OPTIONS}
								bind:value={flightPageSize}
								onValueChange={() => loadFlightPage(1)}
								ariaLabel={t('travel.per_page', 'Results per page')}
							/>
						</div>
					</div>

					<div class="space-y-2">
						{#if flights.length === 0}
							<p class="py-8 text-center text-sm text-[var(--color-muted)]">
								{t('travel.no_filtered', 'No offers match this airline — clear the filter to see them all.')}
							</p>
						{/if}
						{#each flights as f, i (i)}
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
											{t('travel.view_deal', 'View deal')} <ExternalLink class="rtl:-scale-x-100" size={12} />
										</a>
									{/if}
								</div>
							</div>
						{/each}
					</div>
					{#if flightTotalPages > 1}
						<div class="mt-4 flex justify-center">
							<PaginationControls
								page={flightPage}
								totalPages={flightTotalPages}
								onPageChange={(p) => { loadFlightPage(p); scrollToResults(); }}
							/>
						</div>
					{/if}
				{:else if flightsSearched && !flightError}
					<EmptyState
						icon={Plane}
						title={t('travel.no_flights', 'No flights found')}
						body={t('travel.no_flights_body', 'Try a different route or nearby airports.')}
					/>
				{/if}
			</div>
		{/if}

		<!-- ═══ Discovery rails (§67 mockups — all data from the directories) ═══ -->
		<div class="travel-content travel-rails" id="travel-panel-rails" role="tabpanel" aria-labelledby="travel-tab-{tab}">
			{#if tab === 'flights'}
				<!-- Popular destinations — derived from the hotel directory -->
				{#if destinations.length > 0}
					<section class="travel-section" aria-labelledby="travel-dest-h">
						<div class="travel-section-head">
							<div>
								<h2 id="travel-dest-h">{t('travel.popular_destinations', 'Popular destinations')}</h2>
								<p>{t('travel.popular_destinations_sub', 'Top picks from travelers like you')}</p>
							</div>
							<button type="button" class="travel-link" onclick={browseAllStays}>
								{t('travel.view_all_destinations', 'View all destinations')} <ArrowRight size={13} />
							</button>
						</div>
						<div class="dest-row">
							{#each destinations as d (d.key)}
								<button type="button" class="dest-card" onclick={() => openDestinationInHotels(d.city)}>
									{#if d.image}
										<img src={d.image} alt="" loading="lazy" />
									{:else}
										<div class="dest-card-noimg"><MapPin size={22} /></div>
									{/if}
									<span class="dest-card-scrim"></span>
									<span class="dest-card-copy">
										<strong>{d.city}</strong>
										{#if d.fromPrice != null}
											<em>{t('travel.stays_from', 'Stays from')} {formatMoney(d.fromPrice, d.currency, data.currencies)}</em>
										{/if}
									</span>
								</button>
							{/each}
						</div>
					</section>
				{/if}

				<!-- Airline compare (mockup 3 — folded under Flights).
				     Catalog data from GET /travel/airlines. -->
				{#if alError}
					<div class="travel-alert" role="alert">
						{alError}
						<button type="button" class="travel-alert-retry" onclick={() => loadAirlines(alPage)}>{t('common.retry', 'Try again')}</button>
					</div>
				{/if}
				{#if alLoading && showcaseItems.length === 0}
					<section class="travel-section"><LoadingSkeleton variant="lines" /></section>
				{/if}
				{#if showcaseItems.length > 0}
					<section class="travel-section" aria-labelledby="pop-airlines-h">
						<div class="travel-section-head">
							<div>
								<h2 id="pop-airlines-h">{t('travel.popular_airlines', 'Popular airlines')}</h2>
								<p>{t('travel.popular_airlines_sub', 'Top airlines trusted by travelers on {site}')}</p>
							</div>
						</div>
						<div class="airline-row">
							{#each showcaseItems as a (a.airline_key)}
								{@const picked = comparePicked.some((x) => x.airline_key === a.airline_key)}
								{@const capped = !picked && comparePicked.length >= 4}
								<article class="airline-card">
									<span class="airline-mark">{a.iata_code ?? a.name.slice(0, 2).toUpperCase()}</span>
									<h4>{a.name}</h4>
									<p class="airline-meta">
										{#if a.iata_code}IATA: {a.iata_code}{/if}
										{#if a.country} · {a.country}{/if}
									</p>
									<p class="airline-via">
										{#if a.providers.length > 0}
											{t('travel.bookable_via', 'via')} {a.providers.join(', ')}
										{:else}
											{t('travel.catalog_only', 'catalog only')}
										{/if}
									</p>
									<button
										type="button"
										class="airline-compare"
										class:airline-compare-on={picked}
										disabled={capped}
										title={capped ? t('travel.compare_max', 'Compare up to 4 airlines') : undefined}
										onclick={() => toggleCompare(a)}
									>
										{picked ? t('travel.comparing', 'Comparing') : t('travel.compare', 'Compare')}
									</button>
								</article>
							{/each}
						</div>
					</section>

					<div class="airlines-compare-layout">
						<section class="travel-section compare-card" aria-labelledby="compare-h">
							<h2 id="compare-h">{t('travel.compare_title', 'Compare airlines side-by-side')}</h2>
							<p class="compare-sub">{t('travel.compare_sub', 'Coverage and routes from observed offers.')}</p>
							<div class="compare-table-wrap">
								<table class="compare-table">
									<thead>
										<tr>
											<th>{t('travel.compare_airline', 'Airline')}</th>
											{#each compareAirlines as a (a.airline_key)}
												<th>
													<span class="airline-mark airline-mark-sm">{a.iata_code ?? a.name.slice(0, 2).toUpperCase()}</span>
													{a.name}
												</th>
											{/each}
										</tr>
									</thead>
									<tbody>
										<tr>
											<td>{t('travel.compare_iata', 'IATA')}</td>
											{#each compareAirlines as a (a.airline_key)}<td>{a.iata_code ?? '—'}</td>{/each}
										</tr>
										<tr>
											<td>{t('travel.compare_icao', 'ICAO')}</td>
											{#each compareAirlines as a (a.airline_key)}<td>{a.icao_code ?? '—'}</td>{/each}
										</tr>
										<tr>
											<td>{t('travel.compare_country', 'Country')}</td>
											{#each compareAirlines as a (a.airline_key)}<td>{a.country || '—'}</td>{/each}
										</tr>
										<tr>
											<td>{t('travel.compare_coverage', 'Bookable via')}</td>
											{#each compareAirlines as a (a.airline_key)}
												<td>{a.providers.length > 0 ? a.providers.join(', ') : '—'}</td>
											{/each}
										</tr>
										<tr>
											<td>{t('travel.compare_status', 'Status')}</td>
											{#each compareAirlines as a (a.airline_key)}
												<td>
													{#if a.observed}
														<span class="compare-bookable">{t('travel.bookable', 'bookable')}</span>
													{:else}
														<span class="text-[var(--color-muted)]">{t('travel.catalog_only', 'catalog only')}</span>
													{/if}
												</td>
											{/each}
										</tr>
										<tr>
											<td>{t('travel.compare_action', 'Action')}</td>
											{#each compareAirlines as a (a.airline_key)}
												<td>
													<button type="button" class="airline-compare" onclick={() => viewAirlineFlights(a)}>
														{t('travel.view_flights', 'View flights')}
													</button>
												</td>
											{/each}
										</tr>
									</tbody>
								</table>
							</div>
						</section>

						{#if destinations.length > 0}
							<aside class="rail-card airlines-rail">
								<h3>{t('travel.trending_destinations', 'Trending destinations')}</h3>
								<p class="rail-sub">{t('travel.trending_destinations_sub', 'Popular places and the best recent stay rates')}</p>
								<ul class="rail-citylist">
									{#each destinations.slice(0, 4) as d (d.key)}
										<li>
											<button type="button" onclick={() => openDestinationInHotels(d.city)}>
												<span class="rail-city-icon"><MapPin size={14} /></span>
												<span class="rail-city-name">{d.city}</span>
												{#if d.fromPrice != null}
													<span class="rail-city-count">{t('travel.from', 'from')} {formatMoney(d.fromPrice, d.currency, data.currencies)}</span>
												{/if}
											</button>
										</li>
									{/each}
								</ul>
							</aside>
						{/if}
					</div>

					<div class="feature-strip">
						<div class="feature-item"><span class="feature-icon f-sky"><Luggage size={16} /></span><div><strong>{t('travel.feature_baggage', 'Baggage made easy')}</strong><span>{t('travel.feature_baggage_sub', 'Compare baggage allowance and avoid surprise fees.')}</span></div></div>
						<div class="feature-item"><span class="feature-icon f-gold"><Ticket size={16} /></span><div><strong>{t('travel.feature_flextix', 'Flexible tickets')}</strong><span>{t('travel.feature_flextix_sub', 'Find airlines with free changes and refunds.')}</span></div></div>
						<div class="feature-item"><span class="feature-icon f-violet"><Star size={16} /></span><div><strong>{t('travel.feature_coverage', 'Wide coverage')}</strong><span>{t('travel.feature_coverage_sub', 'Compare offers across many carriers.')}</span></div></div>
						<div class="feature-item"><span class="feature-icon f-mint"><ShieldCheck size={16} /></span><div><strong>{t('travel.feature_promise', 'Best price promise')}</strong><span>{t('travel.feature_promise_sub', 'We help you find the best fares available.')}</span></div></div>
					</div>
				{/if}

				<!-- Airline directory browser — outside the showcase guard so a
				     0-result search keeps the form reachable. -->
				{#if alLoaded || alLoading}
					<section class="travel-section" aria-labelledby="al-dir-h" bind:this={airlineDirEl}>
						<div class="travel-section-head">
							<div>
								<h2 id="al-dir-h">{t('travel.airline_directory', 'Airline directory')}</h2>
								<p>{t('travel.airline_directory_sub', 'Every carrier in the catalog — search by name, code or country.')}</p>
							</div>
						</div>
						<form onsubmit={(e) => { e.preventDefault(); alSearch(); }} class="travel-form travel-form-inline">
							<div class="travel-field travel-field-wide">
								<label for="al-q">{t('travel.airline_name', 'Airline')}</label>
								<div class="travel-input">
									<input id="al-q" type="search" bind:value={alQuery} placeholder={t('travel.airline_hint', 'Name, IATA code or country…')} />
								</div>
							</div>
							<button type="submit" disabled={alLoading} class="travel-search-btn">
								<Search size={15} />
								{alLoading ? t('common.searching', 'Searching…') : t('travel.browse', 'Browse')}
							</button>
						</form>

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
									{@const picked = comparePicked.some((x) => x.airline_key === a.airline_key)}
									{@const capped = !picked && comparePicked.length >= 4}
									<div class="flex items-center justify-between gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
										<div class="min-w-0 flex-1">
											<span class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
												{#if a.iata_code}
													<span class="rounded-[var(--radius)] bg-[var(--color-primary)]/10 px-1.5 py-0.5 font-mono text-xs font-semibold text-[var(--color-primary-text)]">{a.iata_code}</span>
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
										<div class="flex shrink-0 items-center gap-2">
											{#if a.observed}
												<span class="rounded-[var(--radius)] bg-green-500/10 px-2 py-1 text-xs font-medium text-green-600 dark:text-green-400">
													{t('travel.bookable', 'bookable')}
												</span>
											{/if}
											<button
												type="button"
												class="airline-compare"
												class:airline-compare-on={picked}
												disabled={capped}
												title={capped ? t('travel.compare_max', 'Compare up to 4 airlines') : undefined}
												onclick={() => toggleCompare(a)}
											>
												{picked ? t('travel.comparing', 'Comparing') : t('travel.compare', 'Compare')}
											</button>
										</div>
									</div>
								{/each}
							</div>
							{#if alTotalPages > 1}
								<div class="mt-4 flex justify-center">
									<PaginationControls page={alPage} totalPages={alTotalPages} onPageChange={(p) => { loadAirlines(p); scrollToEl(airlineDirEl); }} />
								</div>
							{/if}
						{:else if !alError}
							<EmptyState
								icon={Plane}
								title={t('travel.no_airlines', 'No airlines match')}
								body={t('travel.no_airlines_body', 'Try a different name, code or country.')}
							/>
						{/if}
					</section>
				{/if}

				<!-- More than a trip — links out to the relocation toolset -->
				<section class="travel-section travel-future" aria-labelledby="travel-future-h">
					<div class="travel-future-copy">
						<h2 id="travel-future-h">{t('travel.future_title', 'More than a trip. A step toward your future.')}</h2>
						<p>{t('travel.future_sub', 'Travel with purpose. Plan your journey with tools built for relocation, study, work and new opportunities.')}</p>
						<a href="/tools" class="travel-outline-btn">{t('travel.explore_tools', 'Explore Relocation Tools')} <ArrowRight size={13} /></a>
					</div>
					<div class="travel-future-grid">
						<a href="/tools/readiness" class="future-item">
							<span class="future-icon"><Briefcase size={18} /></span>
							<strong>{t('travel.future_relocation', 'Relocation Ready')}</strong>
							<span>{t('travel.future_relocation_sub', 'Find routes and info that support your relocation plans.')}</span>
						</a>
						<a href="/scholarships" class="future-item">
							<span class="future-icon"><GraduationCap size={18} /></span>
							<strong>{t('travel.future_study', 'Study Abroad')}</strong>
							<span>{t('travel.future_study_sub', 'Travel to your dream school with ease and confidence.')}</span>
						</a>
						<a href="/jobs" class="future-item">
							<span class="future-icon"><Building2 size={18} /></span>
							<strong>{t('travel.future_work', 'Work Opportunities')}</strong>
							<span>{t('travel.future_work_sub', 'Explore business trips and job opportunities worldwide.')}</span>
						</a>
						<a href="/tools" class="future-item">
							<span class="future-icon"><Globe size={18} /></span>
							<strong>{t('travel.future_migrate', 'Migrate Smarter')}</strong>
							<span>{t('travel.future_migrate_sub', 'Get travel tips and docs guidance for a smooth move.')}</span>
						</a>
					</div>
				</section>

				<!-- Tips / deals / help cards -->
				<section class="travel-section travel-trio" aria-label={t('travel.extras', 'Travel extras')}>
					<article class="trio-card trio-lavender">
						<h3>{t('travel.tips_title', 'Travel tips & guides')}</h3>
						<p>{t('travel.tips_sub', 'Helpful tips to travel smarter and save more.')}</p>
						<ul>
							<li><Check size={13} />{t('travel.tips_1', 'Best time to book flights')}</li>
							<li><Check size={13} />{t('travel.tips_2', 'Visa requirements by country')}</li>
							<li><Check size={13} />{t('travel.tips_3', 'Packing checklists')}</li>
							<li><Check size={13} />{t('travel.tips_4', 'Travel safety tips')}</li>
						</ul>
						<a href="/guides" class="travel-link">{t('travel.tips_cta', 'View all guides')} <ArrowRight size={13} /></a>
					</article>
					<article class="trio-card trio-mint">
						<h3>{t('travel.deals_title', 'Travel deals for you')}</h3>
						<p>{t('travel.deals_sub', 'Handpicked offers on flights and stays.')}</p>
						<ul>
							<li><Ticket size={13} />{t('travel.deals_1', 'Flight and stay deals picked for you')}</li>
							<li><Percent size={13} />{t('travel.deals_2', 'Save on hotels worldwide')}</li>
							<li><GraduationCap size={13} />{t('travel.deals_3', 'Student travel deals')}</li>
						</ul>
						<a href="/deals" class="travel-link">{t('travel.deals_cta', 'See all deals')} <ArrowRight size={13} /></a>
					</article>
					<article class="trio-card trio-peach">
						<h3>{t('travel.help_title', 'Need help planning?')}</h3>
						<p>{t('travel.help_sub', 'Our travel partners are here to help.')}</p>
						<ul>
							<li><Check size={13} />{t('travel.help_1', 'Flight search help')}</li>
							<li><Check size={13} />{t('travel.help_2', 'Hotel & stay assistance')}</li>
							<li><Check size={13} />{t('travel.help_3', 'Itinerary planning')}</li>
						</ul>
						<a href="/services" class="travel-link">{t('travel.help_cta', 'Contact a Travel Expert')} <ArrowRight size={13} /></a>
					</article>
				</section>
			{:else}
				<div class="hotels-layout">
					<div class="hotels-main">
						{#if destinations.length > 0}
							<section class="travel-section" aria-labelledby="hotel-dest-h">
								<div class="travel-section-head">
									<div>
										<h2 id="hotel-dest-h">{t('travel.popular_destinations', 'Popular destinations')}</h2>
										<p>{t('travel.destinations_sub_hotels', 'Top places travelers love')}</p>
									</div>
									<button type="button" class="travel-link" onclick={browseAllStays}>
										{t('travel.view_all_destinations', 'View all destinations')} <ArrowRight size={13} />
									</button>
								</div>
								<div class="dest-row">
									{#each destinations.slice(0, 4) as d (d.key)}
										<button type="button" class="dest-card" onclick={() => openDestinationInHotels(d.city)}>
											{#if d.image}<img src={d.image} alt="" loading="lazy" />{:else}<div class="dest-card-noimg"><MapPin size={22} /></div>{/if}
											<span class="dest-card-scrim"></span>
											<span class="dest-card-copy">
												<strong>{d.city}</strong>
												{#if d.fromPrice != null}<em>{t('travel.stays_from', 'Stays from')} {formatMoney(d.fromPrice, d.currency, data.currencies)}</em>{/if}
											</span>
										</button>
									{/each}
								</div>
							</section>
						{/if}

						{#if topStays.length > 0}
							<section class="travel-section" aria-labelledby="hotel-stays-h">
								<div class="travel-section-head">
									<div>
										<h2 id="hotel-stays-h">{t('travel.recommended_stays', 'Recommended stays')}</h2>
										<p>{t('travel.recommended_stays_sub', 'Top-rated properties for your next trip')}</p>
									</div>
									<button type="button" class="travel-link" onclick={browseAllStays}>
										{t('travel.view_all_stays', 'View all stays')} <ArrowRight size={13} />
									</button>
								</div>
								<div class="stay-grid">
									{#each topStays as h (h.hotel_id || h.name)}
										<button type="button" class="stay-card stay-card-btn" onclick={() => openStayInDirectory(h.name)}>
											{#if (h.star_rating ?? 0) >= 5}<span class="stay-badge">{t('travel.badge_luxury', 'Luxury')}</span>{/if}
											{#if h.image_url}
												<img src={h.image_url} alt="" loading="lazy" class="stay-card-img" />
											{/if}
											<div class="stay-card-body">
												<h4>{h.name}</h4>
												<span class="stay-stars">
													{#each Array(Math.min(5, Math.floor(h.star_rating ?? 0))) as _}<Star size={12} fill="currentColor" />{/each}
												</span>
												<p class="stay-card-meta">
													{#if h.address}{h.address} · {/if}{titleCase(h.city)}
												</p>
												{#if h.last_price}
													<p class="stay-card-price">
														{t('travel.from', 'From')} <strong>{formatMoney(h.last_price, h.last_currency ?? 'USD', data.currencies)}</strong>
														<span>{t('travel.recent_rate', 'recent rate')}</span>
													</p>
												{/if}
											</div>
										</button>
									{/each}
								</div>
							</section>
						{/if}
					</div>

					<aside class="hotels-rail">
						<div class="rail-card">
							<h3>{t('travel.why_book_title', 'Why compare with {site}?')}</h3>
							<ul class="rail-checklist">
								<li><BadgeCheck size={16} /><div><strong>{t('travel.why_book_1', 'Best price finder')}</strong><span>{t('travel.why_book_1_sub', 'We compare provider offers')}</span></div></li>
								<li><ShieldCheck size={16} /><div><strong>{t('travel.why_book_2', 'Private & secure')}</strong><span>{t('travel.why_book_2_sub', 'Your data is safe with us')}</span></div></li>
								<li><Check size={16} /><div><strong>{t('travel.why_book_3', 'Provider-direct rates')}</strong><span>{t('travel.why_book_3_sub', 'Prices reported by providers')}</span></div></li>
								<li><Headphones size={16} /><div><strong>{t('travel.why_book_4', '24/7 customer support')}</strong><span>{t('travel.why_book_4_sub', "We're here to help")}</span></div></li>
							</ul>
						</div>

						{#if !data.user}
							<div class="rail-card rail-member">
								<span class="rail-member-icon"><Gem size={18} /></span>
								<strong>{t('travel.members_title', 'Members save more!')}</strong>
								<p>{t('travel.members_sub', 'Sign in to save your searches and picks.')}</p>
								<a href="/login" class="travel-outline-btn">{t('nav.login', 'Log in')} / {t('nav.signup', 'Sign up')}</a>
							</div>
						{/if}

						{#if cityStayCounts.size > 0}
							<div class="rail-card">
								<h3>{t('travel.browse_by_city', 'Browse by destination')}</h3>
								<ul class="rail-citylist">
									{#each [...cityStayCounts.entries()] as [city, count] (city)}
										<li>
											<button type="button" onclick={() => openDestinationInHotels(city)}>
												<span class="rail-city-icon"><Hotel size={14} /></span>
												<span class="rail-city-name">{titleCase(city)}</span>
												<span class="rail-city-count">{t('travel.stays_count', '{count} stays', { count })}</span>
											</button>
										</li>
									{/each}
								</ul>
							</div>
						{/if}
					</aside>
				</div>

				<!-- Curated stay directory (ADR-124) — the tab's browsable
				     inventory, listed after the discovery rails. -->
				<section class="travel-section" aria-labelledby="dir-h" bind:this={resultsEl}>
					{#if dirError}
						<div class="travel-alert" role="alert">
							{dirError}
							<button type="button" class="travel-alert-retry" onclick={() => loadDirectory(dirPage)}>{t('common.retry', 'Try again')}</button>
						</div>
					{/if}

					{#if dirLoading && dirItems.length === 0}
						<LoadingSkeleton variant="lines" />
					{:else if dirItems.length > 0}
						<div class="travel-section-head">
							<div>
								<h2 id="dir-h">{t('travel.stay_directory', 'Stay directory')}</h2>
								<p>{t('travel.directory_count', '{count} hotels in the directory', { count: dirTotal })}</p>
							</div>
							<div class="w-24"><AutocompleteSelect options={PAGE_SIZE_OPTIONS} bind:value={dirPageSize} onValueChange={() => dirSearch()} ariaLabel={t('travel.per_page', 'Results per page')} /></div>
						</div>
						<div class="stay-grid">
							{#each dirItems as h, i (i)}
								<article class="stay-card">
									{#if h.image_url}
										<img src={h.image_url} alt="" loading="lazy" class="stay-card-img" />
									{:else}
										<div class="stay-card-img stay-card-noimg"><Hotel size={22} /></div>
									{/if}
									<div class="stay-card-body">
										<h4>{h.name}</h4>
										{#if (h.star_rating ?? 0) > 0}
											<span class="stay-stars" aria-label={t('travel.stars_label', '{count} star rating', { count: h.star_rating ?? 0 })}>
												{#each Array(Math.min(5, Math.floor(h.star_rating ?? 0))) as _}<Star size={12} fill="currentColor" />{/each}
											</span>
										{/if}
										<p class="stay-card-meta">
											{#if h.address}{h.address} · {/if}{titleCase(h.city)}
										</p>
										{#if h.last_price}
											<p class="stay-card-price">
												{t('travel.from', 'From')} <strong>{formatMoney(h.last_price, h.last_currency ?? 'USD', data.currencies)}</strong>
												<span>{t('travel.recent_rate', 'recent rate')}</span>
											</p>
										{/if}
									</div>
								</article>
							{/each}
						</div>
						{#if dirTotalPages > 1}
							<div class="mt-4 flex justify-center">
								<PaginationControls page={dirPage} totalPages={dirTotalPages} onPageChange={(p) => { loadDirectory(p); scrollToResults(); }} />
							</div>
						{/if}
					{:else if dirLoaded && !dirError}
						<EmptyState
							icon={Hotel}
							title={t('travel.no_stays', 'No stays match your search')}
							body={t('travel.no_stays_body', 'Try a different city or clear the filters — the directory grows as travelers search.')}
						/>
					{/if}
				</section>

				<div class="feature-strip">
					<div class="feature-item"><span class="feature-icon f-mint"><Check size={16} /></span><div><strong>{t('travel.feature_flexible', 'Flexible options')}</strong><span>{t('travel.feature_flexible_sub', 'Many providers offer free cancellation and flexible fares.')}</span></div></div>
					<div class="feature-item"><span class="feature-icon f-gold"><Ticket size={16} /></span><div><strong>{t('travel.feature_payway', 'Provider-direct deals')}</strong><span>{t('travel.feature_payway_sub', 'Offers and rates straight from travel providers.')}</span></div></div>
					<div class="feature-item"><span class="feature-icon f-sky"><BadgeCheck size={16} /></span><div><strong>{t('travel.feature_reviews', 'Curated stays')}</strong><span>{t('travel.feature_reviews_sub', 'Properties and rates curated from provider data.')}</span></div></div>
					<div class="feature-item"><span class="feature-icon f-violet"><Gem size={16} /></span><div><strong>{t('travel.feature_rewards', 'Smart savings')}</strong><span>{t('travel.feature_rewards_sub', 'Compare rates to find the best-value options.')}</span></div></div>
				</div>
			{/if}
		</div>

		<!-- ═══ Newsletter band + trust strip + footer (§67 mockups) ═══ -->
		<div class="travel-content">
			{#if data.features?.['feature.newsletter']}
				<section class="newsletter-band" aria-labelledby="travel-news-h">
					<div class="newsletter-copy">
						<span class="newsletter-icon"><Send size={22} /></span>
						<div>
							<h2 id="travel-news-h">{t('travel.newsletter_title', 'Get travel inspiration & deals')}</h2>
							<p>{t('travel.newsletter_sub', 'Subscribe to get the best travel deals, tips and inspiration straight to your inbox.')}</p>
						</div>
					</div>
					<form class="newsletter-form" onsubmit={subscribeNewsletter}>
						<input
							type="email"
							required
							bind:value={newsletterEmail}
							placeholder={t('travel.newsletter_placeholder', 'Enter your email address')}
							aria-label={t('travel.newsletter_placeholder', 'Enter your email address')}
						/>
						<button type="submit" disabled={newsletterStatus === 'loading'}>
							{newsletterStatus === 'loading' ? t('common.loading', 'Loading...') : t('travel.newsletter_cta', 'Subscribe')}
						</button>
					</form>
					{#if newsletterMessage}
						<p class="newsletter-msg" class:newsletter-msg-err={newsletterStatus === 'error'} role="status">{newsletterMessage}</p>
					{/if}
				</section>
			{/if}

			<div class="trust-strip">
				<div class="trust-item"><BadgeCheck size={20} /><div><strong>{t('travel.trust_price', 'Best price finder')}</strong><span>{t('travel.trust_price_sub', 'We compare provider offers')}</span></div></div>
				<div class="trust-item"><Lock size={20} /><div><strong>{t('travel.trust_secure', 'Private & secure')}</strong><span>{t('travel.trust_secure_sub', 'Your data is safe with us')}</span></div></div>
				<div class="trust-item"><Headphones size={20} /><div><strong>{t('travel.trust_support', '24/7 Support')}</strong><span>{t('travel.trust_support_sub', "We're here to help")}</span></div></div>
				<div class="trust-item"><Users size={20} /><div><strong>{t('travel.trust_travelers', 'Trusted by Travelers')}</strong><span>{t('travel.trust_travelers_sub', 'Join thousands globally')}</span></div></div>
			</div>
		</div>

		<footer class="travel-footer">
			<div class="travel-footer-inner">
				<div class="travel-footer-brand">
					<ThemeLogo alt={t('app.name')} class="h-8 w-auto max-w-[110px] object-contain" />
					<p>{t('travel.footer_tagline', 'Your all-in-one platform for relocation, opportunities and a better future. Plan. Move. Thrive.')}</p>
				</div>
				<!-- main_footer menu, same guest-visible + feature-gated filter as
				     GuestFooter (ADR-054). -->
				{#if footerItems.length > 0}
					<nav class="travel-footer-links" aria-label={t('a11y.footer_nav', 'Footer')}>
						{#each footerItems as item (item.id)}
							<a href={item.url} target={item.target === '_blank' ? '_blank' : undefined} rel={item.target === '_blank' ? 'noopener noreferrer' : undefined}>{item.label}</a>
						{/each}
					</nav>
				{/if}
				<p class="travel-footer-copy">© {new Date(data.serverNow ?? Date.now()).getFullYear()} {t('app.name')}. {t('landing.footer_rights', 'All rights reserved.')}</p>
			</div>
		</footer>
	{/if}
</div>

<style>
	/* §67 travel portal — page-local styles on the shared design tokens. */
	.travel-page {
		min-height: 100%;
		background: var(--color-bg);
	}

	/* ── Hero ─────────────────────────────────────────────────────────── */
	.travel-hero {
		position: relative;
		min-height: 21rem;
		display: flex;
		align-items: stretch;
		overflow: hidden;
	}
	.travel-hero-img {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.travel-hero-overlay {
		position: absolute;
		inset: 0;
		background: linear-gradient(
			to bottom,
			rgb(8 8 20 / 0.55) 0%,
			rgb(8 8 20 / 0.25) 45%,
			rgb(8 8 20 / 0.65) 100%
		);
	}
	.travel-hero-inner {
		position: relative;
		z-index: 1;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 2rem;
		width: 100%;
		max-width: 72rem;
		margin: 0 auto;
		padding: 2rem 1.5rem 5rem;
	}
	.travel-hero-copy {
		max-width: 34rem;
	}
	.travel-crumbs {
		display: flex;
		align-items: center;
		gap: 0.375rem;
		font-size: 0.8125rem;
		color: rgb(255 255 255 / 0.85);
		margin-bottom: 1rem;
	}
	.travel-crumbs a {
		color: rgb(255 255 255 / 0.85);
		text-decoration: none;
	}
	.travel-crumbs a:hover {
		color: #fff;
		text-decoration: underline;
	}
	.travel-hero-title {
		font-size: clamp(2rem, 4.5vw, 3.25rem);
		font-weight: 800;
		line-height: 1.1;
		letter-spacing: -0.02em;
		color: #fff;
	}
	.travel-hero-sub {
		margin-top: 0.75rem;
		font-size: 1rem;
		line-height: 1.5;
		color: rgb(255 255 255 / 0.9);
		max-width: 30rem;
	}
	/* ── Search card ──────────────────────────────────────────────────── */
	.travel-search-wrap {
		position: relative;
		z-index: 5;
		max-width: 72rem;
		margin: -3.5rem auto 0;
		padding: 0 1.5rem;
	}
	.travel-search-card {
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: 1rem;
		box-shadow: 0 12px 32px rgb(0 0 0 / 0.12);
		overflow: visible;
	}
	.travel-search-tabs {
		display: flex;
		align-items: center;
		gap: 0.25rem;
		padding: 0 0.75rem;
		border-bottom: 1px solid var(--color-border);
		overflow-x: auto;
	}
	.travel-tab {
		display: inline-flex;
		align-items: center;
		gap: 0.4375rem;
		padding: 0.875rem 0.875rem;
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--color-muted);
		border-bottom: 2px solid transparent;
		margin-bottom: -1px;
		white-space: nowrap;
		cursor: pointer;
		transition: color 0.15s;
	}
	.travel-tab:hover {
		color: var(--color-text);
	}
	.travel-tab-active {
		color: var(--color-primary-text);
		border-bottom-color: var(--color-primary);
	}
	.travel-search-tools {
		margin-inline-start: auto;
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding-inline-start: 0.75rem;
	}
	.travel-search-tools :global(.travel-tool-chip) {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		font-size: 0.8125rem;
		font-weight: 500;
		color: var(--color-text-subtle);
		white-space: nowrap;
		cursor: pointer;
	}
	.travel-search-body {
		padding: 1.25rem 1.5rem 1.5rem;
	}
	.travel-form {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(9.5rem, 1fr));
		gap: 0.75rem;
		align-items: end;
	}
	.travel-field {
		min-width: 0;
	}
	.travel-field-wide {
		/* 1/-1 spans however many columns the auto-fit grid actually
		   produced — span 2 would overflow onto an implicit track on
		   viewports narrow enough for a single column. */
		grid-column: 1 / -1;
	}
	.travel-field > label {
		display: block;
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--color-muted);
		margin-bottom: 0.375rem;
	}
	.travel-input {
		position: relative;
		display: flex;
		align-items: center;
		min-height: 2.75rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
	}
	.travel-input:focus-within {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}
	.travel-input :global(input),
	.travel-input :global(.autocomplete input) {
		width: 100%;
		min-width: 0;
		height: 2.75rem;
		padding: 0 2rem 0 0.75rem;
		border: none;
		background: transparent;
		font-size: 0.875rem;
		color: var(--color-text);
		outline: none;
	}
	/* The shared Autocomplete renders its own chrome — strip it so the
	   .travel-input shell owns border/focus for the mockup's boxed look. */
	.travel-input :global(.autocomplete) {
		width: 100%;
	}
	.travel-input :global(.autocomplete input) {
		border: none !important;
		box-shadow: none !important;
	}
	.travel-input :global(.travel-input-icon) {
		position: absolute;
		inset-inline-end: 0.625rem;
		color: var(--color-muted);
		pointer-events: none;
	}
	.travel-swap {
		display: grid;
		place-items: center;
		width: 2.75rem;
		height: 2.75rem;
		border-radius: var(--radius);
		border: 1px solid var(--color-border);
		background: var(--color-surface);
		color: var(--color-muted);
		cursor: pointer;
		transition: color 0.15s, border-color 0.15s;
		justify-self: center;
	}
	.travel-swap:hover {
		color: var(--color-primary-text);
		border-color: var(--color-primary);
	}
	.travel-search-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 0.4375rem;
		min-height: 2.75rem;
		padding: 0 1.25rem;
		border-radius: var(--radius);
		background: var(--color-primary);
		color: var(--color-primary-contrast);
		font-size: 0.875rem;
		font-weight: 600;
		white-space: nowrap;
		cursor: pointer;
		transition: opacity 0.15s, background 0.15s;
	}
	.travel-search-btn:hover {
		background: var(--color-primary-hover);
	}
	.travel-search-btn:disabled {
		opacity: 0.6;
	}
	.travel-chips {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.5rem;
		margin-top: 1rem;
	}
	.travel-chips-label {
		font-size: 0.8125rem;
		color: var(--color-muted);
	}
	.travel-chip {
		padding: 0.375rem 0.875rem;
		border-radius: 999px;
		border: 1px solid var(--color-border);
		background: var(--color-surface-subtle);
		font-size: 0.8125rem;
		font-weight: 500;
		color: var(--color-text);
		cursor: pointer;
		transition: border-color 0.15s, color 0.15s;
	}
	.travel-chip:hover {
		border-color: var(--color-primary);
		color: var(--color-primary-text);
	}

	/* ── Shared content width + sections ─────────────────────────────── */
	.travel-content {
		max-width: 72rem;
		margin: 0 auto;
		padding: 1.5rem 1.5rem 0;
	}
	.travel-section {
		margin-bottom: 2.5rem;
	}
	.travel-section-head {
		display: flex;
		align-items: end;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1rem;
	}
	.travel-section-head h2 {
		font-size: 1.375rem;
		font-weight: 700;
		letter-spacing: -0.01em;
		color: var(--color-text);
	}
	.travel-section-head p {
		font-size: 0.875rem;
		color: var(--color-muted);
		margin-top: 0.125rem;
	}
	.travel-link {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--color-primary-text);
		text-decoration: none;
		white-space: nowrap;
		cursor: pointer;
	}
	.travel-link:hover {
		text-decoration: underline;
	}
	.travel-alert {
		margin-bottom: 1rem;
		border-radius: var(--radius);
		background: rgb(239 68 68 / 0.1);
		padding: 0.75rem;
		font-size: 0.875rem;
		color: var(--color-danger);
	}
	.travel-alert-retry {
		margin-inline-start: 0.5rem;
		font-weight: 600;
		text-decoration: underline;
		cursor: pointer;
	}
	.travel-form-inline {
		margin-bottom: 1rem;
	}

	/* ── Destination cards ───────────────────────────────────────────── */
	.dest-row {
		display: grid;
		grid-auto-flow: column;
		grid-auto-columns: minmax(10.5rem, 1fr);
		gap: 1rem;
		overflow-x: auto;
		padding-bottom: 0.25rem;
		scroll-snap-type: x mandatory;
	}
	.dest-card {
		position: relative;
		height: 13rem;
		border-radius: 0.875rem;
		overflow: hidden;
		cursor: pointer;
		text-align: start;
		scroll-snap-align: start;
		transition: transform 0.2s ease;
	}
	.dest-card:hover {
		transform: translateY(-3px);
	}
	.dest-card img,
	.dest-card-noimg {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.dest-card-noimg {
		display: grid;
		place-items: center;
		background: color-mix(in srgb, var(--color-primary) 18%, var(--color-surface-subtle));
		color: var(--color-primary-text);
	}
	.dest-card-scrim {
		position: absolute;
		inset: 0;
		background: linear-gradient(to top, rgb(0 0 0 / 0.72) 0%, rgb(0 0 0 / 0.1) 55%, transparent 100%);
	}
	.dest-card-copy {
		position: absolute;
		inset-inline: 0;
		bottom: 0;
		display: flex;
		flex-direction: column;
		gap: 0.125rem;
		padding: 0.875rem;
		color: #fff;
	}
	.dest-card-copy strong {
		font-size: 1rem;
		font-weight: 700;
		line-height: 1.2;
	}
	.dest-card-copy em {
		font-style: normal;
		font-size: 0.75rem;
		font-weight: 600;
		color: rgb(255 255 255 / 0.92);
		background: rgb(255 255 255 / 0.18);
		backdrop-filter: blur(4px);
		border-radius: 999px;
		padding: 0.125rem 0.5rem;
		align-self: flex-start;
		margin-top: 0.25rem;
	}

	/* ── "More than a trip" ──────────────────────────────────────────── */
	.travel-future {
		display: grid;
		grid-template-columns: minmax(0, 1.2fr) minmax(0, 2fr);
		gap: 2rem;
		align-items: start;
	}
	.travel-future-copy h2 {
		font-size: 1.375rem;
		font-weight: 700;
		letter-spacing: -0.01em;
		line-height: 1.25;
		color: var(--color-text);
	}
	.travel-future-copy p {
		font-size: 0.875rem;
		color: var(--color-muted);
		margin-top: 0.5rem;
		margin-bottom: 1rem;
	}
	.travel-outline-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		padding: 0.625rem 1.125rem;
		border-radius: var(--radius);
		border: 1px solid var(--color-primary);
		color: var(--color-primary-text);
		font-size: 0.8125rem;
		font-weight: 600;
		text-decoration: none;
		cursor: pointer;
		transition: background 0.15s;
	}
	.travel-outline-btn:hover {
		background: color-mix(in srgb, var(--color-primary) 8%, transparent);
	}
	.travel-future-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(11rem, 1fr));
		gap: 1rem;
	}
	.future-item {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		text-decoration: none;
	}
	.future-icon {
		display: inline-grid;
		place-items: center;
		width: 2.5rem;
		height: 2.5rem;
		border-radius: 0.75rem;
		background: color-mix(in srgb, var(--color-primary) 10%, transparent);
		color: var(--color-primary-text);
		margin-bottom: 0.375rem;
	}
	.future-item strong {
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--color-text);
	}
	.future-item > span:last-child {
		font-size: 0.75rem;
		line-height: 1.45;
		color: var(--color-muted);
	}

	/* ── Trio cards (tips / deals / help) ────────────────────────────── */
	.travel-trio {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(15rem, 1fr));
		gap: 1rem;
	}
	.trio-card {
		border-radius: 1rem;
		padding: 1.5rem;
		border: 1px solid var(--color-border);
	}
	.trio-lavender {
		background: color-mix(in srgb, var(--color-primary) 8%, var(--color-surface));
	}
	.trio-mint {
		background: color-mix(in srgb, var(--color-success) 8%, var(--color-surface));
	}
	.trio-peach {
		background: color-mix(in srgb, var(--color-warning) 8%, var(--color-surface));
	}
	.trio-card h3 {
		font-size: 1rem;
		font-weight: 700;
		color: var(--color-text);
	}
	.trio-card > p {
		font-size: 0.8125rem;
		color: var(--color-muted);
		margin-top: 0.25rem;
	}
	.trio-card ul {
		display: grid;
		gap: 0.5rem;
		margin: 1rem 0;
	}
	.trio-card li {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.8125rem;
		color: var(--color-text);
	}
	.trio-card li :global(svg) {
		flex-shrink: 0;
		color: var(--color-primary-text);
	}

	/* ── Hotels tab layout ───────────────────────────────────────────── */
	.hotels-layout {
		display: grid;
		grid-template-columns: minmax(0, 2fr) minmax(16rem, 1fr);
		gap: 1.5rem;
		align-items: start;
	}
	.hotels-rail {
		display: grid;
		gap: 1rem;
		position: sticky;
		top: 1rem;
	}
	.rail-card {
		border: 1px solid var(--color-border);
		border-radius: 1rem;
		background: var(--color-surface);
		padding: 1.25rem;
	}
	.rail-card h3 {
		font-size: 1rem;
		font-weight: 700;
		color: var(--color-text);
		margin-bottom: 0.875rem;
	}
	.rail-sub {
		font-size: 0.8125rem;
		color: var(--color-muted);
		margin: -0.5rem 0 0.875rem;
	}
	.rail-checklist {
		display: grid;
		gap: 0.875rem;
	}
	.rail-checklist li {
		display: flex;
		align-items: flex-start;
		gap: 0.625rem;
	}
	.rail-checklist li :global(svg) {
		flex-shrink: 0;
		color: var(--color-primary-text);
		margin-top: 0.125rem;
	}
	.rail-checklist strong {
		display: block;
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--color-text);
	}
	.rail-checklist span {
		font-size: 0.75rem;
		color: var(--color-muted);
	}
	.rail-member {
		background: color-mix(in srgb, var(--color-primary) 7%, var(--color-surface));
	}
	.rail-member-icon {
		display: inline-grid;
		place-items: center;
		width: 2.25rem;
		height: 2.25rem;
		border-radius: 50%;
		background: var(--color-primary);
		color: var(--color-primary-contrast);
		margin-bottom: 0.5rem;
	}
	.rail-member strong {
		display: block;
		font-size: 0.9375rem;
		font-weight: 700;
		color: var(--color-text);
	}
	.rail-member p {
		font-size: 0.8125rem;
		color: var(--color-muted);
		margin: 0.25rem 0 0.75rem;
	}
	.rail-citylist {
		display: grid;
		gap: 0.125rem;
	}
	.rail-citylist button {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		width: 100%;
		padding: 0.5rem 0.25rem;
		border-radius: var(--radius);
		text-align: start;
		cursor: pointer;
		transition: background 0.15s;
	}
	.rail-citylist button:hover {
		background: var(--color-surface-subtle);
	}
	.rail-city-icon {
		display: grid;
		place-items: center;
		width: 1.75rem;
		height: 1.75rem;
		border-radius: 0.5rem;
		background: color-mix(in srgb, var(--color-primary) 10%, transparent);
		color: var(--color-primary-text);
		flex-shrink: 0;
	}
	.rail-city-name {
		font-size: 0.8125rem;
		font-weight: 500;
		color: var(--color-text);
	}
	.rail-city-count {
		margin-inline-start: auto;
		font-size: 0.75rem;
		color: var(--color-muted);
		white-space: nowrap;
	}

	/* ── Stay cards ──────────────────────────────────────────────────── */
	.stay-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(14rem, 1fr));
		gap: 1rem;
	}
	.stay-card {
		position: relative;
		border: 1px solid var(--color-border);
		border-radius: 0.875rem;
		background: var(--color-surface);
		overflow: hidden;
		transition: transform 0.2s ease, box-shadow 0.2s ease;
	}
	.stay-card-btn {
		display: block;
		width: 100%;
		padding: 0;
		text-align: start;
		cursor: pointer;
		font: inherit;
		color: inherit;
	}
	.stay-card:hover {
		transform: translateY(-3px);
		box-shadow: 0 8px 24px rgb(0 0 0 / 0.08);
	}
	.stay-badge {
		position: absolute;
		top: 0.625rem;
		inset-inline-start: 0.625rem;
		z-index: 1;
		padding: 0.125rem 0.625rem;
		border-radius: 999px;
		background: var(--color-primary);
		color: var(--color-primary-contrast);
		font-size: 0.6875rem;
		font-weight: 700;
	}
	.stay-card-img {
		display: block;
		width: 100%;
		height: 9rem;
		object-fit: cover;
	}
	.stay-card-noimg {
		display: grid;
		place-items: center;
		height: 9rem;
		background: color-mix(in srgb, var(--color-primary) 10%, var(--color-surface-subtle));
		color: var(--color-primary-text);
	}
	.stay-card-body {
		padding: 0.875rem;
	}
	.stay-card-body h4 {
		font-size: 0.9375rem;
		font-weight: 600;
		color: var(--color-text);
		line-height: 1.3;
	}
	.stay-stars {
		display: inline-flex;
		gap: 1px;
		color: var(--color-warning);
		margin-top: 0.25rem;
	}
	.stay-card-meta {
		font-size: 0.75rem;
		color: var(--color-muted);
		margin-top: 0.25rem;
	}
	.stay-card-price {
		font-size: 0.75rem;
		color: var(--color-muted);
		margin-top: 0.625rem;
	}
	.stay-card-price strong {
		font-size: 0.9375rem;
		font-weight: 700;
		color: var(--color-text);
	}
	.stay-card-price span {
		font-size: 0.6875rem;
	}

	/* ── Airline cards + compare ─────────────────────────────────────── */
	.airline-row {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(10.5rem, 1fr));
		gap: 1rem;
	}
	.airline-card {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		border: 1px solid var(--color-border);
		border-radius: 0.875rem;
		background: var(--color-surface);
		padding: 1.125rem;
		transition: transform 0.2s ease, box-shadow 0.2s ease;
	}
	.airline-card:hover {
		transform: translateY(-3px);
		box-shadow: 0 8px 24px rgb(0 0 0 / 0.08);
	}
	.airline-mark {
		display: grid;
		place-items: center;
		min-width: 3rem;
		height: 3rem;
		padding: 0 0.5rem;
		border-radius: 0.75rem;
		background: color-mix(in srgb, var(--color-primary) 10%, transparent);
		color: var(--color-primary-text);
		font-family: ui-monospace, 'SF Mono', Menlo, monospace;
		font-weight: 700;
		font-size: 1rem;
		margin-bottom: 0.75rem;
	}
	.airline-mark-sm {
		min-width: 2rem;
		height: 2rem;
		font-size: 0.75rem;
		border-radius: 0.5rem;
		margin: 0 auto 0.25rem;
	}
	.airline-card h4 {
		font-size: 0.875rem;
		font-weight: 700;
		color: var(--color-text);
	}
	.airline-meta {
		font-size: 0.75rem;
		color: var(--color-muted);
		margin-top: 0.125rem;
	}
	.airline-via {
		font-size: 0.75rem;
		color: var(--color-muted);
		margin-top: 0.25rem;
		margin-bottom: 0.875rem;
	}
	.airline-compare {
		margin-top: auto;
		width: 100%;
		padding: 0.4375rem 0;
		border-radius: var(--radius);
		border: 1px solid var(--color-primary);
		background: transparent;
		color: var(--color-primary-text);
		font-size: 0.8125rem;
		font-weight: 600;
		cursor: pointer;
		transition: background 0.15s;
	}
	.airline-compare:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}
	/* Directory rows reuse the chip inline, not the card's full-width block. */
	.space-y-2 .airline-compare {
		margin-top: 0;
		width: auto;
		padding-inline: 0.875rem;
	}
	.airline-compare:hover {
		background: color-mix(in srgb, var(--color-primary) 8%, transparent);
	}
	.airline-compare-on {
		background: var(--color-primary);
		color: var(--color-primary-contrast);
	}
	.airlines-compare-layout {
		display: grid;
		grid-template-columns: minmax(0, 2fr) minmax(16rem, 1fr);
		gap: 1.5rem;
		align-items: start;
		margin-bottom: 2.5rem;
	}
	.compare-card {
		border: 1px solid var(--color-border);
		border-radius: 1rem;
		background: var(--color-surface);
		padding: 1.5rem;
		margin-bottom: 0;
	}
	.compare-card h2 {
		font-size: 1.125rem;
		font-weight: 700;
		color: var(--color-text);
	}
	.compare-sub {
		font-size: 0.8125rem;
		color: var(--color-muted);
		margin-top: 0.25rem;
		margin-bottom: 1rem;
	}
	.compare-table-wrap {
		overflow-x: auto;
	}
	.compare-table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.8125rem;
	}
	.compare-table th,
	.compare-table td {
		padding: 0.625rem 0.75rem;
		text-align: center;
		border-top: 1px solid var(--color-border);
	}
	.compare-table thead th {
		border-top: none;
		font-size: 0.8125rem;
		font-weight: 700;
		color: var(--color-text);
		vertical-align: top;
	}
	.compare-table td:first-child,
	.compare-table th:first-child {
		text-align: start;
		font-weight: 500;
		color: var(--color-muted);
		white-space: nowrap;
	}
	.compare-table tbody td {
		color: var(--color-text);
	}
	.compare-bookable {
		display: inline-block;
		padding: 0.125rem 0.5rem;
		border-radius: 999px;
		background: color-mix(in srgb, var(--color-success) 12%, transparent);
		color: var(--color-success);
		font-size: 0.6875rem;
		font-weight: 700;
	}
	.airlines-rail {
		position: sticky;
		top: 1rem;
	}

	/* ── Feature strip ───────────────────────────────────────────────── */
	.feature-strip {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(13rem, 1fr));
		gap: 1rem;
		margin-bottom: 2.5rem;
	}
	.feature-item {
		display: flex;
		align-items: flex-start;
		gap: 0.75rem;
	}
	.feature-icon {
		display: grid;
		place-items: center;
		width: 2.5rem;
		height: 2.5rem;
		border-radius: 50%;
		flex-shrink: 0;
	}
	.f-mint { background: color-mix(in srgb, var(--color-success) 12%, transparent); color: var(--color-success); }
	.f-gold { background: color-mix(in srgb, var(--color-warning) 14%, transparent); color: var(--color-warning); }
	.f-sky { background: color-mix(in srgb, var(--color-info) 12%, transparent); color: var(--color-info); }
	.f-violet { background: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary-text); }
	.feature-item strong {
		display: block;
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--color-text);
	}
	.feature-item > div > span {
		font-size: 0.75rem;
		line-height: 1.45;
		color: var(--color-muted);
	}

	/* ── Newsletter band ─────────────────────────────────────────────── */
	.newsletter-band {
		display: flex;
		align-items: center;
		justify-content: space-between;
		flex-wrap: wrap;
		gap: 1.25rem;
		border-radius: 1rem;
		padding: 1.75rem 2rem;
		background: linear-gradient(105deg, var(--color-primary) 0%, color-mix(in srgb, var(--color-primary) 70%, #7c3aed) 100%);
		color: #fff;
		margin-bottom: 2rem;
	}
	.newsletter-copy {
		display: flex;
		align-items: center;
		gap: 1rem;
	}
	.newsletter-icon {
		display: grid;
		place-items: center;
		width: 3rem;
		height: 3rem;
		border-radius: 50%;
		background: rgb(255 255 255 / 0.15);
		flex-shrink: 0;
	}
	.newsletter-band h2 {
		font-size: 1.125rem;
		font-weight: 700;
	}
	.newsletter-band p {
		font-size: 0.8125rem;
		color: rgb(255 255 255 / 0.85);
	}
	.newsletter-form {
		display: flex;
		gap: 0.625rem;
		flex: 1;
		min-width: 16rem;
		max-width: 26rem;
	}
	.newsletter-form input {
		flex: 1;
		min-width: 0;
		height: 2.75rem;
		padding: 0 0.875rem;
		border-radius: var(--radius);
		border: 1px solid rgb(255 255 255 / 0.25);
		background: rgb(255 255 255 / 0.95);
		font-size: 0.875rem;
		color: #18181b;
		outline: none;
	}
	.newsletter-form input::placeholder {
		color: #71717a;
	}
	.newsletter-form button {
		height: 2.75rem;
		padding: 0 1.25rem;
		border-radius: var(--radius);
		border: 1px solid rgb(255 255 255 / 0.4);
		background: transparent;
		color: #fff;
		font-size: 0.875rem;
		font-weight: 600;
		white-space: nowrap;
		cursor: pointer;
		transition: background 0.15s;
	}
	.newsletter-form button:hover {
		background: rgb(255 255 255 / 0.15);
	}
	.newsletter-form button:disabled {
		opacity: 0.6;
	}
	.newsletter-msg {
		width: 100%;
		font-size: 0.8125rem;
		color: rgb(255 255 255 / 0.95);
	}
	.newsletter-msg-err {
		color: #fecaca;
	}

	/* ── Trust strip ─────────────────────────────────────────────────── */
	.trust-strip {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(12rem, 1fr));
		gap: 1rem;
		padding: 1.25rem 0 2rem;
	}
	.trust-item {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0.75rem;
	}
	.trust-item :global(svg) {
		color: var(--color-primary-text);
		flex-shrink: 0;
	}
	.trust-item strong {
		display: block;
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--color-text);
	}
	.trust-item span {
		font-size: 0.75rem;
		color: var(--color-muted);
	}

	/* ── Footer ──────────────────────────────────────────────────────── */
	.travel-footer {
		border-top: 1px solid var(--color-border);
		background: var(--color-surface);
		margin-top: 1rem;
	}
	.travel-footer-inner {
		max-width: 72rem;
		margin: 0 auto;
		padding: 2rem 1.5rem 1.5rem;
	}
	.travel-footer-brand p {
		font-size: 0.8125rem;
		color: var(--color-muted);
		max-width: 22rem;
		margin-top: 0.75rem;
	}
	.travel-footer-links {
		display: flex;
		flex-wrap: wrap;
		gap: 0.375rem 1.5rem;
		margin-top: 1.5rem;
		padding-top: 1.5rem;
		border-top: 1px solid var(--color-border);
	}
	.travel-footer-links a {
		font-size: 0.8125rem;
		color: var(--color-text-subtle);
		text-decoration: none;
	}
	.travel-footer-links a:hover {
		color: var(--color-text);
	}
	.travel-footer-copy {
		margin-top: 1.5rem;
		font-size: 0.75rem;
		color: var(--color-muted);
	}

	/* ── Responsive ──────────────────────────────────────────────────── */
	@media (max-width: 1023px) {
		.hotels-layout,
		.airlines-compare-layout {
			grid-template-columns: 1fr;
		}
		.hotels-rail,
		.airlines-rail {
			position: static;
		}
		.travel-future {
			grid-template-columns: 1fr;
			gap: 1.25rem;
		}
	}
	@media (max-width: 767px) {
		.travel-hero {
			min-height: 17rem;
		}
		.travel-hero-inner {
			padding-bottom: 4.5rem;
		}
		.travel-search-wrap {
			margin-top: -2.5rem;
			padding: 0 1rem;
		}
		.travel-search-body {
			padding: 1rem;
		}
		.travel-field-wide {
			grid-column: span 1;
		}
		.travel-content {
			padding: 1.25rem 1rem 0;
		}
		.newsletter-band {
			padding: 1.25rem;
		}
		.newsletter-form {
			max-width: none;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.dest-card,
		.stay-card,
		.airline-card,
		.travel-swap,
		.travel-search-btn {
			transition: none;
		}
	}
</style>
