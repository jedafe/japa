<script lang="ts">
	// Ref: japaconnect-engineering/sections/98-service-directory.md
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { Briefcase, Plus, Loader2, AlertCircle } from 'lucide-svelte';
	import { serviceCategoryOptions } from '$lib/enumOptions';
	import type { ServiceCategory, ServiceListing, Space } from '$lib/types';
	import type { PageData } from './$types';
	import AiBodyField from '$lib/ai/AiBodyField.svelte';

	let { data }: { data: PageData } = $props();

	const pageTitle = $derived(t('services.new_title', 'List a Service — JapaConnect'));
	const pageDescription = $derived(
		t('services.new_description', 'List a diaspora-relevant service in the Service Directory.')
	);

	// Form state.
	let title = $state('');
	let body = $state('');
	let serviceCategory = $state<ServiceCategory>(untrack(() => data.serviceCategories[0] ?? 'immigration_legal'));
	let destinationSpaceId = $state('');
	let citySpaceId = $state('');
	let originCommunities = $state('');
	let isRemote = $state(false);
	let websiteUrl = $state('');
	let bookingUrl = $state('');
	let priceRange = $state('');

	let submitting = $state(false);
	let errorMsg = $state('');

	// BUG-010: city is a dependent select populated from /spaces?level=city&parent=<dest>.
	let cities = $state<Space[]>([]);
	let loadingCities = $state(false);

	const categoryOptions = $derived(serviceCategoryOptions(t, data.serviceCategories));

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';

	// Fetch cities when the destination country changes (BUG-010).
	let lastFetchedDest = $state('');
	$effect(() => {
		if (!destinationSpaceId || destinationSpaceId === lastFetchedDest) return;
		lastFetchedDest = destinationSpaceId;
		citySpaceId = '';
		cities = [];
		loadingCities = true;
		(async () => {
			try {
				const res = await api.get<Space[]>(`/spaces?level=city&parent=${destinationSpaceId}`);
				cities = Array.isArray(res.data) ? res.data : [];
			} catch {
				cities = [];
			} finally {
				loadingCities = false;
			}
		})();
	});

	function validate(): boolean {
		if (!title.trim()) {
			errorMsg = t('services.errors.title_required', 'Title is required');
			return false;
		}
		if (!destinationSpaceId) {
			errorMsg = t('services.errors.dest_required', 'Destination country is required');
			return false;
		}
		return true;
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errorMsg = '';
		if (!validate()) return;

		// Parse comma-separated origin communities into a string array.
		const origins = originCommunities
			.split(',')
			.map((s) => s.trim())
			.filter((s) => s.length > 0);

		const payload: Record<string, unknown> = {
			title: title.trim(),
			body: body.trim(),
			service_category: serviceCategory,
			destination_space_id: destinationSpaceId,
			is_remote: isRemote
		};
		if (citySpaceId) payload.city_space_id = citySpaceId;
		if (origins.length > 0) payload.origin_communities = origins;
		if (websiteUrl.trim()) payload.website_url = websiteUrl.trim();
		if (bookingUrl.trim()) payload.booking_url = bookingUrl.trim();
		if (priceRange.trim()) payload.price_range = priceRange.trim();

		submitting = true;
		const res = await api.post<ServiceListing>('/services', payload);
		submitting = false;

		if (res.data) {
			goto(`/services/${res.data.slug}`);
		} else {
			errorMsg = res.error?.message ?? t('services.errors.create_failed', 'Failed to create listing');
		}
	}

	function fetchDestinationCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('common.all_countries', 'All countries');
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
		destinationSpaceId ? (data.countries ?? []).find((c) => c.id === destinationSpaceId)?.name ?? null : null
	);
</script>

