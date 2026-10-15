<script lang="ts">
	// Ref: japaconnect-engineering/sections/98-service-directory.md
	// Ref: japaconnect-engineering/sections/114-services-marketplace.md
	// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§3.2)
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import Autocomplete, { type AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { page, navigating } from '$app/state';
	import { goto, invalidateAll } from '$app/navigation';
	import { api } from '$lib/api/client';
	import {
		Briefcase,
		Plus,
		Package,
		X,
		Check,
		ShieldCheck,
		Lock,
		Headset
	} from 'lucide-svelte';
	import {
		EmptyState,
		FilterToggle,
		PageAction,
		SelectField,
		UrlTabs,
		type UrlTab
	} from '$lib/components/ui';
	import MarketplaceHero from '$lib/components/marketplace/MarketplaceHero.svelte';
	import CategoryPills from '$lib/components/marketplace/CategoryPills.svelte';
	import ServiceCard from '$lib/components/marketplace/ServiceCard.svelte';
	import { bookingsInboxEnabled } from '$lib/features';
	import { destinationMarkets, pinMarketsFirst } from '$lib/utils/geoMarkets';
	import type { ServiceListing, Space } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('services.marketplace_title', 'Services Marketplace — JapaConnect'));
	const description = $derived(
		t(
			'services.marketplace_description',
			'Book verified diaspora-relevant services and travel packages — immigration, relocation, study abroad and more.'
		)
	);
	const canonical = $derived(`${page.url.origin}/services`);

	// Load-more pagination — the SSR payload is page N of the filtered list;
	// further chunks are fetched client-side with the same query params and
	// appended. A new SSR payload (filter/nav change) resets the tail.
	let appended = $state<ServiceListing[]>([]);
	let lastFetchedPage = $state(untrack(() => data.filters.page || 1));
	let loadingMore = $state(false);

	$effect(() => {
		data.services;
		appended = [];
		lastFetchedPage = data.filters.page || 1;
	});

	const totalPages = $derived(Math.ceil((data.meta?.total ?? 0) / (data.meta?.limit ?? 20)));
	const currentPage = $derived(data.filters.page || 1);
	const services = $derived<ServiceListing[]>([...data.services, ...appended]);
	const hasMore = $derived(lastFetchedPage < totalPages);

	async function loadMore() {
		if (loadingMore || !hasMore) return;
		loadingMore = true;
		const params = new URLSearchParams(page.url.searchParams);
		params.set('page', String(lastFetchedPage + 1));
		params.delete('submitted');
		// The API only accepts space UUIDs — the URL may carry slug-form
		// geo params (the city picker writes slugs for readable URLs), so
		// swap in the server-resolved ids before fetching the next page.
		if (data.filters.dest) params.set('dest', data.filters.dest);
		if (data.filters.city) {
			if (data.filters.cityId) params.set('city', data.filters.cityId);
			else params.delete('city');
		}
		const res = await api.get<ServiceListing[]>(`/services?${params.toString()}`);
		if (res.data) {
			appended = [...appended, ...res.data];
			lastFetchedPage += 1;
		}
		loadingMore = false;
	}

	// The bookings inbox is a composite surface (directory || packages ||
	// mentorship) — mirror its flag composite so the action never links to
	// a 404 in mentorship-only deployments (000824).
	const features = $derived((page.data.features ?? {}) as Record<string, boolean>);
	const bookingsEnabled = $derived(bookingsInboxEnabled(features));

	// Filter-panel state — seeded from URL params and re-synced on navigation
	// so back/forward and shared links repaint the controls.
	let destInput = $state(untrack(() => data.filters.dest));
	let cityInput = $state(untrack(() => data.filters.city));
	let cityLabelInput = $state(untrack(() => data.filters.cityLabel));
	let originInput = $state(untrack(() => data.filters.origin));
	let verifiedInput = $state(untrack(() => data.filters.verified === 'true'));

	$effect(() => {
		destInput = data.filters.dest;
		cityInput = data.filters.city;
		cityLabelInput = data.filters.cityLabel;
		originInput = data.filters.origin;
		verifiedInput = data.filters.verified === 'true';
	});

	// §111/000824 kind tabs — the row renders once the gated
	// mentorship kind exists (mentorship.enabled); the tab
	// only appears under its own flag. The 'package' kind
	// was removed from the feature (2026-10).
	const kindTabs = $derived<UrlTab[]>([
		{ value: 'all', label: t('services.kind_all', 'All listings') },
		{ value: 'service', label: t('services.kind_service', 'Services') },
		...(data.mentorshipEnabled
			? [{ value: 'mentorship', label: t('services.kind_mentorship', 'Mentorship') }]
			: [])
	]);
	const activeKind = $derived(data.filters.kind || 'all');

	const sortOptions = $derived([
		{ value: 'latest', label: t('services.sort_latest', 'Latest') },
		{ value: 'top', label: t('services.sort_top', 'Top') }
	]);

	// The selected destination's ISO geo code drives the city picker —
	// /spaces?level=city&geo=<code> returns every city in the country,
	// including ones nested under state spaces (unlike ?parent=, which only
	// matches direct children — BUG-010 fix for e.g. Nigeria).
	const destGeoCode = $derived(
		destInput ? (data.countries ?? []).find((c) => c.id === destInput)?.geo_code ?? '' : ''
	);

	async function fetchCities(q: string): Promise<AutocompleteItem[]> {
		if (!destGeoCode) return [];
		const res = await api.get<Space[]>(
			`/spaces?level=city&geo=${encodeURIComponent(destGeoCode)}&q=${encodeURIComponent(q.trim())}&page_size=25`
		);
		// The item id is the space *slug* — the ?city= URL param resolves
		// slug→id server-side so shared links stay human-readable.
		return (res.data ?? []).map((c) => ({ id: c.slug, label: c.name }));
	}

	function fetchCitiesInitial(): Promise<AutocompleteItem[]> {
		return fetchCities('');
	}

	const selectedCityLabel = $derived(cityInput ? cityLabelInput || cityInput : null);

	// ADR-064 curated markets (public settings) — pinned first; every
	// seeded country stays selectable (pin-first, not restricted).
	const destCountries = $derived(
		pinMarketsFirst(data.countries ?? [], destinationMarkets(data.siteSettings), (c) => c.geo_code)
	);

	function fetchDestinationCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('services.filter_dest', 'All countries');
		const query = q.toLowerCase().trim();
		const allItem: AutocompleteItem = { id: '', label: allLabel };
		if (query === '') {
			return Promise.resolve([
				allItem,
				...destCountries.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }))
			]);
		}
		const matchesAll = allLabel.toLowerCase().includes(query);
		const filtered = destCountries.filter(
			(c) => c.name.toLowerCase().includes(query) || c.geo_code.toLowerCase().includes(query)
		);
		const items = filtered.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }));
		return Promise.resolve(matchesAll ? [allItem, ...items] : items);
	}
	function fetchDestinationCountriesInitial(): Promise<AutocompleteItem[]> {
		return fetchDestinationCountries('');
	}
	const selectedDestinationLabel = $derived(
		destInput
			? (data.countries ?? []).find((c) => c.id === destInput)?.name ??
					data.filters.destLabel ??
					destInput
			: null
	);

	// Live suggestions for the hero search — the backend title search (?q=).
	async function fetchServiceSuggestions(q: string) {
		const res = await api.get<ServiceListing[]>(`/services?q=${encodeURIComponent(q.trim())}&limit=5`);
		return (res.data ?? []).map((s) => ({
			id: s.id,
			title: s.title,
			url: `/services/${s.slug}`,
			type: 'service'
		}));
	}

	/**
	 * Build a /services URL from the *current* query string with a mutation
	 * applied — every control (pills, tabs, sort, filters, search) preserves
	 * params it does not own. Pagination always resets on a filter change,
	 * and the one-shot ?submitted=1 notice never propagates into new URLs.
	 */
	function paramsWith(mutate: (p: URLSearchParams) => void): string {
		const params = new URLSearchParams(page.url.searchParams);
		mutate(params);
		params.delete('page');
		params.delete('submitted');
		const qs = params.toString();
		return `/services${qs ? `?${qs}` : ''}`;
	}

	function onHeroSearch(q: string) {
		goto(
			paramsWith((p) => {
				if (q) p.set('q', q);
				else p.delete('q');
			})
		);
	}

	function selectKind(v: string) {
		goto(
			paramsWith((p) => {
				if (v === 'all') p.delete('kind');
				else p.set('kind', v);
			}),
			{ keepFocus: true }
		);
	}

	function selectSort(v: string) {
		goto(
			paramsWith((p) => {
				if (v === 'latest') p.delete('sort');
				else p.set('sort', v);
			}),
			{ keepFocus: true }
		);
	}

	function applyFilters(e?: SubmitEvent) {
		e?.preventDefault();
		goto(
			paramsWith((p) => {
				if (destInput) p.set('dest', destInput);
				else p.delete('dest');
				if (cityInput) p.set('city', cityInput);
				else p.delete('city');
				if (originInput.trim()) p.set('origin', originInput.trim());
				else p.delete('origin');
				if (verifiedInput) p.set('verified', 'true');
				else p.delete('verified');
			}),
			{ keepFocus: true }
		);
	}

	function clearFilters() {
		destInput = '';
		cityInput = '';
		cityLabelInput = '';
		originInput = '';
		verifiedInput = false;
		goto('/services');
	}

	// q is intentionally excluded — the hero search writes ?q= but has no
	// control inside the panel, so a bare search must not auto-open it.
	const hasActiveFilters = $derived(
		!!data.filters.dest ||
			!!data.filters.city ||
			!!data.filters.category ||
			!!data.filters.origin ||
			!!data.filters.verified ||
			!!data.filters.kind
	);

	let showFilters = $state(false);
	$effect(() => {
		if (hasActiveFilters) showFilters = true;
	});

	let submittedDismissed = $state(false);
	const showSubmitted = $derived(data.submitted && !submittedDismissed);

	const trustItems = $derived([
		{
			icon: ShieldCheck,
			title: t('services.trust_verified_title', 'Verified providers'),
			body: t('services.trust_verified_body', 'Every listing is reviewed by our team before it goes live.')
		},
		{
			icon: Lock,
			title: t('services.trust_secure_title', 'Secure booking'),
			body: t('services.trust_secure_body', 'Platform reservations protect your payment until the service is delivered.')
		},
		{
			icon: Headset,
			title: t('services.trust_support_title', 'Expert support'),
			body: t('services.trust_support_body', 'Real humans help you compare options and avoid relocation scams.')
		}
	]);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<link rel="canonical" href={canonical} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={canonical} />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	{#if currentPage > 1}
		{@const svcPrevUrl = (() => { const u = new URL(page.url.href); if (currentPage - 1 <= 1) u.searchParams.delete('page'); else u.searchParams.set('page', String(currentPage - 1)); return u.toString(); })()}
		<link rel="prev" href={svcPrevUrl} />
	{/if}
	{#if currentPage < totalPages}
		{@const svcNextUrl = (() => { const u = new URL(page.url.href); u.searchParams.set('page', String(currentPage + 1)); return u.toString(); })()}
		<link rel="next" href={svcNextUrl} />
	{/if}
</svelte:head>

<MarketplaceHero q={data.filters.q} fetcher={fetchServiceSuggestions} onSearch={onHeroSearch} />

<!-- Edge gutters + vertical rhythm come from the (marketplace) layout's
     `marketplace-center` main — this wrapper only caps the content width. -->
<div class="mx-auto mt-6 w-full max-w-[1400px] sm:mt-8">
	<!-- Category pills -->
	<div class="mb-5">
		<CategoryPills
			categories={data.serviceCategories}
			active={data.filters.category}
			kind={data.filters.kind}
		/>
	</div>

	<!-- Toolbar: kind tabs + filters + sort + list action -->
	<div class="mb-4 flex flex-wrap items-center gap-2">
		{#if data.mentorshipEnabled}
			<UrlTabs
				tabs={kindTabs}
				value={activeKind}
				onValueChange={selectKind}
				class="!min-h-0 !border-b-0 !bg-transparent !px-0"
			/>
		{/if}
		<div class="ml-auto flex items-center gap-2">
			<FilterToggle bind:show={showFilters} active={hasActiveFilters} controls="services-filter-panel" />
			<SelectField
				value={data.filters.sort}
				options={sortOptions}
				ariaLabel={t('services.sort_label', 'Sort by')}
				onValueChange={selectSort}
				class="w-36"
			/>
			{#if data.authed && bookingsEnabled}
				<PageAction
					href="/messages/bookings"
					icon={Package}
					label={t('bookings.my_title', 'Service Bookings')}
					secondary
					fabIndex={1}
				/>
			{/if}
			<PageAction
				href={data.authed ? '/services/new' : '/login?redirect=/services/new'}
				icon={Plus}
				label={t('services.list_a_service', 'List a Service')}
			/>
		</div>
	</div>

	<!-- Filter panel (compact row grid) — all controls write the same URL
	     params the directory page used. -->
	{#if showFilters}
		<div
			id="services-filter-panel"
			class="mb-5 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
		>
			<form onsubmit={applyFilters} class="grid gap-2 sm:grid-cols-2 lg:grid-cols-4" role="search">
				<Autocomplete
					placeholder={t('services.filter_dest', 'All countries')}
					ariaLabel={t('services.filter_dest', 'All countries')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchDestinationCountries}
					initialFetcher={fetchDestinationCountriesInitial}
					selectedId={destInput || null}
					selectedLabel={selectedDestinationLabel}
					onSelect={(item) => {
						destInput = item.id;
						cityInput = '';
						cityLabelInput = '';
						applyFilters();
					}}
					onClear={() => {
						destInput = '';
						cityInput = '';
						cityLabelInput = '';
						applyFilters();
					}}
				/>
				<Autocomplete
					placeholder={destInput
						? t('services.filter_city', 'All cities')
						: t('services.city_select_dest_first', 'Select a country first')}
					ariaLabel={t('services.filter_city', 'All cities')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchCities}
					initialFetcher={fetchCitiesInitial}
					selectedId={cityInput || null}
					selectedLabel={selectedCityLabel}
					disabled={!destInput}
					onSelect={(item) => {
						cityInput = item.id;
						cityLabelInput = item.label;
						applyFilters();
					}}
					onClear={() => {
						cityInput = '';
						cityLabelInput = '';
						applyFilters();
					}}
				/>
				<div>
					<label for="services-origin-filter" class="sr-only">
						{t('services.filter_origin', 'Origin community')}
					</label>
					<input
						id="services-origin-filter"
						type="text"
						bind:value={originInput}
						placeholder={t('services.filter_origin', 'Origin community (e.g. Nigerian)')}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
					/>
				</div>
				<div class="flex items-center gap-2">
					<label class="flex items-center gap-1.5 text-sm text-[var(--color-text)]">
						<input
							type="checkbox"
							checked={verifiedInput}
							onchange={(e) => {
								verifiedInput = e.currentTarget.checked;
								applyFilters();
							}}
							class="h-4 w-4 rounded border-[var(--color-border)]"
						/>
						{t('services.filter_verified', 'Verified only')}
					</label>
				</div>
				<div class="flex items-center gap-2">
					<button
						type="submit"
						class="shrink-0 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
					>
						{t('common.apply', 'Apply')}
					</button>
				</div>
			</form>

			{#if hasActiveFilters}
				<button
					type="button"
					onclick={clearFilters}
					class="mt-2 inline-flex items-center gap-1 text-xs text-[var(--color-muted)] underline hover:text-[var(--color-text)]"
				>
					<X size={12} /> {t('common.clear_filters', 'Clear filters')}
				</button>
			{/if}
		</div>
	{/if}

	{#if showSubmitted}
		<div
			class="mb-4 rounded-[var(--radius)] border border-emerald-500/30 bg-emerald-500/10 p-3 text-sm text-emerald-600"
		>
			<div class="flex items-start gap-2">
				<Check size={16} class="mt-0.5 shrink-0" />
				<div class="flex-1">
					<p class="font-medium">{t('services.submitted_title', 'Listing submitted')}</p>
					<p class="opacity-90">
						{t(
							'services.submitted_body',
							'Your listing is under review and will appear once a moderator approves it.'
						)}
					</p>
				</div>
				<button
					type="button"
					aria-label={t('common.dismiss', 'Dismiss')}
					onclick={() => (submittedDismissed = true)}
					class="shrink-0 text-emerald-600/70 hover:text-emerald-600"
				>
					<X size={14} />
				</button>
			</div>
		</div>
	{/if}

	<!-- Results -->
	{#if data.loadError}
		<EmptyState
			icon={Briefcase}
			title={t('services.load_error', 'Could not load services')}
			body={t('services.load_error_body', 'Please try again later.')}
			action={{ label: t('common.retry', 'Try again'), onclick: () => invalidateAll() }}
		/>
	{:else if services.length === 0}
		<EmptyState
			icon={Briefcase}
			title={t('services.empty', 'No services found')}
			body={t('services.empty_body', 'Try adjusting your filters or list a new service.')}
			action={data.authed
				? { label: t('services.list_a_service', 'List a Service'), href: '/services/new' }
				: { label: t('services.list_a_service', 'List a Service'), href: '/login?redirect=/services/new' }}
		/>
	{:else}
		<div
			class="grid gap-5 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4"
			aria-busy={!!navigating.to}
			class:opacity-60={!!navigating.to}
			class:pointer-events-none={!!navigating.to}
		>
			{#each services as service (service.id)}
				<ServiceCard
					listing={service}
					currencies={data.currencies}
					categories={data.serviceCategories}
					authed={data.authed}
				/>
			{/each}
		</div>

		{#if hasMore}
			<div class="mt-6 flex justify-center">
				<button
					type="button"
					onclick={loadMore}
					disabled={loadingMore}
					class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-5 py-2.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-60"
				>
					{loadingMore ? t('common.loading', 'Loading') : t('common.load_more', 'Load more')}
				</button>
			</div>
		{/if}
	{/if}

	<!-- Trust strip -->
	<section
		class="mt-10 grid gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5 sm:grid-cols-3"
		aria-label={t('services.trust_aria', 'Why book with JapaConnect')}
	>
		{#each trustItems as item (item.title)}
			{@const Icon = item.icon}
			<div class="flex items-start gap-3">
				<span
					class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]"
				>
					<Icon size={18} />
				</span>
				<div>
					<p class="text-sm font-semibold text-[var(--color-text)]">{item.title}</p>
					<p class="mt-0.5 text-xs leading-relaxed text-[var(--color-muted)]">{item.body}</p>
				</div>
			</div>
		{/each}
	</section>

	<!-- Provider CTA band -->
	<section
		class="mt-6 flex flex-col items-start gap-4 rounded-[var(--radius-card)] border border-[var(--color-primary)]/20 bg-[var(--color-primary)]/5 p-6 sm:flex-row sm:items-center sm:justify-between"
	>
		<div>
			<h2 class="text-lg font-semibold text-[var(--color-text)]">
				{t('services.provider_cta_title', 'Are you a service provider?')}
			</h2>
			<p class="mt-1 max-w-xl text-sm text-[var(--color-text-subtle)]">
				{t(
					'services.provider_cta_body',
					'Reach thousands of diaspora customers planning their move — list your service or package for free.'
				)}
			</p>
		</div>
		<a
			href={data.authed ? '/services/new' : '/signup'}
			class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2.5 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
		>
			<Plus size={15} />
			{t('services.provider_cta_button', 'List your service')}
		</a>
	</section>
</div>
