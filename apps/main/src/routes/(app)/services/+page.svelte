<script lang="ts">
	// Ref: japaconnect-engineering/sections/98-service-directory.md
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import SearchInput from '$lib/components/ui/SearchInput.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import {
		Briefcase,
		Plus,
		MapPin,
		Globe,
		BadgeCheck,
		ExternalLink,
		ArrowRight,
		X,
		Wifi,
		Check,
		Package
	} from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { ListPageBar, UrlTabs, type UrlTab, FilterToggle, PageAction } from '$lib/components/ui';
	import PaginationControls from '$lib/components/ui/PaginationControls.svelte';
	import { serviceCategoryOptions, serviceCategoryLabel } from '$lib/enumOptions';
	import { staticAutocompleteFetcher } from '$lib/autocomplete';
	import type { ServiceListing, ServiceCategory, Space } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('services.list_title', 'Service Directory — JapaConnect'));
	const description = $derived(
		t(
			'services.list_description',
			'Find verified diaspora-relevant businesses and service providers — immigration lawyers, relocation agents, financial advisors, and more.'
		)
	);

	const services = $derived<ServiceListing[]>(data.services);

	// Local filter state seeded from URL params (synced on navigation via the
	// $effect below so back/forward and shared links repaint the controls).
	let queryInput = $state(untrack(() => data.filters.q));
	let destInput = $state(untrack(() => data.filters.dest));
	let cityInput = $state(untrack(() => data.filters.city));
	let cityLabelInput = $state(untrack(() => data.filters.cityLabel));
	let categoryInput = $state(untrack(() => data.filters.category));
	let originInput = $state(untrack(() => data.filters.origin));
	let verifiedInput = $state(untrack(() => data.filters.verified === 'true'));
	let sortInput = $state(untrack(() => data.filters.sort));
	let kindInput = $state(untrack(() => data.filters.kind));
	let limitInput = $state(untrack(() => data.filters.limit));
	let priceMaxInput = $state(untrack(() => data.filters.priceMaxMajor));
	let dateFromInput = $state(untrack(() => data.filters.date_from));

	$effect(() => {
		queryInput = data.filters.q;
		destInput = data.filters.dest;
		cityInput = data.filters.city;
		cityLabelInput = data.filters.cityLabel;
		categoryInput = data.filters.category;
		originInput = data.filters.origin;
		verifiedInput = data.filters.verified === 'true';
		sortInput = data.filters.sort;
		kindInput = data.filters.kind;
		limitInput = data.filters.limit;
		priceMaxInput = data.filters.priceMaxMajor;
		dateFromInput = data.filters.date_from;
	});

	const categoryOptions = $derived(serviceCategoryOptions(t, data.serviceCategories));

	const categoryItems = $derived<AutocompleteItem[]>([
		{ id: '', label: t('services.filter_category', 'All categories') },
		...categoryOptions.map((o) => ({ id: o.value, label: o.label }))
	]);
	const categoryFetcher = $derived(staticAutocompleteFetcher(categoryItems));
	const selectedCategoryLabel = $derived(
		categoryInput ? (categoryItems.find((o) => o.id === categoryInput)?.label ?? categoryInput) : null
	);

	// §111 kind axis — only rendered while service_packages.enabled is on.
	const kindItems = $derived<AutocompleteItem[]>([
		{ id: '', label: t('services.kind_all', 'All listings') },
		{ id: 'service', label: t('services.kind_service', 'Services') },
		{ id: 'package', label: t('services.kind_package', 'Packages') }
	]);
	const kindFetcher = $derived(staticAutocompleteFetcher(kindItems));
	const selectedKindLabel = $derived(
		kindInput ? (kindItems.find((o) => o.id === kindInput)?.label ?? null) : null
	);

	const limitItems: AutocompleteItem[] = ['10', '20', '50'].map((n) => ({ id: n, label: n }));
	const limitFetcher = staticAutocompleteFetcher(limitItems);

	function categoryLabel(value: string): string {
		return serviceCategoryLabel(t, value as ServiceCategory);
	}

	const sortOptions = [
		{ value: 'latest', label: t('services.sort_latest', 'Latest') },
		{ value: 'top', label: t('services.sort_top', 'Top') }
	];

	const tabs = $derived<UrlTab[]>(sortOptions.map((o) => ({ value: o.value, label: o.label })));

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

	function buildParams(): URLSearchParams {
		const params = new URLSearchParams();
		if (queryInput.trim()) params.set('q', queryInput.trim());
		if (destInput) params.set('dest', destInput);
		if (cityInput) params.set('city', cityInput);
		if (categoryInput) params.set('category', categoryInput);
		if (originInput.trim()) params.set('origin', originInput.trim());
		if (verifiedInput) params.set('verified', 'true');
		if (sortInput && sortInput !== 'latest') params.set('sort', sortInput);
		if (kindInput && kindInput !== 'all') params.set('kind', kindInput);
		if (limitInput !== 20) params.set('limit', String(limitInput));
		// §111 package-only filters — the API takes price_max in minor
		// units; the input collects major units like the create form.
		if (kindInput === 'package') {
			const major = Number(priceMaxInput);
			if (priceMaxInput.trim() && Number.isFinite(major) && major >= 0) {
				params.set('price_max', String(Math.round(major * 100)));
			}
			if (dateFromInput) params.set('date_from', dateFromInput);
		}
		return params;
	}

	function selectSort(v: string) {
		const params = buildParams();
		if (v === 'latest') params.delete('sort');
		else params.set('sort', v);
		goto(`/services${params.toString() ? `?${params.toString()}` : ''}`, { keepFocus: true });
	}

	let showFilters = $state(false);
	$effect(() => {
		if (hasActiveFilters) showFilters = true;
	});

	let submittedDismissed = $state(false);
	const showSubmitted = $derived(data.submitted && !submittedDismissed);

	function applyFilters(e?: SubmitEvent) {
		e?.preventDefault();
		const params = buildParams().toString();
		goto(`/services${params ? `?${params}` : ''}`, { keepFocus: true });
	}

	function clearFilters() {
		queryInput = '';
		destInput = '';
		cityInput = '';
		cityLabelInput = '';
		categoryInput = '';
		originInput = '';
		verifiedInput = false;
		sortInput = 'latest';
		kindInput = '';
		limitInput = 20;
		priceMaxInput = '';
		dateFromInput = '';
		goto('/services');
	}

	const hasActiveFilters = $derived(
		!!data.filters.q ||
			!!data.filters.dest ||
			!!data.filters.city ||
			!!data.filters.category ||
			!!data.filters.origin ||
			!!data.filters.verified ||
			!!data.filters.kind ||
			!!data.filters.priceMaxMajor ||
			!!data.filters.date_from ||
			data.filters.limit !== 20
	);

	const totalPages = $derived(
		data.meta?.total ? Math.ceil(data.meta.total / data.filters.limit) : 1
	);
	const currentPage = $derived(data.filters.page || 1);

	// Live suggestions for the search box — the backend title search (?q=).
	async function fetchServiceSuggestions(q: string) {
		const res = await api.get<ServiceListing[]>(
			`/services?q=${encodeURIComponent(q.trim())}&limit=5`
		);
		return (res.data ?? []).map((s) => ({
			id: s.id,
			title: s.title,
			url: `/services/${s.slug}`,
			type: 'service'
		}));
	}

	function fetchDestinationCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('services.filter_dest', 'All countries');
		const query = q.toLowerCase().trim();
		const allItem: AutocompleteItem = { id: '', label: allLabel };
		if (query === '') {
			return Promise.resolve([allItem, ...(data.countries ?? []).map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }))]);
		}
		const matchesAll = allLabel.toLowerCase().includes(query);
		const filtered = (data.countries ?? []).filter(
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
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<h1 class="sr-only">{t('services.heading', 'Service Directory')}</h1>
	<ListPageBar>
		<UrlTabs
			{tabs}
			value={data.filters.sort}
			onValueChange={(v) => selectSort(v)}
			class="!border-b-0 !bg-transparent"
		/>
		{#snippet actions()}
			<FilterToggle bind:show={showFilters} active={hasActiveFilters} controls="services-filter-panel" />
			{#if data.authed && data.packagesEnabled}
				<PageAction
					href="/me/service-bookings"
					icon={Package}
					label={t('bookings.my_title', 'Service Bookings')}
				/>
			{/if}
			<PageAction
				href={data.authed ? '/services/new' : '/login?redirect=/services/new'}
				icon={Plus}
				label={t('services.list_a_service', 'List a Service')}
			/>
		{/snippet}
	</ListPageBar>

	<!-- Service list -->
	<div class="p-3">
		{#if showSubmitted}
			<div class="mb-4 rounded-[var(--radius)] border border-emerald-500/30 bg-emerald-500/10 p-3 text-sm text-emerald-700">
				<div class="flex items-start gap-2">
					<Check size={16} class="mt-0.5 shrink-0" />
					<div class="flex-1">
						<p class="font-medium">{t('services.submitted_title', 'Listing submitted')}</p>
						<p class="opacity-90">{t('services.submitted_body', 'Your listing is under review and will appear once a moderator approves it.')}</p>
					</div>
					<button
						type="button"
						aria-label={t('common.dismiss', 'Dismiss')}
						onclick={() => (submittedDismissed = true)}
						class="shrink-0 text-emerald-700/70 hover:text-emerald-700"
					>
						<X size={14} />
					</button>
				</div>
			</div>
		{/if}

		{#if showFilters}
			<!-- Filter panel -->
			<div id="services-filter-panel" class="mb-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
		<form onsubmit={applyFilters} class="grid gap-2 sm:grid-cols-2 lg:grid-cols-3" role="search">
			<div class="sm:col-span-2 lg:col-span-3">
				<SearchInput
					bind:value={queryInput}
					placeholder={t('services.search_placeholder', 'Search services…')}
					ariaLabel={t('services.search_placeholder', 'Search services…')}
					fetcher={fetchServiceSuggestions}
				/>
			</div>
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
			<Autocomplete
				placeholder={t('services.filter_category', 'All categories')}
				ariaLabel={t('services.filter_category', 'All categories')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={categoryFetcher}
				initialFetcher={categoryFetcher}
				selectedId={categoryInput || null}
				selectedLabel={selectedCategoryLabel}
				onSelect={(item) => { categoryInput = item.id; applyFilters(); }}
				onClear={() => { categoryInput = ''; applyFilters(); }}
			/>
			{#if data.packagesEnabled}
				<Autocomplete
					placeholder={t('services.kind_all', 'All listings')}
					ariaLabel={t('services.filter_kind', 'Listing type')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={kindFetcher}
					initialFetcher={kindFetcher}
					selectedId={kindInput || null}
					selectedLabel={selectedKindLabel}
					onSelect={(item) => { kindInput = item.id; applyFilters(); }}
					onClear={() => { kindInput = ''; applyFilters(); }}
				/>
			{/if}
			{#if kindInput === 'package'}
				<!-- §111 package filters — price_max (major-unit input, minor-unit
					URL param) and date_from (availability start). -->
				<div>
					<label for="services-price-max" class="sr-only">{t('services.filter_price_max', 'Max price')}</label>
					<input
						id="services-price-max"
						type="number"
						min="0"
						step="0.01"
						bind:value={priceMaxInput}
						placeholder={t('services.filter_price_max', 'Max price')}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
					/>
				</div>
				<div>
					<label for="services-date-from" class="sr-only">{t('services.filter_date_from', 'Available from')}</label>
					<input
						id="services-date-from"
						type="date"
						bind:value={dateFromInput}
						aria-label={t('services.filter_date_from', 'Available from')}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
					/>
				</div>
			{/if}
			<div class="flex items-center gap-2">
				<label class="flex items-center gap-1.5 text-sm text-[var(--color-text)]">
					<input
						type="checkbox"
						checked={verifiedInput}
						onchange={(e) => { verifiedInput = e.currentTarget.checked; applyFilters(); }}
						class="h-4 w-4 rounded border-[var(--color-border)]"
					/>
					{t('services.filter_verified', 'Verified only')}
				</label>
			</div>
			<div>
				<label for="services-origin-filter" class="sr-only">{t('services.filter_origin', 'Origin community')}</label>
				<input
					id="services-origin-filter"
					type="text"
					bind:value={originInput}
					placeholder={t('services.filter_origin', 'Origin community (e.g. Nigerian)')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
				/>
			</div>
			<div class="flex items-center gap-2">
				<Autocomplete
					placeholder={t('services.page_size', 'Per page')}
					ariaLabel={t('services.page_size', 'Results per page')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={limitFetcher}
					initialFetcher={limitFetcher}
					selectedId={String(limitInput)}
					selectedLabel={String(limitInput)}
					onSelect={(item) => { limitInput = Number(item.id); applyFilters(); }}
				/>
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

		{#if data.loadError}
			<EmptyState
				icon={Briefcase}
				title={t('services.load_error', 'Could not load services')}
				body={t('services.load_error_body', 'Please try again later.')}
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
			{#if data.meta?.total}
				<p class="mb-3 text-xs text-[var(--color-muted)]">
					{t('common.showing', 'Showing {start}-{end} of {total}')
						.replace('{start}', String((currentPage - 1) * data.filters.limit + 1))
						.replace('{end}', String(Math.min(currentPage * data.filters.limit, data.meta.total)))
						.replace('{total}', String(data.meta.total))}
				</p>
			{/if}

			<div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
				{#each services as service (service.id)}
					<a
						href="/services/{service.slug}"
						class="flex flex-col gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<div class="flex items-start justify-between gap-2">
							<h2 class="line-clamp-2 text-sm font-semibold text-[var(--color-text)]">{service.title}</h2>
							{#if service.is_verified}
								<BadgeCheck size={16} class="shrink-0 text-[var(--color-primary)]" />
							{/if}
						</div>

						<!-- Category badge -->
						<span class="w-fit rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs text-[var(--color-text-subtle)]">
							{service.service_category ? categoryLabel(service.service_category as ServiceCategory) : ''}
						</span>

						<!-- Org + location + remote -->
						<div class="flex flex-wrap gap-2 text-xs text-[var(--color-muted)]">
							{#if service.org_name}
								<span class="flex items-center gap-1"><Briefcase size={12} /> {service.org_name}</span>
							{/if}
							{#if service.destination_space_name}
								<span class="flex items-center gap-1"><MapPin size={12} /> {service.destination_space_name}</span>
							{/if}
							{#if service.city_space_name}
								<span>{service.city_space_name}</span>
							{/if}
							{#if service.is_remote}
								<span class="flex items-center gap-1"><Wifi size={12} /> {t('services.remote', 'Remote')}</span>
							{/if}
						</div>

						<!-- Price range -->
						{#if service.price_range}
							<p class="text-xs font-medium text-[var(--color-text-subtle)]">{service.price_range}</p>
						{/if}

						<!-- Links -->
						{#if service.website_url || service.booking_url}
							<div class="flex flex-wrap gap-2">
								{#if service.website_url}
									<span class="flex items-center gap-1 text-xs text-[var(--color-primary)]">
										<Globe size={12} /> {t('services.website', 'Website')}
									</span>
								{/if}
								{#if service.booking_url}
									<span class="flex items-center gap-1 text-xs text-[var(--color-primary)]">
										<ExternalLink size={12} /> {t('services.book', 'Book')}
									</span>
								{/if}
							</div>
						{/if}

						<!-- Verification footer -->
						<div class="mt-auto flex items-center gap-3 border-t border-[var(--color-border)] pt-2 text-xs text-[var(--color-muted)]">
							{#if service.is_verified && service.verification_type}
								<span class="flex items-center gap-1 text-[var(--color-primary)]">
									<BadgeCheck size={12} />
									{service.verification_type}
								</span>
							{/if}
							<ArrowRight size={14} class="ml-auto text-[var(--color-muted)]" />
						</div>
					</a>
				{/each}
			</div>

			{#if totalPages > 1}
				<div class="mt-4 flex justify-center">
					<PaginationControls
						page={currentPage}
						{totalPages}
						onPageChange={(p) => {
							const params = buildParams();
							params.set('page', String(p));
							goto(`/services?${params.toString()}`);
						}}
					/>
				</div>
			{/if}
		{/if}
	</div>
</div>
