<script lang="ts">
	// Ref: japaconnect-engineering/sections/98-service-directory.md
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { Briefcase, Save, Loader2, AlertCircle, Package, Trash2, Plus, ChevronDown } from 'lucide-svelte';
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

	const svc = $derived(data.service);

	const pageTitle = $derived(t('services.edit_title', 'Edit Service — JapaConnect'));
	const pageDescription = $derived(
		t('services.edit_description', 'Update your service listing.')
	);

	// Form state, seeded once from the loaded listing.
	let title = $state(untrack(() => svc.title));
	let body = $state(untrack(() => svc.body ?? ''));
	let serviceCategory = $state<ServiceCategory>(
		untrack(() =>
			(data.serviceCategories.some((o) => o.value === svc.service_category)
				? (svc.service_category as ServiceCategory)
				: (data.serviceCategories[0]?.value as ServiceCategory | undefined)) ?? 'other'
		)
	);
	let destinationSpaceId = $state(untrack(() => svc.destination_space_id ?? ''));
	let citySpaceId = $state(untrack(() => svc.city_space_id ?? ''));
	// City chip label — seeded from the loaded listing (no id→name lookup
	// endpoint exists for spaces), replaced on each picker selection.
	let cityLabel = $state(untrack(() => svc.city_space_name ?? ''));
	let originCommunities = $state(untrack(() => svc.origin_communities?.join(', ') ?? ''));
	let isRemote = $state(untrack(() => svc.is_remote));
	let websiteUrl = $state(untrack(() => svc.website_url ?? ''));
	let bookingUrl = $state(untrack(() => svc.booking_url ?? ''));
	let priceRange = $state(untrack(() => svc.price_range ?? ''));

	// §111 package fields — listing_kind is immutable after creation and
	// org_id is locked (package_org_immutable), so packages edit price,
	// mode, capacity, availability, items and departures only.
	const isPackage = $derived(svc.listing_kind === 'package');
	let purchaseMode = $state<'inquiry' | 'external' | 'paid'>(
		untrack(() => (svc.purchase_mode === 'external' || svc.purchase_mode === 'paid' ? svc.purchase_mode : 'inquiry'))
	);
	let priceAmount = $state(untrack(() => (svc.price_amount != null ? String(svc.price_amount / 100) : '')));
	let priceCurrency = $state(untrack(() => svc.price_currency ?? ''));
	let pricePer = $state<'person' | 'package' | 'group'>(
		untrack(() => (svc.price_per === 'person' || svc.price_per === 'group' ? svc.price_per : 'package'))
	);
	let capacity = $state(untrack(() => (svc.capacity != null ? String(svc.capacity) : '')));
	let availableFrom = $state(untrack(() => svc.available_from ?? ''));
	let availableUntil = $state(untrack(() => svc.available_until ?? ''));
	let pkgItems = $state(
		untrack(() =>
			(svc.items ?? []).map((i) => ({
				title: i.title,
				description: i.description ?? '',
				included: i.included,
				optional: i.optional
			}))
		)
	);
	// §111: data.departures is the editor-scoped list (ALL departures — the
	// public detail hides past/full/cancelled rows). id/status/booked_count
	// ride along so saves update existing rows in place instead of breaking
	// their booking departure_id links via delete+reinsert.
	let departures = $state(
		untrack(() =>
			(data.departures ?? []).map((d) => ({
				id: d.id,
				starts_on: d.starts_on,
				ends_on: d.ends_on ?? '',
				capacity: d.capacity != null ? String(d.capacity) : '',
				status: d.status ?? 'open',
				booked_count: d.booked_count ?? 0
			}))
		)
	);

	// §114 structured content — faqs[]/steps[] are replace-on-save like
	// items[]/departures[]: the edit payload always sends both arrays so
	// removed rows actually clear (absent preserves on PATCH). The client
	// caps mirror the seeded system_settings
	// (service_marketplace.content.max_faqs=12 / max_steps=8); the backend
	// re-validates against the live settings.
	const MAX_FAQS = 12;
	const MAX_STEPS = 8;
	let faqs = $state(
		untrack(() => (svc.faqs ?? []).map((f) => ({ question: f.question, answer: f.answer })))
	);
	let steps = $state(
		untrack(() => (svc.steps ?? []).map((s) => ({ title: s.title, description: s.description })))
	);
	// Open the optional section when the listing already carries content —
	// otherwise a seeded set would be invisible behind the collapsed details.
	let detailsOpen = $state(untrack(() => (svc.faqs?.length ?? 0) > 0 || (svc.steps?.length ?? 0) > 0));

	// §114 marketplace fields — featured image on all listing kinds;
	// tiers + coupon attach are package-only and flag-gated. The tier seed
	// comes from the detail payload's ACTIVE set — no editor-scoped tier
	// list exists yet, so suspended tiers are outside the editor's reach
	// (a replace-save would drop them; is_active is omitted on write so
	// every submitted tier stays active).
	const tiersEnabled = $derived(!!data.features?.['service_packages.tiers_enabled']);
	const couponsEnabled = $derived(!!data.features?.['service_coupons.enabled']);
	let featuredMediaId = $state(untrack(() => svc.featured_media_id ?? ''));
	let featuredPreview = $state(untrack(() => svc.featured_image_url ?? ''));
	let tiers = $state<EditableTier[]>(
		untrack(() =>
			(svc.tiers ?? []).map((tr) => ({
				name: tr.name,
				badge: tr.badge ?? '',
				description: tr.description ?? '',
				priceAmount: tr.price_amount != null ? String(tr.price_amount / 100) : '',
				currency: tr.price_currency ?? '',
				featuresText: (tr.features ?? []).join('\n'),
				isFeatured: tr.is_featured
			}))
		)
	);
	// couponIds is seeded inside ServiceCouponSelector from the
	// editor-scoped GET /services/{id}/coupons (all statuses). The PUT only
	// runs once that seed resolved — sending [] early would detach all.
	let couponIds = $state<string[]>([]);
	let couponsReady = $state(false);
	const currenciesList = $derived((page.data.currencies ?? []) as CurrencyInfo[]);

	let submitting = $state(false);
	let errorMsg = $state('');

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
	// Departure status — editors can open or cancel; 'full' is system-set
	// (capacity exhausted) and only shown as the current value.
	const depStatusItems = $derived<AutocompleteItem[]>([
		{ id: 'open', label: t('services.pkg_dep_status_open', 'Open') },
		{ id: 'cancelled', label: t('services.pkg_dep_status_cancelled', 'Cancelled') }
	]);
	const depStatusFetcher = $derived(staticAutocompleteFetcher(depStatusItems));
	function depStatusLabel(status: string): string {
		if (status === 'full') return t('services.pkg_dep_status_full', 'Full');
		return depStatusItems.find((o) => o.id === status)?.label ?? status;
	}
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

	const categoryOptions = $derived(serviceCategoryOptions(t, data.serviceCategories));
	const categoryItems = $derived<AutocompleteItem[]>(
		categoryOptions.map((o) => ({ id: o.value, label: o.label }))
	);
	const categoryFetcher = $derived(staticAutocompleteFetcher(categoryItems));
	const selectedCategoryLabel = $derived(
		serviceCategory ? (categoryItems.find((o) => o.id === serviceCategory)?.label ?? null) : null
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
		errorMsg = '';
		if (!title.trim()) {
			errorMsg = t('services.errors.title_required', 'Title is required');
			return false;
		}
		if (!destinationSpaceId) {
			errorMsg = t('services.errors.dest_required', 'Destination country is required');
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
		if (isPackage) {
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
		if (!validate()) return;

		const origins = originCommunities
			.split(',')
			.map((s) => s.trim())
			.filter((s) => s.length > 0);

		// Clearable fields are always sent — the backend treats a present-but-
		// empty value as "clear the column" (PATCH semantics), so omitting them
		// would silently keep the old values.
		const payload: Record<string, unknown> = {
			title: title.trim(),
			body: body.trim(),
			service_category: serviceCategory,
			destination_space_id: destinationSpaceId,
			city_space_id: citySpaceId,
			origin_communities: origins,
			is_remote: isRemote,
			website_url: websiteUrl.trim(),
			booking_url: bookingUrl.trim(),
			price_range: priceRange.trim(),
			// §114 — replace-on-save for all listing kinds: always send both
			// arrays; an empty array clears the stored set.
			faqs: faqs.map((f) => ({ question: f.question.trim(), answer: f.answer.trim() })),
			steps: steps.map((s) => ({ title: s.title.trim(), description: s.description.trim() }))
		};

		// §114 — featured_media_id semantics: absent preserves, '' clears,
		// a UUID replaces. Only send when the pick actually changed.
		if (featuredMediaId !== (svc.featured_media_id ?? '')) {
			payload.featured_media_id = featuredMediaId;
		}

		// §111 packages — items[]/departures[] are replace-on-save: always
		// send the arrays so removed rows actually delete. listing_kind and
		// org_id are immutable after creation and never sent. Clear
		// convention: price_amount/capacity take 0 to clear (null would
		// merge-keep the stored value); '' clears currency/dates.
		if (isPackage) {
			payload.purchase_mode = purchaseMode;
			payload.price_per = pricePer;
			// bind:value on type="number" yields number|null — coerce
			// back to string before trimming.
			const amountStr = String(priceAmount ?? '').trim();
			payload.price_amount = amountStr ? Math.round(Number(amountStr) * 100) : 0;
			payload.price_currency = priceCurrency;
			const capStr = String(capacity ?? '').trim();
			payload.capacity = capStr ? Number(capStr) : 0;
			payload.available_from = availableFrom;
			payload.available_until = availableUntil;
			payload.items = pkgItems.map((i) => ({
				title: i.title.trim(),
				description: i.description.trim(),
				included: i.included,
				optional: i.optional
			}));
			payload.departures = departures.map((d) => {
				// bind:value on type="number" yields number|null.
				const depCap = String(d.capacity ?? '').trim();
				return {
					...(d.id ? { id: d.id } : {}),
					starts_on: d.starts_on,
					ends_on: d.ends_on || '',
					capacity: depCap && Number(depCap) > 0 ? Number(depCap) : null,
					status: d.status || 'open'
				};
			});
			// §114 tiers are replace-on-save like items[]/departures[] —
			// always sent while the flag is on so removals clear.
			if (tiersEnabled) {
				payload.tiers = tiersToPayload(tiers);
			}
		}

		submitting = true;
		const res = await api.patch<ServiceListing>(`/services/${svc.id}`, payload);
		submitting = false;

		if (res.data) {
			// §114: coupon attachment lives on its own replace-on-save
			// endpoint — PUT every save while the flag is on so unchecking
			// the last coupon detaches it. A failed attach keeps the edit.
			if (isPackage && couponsEnabled && couponsReady) {
				const attach = await api.put(`/services/${svc.id}/coupons`, { coupon_ids: couponIds });
				if (attach.error) {
					errorMsg = t(
						'services.errors.coupon_attach_failed_edit',
						'Changes saved, but updating coupons failed — save again to retry.'
					);
					return;
				}
			}
			// §111: commercial package edits bounce the listing back to
			// in_review, and the public detail URL only serves published
			// rows — land on the directory banner instead of a 404.
			if (res.data.status && res.data.status !== 'published') {
				goto('/services?submitted=1');
			} else {
				goto(`/services/${res.data.slug ?? svc.slug}`);
			}
		} else {
			errorMsg = res.error?.message ?? t('services.errors.edit_failed', 'Failed to update listing');
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
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="mx-auto w-full max-w-2xl">
	<!-- Header -->
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Briefcase size={20} /> {t('services.edit_heading', 'Edit Service')}
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
				all listing kinds; collapsed unless the listing already has some. -->
			<details class="group rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)]/40" bind:open={detailsOpen}>
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

			<!-- §111 package fields — the selling org is immutable after
				creation (package_org_immutable); shown read-only. -->
			{#if isPackage}
				<div class="space-y-4 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)]/40 p-4">
					<h2 class="flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
						<Package size={16} /> {t('services.pkg_section', 'Package details')}
						{#if svc.org_name}
							<span class="ml-auto text-xs font-normal text-[var(--color-muted)]">{svc.org_name}</span>
						{/if}
					</h2>

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
								<input id="pkg-price" type="number" min="0" step="0.01" bind:value={priceAmount} placeholder="0.00" class={inputClass} />
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
							<input id="pkg-capacity" type="number" min="1" bind:value={capacity} placeholder={t('services.pkg_capacity_ph', 'Leave empty for unlimited')} class={inputClass} />
						</div>
					</div>

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

					<div>
						<span class="mb-1 block text-sm font-medium text-[var(--color-text)]">{t('services.pkg_items', 'Included items')}</span>
						<div class="space-y-2">
							{#each pkgItems as item, idx (idx)}
								<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
									<div class="flex items-start gap-2">
										<div class="flex-1 space-y-2">
											<input type="text" bind:value={item.title} placeholder={t('services.pkg_item_title_ph', 'Item title (e.g. Visa processing)')} class={inputClass} />
											<input type="text" bind:value={item.description} placeholder={t('services.pkg_item_desc_ph', 'Short description (optional)')} class={inputClass} />
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
										<button type="button" onclick={() => (pkgItems = pkgItems.filter((_, i) => i !== idx))} class="mt-1 text-[var(--color-muted)] hover:text-[var(--color-danger)]" aria-label={t('common.remove', 'Remove')}>
											<Trash2 size={16} />
										</button>
									</div>
								</div>
							{/each}
						</div>
						<button type="button" onclick={() => (pkgItems = [...pkgItems, { title: '', description: '', included: true, optional: false }])} class="mt-2 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
							<Plus size={14} /> {t('services.pkg_add_item', 'Add item')}
						</button>
					</div>

					<div>
						<span class="mb-1 block text-sm font-medium text-[var(--color-text)]">{t('services.pkg_departures', 'Scheduled departures')}</span>
						<p class="mb-2 text-xs text-[var(--color-muted)]">
							{t('services.pkg_departures_hint', 'Optional fixed dates (tours, cohort intakes). Leave empty for request-based packages.')}
						</p>
						<div class="space-y-2">
							{#each departures as dep, idx (dep.id ?? `new-${idx}`)}
								<div class="flex flex-wrap items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
									<input type="date" bind:value={dep.starts_on} class="{inputClass} sm:w-auto" aria-label={t('services.pkg_dep_start', 'Start date')} />
									<span class="text-xs text-[var(--color-muted)]">→</span>
									<input type="date" bind:value={dep.ends_on} class="{inputClass} sm:w-auto" aria-label={t('services.pkg_dep_end', 'End date (optional)')} />
									<input type="number" min={dep.booked_count > 0 ? dep.booked_count : 1} bind:value={dep.capacity} placeholder={t('services.pkg_dep_capacity_ph', 'Seats')} class="{inputClass} sm:w-24" aria-label={t('services.pkg_dep_capacity_ph', 'Seats')} />
									<div class="w-32">
										<Autocomplete
											placeholder={t('services.pkg_dep_status', 'Status')}
											ariaLabel={t('services.pkg_dep_status', 'Status')}
											emptyMessage={t('common.no_results', 'No results')}
											fetcher={depStatusFetcher}
											initialFetcher={depStatusFetcher}
											selectedId={dep.status}
											selectedLabel={depStatusLabel(dep.status)}
											onSelect={(item) => (dep.status = item.id)}
										/>
									</div>
									{#if dep.booked_count > 0}
										<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs text-[var(--color-muted)]">
											{t('services.pkg_dep_booked', '{count} booked').replace('{count}', String(dep.booked_count))}
										</span>
									{/if}
									{#if dep.booked_count === 0}
										<button type="button" onclick={() => (departures = departures.filter((_, i) => i !== idx))} class="ml-auto text-[var(--color-muted)] hover:text-[var(--color-danger)]" aria-label={t('common.remove', 'Remove')}>
											<Trash2 size={16} />
										</button>
									{:else}
										<span class="ml-auto" title={t('services.pkg_dep_booked_hint', 'Departures with bookings cannot be removed')}></span>
									{/if}
								</div>
							{/each}
						</div>
						<button type="button" onclick={() => (departures = [...departures, { id: '', starts_on: '', ends_on: '', capacity: '', status: 'open', booked_count: 0 }])} class="mt-2 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
							<Plus size={14} /> {t('services.pkg_add_departure', 'Add departure')}
						</button>
					</div>

					<!-- §114 pricing tiers — service_packages.tiers_enabled -->
					{#if tiersEnabled}
						<ServiceTierEditor bind:tiers {inputClass} currencies={currenciesList} />
					{/if}

					<!-- §114 coupon attach — service_coupons.enabled; seeds from
						the editor-scoped coupon list so suspended attachments
						stay attached. -->
					{#if couponsEnabled}
						<ServiceCouponSelector
							bind:selectedIds={couponIds}
							bind:ready={couponsReady}
							orgId={svc.org_id ?? ''}
							listingContentId={svc.id}
							listingKind="package"
						/>
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
						<Save size={16} />
					{/if}
					{submitting ? t('common.saving', 'Saving…') : t('common.save', 'Save')}
				</button>
			</div>
		</form>
	</div>
</div>
