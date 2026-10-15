<script lang="ts">
	// Ref: japaconnect-engineering/sections/98-service-directory.md
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { Briefcase, Plus, Loader2, AlertCircle, Package } from 'lucide-svelte';
	import { serviceCategoryOptions } from '$lib/enumOptions';
	import { staticAutocompleteFetcher } from '$lib/autocomplete';
	import type { ServiceCategory, ServiceListing, Space } from '$lib/types';
	import { destinationMarkets, pinMarketsFirst } from '$lib/utils/geoMarkets';
	import type { PageData } from './$types';
	import AiBodyField from '$lib/ai/AiBodyField.svelte';
	import CoverUploader from '$lib/components/create/CoverUploader.svelte';
	import ServiceTierEditor from '$lib/components/services/ServiceTierEditor.svelte';
	import ServiceContentEditor from '$lib/components/services/ServiceContentEditor.svelte';
	import type {
		FactEntry,
		FaqEntry,
		IconEntry,
		StepEntry
	} from '$lib/components/services/ServiceContentEditor.svelte';
	import { tiersToPayload, type EditableTier } from '$lib/components/services/serviceForm';
	import type { CurrencyInfo } from '$lib/utils/currency';

	let { data }: { data: PageData } = $props();

	const packagesEnabled = $derived(data.packagesEnabled ?? false);

	const pageTitle = $derived(t('services.new_title', 'List a Service — JapaConnect'));
	const pageDescription = $derived(
		t('services.new_description', 'List a diaspora-relevant service in the Service Directory.')
	);

	// Form state.
	let title = $state('');
	let body = $state('');
	let serviceCategory = $state<ServiceCategory>(
		untrack(() => (data.serviceCategories[0]?.value as ServiceCategory | undefined) ?? 'immigration_legal')
	);
	let destinationSpaceId = $state('');
	let citySpaceId = $state('');
	let cityLabel = $state('');
	let originCommunities = $state('');
	let isRemote = $state(false);
	let websiteUrl = $state('');
	let bookingUrl = $state('');
	let priceRange = $state('');

	// Booking/pricing fields (§114 amendment) — apply to every
	// listing kind. The 'package' listing kind was removed from
	// the feature (2026-10): packages are gone, so these fields
	// describe the service itself.
	let purchaseMode = $state<'inquiry' | 'external'>('inquiry');
	let priceAmount = $state(''); // major units; ×100 on submit
	let priceCurrency = $state('');
	let pricePer = $state<'person' | 'package' | 'group'>('person');

	// §114 marketplace fields — featured image on all listing kinds;
	// tiers are flag-gated and apply to every kind.
	const tiersEnabled = $derived(!!data.features?.['service_packages.tiers_enabled']);
	let featuredMediaId = $state('');
	let featuredPreview = $state('');
	let tiers = $state<EditableTier[]>([]);
	const currenciesList = $derived((page.data.currencies ?? []) as CurrencyInfo[]);

	// §114/000829 structured content — faqs[]/steps[]/selling_points[]/
	// facts[]/included[]/highlights[]/audience[] apply to ALL listing
	// kinds. The client caps live inside ServiceContentEditor; the backend
	// re-validates against the live system_settings.
	let faqs = $state<FaqEntry[]>([]);
	let steps = $state<StepEntry[]>([]);
	let sellingPoints = $state<IconEntry[]>([]);
	let facts = $state<FactEntry[]>([]);
	let included = $state<IconEntry[]>([]);
	let highlights = $state<string[]>([]);
	let audience = $state<string[]>([]);

	let submitting = $state(false);
	let errorMsg = $state('');

	const categoryOptions = $derived(serviceCategoryOptions(t, data.serviceCategories));
	const categoryItems = $derived<AutocompleteItem[]>(
		categoryOptions.map((o) => ({ id: o.value, label: o.label }))
	);
	const categoryFetcher = $derived(staticAutocompleteFetcher(categoryItems));
	const selectedCategoryLabel = $derived(
		serviceCategory ? (categoryItems.find((o) => o.id === serviceCategory)?.label ?? null) : null
	);

	// §114 2026-10-10 amendment: 'paid' mode was package-bound
	// (§111) and the package kind is gone — services offer
	// inquiry/external only.
	const purchaseModeItems = $derived<AutocompleteItem[]>([
		{ id: 'inquiry', label: t('services.pkg_mode_inquiry', 'Inquiry — travellers message you') },
		{ id: 'external', label: t('services.pkg_mode_external', 'External — book on your site') }
	]);
	const purchaseModeFetcher = $derived(staticAutocompleteFetcher(purchaseModeItems));

	const pricePerItems = $derived<AutocompleteItem[]>([
		{ id: 'person', label: t('services.pkg_per_person', 'per person') },
		{ id: 'package', label: t('services.pkg_per_package', 'per package') },
		{ id: 'group', label: t('services.pkg_per_group', 'per group') }
	]);
	const pricePerFetcher = $derived(staticAutocompleteFetcher(pricePerItems));

	const currencyItems = $derived<AutocompleteItem[]>(
		((page.data.currencies ?? []) as { code: string; name: string; symbol: string }[]).map((c) => ({
			id: c.code,
			label: `${c.code} — ${c.name}`,
			sublabel: c.symbol
		}))
	);
	const currencyFetcher = $derived(staticAutocompleteFetcher(currencyItems));
	const selectedCurrencyLabel = $derived(
		priceCurrency ? (currencyItems.find((o) => o.id === priceCurrency)?.label ?? priceCurrency) : null
	);

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';

	// BUG-010: the city picker searches /spaces?level=city&geo=<country code>
	// — geo covers cities nested under state spaces (e.g. Nigeria), which
	// ?parent=<country_id> misses because it only matches direct children.
	const destGeoCode = $derived(
		destinationSpaceId
			? (data.countries ?? []).find((c) => c.id === destinationSpaceId)?.geo_code ?? ''
			: ''
	);

	async function fetchCities(q: string): Promise<AutocompleteItem[]> {
		if (!destGeoCode) return [];
		const res = await api.get<Space[]>(
			`/spaces?level=city&geo=${encodeURIComponent(destGeoCode)}&q=${encodeURIComponent(q.trim())}&page_size=25`
		);
		return (res.data ?? []).map((c) => ({ id: c.id, label: c.name }));
	}

	function fetchCitiesInitial(): Promise<AutocompleteItem[]> {
		return fetchCities('');
	}

	// Reset the city when the destination country changes.
	let prevDest = $state(untrack(() => destinationSpaceId));
	$effect(() => {
		if (destinationSpaceId !== prevDest) {
			prevDest = destinationSpaceId;
			citySpaceId = '';
			cityLabel = '';
		}
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
		// service_category is NOT NULL + enum-validated on the backend —
		// catch a missing pick here instead of surfacing a 500.
		if (!serviceCategory) {
			errorMsg = t('services.errors.category_required', 'Service category is required');
			return false;
		}
		// §114 — the backend rejects blank questions/titles (invalid_faq /
		// invalid_step); catch them here so the row can be fixed in place.
		if (faqs.some((f) => !f.question.trim())) {
			errorMsg = t('services.errors.faq_question_required', 'Every FAQ needs a question');
			return false;
		}
		if (steps.some((s) => !s.title.trim())) {
			errorMsg = t('services.errors.step_title_required', 'Every step needs a title');
			return false;
		}
		// 000829 — icon-entry and fact groups need their primary text; the
		// backend rejects blank titles/labels (invalid_selling_point /
		// invalid_included / invalid_fact).
		if (sellingPoints.some((p) => !p.title.trim())) {
			errorMsg = t('services.errors.point_title_required', 'Every selling point needs a title');
			return false;
		}
		if (included.some((p) => !p.title.trim())) {
			errorMsg = t('services.errors.included_title_required', 'Every included item needs a title');
			return false;
		}
		if (facts.some((f) => !f.label.trim() || !f.value.trim())) {
			errorMsg = t('services.errors.fact_required', 'Every fact needs a label and a value');
			return false;
		}
		if (purchaseMode === 'external' && !bookingUrl.trim()) {
			errorMsg = t('services.errors.pkg_booking_url_required', 'External listings need a booking URL — that is the "book" link travellers follow');
			return false;
		}
		if (tiersEnabled && tiers.some((tr) => !tr.name.trim())) {
			errorMsg = t('services.errors.tier_name_required', 'Every pricing tier needs a name');
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
		if (featuredMediaId) payload.featured_media_id = featuredMediaId;

		// §114 structured content — all listing kinds. Only non-empty sets
		// are sent on create (the columns default to '[]' either way).
		if (faqs.length > 0) {
			payload.faqs = faqs.map((f) => ({
				question: f.question.trim(),
				answer: f.answer.trim()
			}));
		}
		if (steps.length > 0) {
			payload.steps = steps.map((s) => ({
				icon: s.icon,
				title: s.title.trim(),
				description: s.description.trim()
			}));
		}
		if (sellingPoints.length > 0) {
			payload.selling_points = sellingPoints.map((p) => ({
				icon: p.icon,
				title: p.title.trim(),
				description: p.description.trim()
			}));
		}
		if (facts.length > 0) {
			payload.facts = facts.map((f) => ({
				icon: f.icon,
				label: f.label.trim(),
				value: f.value.trim()
			}));
		}
		if (included.length > 0) {
			payload.included = included.map((p) => ({
				icon: p.icon,
				title: p.title.trim(),
				description: p.description.trim()
			}));
		}
		const trimmedHighlights = highlights.map((h) => h.trim()).filter(Boolean);
		if (trimmedHighlights.length > 0) payload.highlights = trimmedHighlights;
		const trimmedAudience = audience.map((a) => a.trim()).filter(Boolean);
		if (trimmedAudience.length > 0) payload.audience = trimmedAudience;

		// §114 2026-10-10 amendment: purchase mode, price and
		// tiers apply to service kind too (tiered pricing cards
		// on the service detail page).
		payload.purchase_mode = purchaseMode;
		{
			// bind:value on type="number" yields number|null — coerce
			// back to string before trimming.
			const amountStr = String(priceAmount ?? '').trim();
			if (amountStr) {
				// UI collects major units; the API stores minor units.
				const major = Number(amountStr);
				if (!Number.isNaN(major)) payload.price_amount = Math.round(major * 100);
			}
			if (priceCurrency) payload.price_currency = priceCurrency;
			if (amountStr) payload.price_per = pricePer;
			if (tiersEnabled && tiers.length > 0) {
				payload.tiers = tiersToPayload(tiers);
			}
		}

		submitting = true;
		const res = await api.post<ServiceListing>('/services', payload);
		submitting = false;

		if (res.data) {
			// §111: new listings enter `in_review` unless review_before_publish
			// is off — land on the detail page when the listing went live,
			// else the directory with a "submitted for review" banner (the
			// public detail URL only serves published rows).
			if (res.data.status === 'published') {
				goto(`/services/${res.data.slug}`);
			} else {
				goto('/services?submitted=1');
			}
		} else {
			errorMsg = res.error?.message ?? t('services.errors.create_failed', 'Failed to create listing');
		}
	}

	// ADR-064 curated markets (public settings) — pinned first in the
	// country pickers; every seeded country stays selectable.
	const destCountries = $derived(
		pinMarketsFirst(data.countries ?? [], destinationMarkets(data.siteSettings), (c) => c.geo_code)
	);

	function fetchDestinationCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('common.all_countries', 'All countries');
		const query = q.toLowerCase().trim();
		const allItem: AutocompleteItem = { id: '', label: allLabel };
		if (query === '') {
			return Promise.resolve([allItem, ...destCountries.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }))]);
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
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="mx-auto w-full max-w-2xl">
	<!-- Header -->
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
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
					{t('services.field_category', 'Service category')} *
				</label>
				<Autocomplete
					id="category"
					placeholder={t('services.field_category', 'Service category')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={categoryFetcher}
					initialFetcher={categoryFetcher}
					selectedId={serviceCategory || null}
					selectedLabel={selectedCategoryLabel}
					onSelect={(item) => (serviceCategory = item.id as ServiceCategory)}
				/>
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
					<Autocomplete
						id="city"
						placeholder={destinationSpaceId
							? t('services.city_select', 'Select a city (optional)')
							: t('services.city_select_dest_first', 'Select a country first')}
						ariaLabel={t('services.field_city', 'City')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchCities}
						initialFetcher={fetchCitiesInitial}
						selectedId={citySpaceId || null}
						selectedLabel={citySpaceId ? cityLabel || null : null}
						disabled={!destinationSpaceId}
						onSelect={(item) => {
							citySpaceId = item.id;
							cityLabel = item.label;
						}}
						onClear={() => {
							citySpaceId = '';
							cityLabel = '';
						}}
					/>
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

			<!-- §114 featured image — contents.featured_image_id via
				featured_media_id on the service payload; all listing kinds. -->
			<div>
				<span class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('services.featured_image', 'Featured image')}
				</span>
				<CoverUploader
					mediaId={featuredMediaId}
					previewUrl={featuredPreview}
					onUploaded={(a) => {
						featuredMediaId = a.id;
						featuredPreview = a.url;
					}}
					onRemove={() => {
						featuredMediaId = '';
						featuredPreview = '';
					}}
				/>
			</div>

			<!-- §114/000829 structured content — one collapsed section
				shared with the edit form (ServiceContentEditor). -->
			<ServiceContentEditor
				{inputClass}
				bind:faqs
				bind:steps
				bind:sellingPoints
				bind:facts
				bind:included
				bind:highlights
				bind:audience
			/>

			<!-- §114 booking/pricing (all kinds) -->
			{#if packagesEnabled}
				<div class="space-y-4 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)]/40 p-4">
					<h2 class="flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
						<Package size={16} />
						{t('services.booking_section', 'Booking & pricing')}
					</h2>

					<!-- Purchase mode + price -->
					<div class="grid gap-4 sm:grid-cols-2">
						<div>
							<label for="pkg-mode" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
								{t('services.pkg_purchase_mode', 'Purchase mode')}
							</label>
							<Autocomplete
								id="pkg-mode"
								placeholder={t('services.pkg_purchase_mode', 'Purchase mode')}
								emptyMessage={t('common.no_results', 'No results found')}
								fetcher={purchaseModeFetcher}
								initialFetcher={purchaseModeFetcher}
								selectedId={purchaseMode}
								selectedLabel={purchaseModeItems.find((o) => o.id === purchaseMode)?.label ?? null}
								onSelect={(item) => (purchaseMode = item.id as typeof purchaseMode)}
							/>
						</div>
						<div>
							<label for="pkg-price" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
								{t('services.pkg_price', 'Price')}
							</label>
							<div class="flex gap-2">
								<input
									id="pkg-price"
									type="number"
									min="0"
									step="0.01"
									bind:value={priceAmount}
									placeholder="0.00"
									class={inputClass}
								/>
								<div class="w-40 shrink-0">
									<Autocomplete
										placeholder={t('services.pkg_currency', 'Currency')}
										ariaLabel={t('services.pkg_currency', 'Currency')}
										emptyMessage={t('common.no_results', 'No results found')}
										fetcher={currencyFetcher}
										initialFetcher={currencyFetcher}
										selectedId={priceCurrency || null}
										selectedLabel={selectedCurrencyLabel}
										onSelect={(item) => (priceCurrency = item.id)}
										onClear={() => (priceCurrency = '')}
									/>
								</div>
							</div>
						</div>
					</div>

					<!-- price_per -->
					<div>
						<label for="pkg-price-per" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
							{t('services.pkg_price_per', 'Price is per')}
						</label>
						<Autocomplete
							id="pkg-price-per"
							placeholder={t('services.pkg_price_per', 'Price is per')}
							emptyMessage={t('common.no_results', 'No results found')}
							fetcher={pricePerFetcher}
							initialFetcher={pricePerFetcher}
							selectedId={pricePer}
							selectedLabel={pricePerItems.find((o) => o.id === pricePer)?.label ?? null}
							onSelect={(item) => (pricePer = item.id as typeof pricePer)}
						/>
					</div>

					<!-- §114 pricing tiers — service_packages.tiers_enabled; all
						listing kinds (service + mentorship cards too). -->
					{#if tiersEnabled}
						<ServiceTierEditor bind:tiers {inputClass} currencies={currenciesList} />
					{/if}
				</div>
			{/if}

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
