<script lang="ts">
	// Ref: japaconnect-engineering/sections/98-service-directory.md
	// Ref: japaconnect-engineering/sections/114-services-marketplace.md
	// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§3.3)
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
	import BookingDialog from '$lib/components/marketplace/BookingDialog.svelte';
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
	// Meta description — bounded excerpt of the rendered HTML body (clean
	// text, no markdown artifacts), falling back to the raw body and then
	// the title when both are empty.
	const description = $derived(
		service ? excerpt(service.body_html || service.body || '', 160) || service.title : ''
	);
	const heroExcerpt = $derived(excerpt(service?.body_html || service?.body || '', 180));
	// Section 26 — self-referencing canonical on the clean slug URL (never
	// the query-string form) so filtered/paginated hits consolidate.
	const canonical = $derived(
		service ? `${page.url.origin}/services/${service.slug}` : `${page.url.origin}/services`
	);
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
	// §114: the detail payload omits min_price — derive the low price
	// from the tier list when tiers exist, else fall back to the
	// listing's own min_price/price_amount (all minor units). Feeds the
	// "From $X" hero/book-card display AND the JSON-LD lowPrice so both
	// agree on a tiers-only listing.
	const offerPriceMinor = $derived(
		service?.tiers?.length
			? Math.min(...service.tiers.map((t) => t.price_amount))
			: (service?.min_price ?? service?.price_amount ?? null)
	);
	const heroPrice = $derived(service ? priceLabel(service, currencies, offerPriceMinor) : null);
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
	// TierGrid "Choose" only makes sense when a booking surface exists for
	// this viewer — owners and authed users without service.booking.create
	// would scroll to an absent #book section.
	const tierCtaEnabled = $derived(canBook || guestBooking);

	// Booking dialog (spec §3.3) — the #book summary card, the TierGrid
	// "Choose X" buttons and the sticky bar CTA all open it. Form state
	// (departure, party size, dates, provider, idempotency key, tier and
	// coupon preview) lives inside BookingDialog.
	let bookingOpen = $state(false);
	// §114 tier preselection — set by TierGrid's "Choose X" and applied by
	// the dialog when it opens; submitted as tier_id on paid bookings.
	let initialTierId = $state('');

	function openBookingDialog() {
		// Non-tier open path — consume any prior "Choose X" preselection so
		// reopening via the card/sticky CTA doesn't resurrect a removed tier.
		initialTierId = '';
		bookingOpen = true;
	}

	// TierGrid "Choose X" — preselect the tier and open the dialog. The
	// button only renders when tierCtaEnabled (a booking surface exists
	// for this viewer), so no scroll-to-anchor fallback is needed.
	function selectTier(tier: PackageTier) {
		if (!tierCtaEnabled) return;
		initialTierId = tier.id;
		bookingOpen = true;
	}

	// Whether the #book section has anything to render — paid/inquiry modes
	// plus either an authed booker (canBook) or a guest who can open the
	// dialog's login prompt (guestBooking). Owners and viewers without
	// service.booking.create get no booking surface, so the CTA must not
	// point at an empty anchor.
	const bookSurface = $derived(
		!!service &&
			service.purchase_mode !== 'external' &&
			service.purchase_mode !== '' &&
			(canBook || guestBooking)
	);

	// booking_url is stored per-listing and the create form exposes it for
	// every kind — any listing with no internal booking mode (external or
	// plain services, the dominant case) uses it as the outbound CTA. Paid
	// and inquiry listings keep the internal #book surface as the primary
	// action even when a booking_url also exists.
	const bookingOutbound = $derived(
		!!service?.booking_url && (service.purchase_mode === 'external' || !service.purchase_mode)
	);

	// Hero CTAs — mode-aware. Primary: paid/inquiry scroll to #book,
	// external/mode-less listings link out to booking_url, then the
	// provider's website (or the provider card). Secondary "Talk to an
	// expert" reuses the inquiry path (#book) or the org profile.
	const primaryCta = $derived.by(():
		| { label: string; href: string; external: boolean }
		| null => {
		if (!service) return null;
		if (bookingOutbound && service.booking_url) {
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

	const secondaryCta = $derived.by(
		(): { label: string; href: string; dialog?: boolean } | null => {
			if (!service) return null;
			// Inquiry intent IS the expert-contact path — open the booking
			// dialog directly instead of bouncing through the org profile.
			if (service.purchase_mode === 'inquiry' && (canBook || guestBooking)) {
				return {
					label: t('services.talk_to_expert', 'Talk to an expert'),
					href: '#book',
					dialog: true
				};
			}
			if (service.org_slug) {
				return {
					label: t('services.talk_to_expert', 'Talk to an expert'),
					href: `/org/${service.org_slug}`
				};
			}
			return null;
		}
	);

	// The mobile sticky bar only makes sense where a booking surface exists —
	// owners manage the listing instead of booking it.
	const stickyCta = $derived.by(
		(): { label: string; href: string; external: boolean; dialog?: boolean } | null => {
			if (!service || canEdit) return null;
			if (bookingOutbound && service.booking_url) {
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
					external: false,
					// The booking form lives in BookingDialog now — the bar's CTA
					// opens it directly instead of scrolling to the #book card.
					dialog: true
				};
			}
			return null;
		}
	);

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
					url: canonical,
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
					// Tiered listings expose a price spread — AggregateOffer w/
					// lowPrice (min tier) matches the "From $X" card semantics.
					...(offerPriceMinor != null && service.price_currency
						? {
								offers: (service.tiers?.length ?? 0) > 0
									? {
											'@type': 'AggregateOffer',
											lowPrice: offerPriceMinor / 100,
											priceCurrency: service.price_currency,
											offerCount: service.tiers?.length
										}
									: {
											'@type': 'Offer',
											price: offerPriceMinor / 100,
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
	<link rel="canonical" href={canonical} />
	<meta property="og:title" content={service?.title ?? title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="article" />
	<meta property="og:url" content={canonical} />
	{#if ogImage}
		<meta property="og:image" content={ogImage} />
	{/if}
	<meta name="twitter:card" content={ogImage ? 'summary_large_image' : 'summary'} />
	<meta name="twitter:title" content={service?.title ?? title} />
	<meta name="twitter:description" content={description} />
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

				<!-- Rating + social proof — the #reviews anchor only mounts when
				     hasReviews, so the stars link only when the section exists. -->
				<div class="mt-4 flex flex-wrap items-center gap-x-4 gap-y-2 text-sm">
					{#if ratingCount > 0}
						{#snippet ratingRow()}
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
						{/snippet}
						{#if hasReviews}
							<a href="#reviews" class="flex items-center gap-1.5">
								{@render ratingRow()}
							</a>
						{:else}
							<span class="flex items-center gap-1.5">
								{@render ratingRow()}
							</span>
						{/if}
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
						{#if secondaryCta.dialog}
							<button
								type="button"
								onclick={openBookingDialog}
								class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-5 py-2.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
							>
								{secondaryCta.label}
							</button>
						{:else}
							<a
								href={secondaryCta.href}
								class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-5 py-2.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
							>
								{secondaryCta.label}
							</a>
						{/if}
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
					ctaEnabled={tierCtaEnabled}
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
				</section>
			{/if}

			<!-- Action links — booking_url + website_url render for ANY
				listing that sets them (the create form exposes both outside the
				package-only block); internal paid/inquiry booking still lives in
				#book. -->
			{#if service.website_url || service.booking_url}
				<div class="flex flex-wrap gap-3">
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
					{#if service.booking_url}
						<a
							href={service.booking_url}
							target="_blank"
							rel="noopener noreferrer"
							class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
						>
							<ExternalLink size={16} aria-hidden="true" />
							{isPackage && service.purchase_mode === 'external'
								? t('services.book_partner', 'Book on partner site')
								: t('services.book_now', 'Book now')}
						</a>
					{/if}
				</div>
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
				<!-- Plan Task 4.10: mobile gets the StickyBookBar; on desktop the
				     summary card itself sticks below the marketplace header while
				     the sections after it scroll underneath. -->
				<section
					id="book"
					aria-labelledby="book-heading"
					class="scroll-mt-20 lg:sticky lg:top-24 lg:z-10 lg:bg-[var(--color-bg)]"
				>
					<h2
						id="book-heading"
						class="text-sm font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]"
					>
						{service.purchase_mode === 'paid'
							? t('services.book_heading_paid', 'Book this package')
							: t('services.book_heading_inquiry', 'Booking & inquiries')}
					</h2>
					<!-- Order summary card — the CTA opens BookingDialog
					     (form, tier picker, coupon preview all live there). -->
					<div
						class="mt-4 flex flex-wrap items-center justify-between gap-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5"
					>
						<div class="min-w-0">
							{#if heroPrice}
								<p class="flex flex-wrap items-baseline gap-x-2">
									<span class="text-xs text-[var(--color-muted)]"
										>{t('services.price_from', 'From')}</span
									>
									<span class="text-xl font-bold text-[var(--color-text)]">{heroPrice}</span>
									<span class="text-xs text-[var(--color-muted)]">{pricePerLabel}</span>
								</p>
							{:else}
								<p class="text-sm text-[var(--color-muted)]">
									{t('services.pkg_price_on_request', 'Price on request')}
								</p>
							{/if}
							{#if service.purchase_mode === 'paid' && service.agent_balance_due != null}
								<p class="mt-1 text-xs text-[var(--color-muted)]">
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
						</div>
						<button
							type="button"
							onclick={openBookingDialog}
							class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-5 py-2.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
						>
							<Send size={16} aria-hidden="true" />
							{service.purchase_mode === 'paid'
								? t('services.book_now', 'Book now')
								: t('services.book_inquiry', 'Send inquiry')}
						</button>
					</div>
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
										<Loader2 size={16} class="motion-safe:animate-spin" aria-hidden="true" />
									{:else}
										<BadgeCheck size={16} aria-hidden="true" />
									{/if}
									{t('services.claim_listing', 'Claim this listing')}
								</button>
								{#if claimResult?.error}
									<span role="alert" class="flex items-center gap-1 text-sm text-[var(--color-danger)]">
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
								class="group flex flex-col overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] transition-all hover:shadow-lg focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary)] motion-safe:hover:-translate-y-0.5"
							>
								{#if item.featured_image_url}
									<div class="aspect-[16/9] w-full overflow-hidden">
										<img
											src={item.featured_image_url}
											alt=""
											loading="lazy"
											class="h-full w-full object-cover transition-transform duration-500 motion-safe:group-hover:scale-105"
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

{#if service}
	<BookingDialog
		bind:open={bookingOpen}
		{service}
		{currencies}
		authed={!!data.authed}
		tiers={service.tiers}
		{initialTierId}
		defaultEmail={data.user?.email ?? ''}
	/>
{/if}

{#if stickyCta && service}
	<StickyBookBar
		price={heroPrice}
		{ratingAvg}
		{ratingCount}
		ctaLabel={stickyCta.label}
		ctaHref={stickyCta.href}
		external={stickyCta.external}
		onCta={stickyCta.dialog ? openBookingDialog : undefined}
	/>
{/if}
