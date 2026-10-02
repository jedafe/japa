<script lang="ts">
	// Ref: japaconnect-engineering/sections/98-service-directory.md
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import UserHoverCard from '$lib/components/ui/UserHoverCard.svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api/client';
	import {
		ArrowLeft,
		Briefcase,
		MapPin,
		Globe,
		ExternalLink,
		BadgeCheck,
		Wifi,
		Building2,
		HandCoins,
		Loader2,
		CheckCircle2,
		AlertCircle,
		Star,
		Package,
		Check,
		Minus,
		CalendarDays,
		Users,
		ShieldAlert,
		Send
	} from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import RecommendedContentCard from '$lib/components/content/RecommendedContentCard.svelte';
	import { serviceCategoryLabel } from '$lib/enumOptions';
	import { staticAutocompleteFetcher } from '$lib/autocomplete';
	import { formatMoney } from '$lib/utils/currency';
	import type {
		ServiceListing,
		ServiceCategory,
		ServiceReview,
		ServiceBookingResult
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

	// Reviews + aggregate from the §29 sidecar (BUG-014).
	const reviews = $derived<ServiceReview[]>(data.reviews ?? []);
	const reviewAggregate = $derived(data.reviewAggregate ?? { count: 0, avg: 0 });
	const hasReviews = $derived(reviews.length > 0 || reviewAggregate.count > 0);

	function formatRating(avg: number): string {
		return avg.toFixed(1);
	}

	function reviewAuthorName(r: ServiceReview): string {
		return r.author_display_name ?? r.author_username ?? t('services.anonymous_reviewer', 'Anonymous');
	}

	function formatDate(iso: string): string {
		try {
			return new Date(iso).toLocaleDateString();
		} catch {
			return iso;
		}
	}

	const title = $derived(
		service
			? `${service.title} — ${t('services.heading', 'Service Directory')}`
			: t('services.heading', 'Service Directory')
	);
	const description = $derived(service?.body ?? service?.title ?? '');

	function categoryLabel(value: string): string {
		return serviceCategoryLabel(t, value as ServiceCategory);
	}

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
	const packagePrice = $derived(
		service?.price_amount != null
			? formatMoney(service.price_amount, service.price_currency, currencies, { minor: true })
			: ''
	);
	const pricePerLabel = $derived(
		service?.price_per === 'person'
			? t('services.pkg_per_person', 'per person')
			: service?.price_per === 'group'
				? t('services.pkg_per_group', 'per group')
				: t('services.pkg_per_package', 'per package')
	);
	const includedItems = $derived((service?.items ?? []).filter((i) => i.included));
	const optionalItems = $derived((service?.items ?? []).filter((i) => !i.included));
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
	// §111/Rule 9 — paid bookings pass through IdempotencyMiddleware and
	// require the header. One key per form-open so a retry replays the
	// cached response instead of double-booking; regenerated after success.
	let bookingIdemKey = $state('');

	const inputCls =
		'rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';

	const departureItems = $derived<AutocompleteItem[]>(
		openDepartures.map((d) => ({
			id: d.id,
			label: formatDate(d.starts_on) + (d.ends_on ? ` → ${formatDate(d.ends_on)}` : ''),
			sublabel:
				d.capacity != null
					? t('services.pkg_seats_left', '{n} seats left').replace('{n}', String(Math.max(0, d.capacity - d.booked_count)))
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

		const body: Record<string, unknown> = {
			intent: isPaid ? 'booking' : 'inquiry',
			party_size: size,
			note: bookingNote.trim()
		};
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

	// JSON-LD LocalBusiness for SEO.
	const jsonLd = $derived(
		service
			? JSON.stringify({
					'@context': 'https://schema.org',
					'@type': 'LocalBusiness',
					name: service.title,
					description: service.body ?? '',
					url: page.url.href,
					...(service.website_url ? { sameAs: service.website_url } : {}),
					...(service.price_range ? { priceRange: service.price_range } : {}),
					...(service.destination_space_name
						? { areaServed: service.destination_space_name }
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
	{#if jsonLd}
		{@html safeJsonLdScript(jsonLd)}
	{/if}
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	{#if data.loadError || !service}
		<div class="p-4">
			<EmptyState
				icon={Briefcase}
				title={t('services.load_error', 'Could not load service')}
				body={t('services.load_error_body', 'Please try again later.')}
				action={{ label: t('services.back_to_dir', 'Back to services'), href: '/services' }}
			/>
		</div>
	{:else}
		<div class="p-4">
			<a
				href="/services"
				class="inline-flex items-center gap-1 text-sm text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
			>
				<ArrowLeft size={16} /> {t('services.back_to_dir', 'Back to services')}
			</a>
		</div>

		<article class="px-4 pb-8">
			<!-- Title + verified badge -->
			<div class="flex items-start gap-2">
				<h1 class="text-2xl font-bold text-[var(--color-text)]">{service.title}</h1>
				{#if service.is_verified}
					<BadgeCheck size={22} class="mt-1 shrink-0 text-[var(--color-primary)]" />
				{/if}
			</div>

			<!-- Category + verification type -->
			<div class="mt-3 flex flex-wrap items-center gap-2 text-xs">
				<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-[var(--color-text-subtle)]">
					{service.service_category ? categoryLabel(service.service_category as ServiceCategory) : categoryLabel('other')}
				</span>
				{#if service.is_verified && service.verification_type}
					<span class="flex items-center gap-1 text-[var(--color-primary)]">
						<BadgeCheck size={12} />
						{t('services.verified', 'Verified')} · {service.verification_type}
					</span>
				{/if}
				{#if service.claim_status && service.claim_status !== 'unclaimed'}
					<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-[var(--color-text-subtle)]">
						{claimStatusLabel(service.claim_status)}
					</span>
				{/if}
			</div>

			<!-- Org + location + remote meta -->
			<div class="mt-4 flex flex-wrap gap-3 text-sm text-[var(--color-muted)]">
				{#if service.org_name}
					<a
						href="/org/{service.org_slug}"
						class="flex items-center gap-1 text-[var(--color-text-subtle)] hover:text-[var(--color-text)]"
					>
						<Building2 size={14} /> {service.org_name}
					</a>
				{/if}
				{#if service.destination_space_name}
					<span class="flex items-center gap-1"><MapPin size={14} /> {service.destination_space_name}</span>
				{/if}
				{#if service.city_space_name}
					<span>{service.city_space_name}</span>
				{/if}
				{#if service.is_remote}
					<span class="flex items-center gap-1"><Wifi size={14} /> {t('services.remote', 'Remote')}</span>
				{/if}
			</div>

			<!-- Origin communities -->
			{#if service.origin_communities && (service.origin_communities as string[]).length > 0}
				<div class="mt-3 flex flex-wrap items-center gap-1.5 text-xs text-[var(--color-muted)]">
					<span>{t('services.serves', 'Serves')}:</span>
					{#each (service.origin_communities as string[]) as community (community)}
						<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-[var(--color-text-subtle)]">{community}</span>
					{/each}
				</div>
			{/if}

			<!-- §111 package hero: price + price_per + availability -->
			{#if isPackage}
				<div class="mt-5 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<div class="flex flex-wrap items-baseline gap-2">
						{#if packagePrice}
							<span class="text-2xl font-bold text-[var(--color-text)]">{packagePrice}</span>
							<span class="text-sm text-[var(--color-muted)]">{pricePerLabel}</span>
						{:else}
							<span class="text-sm text-[var(--color-muted)]">{t('services.pkg_price_on_request', 'Price on request')}</span>
						{/if}
						{#if service.capacity != null}
							<span class="ml-auto flex items-center gap-1 text-xs text-[var(--color-muted)]">
								<Users size={14} /> {t('services.pkg_capacity_label', 'Up to {n} people').replace('{n}', String(service.capacity))}
							</span>
						{/if}
					</div>
					{#if service.available_from || service.available_until}
						<p class="mt-2 flex items-center gap-1.5 text-xs text-[var(--color-muted)]">
							<CalendarDays size={13} />
							{service.available_from ? formatDate(service.available_from) : '…'} — {service.available_until ? formatDate(service.available_until) : t('services.pkg_ongoing', 'ongoing')}
						</p>
					{/if}
				</div>

				<!-- Included / optional items checklist -->
				{#if (service.items ?? []).length > 0}
					<div class="mt-6">
						<h2 class="text-sm font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]">
							{t('services.pkg_items_heading', "What's included")}
						</h2>
						<ul class="mt-3 space-y-2">
							{#each includedItems as item (item.id)}
								<li class="flex items-start gap-2 text-sm text-[var(--color-text)]">
									<Check size={16} class="mt-0.5 shrink-0 text-[var(--color-success)]" />
									<span>
										<span class="font-medium">{item.title}</span>
										{#if item.optional}
											<span class="ml-1.5 rounded-full bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10px] text-[var(--color-muted)]">{t('services.pkg_item_optional', 'Optional')}</span>
										{/if}
										{#if item.description}
											<span class="block text-xs text-[var(--color-muted)]">{item.description}</span>
										{/if}
									</span>
								</li>
							{/each}
							{#each optionalItems as item (item.id)}
								<li class="flex items-start gap-2 text-sm text-[var(--color-text-subtle)]">
									<Minus size={16} class="mt-0.5 shrink-0 text-[var(--color-muted)]" />
									<span>
										<span>{item.title}</span>
										<span class="ml-1.5 rounded-full bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10px] text-[var(--color-muted)]">
											{t('services.pkg_item_not_included', 'Not included')}
										</span>
										{#if item.description}
											<span class="block text-xs text-[var(--color-muted)]">{item.description}</span>
										{/if}
									</span>
								</li>
							{/each}
						</ul>
					</div>
				{/if}

				<!-- Open departures -->
				{#if openDepartures.length > 0}
					<div class="mt-6">
						<h2 class="text-sm font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]">
							{t('services.pkg_departures_heading', 'Upcoming departures')}
						</h2>
						<ul class="mt-3 space-y-2">
							{#each openDepartures as dep (dep.id)}
								<li class="flex items-center justify-between gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm">
									<span class="flex items-center gap-1.5 text-[var(--color-text)]">
										<CalendarDays size={14} class="text-[var(--color-muted)]" />
										{formatDate(dep.starts_on)}{#if dep.ends_on} → {formatDate(dep.ends_on)}{/if}
									</span>
									<span class="text-xs text-[var(--color-muted)]">
										{#if dep.capacity != null}
											{t('services.pkg_seats_left', '{n} seats left').replace('{n}', String(Math.max(0, dep.capacity - dep.booked_count)))}
										{:else}
											{t('services.pkg_unlimited', 'Unlimited seats')}
										{/if}
									</span>
								</li>
							{/each}
						</ul>
					</div>
				{/if}
			{/if}

			<!-- Body -->
			{#if service.body}
				<div class="mt-6 prose prose-sm max-w-none text-[var(--color-text)]">
					{@html sanitizeContentHtml(service.body_html || service.body)}
				</div>
			{/if}

			<!-- Reviews (§29 sidecar — BUG-014) -->
			{#if hasReviews}
				<div class="mt-8 border-t border-[var(--color-border)] pt-6">
					<h2 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
						<Star size={18} class="text-[var(--color-primary)]" />
						{t('services.reviews_heading', 'Reviews')}
						{#if reviewAggregate.count > 0}
							<span class="text-sm font-normal text-[var(--color-muted)]">
								{formatRating(reviewAggregate.avg)} · {reviewAggregate.count} {t('services.reviews_count', reviewAggregate.count === 1 ? 'review' : 'reviews')}
							</span>
						{/if}
					</h2>

					{#if reviews.length > 0}
						<ul class="mt-4 space-y-4">
							{#each reviews as review (review.id)}
								<li class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
									<div class="flex items-center justify-between gap-2">
										<div class="flex items-center gap-2">
											{#if review.author_username}
												<UserHoverCard
													id={review.author_id}
													slug={review.author_username}
													name={review.author_display_name ?? review.author_username}
													avatar={review.author_avatar_url}
													variant="row"
													class="inline-flex max-w-full items-center text-left text-sm font-medium text-[var(--color-text)] hover:underline"
												>
													{#snippet trigger()}
														<span>{reviewAuthorName(review)}</span>
													{/snippet}
												</UserHoverCard>
											{:else}
												<span class="text-sm font-medium text-[var(--color-text)]">{reviewAuthorName(review)}</span>
											{/if}
											<span class="flex items-center gap-0.5 text-[var(--color-primary)]">
												{#each Array(5) as _, i}
													<Star
														size={12}
														fill={i < review.rating ? 'currentColor' : 'none'}
														class={i < review.rating ? '' : 'text-[var(--color-muted)]'}
													/>
												{/each}
											</span>
										</div>
										<span class="text-xs text-[var(--color-muted)]">{formatDate(review.created_at)}</span>
									</div>
									{#if review.title}
										<p class="mt-2 text-sm font-medium text-[var(--color-text)]">{review.title}</p>
									{/if}
									{#if review.body}
										<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{review.body}</p>
									{/if}
								</li>
							{/each}
						</ul>
					{:else}
						<p class="mt-4 text-sm text-[var(--color-muted)]">
							{t('services.no_reviews_yet', 'No reviews yet. Be the first to share your experience.')}
						</p>
					{/if}
				</div>
			{/if}

			<!-- Action links -->
			<div class="mt-6 flex flex-wrap gap-3">
				{#if service.website_url}
					<a
						href={service.website_url}
						target="_blank"
						rel="noopener noreferrer"
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<Globe size={16} /> {t('services.visit_website', 'Visit website')}
					</a>
				{/if}
				{#if service.booking_url}
					<a
						href={service.booking_url}
						target="_blank"
						rel="noopener noreferrer"
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
					>
						<ExternalLink size={16} /> {isPackage && service.purchase_mode === 'external' ? t('services.book_partner', 'Book on partner site') : t('services.book_now', 'Book now')}
					</a>
				{/if}
				{#if service.price_range}
					<span class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)]">
						<HandCoins size={16} /> {service.price_range}
					</span>
				{/if}
			</div>

			<!-- §111 booking CTA — inquiry (leads) and paid (fee checkout) land
				in service_bookings; external already linked above. -->
			{#if service.purchase_mode !== 'external' && service.purchase_mode !== ''}
				{#if bookingResult?.ok}
					<div class="mt-6 flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-success)]/30 bg-[var(--color-success)]/10 px-4 py-3 text-sm text-[var(--color-success)]" role="status">
						<CheckCircle2 size={18} />
						{service.purchase_mode === 'paid'
							? t('services.book_reserved', 'Booking created — complete payment to confirm.')
							: t('services.book_inquiry_sent', 'Inquiry sent — the provider will respond through your bookings inbox.')}
					</div>
				{:else if canBook}
					<div class="mt-6 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						{#if !bookingOpen}
							<button
								type="button"
								onclick={openBookingForm}
								class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
							>
								<Send size={16} />
								{#if service.purchase_mode === 'paid'}
									{service.platform_fee != null
										? t('services.book_reserve_fee', 'Reserve — {fee} platform fee').replace('{fee}', formatMoney(service.platform_fee, service.price_currency, currencies, { minor: true }))
										: t('services.book_reserve', 'Reserve')}
								{:else}
									{t('services.book_inquiry', 'Send inquiry')}
								{/if}
							</button>
							{#if service.purchase_mode === 'paid' && service.agent_balance_due != null}
								<p class="mt-2 text-xs text-[var(--color-muted)]">
									{t('services.book_balance_note', 'This fee secures your booking. Remaining {balance} is payable directly to {org}.')
										.replace('{balance}', formatMoney(service.agent_balance_due, service.price_currency, currencies, { minor: true }))
										.replace('{org}', service.org_name ?? '')}
								</p>
							{/if}
						{:else}
							<h3 class="text-sm font-semibold text-[var(--color-text)]">
								{service.purchase_mode === 'paid' ? t('services.book_reserve', 'Reserve') : t('services.book_inquiry', 'Send inquiry')}
							</h3>
							<div class="mt-3 space-y-3">
								{#if openDepartures.length > 0}
									<div>
										<label for="bk-dep" class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]">
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
											<label for="bk-from" class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]">{t('services.book_pref_start', 'Preferred start')}</label>
											<input id="bk-from" type="date" bind:value={prefStartOn} class="{inputCls} w-full" />
										</div>
										<div>
											<label for="bk-to" class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]">{t('services.book_pref_end', 'Preferred end')}</label>
											<input id="bk-to" type="date" bind:value={prefEndOn} class="{inputCls} w-full" />
										</div>
									</div>
									<label class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
										<input type="checkbox" checked={prefFlexible} onchange={(e) => (prefFlexible = e.currentTarget.checked)} class="h-3.5 w-3.5 rounded border-[var(--color-border)]" />
										{t('services.book_flexible', 'My dates are flexible')}
									</label>
								{/if}
								<div>
									<label for="bk-party" class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]">{t('services.book_party_size', 'Party size')}</label>
									<input id="bk-party" type="number" min="1" bind:value={partySize} class="{inputCls} w-24" />
								</div>
								<div>
									<label for="bk-note" class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]">{t('services.book_note', 'Message to the provider')}</label>
									<textarea id="bk-note" bind:value={bookingNote} rows={3} placeholder={t('services.book_note_ph', 'Questions, requirements, context…')} class="{inputCls} w-full"></textarea>
								</div>
								{#if service.purchase_mode === 'paid'}
									<div class="grid gap-3 sm:grid-cols-2">
										<div>
											<label for="bk-prov" class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]">{t('services.book_provider', 'Payment method')}</label>
											<Autocomplete
												id="bk-prov"
												placeholder={t('services.book_provider_ph', 'Choose a payment method')}
												emptyMessage={t('common.no_results', 'No results found')}
												fetcher={providerFetcher}
												initialFetcher={providerFetcher}
												selectedId={bookingProvider || null}
												selectedLabel={providerItems.find((o) => o.id === bookingProvider)?.label ?? null}
												onSelect={(item) => (bookingProvider = item.id)}
												onClear={() => (bookingProvider = '')}
											/>
										</div>
										<div>
											<label for="bk-email" class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]">{t('services.book_email', 'Email for receipt')}</label>
											<input id="bk-email" type="email" bind:value={bookingEmail} placeholder="you@example.com" class="{inputCls} w-full" />
										</div>
									</div>
								{/if}
								{#if bookingResult?.error}
									<div class="flex items-center gap-2 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
										<AlertCircle size={14} /> {bookingResult.error}
									</div>
								{/if}
								<div class="flex gap-2">
									<button
										type="button"
										onclick={submitBooking}
										disabled={bookingBusy}
										class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
									>
										{#if bookingBusy}<Loader2 size={16} class="animate-spin" />{:else}<Send size={16} />{/if}
										{service.purchase_mode === 'paid' ? t('services.book_pay_reserve', 'Pay & reserve') : t('services.book_send', 'Send inquiry')}
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
					<p class="mt-6 text-sm text-[var(--color-muted)]">
						<a href="/login?redirect={encodeURIComponent(page.url.pathname)}" class="text-[var(--color-primary)] hover:underline">
							{service.purchase_mode === 'paid' ? t('services.book_login_reserve', 'Log in to reserve') : t('services.book_login_inquiry', 'Log in to send an inquiry')}
						</a>
					</p>
				{/if}
			{/if}

			<!-- §111 scam disclaimer — packages render a standing notice +
				a link to the §81 board filtered to this destination. -->
			{#if isPackage}
				<div class="mt-6 flex items-start gap-2 rounded-[var(--radius)] border border-[var(--color-warning)]/30 bg-[var(--color-warning)]/10 px-4 py-3 text-xs text-[var(--color-text-subtle)]">
					<ShieldAlert size={16} class="mt-0.5 shrink-0 text-[var(--color-warning)]" />
					<span>
						{t('services.pkg_scam_disclaimer', 'Always verify providers before paying. JapaConnect never guarantees outcomes — report suspicious packages.')}
						<a href="/scam-alerts?dest={service.destination_space_id}" class="ml-1 text-[var(--color-primary)] hover:underline">
							{t('services.pkg_scam_link', 'See scam alerts for {dest}').replace('{dest}', service.destination_space_name ?? '')}
						</a>
					</span>
				</div>
			{/if}

			<!-- Owner actions: edit + claim -->
			{#if data.authed}
				<div class="mt-8 flex flex-wrap gap-3 border-t border-[var(--color-border)] pt-4">
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
								<CheckCircle2 size={16} />
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
										<Loader2 size={16} class="animate-spin" />
									{:else}
										<BadgeCheck size={16} />
									{/if}
									{t('services.claim_listing', 'Claim this listing')}
								</button>
								{#if claimResult?.error}
									<span class="flex items-center gap-1 text-sm text-[var(--color-danger)]">
										<AlertCircle size={14} /> {claimResult.error}
									</span>
								{/if}
							</div>
						{:else}
							<p class="text-xs text-[var(--color-muted)]">
								{t('services.claim_need_org', 'You must be a member of an organization to claim this listing.')}
							</p>
						{/if}
					{/if}
				</div>
			{/if}
		</article>

		<!-- Same-type recommendations, ranked by embedding similarity. -->
		<div class="px-4 pb-8">
			<RecommendedContentCard items={data.recommendedContent ?? []} contentType="service_listing" />
		</div>
	{/if}
</div>
