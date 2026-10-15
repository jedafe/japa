<script lang="ts">
	// Ref: japaconnect-engineering/sections/98-service-directory.md
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { Briefcase, Save, Loader2, AlertCircle, Package } from 'lucide-svelte';
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

	// Mentorship shadow listings are managed on the mentor settings
	// card — this editor keeps them off the booking/pricing section
	// entirely. (The 'package' listing kind was removed from the
	// feature in 2026-10.)
	const isMentorship = $derived(svc.listing_kind === 'mentorship');
	let purchaseMode = $state<'inquiry' | 'external' | 'paid'>(
		untrack(() => (svc.purchase_mode === 'external' || svc.purchase_mode === 'paid' ? svc.purchase_mode : 'inquiry'))
	);
	let priceAmount = $state(untrack(() => (svc.price_amount != null ? String(svc.price_amount / 100) : '')));
	let priceCurrency = $state(untrack(() => svc.price_currency ?? ''));
	let pricePer = $state<'person' | 'package' | 'group'>(
		untrack(() => (svc.price_per === 'person' || svc.price_per === 'group' ? svc.price_per : 'person'))
	);

	// §114/000829 structured content — faqs[]/steps[]/selling_points[]/
	// facts[]/included[]/highlights[]/audience[] are replace-on-save like
	// items[]/departures[]: the edit payload always sends every array so
	// removed rows actually clear (absent preserves on PATCH). The client
	// caps live inside ServiceContentEditor; the backend re-validates
	// against the live system_settings.
	let faqs = $state<FaqEntry[]>(
		untrack(() => (svc.faqs ?? []).map((f) => ({ question: f.question, answer: f.answer })))
	);
	let steps = $state<StepEntry[]>(
		untrack(() =>
			(svc.steps ?? []).map((s) => ({
				icon: s.icon ?? '',
				title: s.title,
				description: s.description
			}))
		)
	);
	let sellingPoints = $state<IconEntry[]>(
		untrack(() =>
			(svc.selling_points ?? []).map((p) => ({
				icon: p.icon ?? '',
				title: p.title,
				description: p.description ?? ''
			}))
		)
	);
	let facts = $state<FactEntry[]>(
		untrack(() =>
			(svc.facts ?? []).map((f) => ({ icon: f.icon ?? '', label: f.label, value: f.value }))
		)
	);
	let included = $state<IconEntry[]>(
		untrack(() =>
			(svc.included ?? []).map((p) => ({
				icon: p.icon ?? '',
				title: p.title,
				description: p.description ?? ''
			}))
		)
	);
	let highlights = $state<string[]>(untrack(() => [...(svc.highlights ?? [])]));
	let audience = $state<string[]>(untrack(() => [...(svc.audience ?? [])]));
	// Open the optional section when the listing already carries content —
	// otherwise a seeded set would be invisible behind the collapsed details.
	let detailsOpen = $state(
		untrack(
			() =>
				(svc.faqs?.length ?? 0) > 0 ||
				(svc.steps?.length ?? 0) > 0 ||
				(svc.selling_points?.length ?? 0) > 0 ||
				(svc.facts?.length ?? 0) > 0 ||
				(svc.included?.length ?? 0) > 0 ||
				(svc.highlights?.length ?? 0) > 0 ||
				(svc.audience?.length ?? 0) > 0
		)
	);

	// §114 marketplace fields — featured image on all listing
	// kinds; tiers are flag-gated and apply to every kind. The
	// tier seed comes from the detail payload's ACTIVE set — no
	// editor-scoped tier list exists yet, so suspended tiers are
	// outside the editor's reach (a replace-save would drop
	// them; is_active is omitted on write so every submitted
	// tier stays active).
	const tiersEnabled = $derived(!!data.features?.['service_packages.tiers_enabled']);
	let featuredMediaId = $state(untrack(() => svc.featured_media_id ?? ''));
	let featuredPreview = $state(untrack(() => svc.featured_image_url ?? ''));
	let tiers = $state<EditableTier[]>(
		untrack(() =>
			(svc.tiers ?? []).map((tr) => ({
				name: tr.name,
				badge: tr.badge ?? '',
				description: tr.description ?? '',
				icon: tr.icon ?? '',
				priceAmount: tr.price_amount != null ? String(tr.price_amount / 100) : '',
				currency: tr.price_currency ?? '',
				featuresText: (tr.features ?? []).join('\n'),
				isFeatured: tr.is_featured
			}))
		)
	);
	const currenciesList = $derived((page.data.currencies ?? []) as CurrencyInfo[]);

	let submitting = $state(false);
	let errorMsg = $state('');

	// §114 2026-10-10 amendment: 'paid' mode was package-bound
	// (§111) and the package kind is gone — services offer
	// inquiry/external only. A legacy package row can still
	// carry 'paid' in the DB, so the state keeps the value to
	// round-trip it, but the picker no longer offers it.
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
		// Booking/pricing rules apply to every listing kind (§114
		// 2026-10-10 amendment) — services take inquiry/external bookings
		// and tiered pricing too. ('paid' survives only on legacy
		// package rows, whose price must stay set.)
		if (purchaseMode === 'paid' && (!String(priceAmount ?? '').trim() || !priceCurrency)) {
			errorMsg = t('services.errors.pkg_price_required', 'Paid packages require a price and currency');
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
			// §114/000829 — replace-on-save for all listing kinds: always
			// send every array; an empty array clears the stored set.
			faqs: faqs.map((f) => ({ question: f.question.trim(), answer: f.answer.trim() })),
			steps: steps.map((s) => ({
				icon: s.icon,
				title: s.title.trim(),
				description: s.description.trim()
			})),
			selling_points: sellingPoints.map((p) => ({
				icon: p.icon,
				title: p.title.trim(),
				description: p.description.trim()
			})),
			facts: facts.map((f) => ({ icon: f.icon, label: f.label.trim(), value: f.value.trim() })),
			included: included.map((p) => ({
				icon: p.icon,
				title: p.title.trim(),
				description: p.description.trim()
			})),
			highlights: highlights.map((h) => h.trim()).filter(Boolean),
			audience: audience.map((a) => a.trim()).filter(Boolean)
		};

		// §114 — featured_media_id semantics: absent preserves, '' clears,
		// a UUID replaces. Only send when the pick actually changed.
		if (featuredMediaId !== (svc.featured_media_id ?? '')) {
			payload.featured_media_id = featuredMediaId;
		}

		// §114 — purchase-mode/price/tiers apply to every listing
		// kind — the shared detail page renders the same pricing
		// surface for services.
		payload.purchase_mode = purchaseMode;
		payload.price_per = pricePer;
		{
			// bind:value on type="number" yields number|null — coerce
			// back to string before trimming.
			const amountStr = String(priceAmount ?? '').trim();
			payload.price_amount = amountStr ? Math.round(Number(amountStr) * 100) : 0;
			payload.price_currency = priceCurrency;
		}
		// §114 tiers are replace-on-save — always sent while the
		// flag is on so removals clear.
		if (tiersEnabled) {
			payload.tiers = tiersToPayload(tiers);
		}

		submitting = true;
		const res = await api.patch<ServiceListing>(`/services/${svc.id}`, payload);
		submitting = false;

		if (res.data) {
			// §111: commercial edits bounce the listing back to
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

			<!-- §114/000829 structured content — one collapsed section
				shared with the create form (ServiceContentEditor). -->
			<ServiceContentEditor
				{inputClass}
				bind:open={detailsOpen}
				bind:faqs
				bind:steps
				bind:sellingPoints
				bind:facts
				bind:included
				bind:highlights
				bind:audience
			/>

			<!-- §114 booking/pricing (all non-mentorship kinds). -->
			{#if !isMentorship}
				<div class="space-y-4 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)]/40 p-4">
					<h2 class="flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
						<Package size={16} />
						{t('services.booking_section', 'Booking & pricing')}
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
								{t('services.pkg_price', 'Price')}
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
						listing kinds render the tier cards (§114 amendment). -->
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
						<Save size={16} />
					{/if}
					{submitting ? t('common.saving', 'Saving…') : t('common.save', 'Save')}
				</button>
			</div>
		</form>
	</div>
</div>
