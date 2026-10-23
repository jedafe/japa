<script lang="ts">
	// Service reviews index — /services/[slug]/reviews. Full paginated list
	// backed by GET /services/{id}/reviews (§114); the detail page links
	// here via the "View all reviews" footer when its embedded slice is
	// shorter than the aggregate count.
	// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§3.3)
	import { page } from '$app/state';
	import { goto, invalidateAll } from '$app/navigation';
	import { ChevronRight, Star } from 'lucide-svelte';
	import { t, siteName} from '$lib/i18n';
	import { EmptyState, PaginationControls } from '$lib/components/ui';
	import ReviewListItem from '$lib/components/marketplace/ReviewListItem.svelte';
	import ReviewForm from '$lib/components/reviews/ReviewForm.svelte';
	import { excerpt } from '$lib/utils/seo-score';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const service = $derived(data.service);
	const reviews = $derived(data.reviews ?? []);
	const aggregate = $derived(data.reviewAggregate ?? { count: 0, avg: 0 });
	const currentPage = $derived(data.currentPage || 1);
	const totalPages = $derived(
		data.meta?.total ? Math.ceil(data.meta.total / (data.meta.limit ?? 10)) : 1
	);

	const title = $derived(
		`${t('services.reviews_heading', 'Reviews')} — ${service.title} — ${siteName()}`
	);
	const description = $derived(
		excerpt(service.body_html || service.body || '', 160) ||
			t('services.reviews_page_description', 'Traveller reviews for {title}.', {
				title: service.title
			})
	);
	// Self-canonical on the clean /reviews path — ?page= hits consolidate.
	const canonical = $derived(`${page.url.origin}/services/${service.slug}/reviews`);

	function gotoPage(p: number) {
		const params = new URLSearchParams(page.url.searchParams);
		params.set('page', String(p));
		goto(`/services/${service.slug}/reviews?${params.toString()}`);
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<link rel="canonical" href={canonical} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="article" />
	<meta property="og:url" content={canonical} />
</svelte:head>

<div class="mx-auto w-full max-w-full px-4 pt-4 pb-10">
	{#if data.loadError}
		<EmptyState
			icon={Star}
			title={t('services.reviews_load_error', 'Could not load reviews')}
			body={t('services.load_error_body', 'Please try again later.')}
			action={{
				label: t('services.reviews_back', 'Back to listing'),
				href: `/services/${service.slug}`
			}}
		/>
	{:else}
		<!-- Breadcrumb: Home / Services / Title / Reviews -->
		<nav
			class="mb-5 flex flex-wrap items-center gap-1 text-xs text-[var(--color-muted)]"
			aria-label={t('a11y.breadcrumb', 'Breadcrumb')}
		>
			<a href="/" class="transition-colors hover:text-[var(--color-text)]"
				>{t('nav.home', 'Home')}</a
			>
			<ChevronRight class="rtl:-scale-x-100" size={12} aria-hidden="true" />
			<a href="/services" class="transition-colors hover:text-[var(--color-text)]"
				>{t('marketplace.nav_services', 'Services')}</a
			>
			<ChevronRight class="rtl:-scale-x-100" size={12} aria-hidden="true" />
			<a
				href="/services/{service.slug}"
				class="truncate transition-colors hover:text-[var(--color-text)]">{service.title}</a
			>
			<ChevronRight class="rtl:-scale-x-100" size={12} aria-hidden="true" />
			<span class="font-medium text-[var(--color-text)]" aria-current="page"
				>{t('services.reviews_heading', 'Reviews')}</span
			>
		</nav>

		<header>
			<h1 class="text-2xl font-bold text-[var(--color-text)]">
				{t('services.reviews_page_heading', 'Reviews for {title}', { title: service.title })}
			</h1>
			{#if aggregate.count > 0}
				<div class="mt-4 flex items-center gap-3">
					<span class="text-3xl font-bold text-[var(--color-text)]"
						>{aggregate.avg.toFixed(1)}</span
					>
					<span>
						<span
							class="flex items-center gap-0.5"
							aria-label={t('services.rating_aria', '{avg} out of 5 stars', {
								avg: aggregate.avg.toFixed(1)
							})}
						>
							{#each Array(5) as _, i (i)}
								<Star
									size={15}
									class={i < Math.round(aggregate.avg)
										? 'fill-[var(--color-warning)] text-[var(--color-warning)]'
										: 'text-[var(--color-muted)]'}
									aria-hidden="true"
								/>
							{/each}
						</span>
						<span class="mt-0.5 block text-xs text-[var(--color-muted)]">
							{t('services.reviews_summary', '{count} reviews', { count: aggregate.count })}
						</span>
					</span>
				</div>
			{/if}
		</header>

		{#if reviews.length === 0}
			<p class="mt-6 text-sm text-[var(--color-muted)]">
				{t('services.no_reviews_yet', 'No reviews yet. Be the first to share your experience.')}
			</p>
		{:else}
			<ul class="mt-6 space-y-4">
				{#each reviews as review (review.id)}
					<ReviewListItem {review} />
				{/each}
			</ul>

			{#if totalPages > 1}
				<div class="mt-6 flex justify-center">
					<PaginationControls page={currentPage} {totalPages} onPageChange={gotoPage} />
				</div>
			{/if}
		{/if}

		<!-- §29 write path — the listing's reviews feed (detail carousel +
			this index) is only half the loop without a submission surface.
			Guests get a login link; the form redirects to login on submit too. -->
		<div class="mt-8">
			{#if page.data.user}
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
						href="/login?redirect={encodeURIComponent(page.url.pathname)}"
						class="font-medium text-[var(--color-primary)] hover:underline"
						>{t('services.reviews_login', 'Log in to write a review')}</a
					>
				</p>
			{/if}
		</div>
	{/if}
</div>