<svelte:head>
	<title>{pageTitle}</title>
	<meta name="description" content={pageDescription} />
	<meta property="og:title" content={pageTitle} />
	<meta property="og:description" content={pageDescription} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-2xl">
	<!-- Header -->
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Briefcase size={20} /> {t('services.new_heading', 'List a Service')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{pageDescription}</p>
	</div>

	<div class="p-4">
		<form onsubmit={submit} class="space-y-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6" novalidate>
			<!-- Title -->
			<div>
				<label for="title" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('services.field_title', 'Title')} <span class="text-red-500">*</span>
				</label>
				<input
					id="title"
					type="text"
					bind:value={title}
					placeholder={t('services.title_placeholder', 'e.g. Immigration Lawyer — UK Visa Applications')}
					class={inputClass}
				/>
			</div>

			<!-- Body -->
			<AiBodyField
				id="body"
				label={t('services.field_description', 'Description')}
				contentType="service"
				{title}
				body={body}
				onBody={(v) => (body = v)}
				onTitle={(v) => (title = v)}
				user={data.user}
				features={data.features ?? {}}
				placeholder={t('services.description_placeholder', 'Describe the service you offer…')}
				rows={5}
			/>

			<!-- Category -->
			<div>
				<label for="category" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('services.field_category', 'Service category')}
				</label>
				<select id="category" bind:value={serviceCategory} class={inputClass}>
					{#each categoryOptions as opt (opt.value)}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>

			<!-- Destination + City -->
			<div class="grid gap-4 sm:grid-cols-2">
				<div>
					<label for="dest" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('services.field_destination', 'Destination country')} <span class="text-red-500">*</span>
					</label>
					<Autocomplete
						placeholder={t('services.dest_placeholder', 'Select a country')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchDestinationCountries}
						initialFetcher={fetchDestinationCountriesInitial}
						selectedId={destinationSpaceId || null}
						selectedLabel={selectedDestinationLabel}
						onSelect={(item) => (destinationSpaceId = item.id)}
						onClear={() => (destinationSpaceId = '')}
					/>
				</div>
				<div>
					<label for="city" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('services.field_city', 'City')}
					</label>
					<select
						id="city"
						bind:value={citySpaceId}
						class={inputClass}
						disabled={!destinationSpaceId || loadingCities}
					>
						<option value="">
							{loadingCities
								? t('services.city_loading', 'Loading cities…')
								: destinationSpaceId
									? t('services.city_select', 'Select a city (optional)')
									: t('services.city_select_dest_first', 'Select a country first')}
						</option>
						{#each cities as city (city.id)}
							<option value={city.id}>{city.name}</option>
						{/each}
					</select>
				</div>
			</div>

			<!-- Origin communities -->
			<div>
				<label for="origin" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('services.field_origin_communities', 'Origin communities served')}
				</label>
				<input
					id="origin"
					type="text"
					bind:value={originCommunities}
					placeholder={t('services.origin_placeholder', 'e.g. Nigerian, Ghanaian')}
					class={inputClass}
				/>
				<p class="mt-1 text-xs text-[var(--color-muted)]">
					{t('services.origin_hint', 'Comma-separated communities this provider specialises in serving.')}
				</p>
			</div>

			<!-- Remote -->
			<div>
				<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
					<input
						type="checkbox"
						checked={isRemote}
						onchange={(e) => (isRemote = e.currentTarget.checked)}
						class="h-4 w-4 rounded border-[var(--color-border)]"
					/>
					{t('services.field_remote', 'This service is offered remotely')}
				</label>
			</div>

			<!-- Website + Booking -->
			<div class="grid gap-4 sm:grid-cols-2">
				<div>
					<label for="website" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('services.field_website', 'Website URL')}
					</label>
					<input
						id="website"
						type="url"
						bind:value={websiteUrl}
						placeholder={t('services.website_placeholder', 'https://example.com')}
						class={inputClass}
					/>
				</div>
				<div>
					<label for="booking" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('services.field_booking', 'Booking URL')}
					</label>
					<input
						id="booking"
						type="url"
						bind:value={bookingUrl}
						placeholder={t('services.booking_placeholder', 'https://booking.example.com')}
						class={inputClass}
					/>
				</div>
			</div>

			<!-- Price range -->
			<div>
				<label for="price" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('services.field_price_range', 'Price range')}
				</label>
				<input
					id="price"
					type="text"
					bind:value={priceRange}
					placeholder={t('services.price_placeholder', 'e.g. £50–£150/hr')}
					class={inputClass}
				/>
			</div>

			<!-- Error -->
			{#if errorMsg}
				<div class="flex items-center gap-2 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
					<AlertCircle size={16} /> {errorMsg}
				</div>
			{/if}

			<!-- Submit -->
			<div class="flex justify-end">
				<button
					type="submit"
					disabled={submitting}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>
					{#if submitting}
						<Loader2 size={16} class="animate-spin" />
					{:else}
						<Plus size={16} />
					{/if}
					{submitting ? t('common.submitting', 'Submitting…') : t('services.submit', 'List service')}
				</button>
			</div>
		</form>
	</div>
</div>
