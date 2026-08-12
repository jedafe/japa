<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Plane, Lock, Hotel, ExternalLink } from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type {
		TravelFlightResult,
		TravelHotelResult,
		TravelSearchResponse
	} from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('tools.travel', 'Travel Search — JapaConnect'));
	const description = $derived(t('tools.travel_desc', 'Search flights and hotels for your move.'));

	const authed = $derived(data.authed);
	let disabled = $state(false);

	type Tab = 'flights' | 'hotels';
	let tab = $state<Tab>('flights');

	// Flight form.
	let flightOrigin = $state('');
	let flightDest = $state('');
	let flightDate = $state('');
	let flightAdults = $state('1');

	// Hotel form.
	let hotelDest = $state('');
	let hotelCheckIn = $state('');
	let hotelCheckOut = $state('');
	let hotelGuests = $state('1');

	let loading = $state(false);
	let error = $state<string | null>(null);
	let flights = $state<TravelFlightResult[]>([]);
	let hotels = $state<TravelHotelResult[]>([]);

	async function searchFlights() {
		if (!flightOrigin || !flightDest || !flightDate) return;
		loading = true;
		error = null;
		flights = [];
		const params = new URLSearchParams({
			origin: flightOrigin,
			dest: flightDest,
			date: flightDate,
			adults: flightAdults
		});
		const res = await api.get<TravelSearchResponse>(`/travel/search/flights?${params.toString()}`);
		loading = false;
		if (res.error) {
			if (res.error.code === 'travel_disabled') {
				disabled = true;
				return;
			}
			error = res.error.message;
			return;
		}
		// Backend returns { data: { results: [...], cache_hit, search_time_ms } };
		// the API client unwraps body.data, so res.data is the SearchResponse.
		flights = (res.data?.results as TravelFlightResult[] | undefined) ?? [];
	}

	async function searchHotels() {
		if (!hotelDest || !hotelCheckIn || !hotelCheckOut) return;
		loading = true;
		error = null;
		hotels = [];
		const params = new URLSearchParams({
			dest: hotelDest,
			checkin: hotelCheckIn,
			checkout: hotelCheckOut,
			guests: hotelGuests
		});
		const res = await api.get<TravelSearchResponse>(`/travel/search/hotels?${params.toString()}`);
		loading = false;
		if (res.error) {
			if (res.error.code === 'travel_disabled') {
				disabled = true;
				return;
			}
			error = res.error.message;
			return;
		}
		hotels = (res.data?.results as TravelHotelResult[] | undefined) ?? [];
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
	<div class="border-b border-[var(--color-border)] px-4 py-3">
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
		<div class="flex border-b border-[var(--color-border)] px-4">
			<button
				type="button"
				onclick={() => (tab = 'flights')}
				class="flex items-center gap-1.5 border-b-2 px-3 py-2.5 text-sm font-medium transition-colors
					{tab === 'flights' ? 'border-[var(--color-primary)] text-[var(--color-text)]' : 'border-transparent text-[var(--color-muted)]'}"
			>
				<Plane size={16} /> {t('travel.flights', 'Flights')}
			</button>
			<button
				type="button"
				onclick={() => (tab = 'hotels')}
				class="flex items-center gap-1.5 border-b-2 px-3 py-2.5 text-sm font-medium transition-colors
					{tab === 'hotels' ? 'border-[var(--color-primary)] text-[var(--color-text)]' : 'border-transparent text-[var(--color-muted)]'}"
			>
				<Hotel size={16} /> {t('travel.hotels', 'Hotels')}
			</button>
		</div>

		<div class="p-4">
			{#if tab === 'flights'}
				<form onsubmit={(e) => { e.preventDefault(); searchFlights(); }} class="mb-4 grid gap-2 sm:grid-cols-4">
					<input type="text" placeholder={t('travel.origin', 'Origin (IATA)')} bind:value={flightOrigin} class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]" />
					<input type="text" placeholder={t('travel.destination', 'Destination (IATA)')} bind:value={flightDest} class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]" />
					<input type="date" bind:value={flightDate} aria-label={t('travel.departure_date', 'Departure date')} class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]" />
					<button type="submit" class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90">
						{t('travel.search', 'Search')}
					</button>
				</form>

				{#if loading}
					<LoadingSkeleton variant="lines" />
				{:else if error}
					<EmptyState icon={Plane} title={t('travel.search_error', 'Search failed')} body={error} />
				{:else if flights.length > 0}
					<div class="space-y-2">
						{#each flights as f, i (i)}
							<div class="flex items-center justify-between rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
								<div class="min-w-0 flex-1">
									<span class="block text-sm font-medium text-[var(--color-text)]">{f.airline} {f.flight_number}</span>
									<span class="mt-0.5 block text-xs text-[var(--color-muted)]">
										{f.departure_time} → {f.arrival_time} · {f.stops} {t('travel.stops', 'stops')} · {f.duration}
									</span>
								</div>
								<div class="flex items-center gap-3">
									<span class="text-sm font-medium text-[var(--color-text)]">{f.price.toFixed(0)} {f.currency}</span>
									<a href={f.booking_url} target="_blank" rel="noopener sponsored" class="flex items-center gap-1 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-white transition-colors hover:opacity-90">
										{t('travel.book_now', 'Book now')} <ExternalLink size={12} />
									</a>
								</div>
							</div>
						{/each}
					</div>
				{/if}
			{:else}
				<form onsubmit={(e) => { e.preventDefault(); searchHotels(); }} class="mb-4 grid gap-2 sm:grid-cols-4">
					<input type="text" placeholder={t('travel.destination', 'Destination')} bind:value={hotelDest} class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]" />
					<input type="date" bind:value={hotelCheckIn} aria-label={t('travel.check_in', 'Check-in date')} class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]" />
					<input type="date" bind:value={hotelCheckOut} aria-label={t('travel.check_out', 'Check-out date')} class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]" />
					<button type="submit" class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90">
						{t('travel.search', 'Search')}
					</button>
				</form>

				{#if loading}
					<LoadingSkeleton variant="lines" />
				{:else if error}
					<EmptyState icon={Hotel} title={t('travel.search_error', 'Search failed')} body={error} />
				{:else if hotels.length > 0}
					<div class="space-y-2">
						{#each hotels as h, i (i)}
							<div class="flex items-center justify-between rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
								<div class="min-w-0 flex-1">
									<span class="block text-sm font-medium text-[var(--color-text)]">{h.name}</span>
									<span class="mt-0.5 block text-xs text-[var(--color-muted)]">
										{h.address} · {'★'.repeat(h.star_rating)}
									</span>
								</div>
								<div class="flex items-center gap-3">
									<span class="text-sm font-medium text-[var(--color-text)]">{h.price.toFixed(0)} {h.currency}</span>
									<a href={h.booking_url} target="_blank" rel="noopener sponsored" class="flex items-center gap-1 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-white transition-colors hover:opacity-90">
										{t('travel.book_now', 'Book now')} <ExternalLink size={12} />
									</a>
								</div>
							</div>
						{/each}
					</div>
				{/if}
			{/if}
		</div>
	{/if}
</div>
