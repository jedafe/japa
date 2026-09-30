<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { enhance } from '$app/forms';
	import { Plane, Lock, Hotel, ExternalLink, Loader2, GitCompare, Building2 } from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton, UrlTabs } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import { formatMoney } from '$lib/utils/currency';
	import type {
		TravelFlightResult,
		TravelHotelResult,
		TravelSearchResponse
	} from '$lib/types';
	import type { PageData, ActionData } from './$types';

	interface TravelProvider {
		id: string;
		name: string;
		slug: string;
		provider_type: string;
		is_active: boolean;
		created_at: string;
	}

	let { data, form }: { data: PageData; form?: ActionData } = $props();

	const title = $derived(t('tools.travel', 'Travel Search — JapaConnect'));
	const description = $derived(t('tools.travel_desc', 'Search flights and hotels for your move.'));

	const authed = $derived(data.authed);
	const providers = $derived<TravelProvider[]>(data.providers ?? []);
	let disabled = $state(false);

	type Tab = 'flights' | 'hotels' | 'providers';
	const TABS: Array<{ value: Tab; label: string }> = [
		{ value: 'flights', label: t('travel.flights', 'Flights') },
		{ value: 'hotels', label: t('travel.hotels', 'Hotels') },
		{ value: 'providers', label: t('travel.providers', 'Providers') }
	];
	const initialTabParam = page.url.searchParams.get('tab') as Tab;
	let tab = $state<Tab>(
		TABS.some((t) => t.value === initialTabParam) ? initialTabParam : 'flights'
	);

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

	// JCE comparison state for providers (the resolver-backed entity).
	let savingJCE = $state(false);
	let selectedProviderIds = $state<Set<string>>(new Set());

	const canCompareProviders = $derived(
		selectedProviderIds.size >= 2 && selectedProviderIds.size <= 4
	);

	function toggleProviderSelection(id: string) {
		const next = new Set(selectedProviderIds);
		if (next.has(id)) {
			next.delete(id);
		} else {
			if (next.size >= 4) return;
			next.add(id);
		}
		selectedProviderIds = next;
	}

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

	function formatProviderType(type: string): string {
		return type.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
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
				<div class="mb-4 rounded-[var(--radius)] border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">
					{form.message || t('travel.jce_error', 'Could not create comparison')}
				</div>
			{/if}

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
									<span class="text-sm font-medium text-[var(--color-text)]">{formatMoney(f.price, f.currency, data.currencies)}</span>
									<a href={f.booking_url} target="_blank" rel="noopener sponsored" class="flex items-center gap-1 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-white transition-colors hover:opacity-90">
										{t('travel.book_now', 'Book now')} <ExternalLink size={12} />
									</a>
								</div>
							</div>
						{/each}
					</div>
				{/if}
			{:else if tab === 'hotels'}
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
									<span class="text-sm font-medium text-[var(--color-text)]">{formatMoney(h.price, h.currency, data.currencies)}</span>
									<a href={h.booking_url} target="_blank" rel="noopener sponsored" class="flex items-center gap-1 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-white transition-colors hover:opacity-90">
										{t('travel.book_now', 'Book now')} <ExternalLink size={12} />
									</a>
								</div>
							</div>
						{/each}
					</div>
				{/if}
			{:else}
				{#if providers.length === 0}
					<EmptyState
						icon={Building2}
						title={t('travel.no_providers', 'No travel providers')}
						body={t('travel.no_providers_body', 'Providers will appear here once an admin configures them.')}
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
								{t('travel.selected', 'Selected')}: {selectedProviderIds.size} / 4
							</p>
							<button
								type="submit"
								disabled={!canCompareProviders || !authed}
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

						{#each providers as p (p.id)}
							{#if selectedProviderIds.has(p.id)}
								<input type="hidden" name="provider_id" value={p.id} />
							{/if}
							<div class="flex items-start gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)]">
								<input
									type="checkbox"
									checked={selectedProviderIds.has(p.id)}
									onchange={() => toggleProviderSelection(p.id)}
									disabled={selectedProviderIds.size >= 4 && !selectedProviderIds.has(p.id)}
									class="mt-1 accent-[var(--color-primary)]"
								/>
								<div class="min-w-0 flex-1">
									<span class="block text-sm font-medium text-[var(--color-text)]">{p.name}</span>
									<span class="mt-0.5 block text-xs text-[var(--color-muted)]">
										{formatProviderType(p.provider_type)} · {p.slug}
									</span>
								</div>
							</div>
						{/each}
					</form>
				{/if}
			{/if}
		</div>
	{/if}
</div>
