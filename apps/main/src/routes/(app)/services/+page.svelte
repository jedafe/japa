// Ref: japaconnect-engineering/sections/98-service-directory.md
<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
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
	import PaginationControls from '$lib/components/ui/PaginationControls.svelte';
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
	let cityInput = $state(untrack(() => data.filters.city));
	let categoryInput = $state(untrack(() => data.filters.category));
	let originInput = $state(untrack(() => data.filters.origin));
	let verifiedInput = $state(untrack(() => data.filters.verified === 'true'));
	let sortInput = $state(untrack(() => data.filters.sort));

	$effect(() => {
		destInput = data.filters.dest;
		cityInput = data.filters.city;
		categoryInput = data.filters.category;
		originInput = data.filters.origin;
		verifiedInput = data.filters.verified === 'true';
		sortInput = data.filters.sort;
	});

	// Schema-defined service_category enum (§98) — labels via i18n fallback.
	const categoryOptions: { value: ServiceCategory; label: string }[] = [
		{ value: 'immigration_legal', label: t('services.cat_immigration_legal', 'Immigration & Legal') },
		{ value: 'relocation', label: t('services.cat_relocation', 'Relocation') },
		{ value: 'finance', label: t('services.cat_finance', 'Finance') },
		{ value: 'education', label: t('services.cat_education', 'Education') },
		{ value: 'health', label: t('services.cat_health', 'Health') },
		{ value: 'community', label: t('services.cat_community', 'Community') },
		{ value: 'other', label: t('services.cat_other', 'Other') }
	];

	function categoryLabel(value: string): string {
		return categoryOptions.find((o) => o.value === value)?.label ?? value;
	}

	const sortOptions = [
		{ value: 'latest', label: t('services.sort_latest', 'Latest') },
		{ value: 'rating', label: t('services.sort_rating', 'Top rated') },
		{ value: 'distance', label: t('services.sort_distance', 'Distance') }
	];

	function buildParams(): URLSearchParams {
		const params = new URLSearchParams();
		if (destInput) params.set('dest', destInput);
		if (cityInput) params.set('city', cityInput);
		if (categoryInput) params.set('category', categoryInput);
		if (originInput) params.set('origin', originInput);
		if (verifiedInput) params.set('verified', 'true');
		if (sortInput && sortInput !== 'latest') params.set('sort', sortInput);
		return params;
	}

	function applyFilters(e: SubmitEvent) {
		e.preventDefault();
		goto(`/services?${buildParams().toString()}`, { keepFocus: true });
	}

	function clearFilters() {
		destInput = '';
		cityInput = '';
		categoryInput = '';
		originInput = '';
		verifiedInput = false;
		sortInput = 'latest';
		goto('/services');
	}

	const hasActiveFilters = $derived(
		!!data.filters.dest ||
			!!data.filters.city ||
			!!data.filters.category ||
			!!data.filters.origin ||
			!!data.filters.verified ||
			(data.filters.sort && data.filters.sort !== 'latest')
	);

	const totalPages = $derived(
		data.meta?.total ? Math.ceil(data.meta.total / data.pageSize) : 1
	);
	const currentPage = $derived(data.filters.page || 1);

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';
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
	<!-- Header -->
	<div class="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
		<div class="min-w-0">
			<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
				<Briefcase size={20} /> {t('services.heading', 'Service Directory')}
			</h1>
			<p class="mt-1 text-sm text-[var(--color-muted)]">{description}</p>
		</div>
		{#if data.authed}
			<a
				href="/services/new"
				class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				<Plus size={14} /> {t('services.list_a_service', 'List a Service')}
			</a>
		{:else}
			<a
				href="/login?redirect=/services/new"
				class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				<Plus size={14} /> {t('services.list_a_service', 'List a Service')}
			</a>
		{/if}
	</div>

	<!-- Filter bar -->
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<form onsubmit={applyFilters} class="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
			<input
				type="text"
				placeholder={t('services.filter_dest', 'Destination country')}
				value={destInput}
				oninput={(e) => (destInput = e.currentTarget.value)}
				class={inputClass}
			/>
			<input
				type="text"
				placeholder={t('services.filter_city', 'City')}
				value={cityInput}
				oninput={(e) => (cityInput = e.currentTarget.value)}
				class={inputClass}
			/>
			<select
				value={categoryInput}
				onchange={(e) => (categoryInput = e.currentTarget.value)}
				class={inputClass}
			>
				<option value="">{t('services.filter_category', 'All categories')}</option>
				{#each categoryOptions as opt (opt.value)}
					<option value={opt.value}>{opt.label}</option>
				{/each}
			</select>
			<input
				type="text"
				placeholder={t('services.filter_origin', 'Origin community')}
				value={originInput}
				oninput={(e) => (originInput = e.currentTarget.value)}
				class={inputClass}
			/>
			<div class="flex items-center gap-2">
				<label class="flex items-center gap-1.5 text-sm text-[var(--color-text)]">
					<input
						type="checkbox"
						checked={verifiedInput}
						onchange={(e) => (verifiedInput = e.currentTarget.checked)}
						class="h-4 w-4 rounded border-[var(--color-border)]"
					/>
					{t('services.filter_verified', 'Verified only')}
				</label>
			</div>
			<select
				value={sortInput}
				onchange={(e) => (sortInput = e.currentTarget.value)}
				class={inputClass}
			>
				{#each sortOptions as opt (opt.value)}
					<option value={opt.value}>{opt.label}</option>
				{/each}
			</select>
			<button
				type="submit"
				class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:opacity-90"
			>
				{t('common.filter', 'Filter')}
			</button>
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

	<!-- Service list -->
	<div class="p-3">
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
