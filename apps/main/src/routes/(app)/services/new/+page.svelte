<script lang="ts">
	// Ref: japaconnect-engineering/sections/98-service-directory.md
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { Briefcase, Plus, Loader2, AlertCircle, Package, Trash2, ChevronDown } from 'lucide-svelte';
	import { serviceCategoryOptions } from '$lib/enumOptions';
	import { staticAutocompleteFetcher } from '$lib/autocomplete';
	import type { ServiceCategory, ServiceListing, Space } from '$lib/types';
	import type { PageData } from './$types';
	import AiBodyField from '$lib/ai/AiBodyField.svelte';
	import CoverUploader from '$lib/components/create/CoverUploader.svelte';
	import ServiceTierEditor from '$lib/components/services/ServiceTierEditor.svelte';
	import ServiceCouponSelector from '$lib/components/services/ServiceCouponSelector.svelte';
	import { tiersToPayload, type EditableTier } from '$lib/components/services/serviceForm';
	import type { CurrencyInfo } from '$lib/utils/currency';

	let { data }: { data: PageData } = $props();

	const packagesEnabled = $derived(data.packagesEnabled ?? false);
	// §111: package create/edit needs the service.package.create node —
	// without it the backend rejects with 403, so hide the option.
	const canCreatePackage = $derived(
		(data.user?.permissions ?? []).includes('service.package.create') ||
			(data.user?.permissions ?? []).includes('service.edit_any')
	);

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

	// §111 package fields — only sent when listing_kind='package'.
	let listingKind = $state<'service' | 'package'>('service');
	let orgId = $state('');
	let purchaseMode = $state<'inquiry' | 'external' | 'paid'>('inquiry');
	let priceAmount = $state(''); // major units; ×100 on submit
	let priceCurrency = $state('');
	let pricePer = $state<'person' | 'package' | 'group'>('package');
	let capacity = $state('');
	let availableFrom = $state('');
	let availableUntil = $state('');
	let pkgItems = $state<{ title: string; description: string; included: boolean; optional: boolean }[]>([]);
	let departures = $state<{ starts_on: string; ends_on: string; capacity: string }[]>([]);

	// §114 marketplace fields — featured image on all listing kinds;
	// tiers + coupon attach are package-only and flag-gated.
	const tiersEnabled = $derived(!!data.features?.['service_packages.tiers_enabled']);
	const couponsEnabled = $derived(!!data.features?.['service_coupons.enabled']);
	let featuredMediaId = $state('');
	let featuredPreview = $state('');
	let tiers = $state<EditableTier[]>([]);
	let couponIds = $state<string[]>([]);
	const currenciesList = $derived((page.data.currencies ?? []) as CurrencyInfo[]);

	// §114 structured content — faqs[]/steps[] apply to ALL listing kinds.
	// The client caps mirror the seeded system_settings
	// (service_marketplace.content.max_faqs=12 / max_steps=8); the backend
	// re-validates against the live settings, so these constants exist only
	// to hide the "add" button early (settings are not exposed to the form).
	const MAX_FAQS = 12;
	const MAX_STEPS = 8;
	let faqs = $state<{ question: string; answer: string }[]>([]);
	let steps = $state<{ title: string; description: string }[]>([]);

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

	// §111 org picker — packages are org-authored; the selling org must be
	// verified with a business/professional_body verification type unless
	// the backend test setting relaxes it. Ineligible orgs stay visible
	// (with an eligibility sublabel) so the member can see why their org
	// is missing rather than guessing.
	function orgEligible(o: { verified: boolean; verification_type?: string | null }): boolean {
		return o.verified && (o.verification_type === 'business' || o.verification_type === 'professional_body');
	}
	const orgItems = $derived<AutocompleteItem[]>(
		(data.myOrgs ?? []).map((o) => ({
			id: o.id,
			label: o.name,
			sublabel: orgEligible(o)
				? t('services.pkg_org_eligible', 'Verified — can publish packages')
				: t('services.pkg_org_ineligible', 'Not eligible — org must be verified (business/professional body)')
		}))
	);
	const orgFetcher = $derived(staticAutocompleteFetcher(orgItems));
	const selectedOrgLabel = $derived(
		orgId ? (orgItems.find((o) => o.id === orgId)?.label ?? null) : null
	);

	const purchaseModeItems = $derived<AutocompleteItem[]>([
		{ id: 'inquiry', label: t('services.pkg_mode_inquiry', 'Inquiry — travellers message you') },
		{ id: 'external', label: t('services.pkg_mode_external', 'External — book on your site') },
		{ id: 'paid', label: t('services.pkg_mode_paid', 'Paid — reserve with a platform fee') }
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
		if (listingKind === 'package') {
			if (!orgId) {
				errorMsg = t('services.errors.pkg_org_required', 'Select the organization selling this package');
				return false;
			}
			const org = (data.myOrgs ?? []).find((o) => o.id === orgId);
			if (org && !orgEligible(org)) {
				errorMsg = t('services.errors.pkg_org_ineligible', 'Only verified organizations (business or professional body) can publish packages');
				return false;
			}
			if (purchaseMode === 'paid' && (!String(priceAmount ?? '').trim() || !priceCurrency)) {
				errorMsg = t('services.errors.pkg_price_required', 'Paid packages require a price and currency');
				return false;
			}
			if (purchaseMode === 'external' && !bookingUrl.trim()) {
				errorMsg = t('services.errors.pkg_booking_url_required', 'External packages need a booking URL — that is the "book" link travellers follow');
				return false;
			}
			if (pkgItems.some((i) => !i.title.trim())) {
				errorMsg = t('services.errors.pkg_item_title_required', 'Every package item needs a title');
				return false;
			}
			if (departures.some((d) => !d.starts_on)) {
				errorMsg = t('services.errors.pkg_departure_start_required', 'Every departure needs a start date');
				return false;
			}
			if (tiersEnabled && tiers.some((tr) => !tr.name.trim())) {
				errorMsg = t('services.errors.tier_name_required', 'Every pricing tier needs a name');
				return false;
			}
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
				title: s.title.trim(),
				description: s.description.trim()
			}));
		}

		// §111 package payload — backend ignores these fields when
		// listing_kind='service', so they're only sent for packages.
		if (listingKind === 'package') {
			payload.listing_kind = 'package';
			payload.org_id = orgId;
			payload.purchase_mode = purchaseMode;
			payload.price_per = pricePer;
			// bind:value on type="number" yields number|null — coerce
			// back to string before trimming.
			const amountStr = String(priceAmount ?? '').trim();
			if (amountStr) {
				// UI collects major units; the API stores minor units.
				const major = Number(amountStr);
				if (!Number.isNaN(major)) payload.price_amount = Math.round(major * 100);
			}
			if (priceCurrency) payload.price_currency = priceCurrency;
			const capStr = String(capacity ?? '').trim();
			if (capStr) {
				const cap = Number(capStr);
				if (Number.isInteger(cap) && cap > 0) payload.capacity = cap;
			}
			if (availableFrom) payload.available_from = availableFrom;
			if (availableUntil) payload.available_until = availableUntil;
			if (pkgItems.length > 0) {
				payload.items = pkgItems.map((i) => ({
					title: i.title.trim(),
					description: i.description.trim(),
					included: i.included,
					optional: i.optional
				}));
			}
			if (departures.length > 0) {
				payload.departures = departures.map((d) => {
					// bind:value on type="number" yields number|null.
					const depCap = String(d.capacity ?? '').trim();
					return {
						starts_on: d.starts_on,
						ends_on: d.ends_on || undefined,
						capacity: depCap && Number.isInteger(Number(depCap)) ? Number(depCap) : null,
						status: 'open'
					};
				});
			}
			if (tiersEnabled && tiers.length > 0) {
				payload.tiers = tiersToPayload(tiers);
			}
		}

		submitting = true;
		const res = await api.post<ServiceListing>('/services', payload);
		submitting = false;

		if (res.data) {
			// §114: coupon attachment is a separate endpoint — the listing has
			// to exist before PUT /services/{id}/coupons can reference it. A
			// failed attach keeps the listing; the editor retries from edit.
			if (couponsEnabled && couponIds.length > 0) {
				const attach = await api.put(`/services/${res.data.id}/coupons`, { coupon_ids: couponIds });
				if (attach.error) {
					errorMsg = t(
						'services.errors.coupon_attach_failed',
						'Listing created, but attaching coupons failed — retry from the edit page.'
					);
					return;
				}
			}
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
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Briefcase size={20} /> {t('services.new_heading', 'List a Service')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{pageDescription}</p>
	</div>

	<div class="p-4">
		<form onsubmit={submit} class="space-y-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6" novalidate>
			<!-- §111 listing kind — packages are org-authored bundles sold as
				one unit; services are plain directory listings. -->
			{#if packagesEnabled && canCreatePackage}
				<div>
					<span class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('services.field_listing_kind', 'Listing type')}
					</span>
					<div class="grid gap-3 sm:grid-cols-2">
						<button
							type="button"
							onclick={() => (listingKind = 'service')}
							class="flex items-start gap-2 rounded-[var(--radius)] border px-3 py-2.5 text-left transition-colors {listingKind === 'service'
								? 'border-[var(--color-primary)] bg-[var(--color-primary)]/5'
								: 'border-[var(--color-border)] bg-[var(--color-surface)] hover:bg-[var(--color-surface-subtle)]'}"
						>
							<Briefcase size={18} class="mt-0.5 shrink-0 text-[var(--color-primary)]" />
							<span>
								<span class="block text-sm font-medium text-[var(--color-text)]">{t('services.kind_service', 'Service')}</span>
								<span class="block text-xs text-[var(--color-muted)]">{t('services.kind_service_hint', 'A single service listing in the directory')}</span>
							</span>
						</button>
						<button
							type="button"
							onclick={() => (listingKind = 'package')}
							class="flex items-start gap-2 rounded-[var(--radius)] border px-3 py-2.5 text-left transition-colors {listingKind === 'package'
								? 'border-[var(--color-primary)] bg-[var(--color-primary)]/5'
								: 'border-[var(--color-border)] bg-[var(--color-surface)] hover:bg-[var(--color-surface-subtle)]'}"
						>
							<Package size={18} class="mt-0.5 shrink-0 text-[var(--color-primary)]" />
							<span>
								<span class="block text-sm font-medium text-[var(--color-text)]">{t('services.kind_package', 'Package')}</span>
								<span class="block text-xs text-[var(--color-muted)]">{t('services.kind_package_hint', 'A priced bundle sold by your organization')}</span>
							</span>
						</button>
					</div>
				</div>
			{/if}

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

			<!-- §114 structured content — FAQs + how-it-works steps apply to
				all listing kinds; collapsed by default so the form stays lean. -->
			<details class="group rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)]/40">
				<summary class="flex cursor-pointer list-none items-center gap-2 px-4 py-3 text-sm font-semibold text-[var(--color-text)] [&::-webkit-details-marker]:hidden">
					{t('services.content_details_section', 'Content details (optional)')}
					<ChevronDown size={16} class="ml-auto shrink-0 text-[var(--color-muted)] transition-transform group-open:rotate-180" />
				</summary>
				<div class="space-y-4 border-t border-[var(--color-border)] px-4 py-4">
					<!-- FAQs repeater -->
					<div>
						<span class="mb-1 block text-sm font-medium text-[var(--color-text)]">
							{t('services.faq_items', 'FAQs')}
						</span>
						<p class="mb-2 text-xs text-[var(--color-muted)]">
							{t('services.faq_hint', 'Answer the questions travellers ask most — shown as an accordion on your listing.')}
						</p>
						<div class="space-y-2">
							{#each faqs as faq, idx (idx)}
								<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
									<div class="flex items-start gap-2">
										<div class="flex-1 space-y-2">
											<input
												type="text"
												bind:value={faq.question}
												placeholder={t('services.faq_question_ph', 'Question (e.g. How long does processing take?)')}
												class={inputClass}
												aria-label={t('services.faq_question_ph', 'Question')}
											/>
											<textarea
												bind:value={faq.answer}
												rows={2}
												placeholder={t('services.faq_answer_ph', 'Answer')}
												class="{inputClass} resize-y"
												aria-label={t('services.faq_answer_ph', 'Answer')}
											></textarea>
										</div>
										<button
											type="button"
											onclick={() => (faqs = faqs.filter((_, i) => i !== idx))}
											class="mt-1 text-[var(--color-muted)] hover:text-[var(--color-danger)]"
											aria-label={t('common.remove', 'Remove')}
										>
											<Trash2 size={16} />
										</button>
									</div>
								</div>
							{/each}
						</div>
						{#if faqs.length < MAX_FAQS}
							<button
								type="button"
								onclick={() => (faqs = [...faqs, { question: '', answer: '' }])}
								class="mt-2 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
							>
								<Plus size={14} /> {t('services.faq_add', 'Add question')}
							</button>
						{/if}
					</div>

					<!-- Steps repeater -->
					<div>
						<span class="mb-1 block text-sm font-medium text-[var(--color-text)]">
							{t('services.steps_items', 'How it works')}
						</span>
						<p class="mb-2 text-xs text-[var(--color-muted)]">
							{t('services.steps_hint', 'Walk travellers through the process — rendered as numbered steps on your listing.')}
						</p>
						<div class="space-y-2">
							{#each steps as step, idx (idx)}
								<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
									<div class="flex items-start gap-2">
										<div class="flex-1 space-y-2">
											<input
												type="text"
												bind:value={step.title}
												placeholder={t('services.step_title_ph', 'Step title (e.g. Initial consultation)')}
												class={inputClass}
												aria-label={t('services.step_title_ph', 'Step title')}
											/>
											<input
												type="text"
												bind:value={step.description}
												placeholder={t('services.step_desc_ph', 'Short description (optional)')}
												class={inputClass}
												aria-label={t('services.step_desc_ph', 'Short description')}
											/>
										</div>
										<button
											type="button"
											onclick={() => (steps = steps.filter((_, i) => i !== idx))}
											class="mt-1 text-[var(--color-muted)] hover:text-[var(--color-danger)]"
											aria-label={t('common.remove', 'Remove')}
										>
											<Trash2 size={16} />
										</button>
									</div>
								</div>
							{/each}
						</div>
						{#if steps.length < MAX_STEPS}
							<button
								type="button"
								onclick={() => (steps = [...steps, { title: '', description: '' }])}
								class="mt-2 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
							>
								<Plus size={14} /> {t('services.steps_add', 'Add step')}
							</button>
						{/if}
					</div>
				</div>
			</details>

			<!-- §111 package fields -->
			{#if packagesEnabled && listingKind === 'package'}
				<div class="space-y-4 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)]/40 p-4">
					<h2 class="flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
						<Package size={16} /> {t('services.pkg_section', 'Package details')}
					</h2>

					<!-- Selling org -->
					<div>
						<label for="pkg-org" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
							{t('services.pkg_org', 'Selling organization')} <span class="text-red-500">*</span>
						</label>
						{#if (data.myOrgs ?? []).length > 0}
							<Autocomplete
								id="pkg-org"
								placeholder={t('services.pkg_org_select', 'Select your organization')}
								emptyMessage={t('common.no_results', 'No results found')}
								fetcher={orgFetcher}
								initialFetcher={orgFetcher}
								selectedId={orgId || null}
								selectedLabel={selectedOrgLabel}
								onSelect={(item) => (orgId = item.id)}
								onClear={() => (orgId = '')}
							/>
							<p class="mt-1 text-xs text-[var(--color-muted)]">
								{t('services.pkg_org_hint', 'Only verified organizations (business or professional body) can publish packages.')}
							</p>
						{:else}
							<p class="text-sm text-[var(--color-muted)]">
								{t('services.pkg_no_orgs', 'You must be a member of a verified organization to publish a package.')}
							</p>
						{/if}
					</div>

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
								{t('services.pkg_price', 'Price')}{#if purchaseMode === 'paid'} <span class="text-red-500">*</span>{/if}
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

					<!-- price_per + capacity -->
					<div class="grid gap-4 sm:grid-cols-2">
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
						<div>
							<label for="pkg-capacity" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
								{t('services.pkg_capacity', 'Total capacity')}
							</label>
							<input
								id="pkg-capacity"
								type="number"
								min="1"
								bind:value={capacity}
								placeholder={t('services.pkg_capacity_ph', 'Leave empty for unlimited')}
								class={inputClass}
							/>
						</div>
					</div>

					<!-- Availability window -->
					<div class="grid gap-4 sm:grid-cols-2">
						<div>
							<label for="pkg-from" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
								{t('services.pkg_available_from', 'Available from')}
							</label>
							<input id="pkg-from" type="date" bind:value={availableFrom} class={inputClass} />
						</div>
						<div>
							<label for="pkg-until" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
								{t('services.pkg_available_until', 'Available until')}
							</label>
							<input id="pkg-until" type="date" bind:value={availableUntil} class={inputClass} />
						</div>
					</div>

					<!-- Items editor -->
					<div>
						<span class="mb-1 block text-sm font-medium text-[var(--color-text)]">
							{t('services.pkg_items', 'Included items')}
						</span>
						<div class="space-y-2">
							{#each pkgItems as item, idx (idx)}
								<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
									<div class="flex items-start gap-2">
										<div class="flex-1 space-y-2">
											<input
												type="text"
												bind:value={item.title}
												placeholder={t('services.pkg_item_title_ph', 'Item title (e.g. Visa processing)')}
												class={inputClass}
											/>
											<input
												type="text"
												bind:value={item.description}
												placeholder={t('services.pkg_item_desc_ph', 'Short description (optional)')}
												class={inputClass}
											/>
											<div class="flex gap-4 text-xs text-[var(--color-muted)]">
												<label class="flex items-center gap-1.5">
													<input type="checkbox" checked={item.included} onchange={(e) => (item.included = e.currentTarget.checked)} class="h-3.5 w-3.5 rounded border-[var(--color-border)]" />
													{t('services.pkg_item_included', 'Included')}
												</label>
												<label class="flex items-center gap-1.5">
													<input type="checkbox" checked={item.optional} onchange={(e) => (item.optional = e.currentTarget.checked)} class="h-3.5 w-3.5 rounded border-[var(--color-border)]" />
													{t('services.pkg_item_optional', 'Optional add-on')}
												</label>
											</div>
										</div>
										<button
											type="button"
											onclick={() => (pkgItems = pkgItems.filter((_, i) => i !== idx))}
											class="mt-1 text-[var(--color-muted)] hover:text-[var(--color-danger)]"
											aria-label={t('common.remove', 'Remove')}
										>
											<Trash2 size={16} />
										</button>
									</div>
								</div>
							{/each}
						</div>
						<button
							type="button"
							onclick={() => (pkgItems = [...pkgItems, { title: '', description: '', included: true, optional: false }])}
							class="mt-2 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
						>
							<Plus size={14} /> {t('services.pkg_add_item', 'Add item')}
						</button>
					</div>

					<!-- Departures editor -->
					<div>
						<span class="mb-1 block text-sm font-medium text-[var(--color-text)]">
							{t('services.pkg_departures', 'Scheduled departures')}
						</span>
						<p class="mb-2 text-xs text-[var(--color-muted)]">
							{t('services.pkg_departures_hint', 'Optional fixed dates (tours, cohort intakes). Leave empty for request-based packages.')}
						</p>
						<div class="space-y-2">
							{#each departures as dep, idx (idx)}
								<div class="flex flex-wrap items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
									<input type="date" bind:value={dep.starts_on} class="{inputClass} sm:w-auto" aria-label={t('services.pkg_dep_start', 'Start date')} />
									<span class="text-xs text-[var(--color-muted)]">→</span>
									<input type="date" bind:value={dep.ends_on} class="{inputClass} sm:w-auto" aria-label={t('services.pkg_dep_end', 'End date (optional)')} />
									<input
										type="number"
										min="1"
										bind:value={dep.capacity}
										placeholder={t('services.pkg_dep_capacity_ph', 'Seats')}
										class="{inputClass} sm:w-24"
										aria-label={t('services.pkg_dep_capacity_ph', 'Seats')}
									/>
									<button
										type="button"
										onclick={() => (departures = departures.filter((_, i) => i !== idx))}
										class="ml-auto text-[var(--color-muted)] hover:text-[var(--color-danger)]"
										aria-label={t('common.remove', 'Remove')}
									>
										<Trash2 size={16} />
									</button>
								</div>
							{/each}
						</div>
						<button
							type="button"
							onclick={() => (departures = [...departures, { starts_on: '', ends_on: '', capacity: '' }])}
							class="mt-2 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
						>
							<Plus size={14} /> {t('services.pkg_add_departure', 'Add departure')}
						</button>
					</div>

					<!-- §114 pricing tiers — service_packages.tiers_enabled -->
					{#if tiersEnabled}
						<ServiceTierEditor bind:tiers {inputClass} currencies={currenciesList} />
					{/if}

					<!-- §114 coupon attach — service_coupons.enabled; needs the
						selling org picked first (coupons are org-owned). -->
					{#if couponsEnabled}
						<ServiceCouponSelector bind:selectedIds={couponIds} {orgId} listingKind="package" />
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
