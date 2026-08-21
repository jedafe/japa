<script lang="ts">
	// Ref: japaconnect-engineering/sections/98-service-directory.md
	import { t } from '$lib/i18n';
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
		Star
	} from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { serviceCategoryLabel } from '$lib/enumOptions';
	import type { ServiceListing, ServiceCategory, ServiceReview } from '$lib/types';
	import type { PageData } from './$types';
	import { sanitizeContentHtml, safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	const service = $derived<ServiceListing | null>(data.service);
	const canEdit = $derived(!!data.authed && !!service?.author_id && service.author_id === data.currentUserId);

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
						{service.claim_status}
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
											<span class="text-sm font-medium text-[var(--color-text)]">{reviewAuthorName(review)}</span>
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
						<ExternalLink size={16} /> {t('services.book_now', 'Book now')}
					</a>
				{/if}
				{#if service.price_range}
					<span class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)]">
						<HandCoins size={16} /> {service.price_range}
					</span>
				{/if}
			</div>

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
					{/if}

					{#if !canEdit && service.claim_status === 'unclaimed'}
						{#if claimResult?.ok}
							<div class="flex items-center gap-2 text-sm text-[var(--color-success)]">
								<CheckCircle2 size={16} />
								{t('services.claim_submitted', 'Claim submitted for review.')}
							</div>
						{:else if canClaim}
							<div class="flex flex-wrap items-center gap-2">
								<select
									value={claimOrgId}
									onchange={(e) => (claimOrgId = e.currentTarget.value)}
									class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
									aria-label={t('services.claim_select_org', 'Select organization')}
								>
									<option value="">{t('services.claim_select_org', 'Select an organization')}</option>
									{#each data.myOrgs as org (org.id)}
										<option value={org.id}>{org.name}</option>
									{/each}
								</select>
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
	{/if}
</div>
