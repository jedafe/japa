<script lang="ts">
	// Ref: japaconnect-engineering/sections/98-service-directory.md
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import {
		Briefcase,
		Plus,
		MapPin,
		Globe,
		BadgeCheck,
		ExternalLink,
		ArrowRight,
		X,
		Wifi
	} from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { ListPageBar, UrlTabs, type UrlTab, FilterToggle, PageAction } from '$lib/components/ui';
	import PaginationControls from '$lib/components/ui/PaginationControls.svelte';
	import { serviceCategoryOptions, serviceCategoryLabel } from '$lib/enumOptions';
	import { staticAutocompleteFetcher } from '$lib/autocomplete';
	import type { ServiceListing, ServiceCategory } from '$lib/types';
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

	// Local filter state seeded from URL params.
	let destInput = $state(untrack(() => data.filters.dest));
	let categoryInput = $state(untrack(() => data.filters.category));
	let verifiedInput = $state(untrack(() => data.filters.verified === 'true'));
	let sortInput = $state(untrack(() => data.filters.sort));

	$effect(() => {
		destInput = data.filters.dest;
		categoryInput = data.filters.category;
		verifiedInput = data.filters.verified === 'true';
		sortInput = data.filters.sort;
	});

	const categoryOptions = $derived(serviceCategoryOptions(t, data.serviceCategories));

	const categoryItems = $derived<AutocompleteItem[]>(
		categoryOptions.map((o) => ({ id: o.value, label: o.label }))
	);
	const categoryFetcher = $derived(staticAutocompleteFetcher(categoryItems));
	const selectedCategoryLabel = $derived(
		categoryInput ? (categoryItems.find((o) => o.id === categoryInput)?.label ?? categoryInput) : null
	);

	function categoryLabel(value: string): string {
		return serviceCategoryLabel(t, value as ServiceCategory);
	}

	const sortOptions = [
		{ value: 'latest', label: t('services.sort_latest', 'Latest') },
		{ value: 'top', label: t('services.sort_top', 'Top') }
	];

	const tabs = $derived<UrlTab[]>(sortOptions.map((o) => ({ value: o.value, label: o.label })));

	function buildParams(): URLSearchParams {
		const params = new URLSearchParams();
		if (destInput) params.set('dest', destInput);
		if (categoryInput) params.set('category', categoryInput);
		if (verifiedInput) params.set('verified', 'true');
		if (sortInput && sortInput !== 'latest') params.set('sort', sortInput);
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

	function applyFilters(e?: SubmitEvent) {
		e?.preventDefault();
		goto(`/services?${buildParams().toString()}`, { keepFocus: true });
	}

	function clearFilters() {
		destInput = '';
		categoryInput = '';
		verifiedInput = false;
		sortInput = 'latest';
		goto('/services');
	}

	const hasActiveFilters = $derived(
		!!data.filters.dest || !!data.filters.category || !!data.filters.verified
	);

	const totalPages = $derived(
		data.meta?.total ? Math.ceil(data.meta.total / data.pageSize) : 1
	);
	const currentPage = $derived(data.filters.page || 1);

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
		destInput ? (data.countries ?? []).find((c) => c.id === destInput)?.name ?? null : null
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
			<PageAction
				href={data.authed ? '/services/new' : '/login?redirect=/services/new'}
				icon={Plus}
				label={t('services.list_a_service', 'List a Service')}
			/>
		{/snippet}
	</ListPageBar>

	<!-- Service list -->
	<div class="p-3">
		{#if showFilters}
			<!-- Filter panel -->
			<div id="services-filter-panel" class="mb-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
		<form onsubmit={applyFilters} class="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
			<Autocomplete
				placeholder={t('services.filter_dest', 'All countries')}
				ariaLabel={t('services.filter_dest', 'All countries')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={fetchDestinationCountries}
				initialFetcher={fetchDestinationCountriesInitial}
				selectedId={destInput || null}
				selectedLabel={selectedDestinationLabel}
				onSelect={(item) => { destInput = item.id; applyFilters(); }}
				onClear={() => { destInput = ''; applyFilters(); }}
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
						.replace('{start}', String((currentPage - 1) * data.pageSize + 1))
						.replace('{end}', String(Math.min(currentPage * data.pageSize, data.meta.total)))
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
