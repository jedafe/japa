<script lang="ts">
	// Ref: japaconnect-engineering/sections/98-service-directory.md
	// Ref: japaconnect-engineering/sections/114-services-marketplace.md
	// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§3.3)
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { api } from '$lib/api/client';
	import {
		Briefcase,
		MapPin,
		Globe,
		ExternalLink,
		BadgeCheck,
		Wifi,
		HandCoins,
		Loader2,
		CheckCircle2,
		AlertCircle,
		Star,
		CalendarDays,
		Users,
		ShieldAlert,
		ShieldCheck,
		Headset,
		Send,
		ChevronRight,
		Eye,
		ArrowRight
	} from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import IncludedGrid from '$lib/components/marketplace/IncludedGrid.svelte';
	import TierGrid from '$lib/components/marketplace/TierGrid.svelte';
	import CouponCard from '$lib/components/marketplace/CouponCard.svelte';
	import ServiceReviews from '$lib/components/marketplace/ServiceReviews.svelte';
	import ProviderCard from '$lib/components/marketplace/ProviderCard.svelte';
	import StickyBookBar from '$lib/components/marketplace/StickyBookBar.svelte';
	import { serviceCategoryIcon } from '$lib/components/marketplace/serviceCategoryIcon';
	import { accentClasses, priceLabel, serviceCategoryInfo } from '$lib/marketplace';
	import { serviceCategoryLabel } from '$lib/enumOptions';
	import { staticAutocompleteFetcher } from '$lib/autocomplete';
	import { formatMoney } from '$lib/utils/currency';
	import { excerpt } from '$lib/utils/seo-score';
	import { contentHref } from '$lib/utils/contentLink';
	import type {
		ServiceListing,
		ServiceCategory,
		ServiceReview,
		ServiceBookingResult,
		PackageTier
	} from '$lib/types';
	import type { PageData } from './$types';
	import { sanitizeContentHtml, safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	const service = $derived<ServiceListing | null>(data.service);
	// Edit link shows for the listing author, members of the owning org
	// (service.edit_own), and admins (service.edit_any) — mirrors the PATCH
	// /services/:id permission check.
	const canEdit = $derived(
		!!data.authed &&
			!!service &&
			((data.user?.permissions ?? []).includes('service.edit_any') ||
				((service.author_id === data.currentUserId ||
					(service.org_id != null &&
						(data.myOrgs ?? []).some((o) => o.id === service.org_id))) &&
					// §111: package edits additionally need service.package.create —
					// EditOwn rejects org members without the node (403).
					(service.listing_kind !== 'package' ||
						(data.user?.permissions ?? []).includes('service.package.create'))))
	);

	// Reviews + aggregate from the §29 sidecar (BUG-014). The listing row
	// itself also carries rating_avg/review_count joins (§114) — prefer
	// those and fall back to the sidecar so both shapes render.
	const reviews = $derived<ServiceReview[]>(data.reviews ?? []);
	const reviewAggregate = $derived(data.reviewAggregate ?? { count: 0, avg: 0 });
	const hasReviews = $derived(reviews.length > 0 || reviewAggregate.count > 0);
	const ratingAvg = $derived(service?.rating_avg ?? reviewAggregate.avg ?? 0);
	const ratingCount = $derived(service?.review_count ?? reviewAggregate.count ?? 0);

	function formatDate(iso: string): string {
		try {
			return new Date(iso).toLocaleDateString();
		} catch {
			return iso;
		}
	}

	// View tracking (Section 14 — POST /content/{id}/view). Fire-and-forget
	// on mount; the backend deduplicates per user/session per day.
	$effect(() => {
		if (!service?.id) return;
		api.post(`/content/${service.id}/view`, {});
	});

	const title = $derived(
		service
			? `${service.title} — ${t('services.heading', 'Service Directory')}`
			: t('services.heading', 'Service Directory')
	);
	const description = $derived(service?.body ?? service?.title ?? '');
	const heroExcerpt = $derived(excerpt(service?.body_html || service?.body || '', 180));
	const ogImage = $derived(
		service?.featured_image_url
			? service.featured_image_url.startsWith('http')
				? service.featured_image_url
				: `${page.url.origin}${service.featured_image_url}`
			: ''
	);

	// §114 category badge — the taxonomy row carries the admin-managed
	// label/icon/accent_color; the §98 enum label is the fallback when the
	// categories fetch failed or the listing predates the taxonomy.
	const categories = $derived(data.serviceCategories ?? []);
	const category = $derived(serviceCategoryInfo(categories, service?.service_category));
	const accent = $derived(accentClasses(category?.accent_color));
	const CategoryIcon = $derived(serviceCategoryIcon(category?.icon));
	const catLabel = $derived(
		category?.label ??
			(service?.service_category
				? serviceCategoryLabel(t, service.service_category as ServiceCategory)
				: '')
	);

	function claimStatusLabel(status: string): string {
		switch (status) {
			case 'pending':
				return t('services.claim_status_pending', 'Claim pending review');
			case 'approved':
				return t('services.claim_status_approved', 'Claimed');
			case 'rejected':
				return t('services.claim_status_rejected', 'Claim rejected');
			default:
				return status;
		}
	}

	// Claim flow state (POST /services/{id}/claim). The backend requires
	// { org_id } — the user must claim on behalf of one of their orgs
	// (BUG-004). The org picker is populated from data.myOrgs (fetched
	// server-side via GET /me/organizations).
	let claimOrgId = $state('');
	let claiming = $state(false);
	let claimResult = $state<{ ok?: boolean; error?: string } | null>(null);

	const canClaim = $derived(
		!!service &&
			service.claim_status === 'unclaimed' &&
			(data.myOrgs?.length ?? 0) > 0
	);

	// ------------------------------------------------------------------
	// §111 package detail + booking CTA
	// ------------------------------------------------------------------
	const currencies = $derived(
		(page.data.currencies ?? []) as { code: string; name: string; symbol: string }[]
	);
	const isPackage = $derived(service?.listing_kind === 'package');
	const packagesEnabled = $derived(!!page.data.features?.['service_packages.enabled']);
	// §114 flags — the API omits tiers/coupons entirely when the flags are
	// off, so the data-length guard is the authoritative check.
	const tiersVisible = $derived(
		!!page.data.features?.['service_packages.tiers_enabled'] &&
			(service?.tiers?.length ?? 0) > 0
	);
	const couponsVisible = $derived(
		!!page.data.features?.['service_coupons.enabled'] && (service?.coupons?.length ?? 0) > 0
	);
	const heroPrice = $derived(service ? priceLabel(service, currencies) : null);
	const pricePerLabel = $derived(
		service?.price_per === 'person'
			? t('services.pkg_per_person', 'per person')
			: service?.price_per === 'group'
				? t('services.pkg_per_group', 'per group')
				: t('services.pkg_per_package', 'per package')
	);
	const openDepartures = $derived(
		(service?.departures ?? []).filter((d) => d.status === 'open')
	);

	// Bookings: any published org-owned listing with purchase_mode inquiry
	// accepts inquiries; 'paid' opens the fee checkout; 'external' points at
	// booking_url. Owners/org members manage via the inbox instead.
	const canBook = $derived(
		packagesEnabled &&
			!!data.authed &&
			!!service &&
			!!service.org_id &&
			!canEdit &&
			// POST /services/:id/bookings is gated on service.booking.create —
			// hide the CTA rather than letting the submit 403.
			(data.user?.permissions ?? []).includes('service.booking.create')
	);
	const guestBooking = $derived(packagesEnabled && !data.authed && !!service && !!service.org_id);

	let bookingOpen = $state(false);
	let partySize = $state('1');
	let bookingDepartureId = $state('');
	// §114 tier preselection — set by TierGrid's "Choose X"; submitted as
	// tier_id on paid bookings and shown in the order summary. The backend
	// rejects tier_id on inquiry intent, so inquiry submissions carry the
	// tier name inside the note instead.
	let selectedTier = $state<PackageTier | null>(null);
	let prefStartOn = $state('');
	let prefEndOn = $state('');
	let prefFlexible = $state(false);
	let bookingNote = $state('');
	let bookingProvider = $state('');
	let bookingEmail = $state(untrack(() => data.user?.email ?? ''));
	let bookingBusy = $state(false);
	let bookingResult = $state<{ ok?: boolean; error?: string } | null>(null);
	let bookingProviders = $state<{ id: string; name: string; enabled: boolean }[]>([]);
	let providersLoaded = $state(false);
	// §111/Rule 9 — the backend requires the Idempotency-Key header on
	// every booking (intent='booking' → 400 idempotency_key_required),
	// so this form sends it on every submit — paid or inquiry. One key
	// per form-open so a retry replays the cached response instead of
	// double-booking; regenerated after success.
	let bookingIdemKey = $state('');

	const inputCls =
		'rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';

	const departureItems = $derived<AutocompleteItem[]>(
		openDepartures.map((d) => ({
			id: d.id,
			label: formatDate(d.starts_on) + (d.ends_on ? ` → ${formatDate(d.ends_on)}` : ''),
			sublabel:
				d.capacity != null
					? t('services.pkg_seats_left', '{n} seats left', {
							n: Math.max(0, d.capacity - d.booked_count)
						})
					: t('services.pkg_unlimited', 'Unlimited seats')
		}))
	);
	const departureFetcher = $derived(staticAutocompleteFetcher(departureItems));
	const selectedDepartureLabel = $derived(
		bookingDepartureId
			? (departureItems.find((o) => o.id === bookingDepartureId)?.label ?? null)
			: null
	);

	const providerItems = $derived<AutocompleteItem[]>(
		bookingProviders.filter((p) => p.enabled).map((p) => ({ id: p.id, label: p.name }))
	);
	const providerFetcher = $derived(staticAutocompleteFetcher(providerItems));

	async function loadProviders() {
		if (providersLoaded) return;
		const res = await api.get<{ id: string; name: string; enabled: boolean }[]>('/payment-providers');
		if (res.error) return; // leave providersLoaded false so reopening retries
		providersLoaded = true;
		bookingProviders = res.data ?? [];
	}

	function openBookingForm() {
		bookingOpen = true;
		bookingIdemKey = crypto.randomUUID();
		if (service?.purchase_mode === 'paid') loadProviders();
	}

	function scrollToBook() {
		document.getElementById('book')?.scrollIntoView({ behavior: 'smooth', block: 'start' });
	}

	// TierGrid "Choose X" — preselect the tier, open the form, scroll to it.
	function selectTier(tier: PackageTier) {
		selectedTier = tier;
		if (canBook) openBookingForm();
		scrollToBook();
	}

	async function submitBooking() {
		if (!service) return;
		bookingResult = null;
		const size = Number(partySize);
		const isPaid = service.purchase_mode === 'paid';

		if (!Number.isInteger(size) || size < 1) {
			bookingResult = { error: t('services.book_party_size_invalid', 'Enter a valid party size') };
			return;
		}
		if (isPaid && (!bookingProvider || !bookingEmail.trim())) {
			bookingResult = { error: t('services.book_checkout_fields', 'Payment provider and email are required') };
			return;
		}

		let note = bookingNote.trim();
		// Inquiry intent rejects tier_id (ErrInvalidCoupon) — carry the
		// selection in the note so the provider still sees it.
		if (!isPaid && selectedTier) {
			const tierLine = t('services.book_tier_note', 'Interested in the {name} tier.', {
				name: selectedTier.name
			});
			note = note ? `${note}\n${tierLine}` : tierLine;
		}

		const body: Record<string, unknown> = {
			intent: isPaid ? 'booking' : 'inquiry',
			party_size: size,
			note
		};
		if (isPaid && selectedTier) body.tier_id = selectedTier.id;
		if (bookingDepartureId) body.departure_id = bookingDepartureId;
		if (!bookingDepartureId && (prefStartOn || prefEndOn)) {
			body.preferred_dates = { start_on: prefStartOn, end_on: prefEndOn || prefStartOn, flexible: prefFlexible };
		}
		if (isPaid) {
			body.provider = bookingProvider;
			body.email = bookingEmail.trim();
			body.return_url = page.url.href;
		}

		bookingBusy = true;
		const res = await api.post<ServiceBookingResult>(`/services/${service.id}/bookings`, body, {
			'Idempotency-Key': bookingIdemKey || crypto.randomUUID()
		});
		bookingBusy = false;

		if (res.data) {
			// Paid mode opens the §17 fee checkout — send the booker off-site.
			const checkout = res.data.payment?.checkout_url;
			if (isPaid && checkout) {
				window.location.href = checkout;
				return;
			}
			bookingResult = { ok: true };
			bookingOpen = false;
			bookingIdemKey = '';
		} else {
			bookingResult = { error: res.error?.message ?? t('services.book_failed', 'Could not submit') };
		}
	}

	// Whether the #book section has anything to render — paid/inquiry modes
	// plus either an authed booker (canBook), a guest seeing the login
	// prompt (guestBooking), or a success notice. Owners and viewers without
	// service.booking.create get no booking surface, so the CTA must not
	// point at an empty anchor.
	const bookSurface = $derived(
		!!service &&
			service.purchase_mode !== 'external' &&
			service.purchase_mode !== '' &&
			(canBook || guestBooking || !!bookingResult?.ok)
	);

	// Hero CTAs — mode-aware. Primary: paid/inquiry scroll to #book,
	// external links out to booking_url, plain services fall back to the
	// provider's website (or the provider card). Secondary "Talk to an
	// expert" reuses the inquiry path (#book) or the org profile.
	const primaryCta = $derived.by(():
		| { label: string; href: string; external: boolean }
		| null => {
		if (!service) return null;
		if (service.purchase_mode === 'external' && service.booking_url) {
			return {
				label: isPackage
					? t('services.book_partner', 'Book on partner site')
					: t('services.book_now', 'Book now'),
				href: service.booking_url,
				external: true
			};
		}
		if (bookSurface && service.purchase_mode === 'paid') {
			return { label: t('services.book_now', 'Book now'), href: '#book', external: false };
		}
		if (bookSurface && service.purchase_mode === 'inquiry') {
			return { label: t('services.book_inquiry', 'Send inquiry'), href: '#book', external: false };
		}
		if (service.website_url) {
			return {
				label: t('services.visit_website', 'Visit website'),
				href: service.website_url,
				external: true
			};
		}
		if (service.org_slug) {
			return { label: t('services.provider_view', 'View provider'), href: '#provider', external: false };
		}
		return null;
	});

	const secondaryCta = $derived.by((): { label: string; href: string } | null => {
		if (!service) return null;
		if (bookSurface && service.purchase_mode === 'inquiry') {
			return { label: t('services.talk_to_expert', 'Talk to an expert'), href: '#book' };
		}
		if (service.org_slug) {
			return {
				label: t('services.talk_to_expert', 'Talk to an expert'),
				href: `/org/${service.org_slug}`
			};
		}
		return null;
	});

	// The mobile sticky bar only makes sense where a booking surface exists —
	// owners manage the listing instead of booking it.
	const stickyCta = $derived.by(() => {
		if (!service || canEdit) return null;
		if (service.purchase_mode === 'external' && service.booking_url) {
			return {
				label: isPackage
					? t('services.book_partner', 'Book on partner site')
					: t('services.book_now', 'Book now'),
				href: service.booking_url,
				external: true
			};
		}
		if (
			(service.purchase_mode === 'paid' || service.purchase_mode === 'inquiry') &&
			(canBook || guestBooking)
		) {
			return {
				label:
					service.purchase_mode === 'paid'
						? t('services.book_now', 'Book now')
						: t('services.book_inquiry', 'Send inquiry'),
				href: '#book',
				external: false
			};
		}
		return null;
	});

	const orgItems = $derived<AutocompleteItem[]>(
		(data.myOrgs ?? []).map((o) => ({ id: o.id, label: o.name }))
	);
	const orgFetcher = $derived(staticAutocompleteFetcher(orgItems));
	const selectedOrgLabel = $derived(
		claimOrgId ? (orgItems.find((o) => o.id === claimOrgId)?.label ?? null) : null
	);

	async function claimListing() {
		if (!service) return;
		if (!claimOrgId) {
			claimResult = { error: t('services.claim_select_org', 'Select an organization to claim on behalf of.') };
			return;
		}
		claiming = true;
		claimResult = null;
		const res = await api.post<{ claim_status: string }>(`/services/${service.id}/claim`, {
			org_id: claimOrgId
		});
		claiming = false;
		if (res.data) {
			claimResult = { ok: true };
		} else {
			claimResult = { error: res.error?.message ?? t('services.claim_failed', 'Claim failed') };
		}
	}

	// JSON-LD — schema.org Service + provider/offer/aggregateRating so
	// marketplace listings qualify for rich results (§3.3 SEO).
	const jsonLd = $derived(
		service
			? JSON.stringify({
					'@context': 'https://schema.org',
					'@type': 'Service',
					name: service.title,
					description: service.body ?? '',
					url: page.url.href,
					...(ogImage ? { image: ogImage } : {}),
					...(service.website_url ? { sameAs: service.website_url } : {}),
					...(service.org_name
						? {
								provider: {
									'@type': 'Organization',
									name: service.org_name,
									...(service.org_slug
										? { url: `${page.url.origin}/org/${service.org_slug}` }
										: {})
								}
							}
						: {}),
					...(service.destination_space_name
						? { areaServed: service.destination_space_name }
						: {}),
					...(service.price_range ? { priceRange: service.price_range } : {}),
					...((service.min_price ?? service.price_amount) != null && service.price_currency
						? {
								offers: {
									'@type': 'Offer',
									price: ((service.min_price ?? service.price_amount) as number) / 100,
									priceCurrency: service.price_currency
								}
							}
						: {}),
					...(ratingCount > 0
						? {
								aggregateRating: {
									'@type': 'AggregateRating',
									ratingValue: ratingAvg,
									reviewCount: ratingCount
								}
							}
						: {})
				})
			: ''
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={service?.title ?? title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="article" />
	<meta property="og:url" content={page.url.href} />
	{#if ogImage}
		<meta property="og:image" content={ogImage} />
	{/if}
	{#if jsonLd}
		{@html safeJsonLdScript(jsonLd)}
	{/if}
</svelte:head>

<div class="mx-auto w-full max-w-[1200px] px-4 pt-4 {stickyCta ? 'pb-24' : 'pb-10'} lg:pb-10">
	{#if data.loadError || !service}
		<EmptyState
			icon={Briefcase}
			title={t('services.load_error', 'Could not load service')}
			body={t('services.load_error_body', 'Please try again later.')}
			action={{ label: t('services.back_to_dir', 'Back to services'), href: '/services' }}
		/>
	{:else}
		<!-- Breadcrumb: Home / Services / Category / Title -->
		<nav
			class="mb-5 flex flex-wrap items-center gap-1 text-xs text-[var(--color-muted)]"
			aria-label={t('a11y.breadcrumb', 'Breadcrumb')}
		>
			<a href="/" class="transition-colors hover:text-[var(--color-text)]"
				>{t('nav.home', 'Home')}</a
			>
			<ChevronRight size={12} aria-hidden="true" />
			<a href="/services" class="transition-colors hover:text-[var(--color-text)]"
				>{t('marketplace.nav_services', 'Services')}</a
			>
			{#if catLabel}
				<ChevronRight size={12} aria-hidden="true" />
				<a
					href="/services?category={service.service_category}"
					class="transition-colors hover:text-[var(--color-text)]">{catLabel}</a
				>
			{/if}
			<ChevronRight size={12} aria-hidden="true" />
			<span class="truncate font-medium text-[var(--color-text)]" aria-current="page"
				>{service.title}</span
			>
		</nav>

		<!-- Hero: content left, image + facts right -->
		<section class="grid gap-8 lg:grid-cols-2">
			<div>
				<div class="flex flex-wrap items-center gap-2">
					{#if catLabel}
						<span
							class="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-medium {accent.chip}"
						>
							<CategoryIcon size={13} aria-hidden="true" />
							{catLabel}
						</span>
					{/if}
					{#if service.is_verified && service.verification_type}
						<span class="flex items-center gap-1 text-xs text-[var(--color-primary)]">
							<BadgeCheck size={13} aria-hidden="true" />
							{t('services.verified', 'Verified')} · {service.verification_type}
						</span>
					{/if}
					{#if service.claim_status && service.claim_status !== 'unclaimed'}
						<span
							class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs text-[var(--color-text-subtle)]"
						>
							{claimStatusLabel(service.claim_status)}
						</span>
					{/if}
				</div>

				<h1 class="mt-3 flex items-start gap-2 text-2xl font-bold text-[var(--color-text)] sm:text-3xl">
					{service.title}
					{#if service.is_verified}
						<BadgeCheck size={24} class="mt-1 shrink-0 text-[var(--color-primary)]" />
					{/if}
				</h1>

				{#if heroExcerpt}
					<p class="mt-3 text-sm leading-relaxed text-[var(--color-text-subtle)]">
						{heroExcerpt}
					</p>
				{/if}

				<!-- Rating + social proof -->
				<div class="mt-4 flex flex-wrap items-center gap-x-4 gap-y-2 text-sm">
					{#if ratingCount > 0}
						<a href="#reviews" class="flex items-center gap-1.5">
							<span
								class="flex items-center gap-0.5"
								aria-label={t('services.rating_aria', '{avg} out of 5 stars', {
									avg: ratingAvg.toFixed(1)
								})}
							>
								{#each Array(5) as _, i (i)}
									<Star
										size={14}
										class={i < Math.round(ratingAvg)
											? 'fill-[var(--color-warning)] text-[var(--color-warning)]'
											: 'text-[var(--color-muted)]'}
										aria-hidden="true"
									/>
								{/each}
							</span>
							<span class="font-medium text-[var(--color-text)]">{ratingAvg.toFixed(1)}</span>
							<span class="text-[var(--color-muted)]"
								>({t('services.reviews_summary', '{count} reviews', { count: ratingCount })})</span
							>
						</a>
					{/if}
					{#if service.view_count > 0}
						<span class="flex items-center gap-1 text-[var(--color-muted)]">
							<Eye size={14} aria-hidden="true" />
							{t('services.supported_stat', '{n} supported', { n: service.view_count })}
						</span>
					{/if}
				</div>

				<!-- Trust chips -->
				<div class="mt-4 flex flex-wrap gap-2 text-xs">
					{#if service.is_verified}
						<span
							class="inline-flex items-center gap-1.5 rounded-full bg-[var(--color-surface-subtle)] px-3 py-1.5 text-[var(--color-text-subtle)]"
						>
							<BadgeCheck size={13} class="text-[var(--color-primary)]" aria-hidden="true" />
							{t('services.verified_provider', 'Verified provider')}
						</span>
					{/if}
					<span
						class="inline-flex items-center gap-1.5 rounded-full bg-[var(--color-surface-subtle)] px-3 py-1.5 text-[var(--color-text-subtle)]"
					>
						<ShieldCheck size={13} class="text-[var(--color-primary)]" aria-hidden="true" />
						{t('services.trust_secure_title', 'Secure booking')}
					</span>
					<span
						class="inline-flex items-center gap-1.5 rounded-full bg-[var(--color-surface-subtle)] px-3 py-1.5 text-[var(--color-text-subtle)]"
					>
						<Headset size={13} class="text-[var(--color-primary)]" aria-hidden="true" />
						{t('services.trust_support_title', 'Expert support')}
					</span>
				</div>

				<!-- Origin communities -->
				{#if service.origin_communities && (service.origin_communities as string[]).length > 0}
					<div class="mt-3 flex flex-wrap items-center gap-1.5 text-xs text-[var(--color-muted)]">
						<span>{t('services.serves', 'Serves')}:</span>
						{#each (service.origin_communities ?? []) as community (community)}
							<span
								class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-[var(--color-text-subtle)]"
								>{community}</span
							>
						{/each}
					</div>
				{/if}

				<!-- CTA pair -->
				<div class="mt-6 flex flex-wrap gap-3">
					{#if primaryCta}
						<a
							href={primaryCta.href}
							target={primaryCta.external ? '_blank' : undefined}
							rel={primaryCta.external ? 'noopener noreferrer' : undefined}
							class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-5 py-2.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
						>
							{primaryCta.label}
							{#if primaryCta.external}
								<ExternalLink size={15} aria-hidden="true" />
							{/if}
						</a>
					{/if}
					{#if secondaryCta}
						<a
							href={secondaryCta.href}
							class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-5 py-2.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							{secondaryCta.label}
						</a>
					{/if}
				</div>
			</div>

			<!-- Right: featured image + fact grid -->
			<div>
				<div
					class="relative aspect-[16/10] w-full overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)]"
				>
					{#if service.featured_image_url}
						<img
							src={service.featured_image_url}
							alt={service.title}
							class="h-full w-full object-cover"
						/>
					{:else}
						<div class="grid h-full w-full place-items-center {accent.chip} border-0">
							<CategoryIcon size={72} strokeWidth={1.25} class={accent.icon} />
						</div>
					{/if}
					{#if service.has_coupon}
						<span
							class="absolute left-3 top-3 inline-flex items-center gap-1 rounded-full bg-[var(--color-surface)]/95 px-2.5 py-1 text-xs font-semibold shadow-sm {accent.icon}"
						>
							{t('services.coupon_badge', 'Deal')}
						</span>
					{/if}
				</div>

				<!-- Fact grid — only populated fields render. -->
				<dl class="mt-4 grid grid-cols-2 gap-3">
					{#if service.destination_space_name}
						<div
							class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
						>
							<dt class="flex items-center gap-1 text-[11px] uppercase tracking-wide text-[var(--color-muted)]">
								<MapPin size={11} aria-hidden="true" />
								{t('services.fact_destination', 'Destination')}
							</dt>
							<dd class="mt-1 text-sm font-medium text-[var(--color-text)]">
								{service.destination_space_name}
							</dd>
						</div>
					{/if}
					{#if service.city_space_name}
						<div
							class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
						>
							<dt class="flex items-center gap-1 text-[11px] uppercase tracking-wide text-[var(--color-muted)]">
								<MapPin size={11} aria-hidden="true" />
								{t('services.fact_city', 'City')}
							</dt>
							<dd class="mt-1 text-sm font-medium text-[var(--color-text)]">
								{service.city_space_name}
							</dd>
						</div>
					{/if}
					{#if service.available_from || service.available_until}
						<div
							class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
						>
							<dt class="flex items-center gap-1 text-[11px] uppercase tracking-wide text-[var(--color-muted)]">
								<CalendarDays size={11} aria-hidden="true" />
								{t('services.fact_availability', 'Availability')}
							</dt>
							<dd class="mt-1 text-sm font-medium text-[var(--color-text)]">
								{service.available_from ? formatDate(service.available_from) : '…'} — {service.available_until
									? formatDate(service.available_until)
									: t('services.pkg_ongoing', 'ongoing')}
							</dd>
						</div>
					{/if}
					{#if service.capacity != null}
						<div
							class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
						>
							<dt class="flex items-center gap-1 text-[11px] uppercase tracking-wide text-[var(--color-muted)]">
								<Users size={11} aria-hidden="true" />
								{t('services.fact_capacity', 'Capacity')}
							</dt>
							<dd class="mt-1 text-sm font-medium text-[var(--color-text)]">
								{t('services.pkg_capacity_label', 'Up to {n} people', { n: service.capacity })}
							</dd>
						</div>
					{/if}
					{#if isPackage && service.price_per}
						<div
							class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
						>
							<dt class="flex items-center gap-1 text-[11px] uppercase tracking-wide text-[var(--color-muted)]">
								<HandCoins size={11} aria-hidden="true" />
								{t('services.fact_pricing', 'Pricing')}
							</dt>
							<dd class="mt-1 text-sm font-medium text-[var(--color-text)]">{pricePerLabel}</dd>
						</div>
					{/if}
					{#if service.is_remote}
						<div
							class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
						>
							<dt class="flex items-center gap-1 text-[11px] uppercase tracking-wide text-[var(--color-muted)]">
								<Wifi size={11} aria-hidden="true" />
								{t('services.fact_delivery', 'Delivery')}
							</dt>
							<dd class="mt-1 text-sm font-medium text-[var(--color-text)]">
								{t('services.remote', 'Remote')}
							</dd>
						</div>
					{/if}
				</dl>
			</div>
		</section>

		<div class="mt-10 space-y-10">
			<!-- What's included -->
			{#if service.items && service.items.length > 0}
				<IncludedGrid items={service.items} />
			{/if}

			<!-- Body -->
			{#if service.body}
				<div class="prose prose-sm max-w-none text-[var(--color-text)]">
					{@html sanitizeContentHtml(service.body_html || service.body)}
				</div>
			{/if}

			<!-- Open departures -->
			{#if openDepartures.length > 0}
				<section aria-labelledby="departures-heading">
					<h2
						id="departures-heading"
						class="text-sm font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]"
					>
						{t('services.pkg_departures_heading', 'Upcoming departures')}
					</h2>
					<ul class="mt-3 grid gap-2 sm:grid-cols-2">
						{#each openDepartures as dep (dep.id)}
							<li
								class="flex items-center justify-between gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm"
							>
								<span class="flex items-center gap-1.5 text-[var(--color-text)]">
									<CalendarDays size={14} class="text-[var(--color-muted)]" aria-hidden="true" />
									{formatDate(dep.starts_on)}{#if dep.ends_on}
										→ {formatDate(dep.ends_on)}{/if}
								</span>
								<span class="text-xs text-[var(--color-muted)]">
									{#if dep.capacity != null}
										{t('services.pkg_seats_left', '{n} seats left', {
											n: Math.max(0, dep.capacity - dep.booked_count)
										})}
									{:else}
										{t('services.pkg_unlimited', 'Unlimited seats')}
									{/if}
								</span>
							</li>
						{/each}
					</ul>
				</section>
			{/if}

			<!-- Pricing: tier cards (§114) or the single price card. -->
			{#if tiersVisible}
				<TierGrid
					tiers={service.tiers ?? []}
					{currencies}
					perLabel={pricePerLabel}
					onSelect={selectTier}
				/>
			{:else if isPackage || heroPrice || service.price_range}
				<section
					aria-labelledby="pricing-heading"
					class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5"
				>
					<h2
						id="pricing-heading"
						class="text-sm font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]"
					>
						{t('services.pricing_heading', 'Pricing')}
					</h2>
					<div class="mt-3 flex flex-wrap items-baseline gap-2">
						{#if heroPrice}
							<span class="text-sm text-[var(--color-muted)]"
								>{t('services.price_from', 'From')}</span
							>
							<span class="text-2xl font-bold text-[var(--color-text)]">{heroPrice}</span>
						{:else if service.price_range}
							<span class="flex items-center gap-1.5 text-xl font-semibold text-[var(--color-text)]">
								<HandCoins size={20} aria-hidden="true" /> {service.price_range}
							</span>
						{:else}
							<span class="text-sm text-[var(--color-muted)]"
								>{t('services.pkg_price_on_request', 'Price on request')}</span
							>
						{/if}
					</div>
					<!-- Action links -->
					<div class="mt-4 flex flex-wrap gap-3">
						{#if service.website_url}
							<a
								href={service.website_url}
								target="_blank"
								rel="noopener noreferrer"
								class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
							>
								<Globe size={16} aria-hidden="true" /> {t('services.visit_website', 'Visit website')}
							</a>
						{/if}
						{#if service.purchase_mode === 'external' && service.booking_url}
							<a
								href={service.booking_url}
								target="_blank"
								rel="noopener noreferrer"
								class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
							>
								<ExternalLink size={16} aria-hidden="true" />
								{isPackage
									? t('services.book_partner', 'Book on partner site')
									: t('services.book_now', 'Book now')}
							</a>
						{/if}
					</div>
				</section>
			{/if}

			<!-- Coupons (§114) -->
			{#if couponsVisible}
				<section aria-labelledby="coupons-heading">
					<h2
						id="coupons-heading"
						class="text-sm font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]"
					>
						{t('services.coupons_heading', 'Offers & coupons')}
					</h2>
					<div class="mt-4 grid gap-3 sm:grid-cols-2">
						{#each (service.coupons ?? []) as coupon (coupon.id)}
							<CouponCard {coupon} {currencies} />
						{/each}
					</div>
				</section>
			{/if}

			<!-- Reviews (§29 sidecar — BUG-014) -->
			{#if hasReviews}
				<div id="reviews" class="scroll-mt-20">
					<ServiceReviews {reviews} aggregate={reviewAggregate} />
				</div>
			{/if}

			<!-- Provider -->
			<div id="provider" class="scroll-mt-20">
				<ProviderCard {service} />
			</div>

			<!-- Booking / inquiry — id anchor for hero + tier CTAs. Only
				rendered when a booking surface exists (bookSurface). -->
			{#if bookSurface}
				<section id="book" aria-labelledby="book-heading" class="scroll-mt-20">
					<h2
						id="book-heading"
						class="text-sm font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]"
					>
						{service.purchase_mode === 'paid'
							? t('services.book_heading_paid', 'Book this package')
							: t('services.book_heading_inquiry', 'Booking & inquiries')}
					</h2>
					{#if bookingResult?.ok}
						<div
							class="mt-4 flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-success)]/30 bg-[var(--color-success)]/10 px-4 py-3 text-sm text-[var(--color-success)]"
							role="status"
						>
							<CheckCircle2 size={18} aria-hidden="true" />
							{service.purchase_mode === 'paid'
								? t('services.book_reserved', 'Booking created — complete payment to confirm.')
								: t('services.book_inquiry_sent', 'Inquiry sent — the provider will respond through your bookings inbox.')}
						</div>
					{:else if canBook}
						<div
							class="mt-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5"
						>
							{#if !bookingOpen}
								<button
									type="button"
									onclick={openBookingForm}
									class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
								>
									<Send size={16} aria-hidden="true" />
									{#if service.purchase_mode === 'paid'}
										{service.platform_fee != null
											? t('services.book_reserve_fee', 'Reserve — {fee} platform fee', {
													fee: formatMoney(service.platform_fee, service.price_currency, currencies, {
														minor: true
													})
												})
											: t('services.book_reserve', 'Reserve')}
									{:else}
										{t('services.book_inquiry', 'Send inquiry')}
									{/if}
								</button>
								{#if service.purchase_mode === 'paid' && service.agent_balance_due != null}
									<p class="mt-2 text-xs text-[var(--color-muted)]">
										{t(
											'services.book_balance_note',
											'This fee secures your booking. Remaining {balance} is payable directly to {org}.',
											{
												balance: formatMoney(
													service.agent_balance_due,
													service.price_currency,
													currencies,
													{ minor: true }
												),
												org: service.org_name ?? ''
											}
										)}
									</p>
								{/if}
							{:else}
								<h3 class="text-sm font-semibold text-[var(--color-text)]">
									{service.purchase_mode === 'paid'
										? t('services.book_reserve', 'Reserve')
										: t('services.book_inquiry', 'Send inquiry')}
								</h3>
								<div class="mt-3 space-y-3">
									<!-- Selected tier summary (§114) — cleared via Remove. -->
									{#if selectedTier}
										<div
											class="flex items-center justify-between gap-2 rounded-[var(--radius)] border border-[var(--color-primary)]/30 bg-[var(--color-primary)]/5 px-3 py-2 text-sm"
										>
											<span class="text-[var(--color-text)]">
												<span class="text-xs text-[var(--color-muted)]"
													>{t('services.book_tier_label', 'Package')}:</span
												>
												<span class="font-medium">{selectedTier.name}</span>
												<span class="text-[var(--color-muted)]">
													· {formatMoney(
														selectedTier.price_amount,
														selectedTier.price_currency,
														currencies,
														{ minor: true }
													)}
												</span>
											</span>
											<button
												type="button"
												onclick={() => (selectedTier = null)}
												class="text-xs text-[var(--color-muted)] underline hover:text-[var(--color-text)]"
											>
												{t('common.remove', 'Remove')}
											</button>
										</div>
									{/if}
									{#if openDepartures.length > 0}
										<div>
											<label
												for="bk-dep"
												class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]"
											>
												{t('services.book_departure', 'Departure')}
											</label>
											<Autocomplete
												id="bk-dep"
												placeholder={t('services.book_departure_ph', 'Choose a date')}
												emptyMessage={t('common.no_results', 'No results found')}
												fetcher={departureFetcher}
												initialFetcher={departureFetcher}
												selectedId={bookingDepartureId || null}
												selectedLabel={selectedDepartureLabel}
												onSelect={(item) => (bookingDepartureId = item.id)}
												onClear={() => (bookingDepartureId = '')}
											/>
										</div>
									{/if}
									{#if openDepartures.length === 0 || !bookingDepartureId}
										<div class="grid gap-3 sm:grid-cols-2">
											<div>
												<label
													for="bk-from"
													class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]"
													>{t('services.book_pref_start', 'Preferred start')}</label
												>
												<input
													id="bk-from"
													type="date"
													bind:value={prefStartOn}
													class="{inputCls} w-full"
												/>
											</div>
											<div>
												<label
													for="bk-to"
													class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]"
													>{t('services.book_pref_end', 'Preferred end')}</label
												>
												<input
													id="bk-to"
													type="date"
													bind:value={prefEndOn}
													class="{inputCls} w-full"
												/>
											</div>
										</div>
										<label class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
											<input
												type="checkbox"
												checked={prefFlexible}
												onchange={(e) => (prefFlexible = e.currentTarget.checked)}
												class="h-3.5 w-3.5 rounded border-[var(--color-border)]"
											/>
											{t('services.book_flexible', 'My dates are flexible')}
										</label>
									{/if}
									<div>
										<label
											for="bk-party"
											class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]"
											>{t('services.book_party_size', 'Party size')}</label
										>
										<input
											id="bk-party"
											type="number"
											min="1"
											bind:value={partySize}
											class="{inputCls} w-24"
										/>
									</div>
									<div>
										<label
											for="bk-note"
											class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]"
											>{t('services.book_note', 'Message to the provider')}</label
										>
										<textarea
											id="bk-note"
											bind:value={bookingNote}
											rows={3}
											placeholder={t('services.book_note_ph', 'Questions, requirements, context…')}
											class="{inputCls} w-full"
										></textarea>
									</div>
									{#if service.purchase_mode === 'paid'}
										<div class="grid gap-3 sm:grid-cols-2">
											<div>
												<label
													for="bk-prov"
													class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]"
													>{t('services.book_provider', 'Payment method')}</label
												>
												<Autocomplete
													id="bk-prov"
													placeholder={t('services.book_provider_ph', 'Choose a payment method')}
													emptyMessage={t('common.no_results', 'No results found')}
													fetcher={providerFetcher}
													initialFetcher={providerFetcher}
													selectedId={bookingProvider || null}
													selectedLabel={providerItems.find((o) => o.id === bookingProvider)
														?.label ?? null}
													onSelect={(item) => (bookingProvider = item.id)}
													onClear={() => (bookingProvider = '')}
												/>
											</div>
											<div>
												<label
													for="bk-email"
													class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]"
													>{t('services.book_email', 'Email for receipt')}</label
												>
												<input
													id="bk-email"
													type="email"
													bind:value={bookingEmail}
													placeholder="you@example.com"
													class="{inputCls} w-full"
												/>
											</div>
										</div>
									{/if}
									{#if bookingResult?.error}
										<div
											class="flex items-center gap-2 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500"
											role="alert"
										>
											<AlertCircle size={14} aria-hidden="true" /> {bookingResult.error}
										</div>
									{/if}
									<div class="flex gap-2">
										<button
											type="button"
											onclick={submitBooking}
											disabled={bookingBusy}
											class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
										>
											{#if bookingBusy}<Loader2
													size={16}
													class="animate-spin"
													aria-hidden="true"
												/>{:else}<Send size={16} aria-hidden="true" />{/if}
											{service.purchase_mode === 'paid'
												? t('services.book_pay_reserve', 'Pay & reserve')
												: t('services.book_send', 'Send inquiry')}
										</button>
										<button
											type="button"
											onclick={() => (bookingOpen = false)}
											class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
										>
											{t('common.cancel', 'Cancel')}
										</button>
									</div>
								</div>
							{/if}
						</div>
					{:else if guestBooking}
						<p class="mt-4 text-sm text-[var(--color-muted)]">
							<a
								href="/login?redirect={encodeURIComponent(page.url.pathname)}"
								class="text-[var(--color-primary)] hover:underline"
							>
								{service.purchase_mode === 'paid'
									? t('services.book_login_reserve', 'Log in to reserve')
									: t('services.book_login_inquiry', 'Log in to send an inquiry')}
							</a>
						</p>
					{/if}
				</section>
			{/if}

			<!-- §111 scam disclaimer — packages render a standing notice +
				a link to the §81 board filtered to this destination. -->
			{#if isPackage}
				<div
					class="flex items-start gap-2 rounded-[var(--radius)] border border-[var(--color-warning)]/30 bg-[var(--color-warning)]/10 px-4 py-3 text-xs text-[var(--color-text-subtle)]"
				>
					<ShieldAlert size={16} class="mt-0.5 shrink-0 text-[var(--color-warning)]" aria-hidden="true" />
					<span>
						{t(
							'services.pkg_scam_disclaimer',
							'Always verify providers before paying. JapaConnect never guarantees outcomes — report suspicious packages.'
						)}
						<a
							href="/scam-alerts?dest={service.destination_space_id}"
							class="ml-1 text-[var(--color-primary)] hover:underline"
						>
							{t('services.pkg_scam_link', 'See scam alerts for {dest}', {
								dest: service.destination_space_name ?? ''
							})}
						</a>
					</span>
				</div>
			{/if}

			<!-- Owner actions: edit + claim -->
			{#if data.authed}
				<div class="flex flex-wrap gap-3 border-t border-[var(--color-border)] pt-6">
					{#if canEdit}
						<a
							href="/services/{service.slug}/edit"
							class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							{t('services.edit', 'Edit listing')}
						</a>
						{#if isPackage && service.org_slug}
							<a
								href="/org/{service.org_slug}/service-bookings"
								class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
							>
								{t('bookings.org_inbox_link', 'Bookings inbox')}
							</a>
						{/if}
					{/if}

					{#if !canEdit && service.claim_status === 'unclaimed'}
						{#if claimResult?.ok}
							<div class="flex items-center gap-2 text-sm text-[var(--color-success)]">
								<CheckCircle2 size={16} aria-hidden="true" />
								{t('services.claim_submitted', 'Claim submitted for review.')}
							</div>
						{:else if canClaim}
							<div class="flex flex-wrap items-center gap-2">
								<Autocomplete
									placeholder={t('services.claim_select_org', 'Select an organization')}
									ariaLabel={t('services.claim_select_org', 'Select organization')}
									emptyMessage={t('common.no_results', 'No results found')}
									fetcher={orgFetcher}
									initialFetcher={orgFetcher}
									selectedId={claimOrgId || null}
									selectedLabel={selectedOrgLabel}
									onSelect={(item) => (claimOrgId = item.id)}
									onClear={() => (claimOrgId = '')}
								/>
								<button
									type="button"
									onclick={claimListing}
									disabled={claiming}
									class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
								>
									{#if claiming}
										<Loader2 size={16} class="animate-spin" aria-hidden="true" />
									{:else}
										<BadgeCheck size={16} aria-hidden="true" />
									{/if}
									{t('services.claim_listing', 'Claim this listing')}
								</button>
								{#if claimResult?.error}
									<span class="flex items-center gap-1 text-sm text-[var(--color-danger)]">
										<AlertCircle size={14} aria-hidden="true" /> {claimResult.error}
									</span>
								{/if}
							</div>
						{:else}
							<p class="text-xs text-[var(--color-muted)]">
								{t(
									'services.claim_need_org',
									'You must be a member of an organization to claim this listing.'
								)}
							</p>
						{/if}
					{/if}
				</div>
			{/if}

			<!-- Related services — same content_type items ranked by the
				related endpoint's embedding similarity. ContentArticle rows (no
				service meta sidecars) so these are lightweight link cards, not
				ServiceCard. -->
			{#if (data.recommendedContent ?? []).length > 0}
				<section aria-labelledby="related-heading">
					<h2
						id="related-heading"
						class="text-sm font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]"
					>
						{t('services.related_heading', 'You might also like')}
					</h2>
					<div class="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
						{#each (data.recommendedContent ?? []) as item (item.id)}
							<a
								href={contentHref(item)}
								class="group flex flex-col overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] transition-all hover:-translate-y-0.5 hover:shadow-lg focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary)]"
							>
								{#if item.featured_image_url}
									<div class="aspect-[16/9] w-full overflow-hidden">
										<img
											src={item.featured_image_url}
											alt=""
											loading="lazy"
											class="h-full w-full object-cover transition-transform duration-500 group-hover:scale-105"
										/>
									</div>
								{/if}
								<div class="flex flex-1 flex-col gap-1.5 p-4">
									<h3
										class="line-clamp-2 text-sm font-semibold leading-snug text-[var(--color-text)] transition-colors group-hover:text-[var(--color-primary)]"
									>
										{item.title}
									</h3>
									{#if item.excerpt || item.body}
										<p class="line-clamp-2 text-xs text-[var(--color-text-subtle)]">
											{item.excerpt || excerpt(item.body_html || item.body, 100)}
										</p>
									{/if}
									<span
										class="mt-auto inline-flex items-center gap-1 pt-2 text-xs font-medium text-[var(--color-primary)]"
									>
										{t('services.card_view', 'View')}
										<ArrowRight size={12} aria-hidden="true" />
									</span>
								</div>
							</a>
						{/each}
					</div>
				</section>
			{/if}
		</div>
	{/if}
</div>

{#if stickyCta && service}
	<StickyBookBar
		price={heroPrice}
		{ratingAvg}
		{ratingCount}
		ctaLabel={stickyCta.label}
		ctaHref={stickyCta.href}
		external={stickyCta.external}
	/>
{/if}
