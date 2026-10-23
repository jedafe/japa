<script lang="ts">
	// §114 marketplace detail — one page, three kind-aware layouts matching
	// the 2026-10-10 mockups:
	//   service    → two-column hero (image + facts card), included grid,
	//                steps rail, tier cards, reviews+FAQ row, CTA banner
	//   package    → same skeleton with overlay facts, highlights card,
	//                departures, scam notice
	//   mentorship → mentor-profile hero card, About/details 2-col body,
	//                mentee reviews + steps, apply CTA banner
	// Ref: japaconnect-engineering/sections/98-service-directory.md
	// Ref: japaconnect-engineering/sections/114-services-marketplace.md
	// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§3.3)
	import { t } from '$lib/i18n';
	import { goto, invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import { api } from '$lib/api/client';
	import {
		AlertCircle,
		ArrowRight,
		BadgeCheck,
		Briefcase,
		CalendarDays,
		CheckCircle2,
		ChevronRight,
		ExternalLink,
		Globe,
		GraduationCap,
		HandCoins,
		Heart,
		Loader2,
		MapPin,
		MessageSquare,
		Send,
		ShieldCheck,
		Star,
		Users
		TriangleAlert,
} from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import ShareButton from '$lib/components/ui/ShareButton.svelte';
	import SellingPoints from '$lib/components/marketplace/SellingPoints.svelte';
	import FactsCard from '$lib/components/marketplace/FactsCard.svelte';
	import IconCardGrid from '$lib/components/marketplace/IconCardGrid.svelte';
	import ChecklistSection from '$lib/components/marketplace/ChecklistSection.svelte';
	import HowItWorks from '$lib/components/marketplace/HowItWorks.svelte';
	import TierGrid from '$lib/components/marketplace/TierGrid.svelte';
	import CouponCard from '$lib/components/marketplace/CouponCard.svelte';
	import ServiceFaq from '$lib/components/marketplace/ServiceFaq.svelte';
	import ServiceReviews from '$lib/components/marketplace/ServiceReviews.svelte';
	import ReviewForm from '$lib/components/reviews/ReviewForm.svelte';
	import UserHoverCard from '$lib/components/ui/UserHoverCard.svelte';
	import ProviderCard from '$lib/components/marketplace/ProviderCard.svelte';
	import MentorSidebar from '$lib/components/marketplace/MentorSidebar.svelte';
	import CtaBanner from '$lib/components/marketplace/CtaBanner.svelte';
	import StickyBookBar from '$lib/components/marketplace/StickyBookBar.svelte';
	import BookingDialog from '$lib/components/marketplace/BookingDialog.svelte';
	import MentorshipRequestDialog from '$lib/components/marketplace/MentorshipRequestDialog.svelte';
	import MessageDialog from '$lib/components/marketplace/MessageDialog.svelte';
	import AppealButton from '$lib/components/appeals/AppealButton.svelte';
	import { serviceCategoryIcon } from '$lib/components/marketplace/serviceCategoryIcon';
	import { getViewerState, updateViewerState } from '$lib/api/viewerState';
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
		ServiceFact,
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
	const description = $derived(
		service ? excerpt(service.body_html || service.body || '', 160) || service.title : ''
	);
	const heroExcerpt = $derived(excerpt(service?.body_html || service?.body || '', 200));
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

	// Claim flow state (POST /services/{id}/claim) — the backend requires
	// { org_id } so the user claims on behalf of one of their orgs (BUG-004).
	let claimOrgId = $state('');
	let claiming = $state(false);
	let claimResult = $state<{ ok?: boolean; error?: string } | null>(null);

	const canClaim = $derived(
		!!service && service.claim_status === 'unclaimed' && (data.myOrgs?.length ?? 0) > 0
	);

	// §22 appeal surface — a rejected claim can be appealed by members of
	// the rejected org.
	const canAppealClaim = $derived(
		!!data.authed &&
			!!service &&
			service.claim_status === 'rejected' &&
			service.claimed_by_org_id != null &&
			(data.myOrgs ?? []).some((o) => o.id === service.claimed_by_org_id)
	);

	// ------------------------------------------------------------------
	// Kind surfaces (§111 package, 000824 mentorship shadow listing)
	// ------------------------------------------------------------------
	const currencies = $derived(
		(page.data.currencies ?? []) as { code: string; name: string; symbol: string }[]
	);
	const isMentorship = $derived(service?.listing_kind === 'mentorship');
	const mentor = $derived(service?.mentor ?? null);
	const mentorUnavailable = $derived(!mentor || mentor.is_mentor === false);
	const mentorFull = $derived(!!mentor && mentor.available_slots <= 0);
	const isOwnMentorListing = $derived(
		isMentorship && !!service && service.author_id === data.currentUserId
	);
	const packagesEnabled = $derived(!!page.data.features?.['service_packages.enabled']);
	const mentorshipEnabled = $derived(!!page.data.features?.['mentorship.enabled']);
	// Guests still get the CTA — MentorshipRequestDialog renders a login
	// prompt (same pattern as BookingDialog). Authed viewers need
	// service.booking.create — the same node POST /services/:id/bookings
	// enforces — and the request path is dead when mentorship.enabled is off.
	const mentorCanRequest = $derived(
		isMentorship &&
			!isOwnMentorListing &&
			!mentorUnavailable &&
			!mentorFull &&
			mentorshipEnabled &&
			(!data.authed || (data.user?.permissions ?? []).includes('service.booking.create'))
	);
	let mentorDialogOpen = $state(false);
	let messageOpen = $state(false);
	// §114 flags — the API omits tiers/coupons entirely when the flags are
	// off, so the data-length guard is the authoritative check.
	const tiersVisible = $derived(
		!!page.data.features?.['service_packages.tiers_enabled'] &&
			(service?.tiers?.length ?? 0) > 0
	);
	const couponsVisible = $derived(
		!!page.data.features?.['service_coupons.enabled'] && (service?.coupons?.length ?? 0) > 0
	);
	// §114: the detail payload omits min_price — derive the low price from
	// the tier list when tiers exist, else fall back to min_price/price_amount.
	const offerPriceMinor = $derived(
		service?.tiers?.length
			? Math.min(...service.tiers.map((tier) => tier.price_amount))
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
	// Bookings: any published listing with purchase_mode inquiry accepts
	// inquiries (org inbox for org-owned rows, the author's provider inbox
	// for org-less ones); 'paid' opens the fee checkout; 'external' points
	// at booking_url. Owners/org members manage via the inbox instead.
	const canBook = $derived(
		packagesEnabled &&
			!!data.authed &&
			!!service &&
			!canEdit &&
			// POST /services/:id/bookings is gated on service.booking.create —
			// hide the CTA rather than letting the submit 403.
			(data.user?.permissions ?? []).includes('service.booking.create')
	);
	const guestBooking = $derived(packagesEnabled && !data.authed && !!service);
	// Mentorship tier CTAs need no booking surface — they open the request
	// dialog, which renders a login prompt for guests.
	const tierCtaEnabled = $derived(canBook || guestBooking || isMentorship);

	// Booking dialog — hero CTA, TierGrid "Choose X", CTA banner and the
	// sticky bar all open it; form state lives inside BookingDialog.
	let bookingOpen = $state(false);
	let initialTierId = $state('');

	function openBookingDialog() {
		// Consume any prior "Choose X" preselection so a later non-tier open
		// doesn't resurrect a removed tier.
		initialTierId = '';
		bookingOpen = true;
	}

	// Single CTA dispatcher — mentorship listings open the request dialog;
	// every other kind opens the booking dialog.
	function openCtaDialog() {
		if (isMentorship) {
			// Consume any prior tier prefill — a plain Apply opens a blank form.
			mentorTierGoals = '';
			mentorDialogOpen = true;
			return;
		}
		openBookingDialog();
	}

	// Mentorship tier CTAs open the request dialog (inquiry bookings reject
	// tier_id) with the chosen tier named in the goals; the note prefill is
	// consumed on open like initialTierId.
	let mentorTierGoals = $state('');

	function selectTier(tier: PackageTier) {
		if (!tierCtaEnabled) return;
		if (isMentorship) {
			mentorTierGoals = t('services.tier_request_prefill', 'Interested in the {name} package.', {
				name: tier.name
			});
			mentorDialogOpen = true;
			return;
		}
		initialTierId = tier.id;
		bookingOpen = true;
	}

	// Whether an internal booking surface exists — paid/inquiry modes plus
	// either an authed booker (canBook) or a guest who can open the
	// dialog's login prompt (guestBooking).
	const bookSurface = $derived(
		!!service &&
			service.purchase_mode !== 'external' &&
			service.purchase_mode !== '' &&
			(canBook || guestBooking)
	);

	// booking_url is stored per-listing — any listing with no internal
	// booking mode uses it as the outbound CTA.
	const bookingOutbound = $derived(
		!!service?.booking_url && (service.purchase_mode === 'external' || !service.purchase_mode)
	);

	// Primary hero CTA — mode-aware (paid/inquiry dialog, external link,
	// provider page). Mentorship resolves through the request-dialog path.
	const primaryCta = $derived.by(():
		| { label: string; href: string; external: boolean; dialog?: boolean }
		| null => {
		if (!service) return null;
		if (isMentorship) {
			if (!mentorCanRequest) return null;
			return {
				label: t('services.apply_mentorship', 'Apply for mentorship'),
				href: '#request-mentorship',
				external: false,
				dialog: true
			};
		}
		if (bookingOutbound && service.booking_url) {
			return {
				label: t('services.get_started', 'Get Started'),
				href: service.booking_url,
				external: true
			};
		}
		if (bookSurface && service.purchase_mode === 'paid') {
			return {
				label: t('services.get_started', 'Get Started'),
				href: '#book',
				external: false,
				dialog: true
			};
		}
		if (bookSurface && service.purchase_mode === 'inquiry') {
			return {
				label: t('services.get_started', 'Get Started'),
				href: '#book',
				external: false,
				dialog: true
			};
		}
		if (service.website_url) {
			return {
				label: t('services.visit_website', 'Visit website'),
				href: service.website_url,
				external: true
			};
		}
		if (service.org_slug) {
			return {
				label: t('services.provider_view', 'View provider'),
				href: `/org/${service.org_slug}`,
				external: false
			};
		}
		return null;
	});

	// "Talk to an Expert" / "Message Mentor" — the mockup's secondary
	// outline button. An author/mentor DM is the direct path (MessageDialog
	// → /dm/conversations); without a messageable author it falls back to
	// the inquiry dialog or the org profile.
	const secondaryCta = $derived.by((): { label: string; dialog?: 'message' | 'booking'; href?: string } | null => {
		if (!service || isOwnMentorListing) return null;
		if (isMentorship) {
			if (mentorUnavailable) return null;
			return { label: t('services.mentor_contact', 'Message Mentor'), dialog: 'message' };
		}
		if (service.purchase_mode === 'inquiry' && (canBook || guestBooking)) {
			return { label: t('services.talk_to_expert', 'Talk to an Expert'), dialog: 'booking' };
		}
		if (service.author_id && service.author_id !== data.currentUserId) {
			return { label: t('services.talk_to_expert', 'Talk to an Expert'), dialog: 'message' };
		}
		if (service.org_slug) {
			return {
				label: t('services.talk_to_expert', 'Talk to an Expert'),
				href: `/org/${service.org_slug}`
			};
		}
		return null;
	});

	// The mobile sticky bar only makes sense where a booking surface exists.
	const stickyCta = $derived.by(
		(): { label: string; href: string; external: boolean; dialog?: boolean } | null => {
			if (!service || canEdit) return null;
			if (isMentorship) {
				if (!mentorCanRequest) return null;
				return {
					label: t('services.apply_mentorship', 'Apply for mentorship'),
					href: '#request-mentorship',
					external: false,
					dialog: true
				};
			}
			if (bookingOutbound && service.booking_url) {
				return {
					label: t('services.book_now', 'Book now'),
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
					dialog: true
				};
			}
			return null;
		}
	);

	// ---------------------------------------------------------------
	// Mockup surfaces (§114 2026-10-10) — structured listing fields with
	// real-data fallbacks so unauthored listings keep the mockup shape.
	// ---------------------------------------------------------------
	const sellingPoints = $derived(service?.selling_points ?? []);

	// Hero facts — authored facts[] first; otherwise synthesized from the
	// listing's own fields (all real data, never hardcoded).
	const heroFacts = $derived<ServiceFact[]>(
		service?.facts?.length
			? service.facts
			: service
				? ([
						service.destination_space_name && {
							icon: 'map-pin',
							label: t('services.fact_destination', 'Destination'),
							value: service.destination_space_name
						},
						(service.available_from || service.available_until) && {
							icon: 'calendar-days',
							label: t('services.fact_availability', 'Availability'),
							value: `${service.available_from ? formatDate(service.available_from) : '…'} — ${
								service.available_until
									? formatDate(service.available_until)
									: t('services.pkg_ongoing', 'ongoing')
							}`
						},
						service.capacity != null && {
							icon: 'users',
							label: t('services.fact_capacity', 'Capacity'),
							value: t('services.pkg_capacity_label', 'Up to {n} people', {
								n: service.capacity
							})
						},
						service.is_remote && {
							icon: 'wifi',
							label: t('services.fact_delivery', 'Delivery'),
							value: t('services.remote', 'Remote')
						}
					].filter(Boolean) as ServiceFact[])
				: []
	);

	const mentorName = $derived(mentor?.display_name || mentor?.username || '');

	// ---------------------------------------------------------
	// Save (bookmark) — POST/DELETE /content/{id}/bookmark (§14).
	// Same flag gate as ServiceCard's wishlist heart.
	// ---------------------------------------------------------
	const bookmarksEnabled = $derived(
		((page.data.features as Record<string, boolean> | undefined)?.['feature.bookmarks']) ?? true
	);
	let bookmarked = $state(false);
	let bookmarkPending = $state(false);
	let bookmarkLoaded = $state(false);
	$effect(() => {
		if (bookmarkLoaded || !data.authed || !bookmarksEnabled || !service?.id) return;
		bookmarkLoaded = true;
		getViewerState(service.id).then((s) => {
			bookmarked = s.bookmarked;
		});
	});
	async function toggleSave() {
		if (!service || bookmarkPending) return;
		if (!data.authed) {
			await goto(`/login?redirect=${encodeURIComponent(page.url.pathname + page.url.search)}`);
			return;
		}
		bookmarkPending = true;
		const next = !bookmarked;
		bookmarked = next;
		updateViewerState(service.id, { bookmarked: next });
		// No finally: Svelte 5 $state set in finally does not re-render —
		// clear the lock on both branches instead.
		try {
			const res = next
				? await api.post(`/content/${service.id}/bookmark`, {})
				: await api.delete(`/content/${service.id}/bookmark`);
			if (res.error) throw new Error(res.error.message);
			bookmarkPending = false;
		} catch {
			bookmarked = !next;
			updateViewerState(service.id, { bookmarked: !next });
			bookmarkPending = false;
		}
	}

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
			claimResult = {
				error: t('services.claim_select_org', 'Select an organization to claim on behalf of.')
			};
			return;
		}
		claiming = true;
		claimResult = null;
		const res = await api.post<{ claim_status: string }>(`/services/${service.id}/claim`, {
			org_id: claimOrgId
		});
		if (res.data) {
			claiming = false;
			claimResult = { ok: true };
		} else {
			claiming = false;
			claimResult = { error: res.error?.message ?? t('services.claim_failed', 'Claim failed') };
		}
	}

	// Breadcrumb middle segment — mentorship lists under
	// ?kind=mentorship; everything else under plain services.
	const breadcrumbMid = $derived(
		isMentorship
			? { label: t('services.kind_mentorship', 'Mentorship'), href: '/services?kind=mentorship' }
			: { label: t('marketplace.nav_services', 'Services'), href: '/services' }
	);

	// Per-kind section headings (the mockups name these differently).
	const includedHeading = $derived(
		isMentorship
			? t('services.included_heading_mentor', "What You'll Get")
			: t('services.included_heading', "What's Included")
	);
	const reviewsHeading = $derived(
		isMentorship
			? t('services.reviews_heading_mentor', 'What Mentees Say')
			: t('services.reviews_heading_svc', 'What Our Clients Say')
	);
	const tiersHeading = $derived(
		isMentorship
			? t('services.tiers_heading_mentor', 'Mentorship Packages')
			: t('services.tiers_heading', 'Packages & pricing')
	);

	// CTA banner copy per kind — primary reuses the mode-aware dispatcher.
	const bannerCopy = $derived(
		isMentorship
			? {
					eyebrow: '',
					heading: t('services.banner_mentor_heading', 'Ready to achieve your goals?'),
					body: t(
						'services.banner_mentor_body',
						'Apply for mentorship today and get the guidance you need to succeed.'
					)
				}
			: {
					eyebrow: '',
					heading: t('services.banner_svc_heading', 'Ready to get started?'),
					body: t(
						'services.banner_svc_body',
						'Let our experts handle the process while you focus on your journey.'
					)
				}
	);

	// JSON-LD — schema.org Service + provider/offer/aggregateRating (§3.3).
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

	// §114: FAQPage JSON-LD only when the listing carries authored faqs[].
	const faqJsonLd = $derived(
		service?.faqs?.length
			? JSON.stringify({
					'@context': 'https://schema.org',
					'@type': 'FAQPage',
					mainEntity: service.faqs.map((f) => ({
						'@type': 'Question',
						name: f.question,
						acceptedAnswer: { '@type': 'Answer', text: f.answer }
					}))
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
	{#if faqJsonLd}
		{@html safeJsonLdScript(faqJsonLd)}
	{/if}
</svelte:head>

{#if data.loadError || !service}
	<EmptyState
		icon={Briefcase}
		title={t('services.load_error', 'Could not load service')}
		body={t('services.load_error_body', 'Please try again later.')}
		action={{ label: t('services.back_to_dir', 'Back to services'), href: '/services' }}
	/>
{:else}
	<!-- Zero-review CTA — keeps the "first review" path discoverable when
	     the listing has no reviews yet (ServiceReviews only renders with
	     data). Both kind layouts render it in the reviews cell. -->
	{#snippet reviewsCta()}
		<section
			aria-labelledby="reviews-heading"
			class="rounded-[var(--radius-card)] border border-dashed border-[var(--color-border)] bg-[var(--color-surface)] p-6"
		>
			<h2 id="reviews-heading" class="text-xl font-bold text-[var(--color-text)]">
				{reviewsHeading}
			</h2>
			<p class="mt-2 text-sm text-[var(--color-muted)]">
				{t('services.no_reviews_yet', 'No reviews yet. Be the first to share your experience.')}
			</p>
			<a
				href="/services/{service.slug}/reviews"
				class="mt-4 inline-flex items-center gap-1.5 rounded-lg bg-[var(--color-primary)] px-4 py-2 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				<Star size={15} aria-hidden="true" />
				{t('reviews.write_title', 'Write a review')}
			</a>
		</section>
	{/snippet}

	<!-- §29 inline write path — lets a signed-in traveller review without
	     leaving the detail page; invalidateAll re-pulls the listing's
	     reviews slice + aggregate. Guests get the login prompt (same
	     contract as /services/[slug]/reviews). -->
	{#snippet reviewWrite()}
		<div class="mt-6">
			<!-- Editors (author / org members / service.edit_any) manage the
			     listing rather than review it — hidden; the backend's
			     self_review guard is the backstop. -->
			{#if data.authed && !canEdit}
				<ReviewForm
					entityType="service_listing"
					entityId={service.id}
					onCreated={() => invalidateAll()}
				/>
			{:else}
				<p
					class="rounded-[var(--radius)] border border-dashed border-[var(--color-border)] p-4 text-center text-sm text-[var(--color-muted)]"
				>
					<a
						href="/login?redirect={encodeURIComponent(page.url.pathname + page.url.search)}"
						class="font-medium text-[var(--color-primary)] hover:underline"
						>{t('services.reviews_login', 'Log in to write a review')}</a
					>
				</p>
			{/if}
		</div>
	{/snippet}

	<div class="mx-auto w-full max-w-6xl {stickyCta ? 'pb-24' : 'pb-14'} lg:pb-14">
		<!-- Breadcrumb: Home / Services|Packages|?kind=mentorship / Category / Title -->
		<nav
			class="mb-6 flex flex-wrap items-center gap-1 text-xs text-[var(--color-muted)]"
			aria-label={t('a11y.breadcrumb', 'Breadcrumb')}
		>
			<a href="/" class="transition-colors hover:text-[var(--color-text)]"
				>{t('nav.home', 'Home')}</a
			>
			<ChevronRight class="rtl:-scale-x-100" size={12} aria-hidden="true" />
			<a href={breadcrumbMid.href} class="transition-colors hover:text-[var(--color-text)]"
				>{breadcrumbMid.label}</a
			>
			<!-- Mentorship shadow listings sit under the seeded 'mentorship'
			     category — its crumb would repeat the kind crumb
			     (Home / Mentorship / Mentorship / Title), so it is suppressed. -->
			{#if catLabel && !(isMentorship && service.service_category === 'mentorship')}
				<ChevronRight class="rtl:-scale-x-100" size={12} aria-hidden="true" />
				<a
					href="/services?category={service.service_category}"
					class="transition-colors hover:text-[var(--color-text)]">{catLabel}</a
				>
			{/if}
			<ChevronRight class="rtl:-scale-x-100" size={12} aria-hidden="true" />
			<span class="truncate font-medium text-[var(--color-text)]" aria-current="page"
				>{service.title}</span
			>
		</nav>

		{#if isMentorship}
			<!-- ================================================
			     MENTORSHIP LAYOUT (mockup 02)
			     ================================================ -->
			<section class="grid gap-8 lg:grid-cols-2 lg:items-center">
				<!-- Left: pill / title / excerpt / rating / selling points / CTAs -->
				<div>
					<div class="flex flex-wrap items-center gap-2">
						<span
							class="inline-flex items-center gap-1.5 rounded-full bg-[var(--color-primary)]/10 px-3 py-1 text-xs font-semibold text-[var(--color-primary)]"
						>
							<GraduationCap size={13} aria-hidden="true" />
							{t('services.mentor_pill', 'Mentorship Service')}
						</span>
						{#if service.claim_status && service.claim_status !== 'unclaimed'}
							<span
								class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs text-[var(--color-text-subtle)]"
							>
								{claimStatusLabel(service.claim_status)}
							</span>
						{/if}
					</div>

					<h1 class="mt-4 text-3xl font-bold tracking-tight text-[var(--color-text)] sm:text-4xl">
						{service.title}
					</h1>

					{#if heroExcerpt}
						<p class="mt-3 max-w-xl text-sm leading-relaxed text-[var(--color-text-subtle)]">
							{heroExcerpt}
						</p>
					{/if}

					<!-- Rating + mentees social proof -->
					<div class="mt-4 flex flex-wrap items-center gap-x-4 gap-y-2 text-sm">
						{#if ratingCount > 0}
							{#if hasReviews}
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
									<span class="font-medium text-[var(--color-text)]"
										>{ratingAvg.toFixed(1)}</span
									>
									<span class="text-[var(--color-muted)]"
										>({t('services.reviews_summary', '{count} reviews', {
											count: ratingCount
										})})</span
									>
								</a>
							{:else}
								<span class="flex items-center gap-1.5">
									<span class="font-medium text-[var(--color-text)]"
										>{ratingAvg.toFixed(1)}</span
									>
									<span class="text-[var(--color-muted)]"
										>({t('services.reviews_summary', '{count} reviews', {
											count: ratingCount
										})})</span
									>
								</span>
							{/if}
						{/if}
						{#if mentor && (mentor.mentees_helped ?? mentor.active_mentee_count) > 0}
							<span class="flex items-center gap-1 text-[var(--color-muted)]">
								<Users size={14} aria-hidden="true" />
								{t('services.mentor_mentees', '{n} mentees', {
									n: mentor.mentees_helped ?? mentor.active_mentee_count
								})}
							</span>
						{/if}
					</div>

					{#if sellingPoints.length > 0}
						<div class="mt-5">
							<SellingPoints points={sellingPoints} />
						</div>
					{/if}

					<!-- CTAs: Apply for Mentorship / Message Mentor / Save -->
					<div class="mt-7 flex flex-wrap items-center gap-3">
						{#if isOwnMentorListing}
							<p class="text-sm text-[var(--color-muted)]">
								{t(
									'services.mentor_own_listing',
									'This is your mentor listing — manage it from your mentor profile.'
								)}
							</p>
							<a
								href="/settings/mentorship"
								class="inline-flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-5 py-2.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
							>
								{t('mentors.edit_profile', 'Edit Mentor Profile')}
							</a>
						{:else if mentorUnavailable || mentorFull}
							<button
								type="button"
								disabled
								class="inline-flex items-center gap-1.5 rounded-lg bg-[var(--color-primary)] px-6 py-2.5 text-sm font-semibold text-[var(--color-primary-contrast)] disabled:opacity-50"
							>
								{t('services.apply_mentorship', 'Apply for Mentorship')}
							</button>
							<p class="w-full text-xs text-[var(--color-muted)]">
								{mentorUnavailable
									? t(
											'services.mentor_unavailable',
											'This mentor is not accepting requests right now.'
										)
									: t('services.mentor_full', "This mentor's roster is full — check back soon.")}
							</p>
						{:else if primaryCta}
							<button
								type="button"
								onclick={openCtaDialog}
								class="inline-flex items-center gap-1.5 rounded-lg bg-[var(--color-primary)] px-6 py-2.5 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
							>
								{primaryCta.label}
							</button>
						{/if}
						{#if secondaryCta}
							<button
								type="button"
								onclick={() => (messageOpen = true)}
								class="inline-flex items-center gap-1.5 rounded-lg border border-[var(--color-primary)]/40 bg-[var(--color-surface)] px-5 py-2.5 text-sm font-semibold text-[var(--color-primary)] transition-colors hover:bg-[var(--color-primary)]/5"
							>
								<MessageSquare size={15} aria-hidden="true" />
								{secondaryCta.label}
							</button>
						{/if}
						{#if bookmarksEnabled}
							<button
								type="button"
								onclick={toggleSave}
								disabled={bookmarkPending}
								class="inline-flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-5 py-2.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
							>
								<Heart
									size={15}
									class={bookmarked ? 'fill-[var(--color-danger)] text-[var(--color-danger)]' : ''}
									aria-hidden="true"
								/>
								{bookmarked ? t('services.saved', 'Saved') : t('services.save', 'Save')}
							</button>
						{/if}
					</div>
				</div>

				<!-- Right: hero panel + mentor profile overlay card -->
				<div class="relative">
					<div
						class="relative aspect-[4/3] w-full overflow-hidden rounded-2xl bg-[var(--color-surface-subtle)]"
					>
						{#if service.featured_image_url}
							<img
								src={service.featured_image_url}
								alt={service.title}
								class="h-full w-full object-cover"
							/>
						{:else if mentor?.avatar_url}
							<img
								src={mentor.avatar_url}
								alt={mentorName}
								class="h-full w-full object-cover object-top"
							/>
						{:else}
							<div class="grid h-full w-full place-items-center">
								<GraduationCap size={96} strokeWidth={1} class="text-[var(--color-muted)]" />
							</div>
						{/if}
					</div>
					{#if mentor}
						<!-- Overlay profile card — mockup's floating mentor card.
						     Avatar + name link to /u/{username} via UserHoverCard's
						     trigger (click navigates, hover shows the user card). -->
						<div
							class="absolute bottom-4 start-4 end-4 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)]/95 p-4 shadow-lg backdrop-blur sm:end-auto sm:min-w-64"
						>
							<div class="flex items-center gap-3">
								{#if mentor.username}
									<UserHoverCard
										id={service.author_id ?? ''}
										slug={mentor.username}
										name={mentorName}
										avatar={mentor.avatar_url ?? null}
										variant="row"
										secondary={mentor.title ?? undefined}
										class="flex items-center gap-3"
									>
										{#snippet trigger()}
											{#if mentor.avatar_url}
												<img
													src={mentor.avatar_url}
													alt={mentorName}
													class="h-12 w-12 shrink-0 rounded-full object-cover"
												/>
											{:else}
												<div
													class="grid h-12 w-12 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]"
												>
													<GraduationCap size={20} aria-hidden="true" />
												</div>
											{/if}
											<span class="block min-w-0 text-start">
												<span
													class="flex items-center gap-1 truncate text-sm font-bold text-[var(--color-text)]"
												>
													{mentorName}
													{#if service.is_verified}
														<BadgeCheck size={15} class="shrink-0 text-[var(--color-primary)]" />
													{/if}
												</span>
												{#if mentor.title}
													<span class="block truncate text-xs text-[var(--color-muted)]"
														>{mentor.title}</span
													>
												{/if}
											</span>
										{/snippet}
									</UserHoverCard>
								{:else}
									{#if mentor.avatar_url}
										<img
											src={mentor.avatar_url}
											alt={mentorName}
											class="h-12 w-12 shrink-0 rounded-full object-cover"
										/>
									{:else}
										<div
											class="grid h-12 w-12 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]"
										>
											<GraduationCap size={20} aria-hidden="true" />
										</div>
									{/if}
									<div class="min-w-0">
										<p
											class="flex items-center gap-1 truncate text-sm font-bold text-[var(--color-text)]"
										>
											{mentorName}
											{#if service.is_verified}
												<BadgeCheck size={15} class="shrink-0 text-[var(--color-primary)]" />
											{/if}
										</p>
										{#if mentor.title}
											<p class="truncate text-xs text-[var(--color-muted)]">{mentor.title}</p>
										{/if}
									</div>
								{/if}
							</div>
							<ul class="mt-3 space-y-1.5 text-xs text-[var(--color-text-subtle)]">
								{#if mentor.location}
									<li class="flex items-center gap-2">
										<MapPin size={12} class="shrink-0 text-[var(--color-muted)]" aria-hidden="true" />
										{mentor.location}
									</li>
								{/if}
								{#each (mentor.credentials ?? []).slice(0, 2) as cred, i (i)}
									<li class="flex items-center gap-2">
										<BadgeCheck size={12} class="shrink-0 text-[var(--color-muted)]" aria-hidden="true" />
										{cred}
									</li>
								{/each}
								{#if mentor.years_experience != null}
									<li class="flex items-center gap-2">
										<CalendarDays size={12} class="shrink-0 text-[var(--color-muted)]" aria-hidden="true" />
										{t('services.mentor_years_exp', '{n}+ years mentoring experience', {
											n: mentor.years_experience
										})}
									</li>
								{/if}
							</ul>
						</div>
					{/if}
				</div>
			</section>

			<!-- Main 2-col: content (left ~2/3) + mentor sidebar (right ~1/3) -->
			<div class="mt-12 grid gap-10 lg:grid-cols-[1fr_320px]">
				<div class="min-w-0 space-y-12">
					<!-- About Your Mentor — body + quick-stats row -->
					{#if service.body || mentor}
						<section id="request-mentorship" aria-labelledby="about-mentor-heading" class="scroll-mt-20">
							<h2 id="about-mentor-heading" class="text-xl font-bold text-[var(--color-text)]">
								{t('services.mentor_heading', 'About your mentor')}
							</h2>
							{#if service.body}
								<div class="prose prose-sm mt-4 max-w-none text-[var(--color-text-subtle)]">
									{@html sanitizeContentHtml(service.body_html || service.body)}
								</div>
							{:else if mentorUnavailable}
								<p class="mt-4 text-sm text-[var(--color-muted)]">
									{t(
										'services.mentor_unavailable',
										'This mentor is not accepting requests right now.'
									)}
								</p>
							{/if}
							{#if mentor}
								<!-- Quick stats — the mockup's 4-icon fact row -->
								{@const stats = [
									{
										icon: MessageSquare,
										label: t('services.mentor_response_time', 'Response Time'),
										value: mentor.response_time
									},
									{
										icon: Globe,
										label: t('services.mentor_languages', 'Languages'),
										value: (mentor.languages ?? []).join(', ') || undefined
									},
									{
										icon: CalendarDays,
										label: t('services.mentor_availability', 'Availability'),
										value: mentor.availability
									},
									{
										icon: Users,
										label: t('services.mentor_sessions', 'Sessions'),
										value: mentor.session_type
									}
								].filter((s) => !!s.value)}
								{#if stats.length > 0}
									<dl class="mt-6 grid grid-cols-2 gap-4 sm:grid-cols-4">
										{#each stats as stat, i (i)}
											{@const Icon = stat.icon}
											<div class="min-w-0">
												<dt class="flex items-center gap-1.5 text-xs text-[var(--color-muted)]">
													<Icon size={13} class="shrink-0 text-[var(--color-primary)]" aria-hidden="true" />
													{stat.label}
												</dt>
												<dd class="mt-1 truncate text-sm font-medium text-[var(--color-text)]">
													{stat.value}
												</dd>
											</div>
										{/each}
									</dl>
								{/if}
								<!-- Focus areas chips -->
								{#if mentor.areas && mentor.areas.length > 0}
									<div class="mt-6 flex flex-wrap gap-1.5">
										{#each mentor.areas as area, i (i)}
											<span
												class="inline-flex items-center rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2.5 py-0.5 text-xs font-medium text-[var(--color-text-subtle)]"
											>
												{area}
											</span>
										{/each}
									</div>
								{/if}
							{/if}
						</section>
					{/if}

					<!-- What You'll Get — included[] icon cards -->
					<IconCardGrid heading={includedHeading} included={service.included ?? []} />

					<!-- Who This Is For — audience[] checklist -->
					<ChecklistSection
						heading={t('services.audience_heading', 'Who This Is For')}
						lines={service.audience ?? []}
					/>

					<!-- How it works — the mentor mockup orders it ahead of the
					     packages so the process lands before the ask. -->
					{#if service.steps && service.steps.length > 0}
						<HowItWorks steps={service.steps} />
					{/if}

					<!-- Mentorship Packages — tiers + secure-payments footer -->
					{#if tiersVisible}
						<TierGrid
							tiers={service.tiers ?? []}
							{currencies}
							heading={tiersHeading}
							compareHref="/compare?type=mentors"
							ctaEnabled={tierCtaEnabled}
							onSelect={selectTier}
						/>
						<p class="flex items-center justify-center gap-1.5 text-center text-xs text-[var(--color-muted)]">
							<ShieldCheck size={13} aria-hidden="true" />
							{t('services.secure_payments_note', '100% Secure Payments · Money-back guarantee')}
						</p>
					{:else if heroPrice}
						<section
							aria-labelledby="pricing-heading"
							class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5"
						>
							<h2 id="pricing-heading" class="text-xl font-bold text-[var(--color-text)]">
								{tiersHeading}
							</h2>
							<div class="mt-3 flex flex-wrap items-baseline gap-2">
								<span class="text-sm text-[var(--color-muted)]"
									>{t('services.price_from', 'From')}</span
								>
								<span class="text-2xl font-bold text-[var(--color-text)]">{heroPrice}</span>
							</div>
						</section>
					{/if}

					{#if couponsVisible}
						<section aria-labelledby="coupons-heading">
							<h2 id="coupons-heading" class="text-xl font-bold text-[var(--color-text)]">
								{t('services.coupons_heading', 'Offers & coupons')}
							</h2>
							<div class="mt-4 grid gap-3 sm:grid-cols-2">
								{#each (service.coupons ?? []) as coupon (coupon.id)}
									<CouponCard {coupon} {currencies} />
								{/each}
							</div>
						</section>
					{/if}
				</div>

				<!-- Right column — details / apply / questions / share -->
				<div>
					<MentorSidebar
						{service}
						{currencies}
						canApply={mentorCanRequest}
						onApply={() => {
							mentorTierGoals = '';
							mentorDialogOpen = true;
						}}
						onMessage={() => (messageOpen = true)}
					/>
					<div class="mt-6">
						<p class="text-sm font-semibold text-[var(--color-text)]">
							{t('services.share_mentor', 'Share this Mentor')}
						</p>
						<div class="mt-2.5 flex items-center gap-2">
							<ShareButton contentId={service.id} url={canonical} title={service.title} />
						</div>
						{#if page.data.features?.['scam_alert.enabled']}
							<a
								href="/scam-alerts/new"
								class="mt-3 inline-flex items-center gap-1.5 text-sm font-medium text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
							>
								<TriangleAlert size={14} />
								{t('scam_alerts.report_button', 'Report a Scam')}
							</a>
						{/if}
					</div>
					{#if service.org_id}
						<div class="mt-6">
							<ProviderCard {service} />
						</div>
					{/if}
				</div>
			</div>

			<!-- Bottom 2-col: What Mentees Say | Frequently asked questions
			     (FAQs take the cell How it works vacated). -->
			<div class="mt-12 grid gap-12 lg:grid-cols-2">
				<div id="reviews" class="scroll-mt-20">
					{#if hasReviews}
						<ServiceReviews
							{reviews}
							aggregate={reviewAggregate}
							heading={reviewsHeading}
							viewAllHref="/services/{service.slug}/reviews"
						/>
					{:else}
						{@render reviewsCta()}
					{/if}
					{@render reviewWrite()}
				</div>
				{#if service.faqs && service.faqs.length > 0}
					<ServiceFaq faqs={service.faqs} />
				{/if}
			</div>
		{:else}
			<!-- ================================================
			     SERVICE / PACKAGE LAYOUT (mockups 01 + 03)
			     ================================================ -->
			<section class="grid gap-10 lg:grid-cols-2 lg:items-start">
				<!-- Left: pill / title / excerpt / rating / selling points / CTAs -->
				<div class="lg:pt-2">
					<div class="flex flex-wrap items-center gap-2">
						{#if catLabel}
							<span
								class="inline-flex items-center gap-1.5 rounded-full bg-[var(--color-primary)]/10 px-3 py-1 text-xs font-semibold text-[var(--color-primary)]"
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

					<h1
						class="mt-4 flex items-start gap-2 text-3xl font-bold tracking-tight text-[var(--color-text)] sm:text-4xl"
					>
						{service.title}
						{#if service.is_verified}
							<BadgeCheck size={26} class="mt-1.5 shrink-0 text-[var(--color-primary)]" />
						{/if}
					</h1>

					{#if heroExcerpt}
						<p class="mt-3 max-w-xl text-sm leading-relaxed text-[var(--color-text-subtle)]">
							{heroExcerpt}
						</p>
					{/if}

					<!-- Rating + supported count -->
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
							<span class="text-[var(--color-muted)]">
								{t('services.supported_stat', '{n} supported', { n: service.view_count })}
							</span>
						{/if}
					</div>

					{#if sellingPoints.length > 0}
						<div class="mt-6">
							<SellingPoints points={sellingPoints} />
						</div>
					{/if}

					<!-- Origin communities -->
					{#if service.origin_communities && (service.origin_communities as string[]).length > 0}
						<div class="mt-4 flex flex-wrap items-center gap-1.5 text-xs text-[var(--color-muted)]">
							<span>{t('services.serves', 'Serves')}:</span>
							{#each (service.origin_communities ?? []) as community (community)}
								<span
									class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-[var(--color-text-subtle)]"
									>{community}</span
								>
							{/each}
						</div>
					{/if}

					<!-- CTA row -->
					<div class="mt-7 flex flex-wrap items-center gap-3">
						{#if primaryCta}
							{#if primaryCta.dialog}
								<button
									type="button"
									onclick={openCtaDialog}
									class="inline-flex items-center gap-1.5 rounded-lg bg-[var(--color-primary)] px-6 py-2.5 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
								>
									<Send class="rtl:-scale-x-100" size={15} aria-hidden="true" />
									{primaryCta.label}
								</button>
							{:else}
								<a
									href={primaryCta.href}
									target={primaryCta.external ? '_blank' : undefined}
									rel={primaryCta.external ? 'noopener noreferrer' : undefined}
									class="inline-flex items-center gap-1.5 rounded-lg bg-[var(--color-primary)] px-6 py-2.5 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
								>
									{primaryCta.label}
									{#if primaryCta.external}
										<ExternalLink class="rtl:-scale-x-100" size={15} aria-hidden="true" />
									{/if}
								</a>
							{/if}
						{/if}
						{#if secondaryCta}
							{#if secondaryCta.dialog === 'booking'}
								<button
									type="button"
									onclick={openBookingDialog}
									class="inline-flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-5 py-2.5 text-sm font-semibold text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
								>
									<MessageSquare size={15} aria-hidden="true" />
									{secondaryCta.label}
								</button>
							{:else if secondaryCta.dialog === 'message'}
								<button
									type="button"
									onclick={() => (messageOpen = true)}
									class="inline-flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-5 py-2.5 text-sm font-semibold text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
								>
									<MessageSquare size={15} aria-hidden="true" />
									{secondaryCta.label}
								</button>
							{:else if secondaryCta.href}
								<a
									href={secondaryCta.href}
									class="inline-flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-5 py-2.5 text-sm font-semibold text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
								>
									<MessageSquare size={15} aria-hidden="true" />
									{secondaryCta.label}
								</a>
							{/if}
						{/if}
						{#if bookmarksEnabled && !isMentorship}
							<button
								type="button"
								onclick={toggleSave}
								disabled={bookmarkPending}
								class="inline-flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-5 py-2.5 text-sm font-semibold text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
							>
								<Heart
									size={15}
									class={bookmarked ? 'fill-[var(--color-danger)] text-[var(--color-danger)]' : ''}
									aria-hidden="true"
								/>
								{bookmarked ? t('services.saved', 'Saved') : t('services.save', 'Save')}
							</button>
						{/if}
					</div>
				</div>

				<!-- Right: hero image + facts/highlights column -->
				<div>
					<div class="relative">
						<div
							class="relative w-full overflow-hidden rounded-2xl border border-[var(--color-border)] aspect-[4/3]"
						>
							{#if service.featured_image_url}
								<img
									src={service.featured_image_url}
									alt={service.title}
									class="h-full w-full object-cover"
								/>
							{:else}
								<div class="grid h-full w-full place-items-center {accent.chip} border-0">
									<CategoryIcon size={96} strokeWidth={1} class={accent.icon} />
								</div>
							{/if}
							{#if service.has_coupon}
								<span
									class="absolute start-3 top-3 inline-flex items-center gap-1 rounded-full bg-[var(--color-surface)]/95 px-2.5 py-1 text-xs font-semibold shadow-sm {accent.icon}"
								>
									{t('services.coupon_badge', 'Deal')}
								</span>
							{/if}
						</div>
						{#if heroFacts.length > 0}
							<!-- Facts card under the image -->
							<div class="mt-4">
								<FactsCard facts={heroFacts} variant="rows" />
							</div>
						{/if}
					</div>
				</div>
			</section>

			<div class="mt-14 space-y-14">
				<!-- Body -->
				{#if service.body}
					<section id="details" aria-labelledby="details-heading" class="scroll-mt-20">
						<h2 id="details-heading" class="text-xl font-bold text-[var(--color-text)]">
							{t('services.about_heading', 'About this service')}
						</h2>
						<div class="prose prose-sm mt-4 max-w-none text-[var(--color-text-subtle)]">
							{@html sanitizeContentHtml(service.body_html || service.body)}
						</div>
					</section>
				{/if}

				<!-- What's Included — included[] icon cards (package items fallback) -->
				<IconCardGrid
					heading={includedHeading}
					included={service.included ?? []}
					items={service.items ?? []}
				/>

				<!-- Highlights as its own section when authored -->
				<ChecklistSection
					heading={service.highlights?.length
						? t('services.highlights_heading_svc', 'Service Highlights')
						: ''}
					lines={service.highlights ?? []}
					columns={2}
				/>
				<ChecklistSection
					heading={service.audience?.length
						? t('services.audience_heading_svc', 'Who This Is For')
						: ''}
					lines={service.audience ?? []}
				/>

				<!-- How it works -->
				{#if service.steps && service.steps.length > 0}
					<HowItWorks steps={service.steps} />
				{/if}

				<!-- Pricing: tier cards (§114) or the single price card -->
				{#if tiersVisible}
					<div>
						<TierGrid
							tiers={service.tiers ?? []}
							{currencies}
							heading={tiersHeading}
							perLabel={pricePerLabel}
							compareHref="/compare"
							ctaEnabled={tierCtaEnabled}
							onSelect={selectTier}
						/>
						<p
							class="mt-5 flex flex-wrap items-center justify-center gap-x-4 gap-y-1 text-center text-xs text-[var(--color-muted)]"
						>
							<span class="inline-flex items-center gap-1.5">
								<ShieldCheck size={13} aria-hidden="true" />
								{t('services.secure_payments', '100% Secure Payments')}
							</span>
							<span class="inline-flex items-center gap-1.5">
								<BadgeCheck size={13} aria-hidden="true" />
								{t('services.money_back', "Money-back guarantee if we can't deliver")}
							</span>
						</p>
					</div>
				{:else if heroPrice || service.price_range}
					<section
						aria-labelledby="pricing-heading"
						class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5"
					>
						<h2 id="pricing-heading" class="text-xl font-bold text-[var(--color-text)]">
							{tiersHeading}
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
							{#if bookSurface}
								<button
									type="button"
									onclick={openBookingDialog}
									class="ms-auto inline-flex items-center gap-1.5 rounded-lg bg-[var(--color-primary)] px-5 py-2.5 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
								>
									<Send class="rtl:-scale-x-100" size={15} aria-hidden="true" />
									{service.purchase_mode === 'paid'
										? t('services.book_now', 'Book now')
										: t('services.book_inquiry', 'Send inquiry')}
								</button>
							{/if}
						</div>
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
					</section>
				{/if}

				<!-- Coupons (§114) -->
				{#if couponsVisible}
					<section aria-labelledby="coupons-heading">
						<h2 id="coupons-heading" class="text-xl font-bold text-[var(--color-text)]">
							{t('services.coupons_heading', 'Offers & coupons')}
						</h2>
						<div class="mt-4 grid gap-3 sm:grid-cols-2">
							{#each (service.coupons ?? []) as coupon (coupon.id)}
								<CouponCard {coupon} {currencies} />
							{/each}
						</div>
					</section>
				{/if}

				<!-- Outbound links — booking_url + website_url render for any
				     listing that sets them -->
				{#if service.website_url || service.booking_url}
					<div class="flex flex-wrap gap-3">
						{#if service.website_url}
							<a
								href={service.website_url}
								target="_blank"
								rel="noopener noreferrer"
								class="inline-flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
							>
								<Globe size={16} aria-hidden="true" /> {t('services.visit_website', 'Visit website')}
							</a>
						{/if}
						{#if service.booking_url}
							<a
								href={service.booking_url}
								target="_blank"
								rel="noopener noreferrer"
								class="inline-flex items-center gap-1.5 rounded-lg bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
							>
								<ExternalLink class="rtl:-scale-x-100" size={16} aria-hidden="true" />
								{t('services.book_now', 'Book now')}
							</a>
						{/if}
					</div>
				{/if}

				<!-- Provider card -->
				{#if service.org_id}
					<ProviderCard {service} />
				{/if}

				<!-- Reviews | FAQs side by side (the mockup's bottom split row);
				     the reviews cell always renders — a zero-review listing gets
				     the first-review CTA so the write path stays discoverable. -->
				<div class="grid gap-12 lg:grid-cols-2">
					<div id="reviews" class="scroll-mt-20">
						{#if hasReviews}
							<ServiceReviews
								{reviews}
								aggregate={reviewAggregate}
								heading={reviewsHeading}
								viewAllHref="/services/{service.slug}/reviews"
							/>
						{:else}
							{@render reviewsCta()}
						{/if}
						{@render reviewWrite()}
					</div>
					{#if service.faqs && service.faqs.length > 0}
						<ServiceFaq faqs={service.faqs} />
					{/if}
				</div>
			</div>
		{/if}

		<!-- CTA banner — all three mockups end with the gradient block -->
		<div class="mt-14">
			<CtaBanner
				eyebrow={bannerCopy.eyebrow}
				heading={bannerCopy.heading}
				body={bannerCopy.body}
				primaryLabel={isMentorship
					? t('services.apply_mentorship', 'Apply for Mentorship')
					: (primaryCta?.label ?? t('services.get_started', 'Get Started Now'))}
				onPrimary={primaryCta?.dialog || isMentorship ? openCtaDialog : undefined}
				primaryHref={primaryCta && !primaryCta.dialog ? primaryCta.href : ''}
				secondaryLabel={isMentorship
					? t('services.mentor_contact', 'Message Mentor')
					: t('services.talk_to_expert', 'Chat with an Expert')}
				onSecondary={isMentorship ? () => (messageOpen = true) : secondaryCta?.dialog === 'message' ? () => (messageOpen = true) : secondaryCta?.dialog === 'booking' ? openBookingDialog : undefined}
				secondaryHref={isMentorship || secondaryCta?.dialog ? '' : (secondaryCta?.href ?? '')}
			/>
		</div>

		<!-- Owner actions: edit + claim + appeal -->
		{#if data.authed}
			<div class="mt-10 flex flex-wrap gap-3 border-t border-[var(--color-border)] pt-6">
				{#if canEdit}
					{#if !isMentorship}
						<!-- 000824: mentorship listings are managed via the mentor
						     profile (/settings/mentorship) — the generic edit route
						     rejects them (mentorship_listing_managed). -->
						<a
							href="/services/{service.slug}/edit"
							class="inline-flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							{t('services.edit', 'Edit listing')}
						</a>
					{/if}
				{/if}

				{#if !canEdit && service.claim_status === 'unclaimed' && !service.org_id && !isMentorship}
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

				{#if canAppealClaim}
					<div class="flex flex-wrap items-center gap-3">
						<p class="text-sm text-[var(--color-text-subtle)]">
							{t(
								'services.claim_rejected_hint',
								"Your organization's claim was rejected. You can appeal the decision."
							)}
						</p>
						<AppealButton
							entityType="service_claim"
							entityId={service.id}
							label={t('appeals.appeal_decision', 'Appeal rejection')}
						/>
					</div>
				{/if}
			</div>
		{/if}

		<!-- Related services — same content_type items ranked by the related
		     endpoint's embedding similarity. -->
		{#if (data.recommendedContent ?? []).length > 0}
			<section aria-labelledby="related-heading" class="mt-12">
				<h2 id="related-heading" class="text-xl font-bold text-[var(--color-text)]">
					{t('services.related_heading', 'You might also like')}
				</h2>
				<div class="mt-5 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
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
									<ArrowRight class="rtl:-scale-x-100" size={12} aria-hidden="true" />
								</span>
							</div>
						</a>
					{/each}
				</div>
			</section>
		{/if}
	</div>
{/if}

{#if service && isMentorship}
	<MentorshipRequestDialog
		bind:open={mentorDialogOpen}
		{service}
		authed={!!data.authed}
		initialGoals={mentorTierGoals}
	/>
{:else if service}
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

{#if service}
	<MessageDialog bind:open={messageOpen} {service} authed={!!data.authed} />
{/if}

{#if stickyCta && service}
	<StickyBookBar
		price={heroPrice}
		{ratingAvg}
		{ratingCount}
		ctaLabel={stickyCta.label}
		ctaHref={stickyCta.href}
		external={stickyCta.external}
		onCta={stickyCta.dialog ? openCtaDialog : undefined}
	/>
{/if}
