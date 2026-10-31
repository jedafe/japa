<script lang="ts">
	// Track Detail — widened overview (ADR-060).
	// SSR hero + stats · Tabs (Overview | Curriculum | Reviews) · Accordion curriculum ·
	// instructor card · enroll CTA · Course JSON-LD.
	// Enrollment is CSR-only (ADR-060 SSR/CSR boundary).
	import { Tabs, Accordion } from 'bits-ui';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import LevelBadge from '$lib/components/academy/LevelBadge.svelte';
	import StarRating from '$lib/components/academy/StarRating.svelte';
	import CheckoutModal from '$lib/components/academy/CheckoutModal.svelte';
	import { LoadingSkeleton } from '$lib/components/ui';
	import { formatDuration, formatCount, formatPriceKobo, isFreeForUser } from '$lib/format';
	import { loginUrl, ACADEMY_ORIGIN } from '$lib/appOrigins';
	import { Star, Users, Clock, PlayCircle, CheckCircle, ChevronDown, Lock, Bookmark as BookmarkIcon, Loader2 } from 'lucide-svelte';
	import { enrollmentsStore } from '$lib/stores/enrollments.svelte';
	import type { PageData } from './$types';
	import type { Bookmark, Enrollment, TrackReview, ReviewAggregate } from '$lib/types';
	import { safeJsonLd } from '$lib/seo';

	let { data }: { data: PageData } = $props();
	const track = $derived(data.track);

	// CSR ONLY — enrollment state. Uses the shared enrollmentsStore so sidebar
	// widgets (Continue Learning, My Progress, My Learning) stay in sync after
	// enrolling. The store deduplicates fetches across components.
	let enrolling = $state(false);
	let enrollError = $state<string | null>(null);
	let checkoutOpen = $state(false);

	// Bookmark (save-for-later) toggle. There is no per-entity "is bookmarked"
	// lookup route, so authed users load their academy_track bookmarks once
	// (max page size) and match entity_id client-side.
	let bookmarkId = $state<string | null>(null);
	let bookmarkLoading = $state(false);
	let bookmarkSaving = $state(false);

	// Review submission state. Reviews are user-specific mutations (POST) on top
	// of the SSR reviews list; we keep local copies so the tab updates after submit
	// without a full page reload.
	let reviews = $state<TrackReview[]>([]);
	let reviewAggregate = $state<ReviewAggregate | null>(null);
	let reviewRating = $state(0);
	let reviewTitle = $state('');
	let reviewBody = $state('');
	let reviewSubmitting = $state(false);
	let reviewError = $state<string | null>(null);
	let reviewSuccess = $state(false);
	let reviewsLoadingMore = $state(false);
	let reviewsLoadMoreError = $state<string | null>(null);
	const REVIEW_PAGE_SIZE = 10;
	const hasMoreReviews = $derived((reviewAggregate?.count ?? 0) > reviews.length);

	// Sync local review state when navigating between tracks (data is a prop).
	$effect(() => {
		reviews = data.reviews;
		reviewAggregate = data.reviewAggregate;
		reviewRating = 0;
		reviewTitle = '';
		reviewBody = '';
		reviewError = null;
		reviewSuccess = false;
		reviewsLoadingMore = false;
		reviewsLoadMoreError = null;
	});

	// Whether this is a paid course that requires checkout.
	const isPaid = $derived(track.price_kobo != null && track.price_kobo > 0);
	const isFreeOrPro = $derived(isFreeForUser(track.price_kobo, data.user?.is_pro));
	// Subscription-eligible tracks can be enrolled via the user's subscription plan.
	const isSubscriptionEligible = $derived(!!track.subscription_eligible);

	$effect(() => {
		if (!data.user) {
			return;
		}
		// Trigger the shared fetch (deduplicated — if a sidebar widget already
		// fetched, this is a no-op).
		enrollmentsStore.fetch(!!data.user);
	});

	// Resolve whether this track is already bookmarked (authed users only).
	// Re-runs when navigating between tracks (track.id is derived from data).
	$effect(() => {
		const trackId = track.id;
		bookmarkId = null;
		if (!data.user) return;
		let stale = false;
		bookmarkLoading = true;
		api
			.get<Bookmark[]>('/me/bookmarks?entity_type=academy_track&page_size=100')
			.then((r) => {
				if (stale) return;
				bookmarkId = r.data?.find((b) => b.entity_id === trackId)?.id ?? null;
				bookmarkLoading = false;
			});
		return () => {
			stale = true;
		};
	});

	// Derive the enrollment for this track from the shared store.
	const enrollment = $derived(
		enrollmentsStore.academy_enrollments?.find((e) => e.track_id === track.id) ?? null
	);
	const loadingEnrollment = $derived(enrollmentsStore.loading && !enrollmentsStore.academy_enrollments);

	const isEnrolled = $derived(!!enrollment);
	// Platform admins bypass learner restrictions (backend grants the same
	// bypass — this only mirrors it for UI visibility).
	const isAdmin = $derived(
		(data.user?.permissions ?? []).some(
			(p) => p === '*' || p === 'academy.admin.manage' || p === 'academy.instructor.approve'
		)
	);
	const progressPct = $derived(enrollment?.progress_pct ?? 0);
	const isComplete = $derived(progressPct >= 100);

	// Continue target: resume at the enrollment's next lesson, falling back to
	// the first lesson in the curriculum. Null when the track has no lessons.
	const continueHref = $derived.by(() => {
		const next = enrollment?.next_lesson;
		if (next?.module_slug && next?.slug) {
			return `/tracks/${track.slug}/${next.module_slug}/${next.slug}`;
		}
		const firstModule = track.modules?.[0];
		const firstLesson = firstModule?.lessons?.[0];
		if (firstModule && firstLesson) {
			return `/tracks/${track.slug}/${firstModule.slug}/${firstLesson.slug}`;
		}
		return null;
	});

	// Price label — always shown on the unified Academy surface.
	const priceLabel = $derived.by(() => {
		if (isFreeForUser(track.price_kobo, data.user?.is_pro)) {
			return data.user?.is_pro ? t('academy.included_pro') : t('academy.free');
		}
		if (isSubscriptionEligible) {
			return t('academy.subscription_or_purchase', 'Subscription / {price}').replace('{price}', formatPriceKobo(track.price_kobo, track.currency, data.locale, data.currencies));
		}
		return formatPriceKobo(track.price_kobo, track.currency, data.locale, data.currencies);
	});

	async function enroll() {
		if (!data.user) {
			// Login lives on the Main app (shared jc_token cookie). Send the user
			// there with a redirect back to this track. Use window.location.href
			// (not goto) because the login URL is cross-origin.
			window.location.href = loginUrl(`/tracks/${track.slug}`);
			return;
		}
		// Paid course: open checkout modal (unless Pro subscriber or
		// subscription-eligible — the backend will check plan access first).
		if (isPaid && !isFreeOrPro && !isSubscriptionEligible) {
			checkoutOpen = true;
			return;
		}
		enrolling = true;
		enrollError = null;
		const r = await api.post<Enrollment>(`/academy/tracks/${track.id}/enroll`);
		enrolling = false;
		if (r.data) {
			// Refresh the shared store so sidebar widgets reflect the new
			// enrollment immediately (Continue Learning, My Progress, etc.).
			enrollmentsStore.refresh();
			// Navigate to the first lesson.
			const firstModule = track.modules?.[0];
			const firstLesson = firstModule?.lessons?.[0];
			if (firstModule && firstLesson) {
				goto(`/tracks/${track.slug}/${firstModule.slug}/${firstLesson.slug}`);
			}
		} else if (r.error) {
			// 402 payment_required — open checkout modal as fallback.
			// This happens when a subscription-eligible track isn't covered
			// by the user's plan and has a price > 0.
			if (r.error.status === 402) {
				checkoutOpen = true;
			} else {
				enrollError = r.error.message;
			}
		}
	}

	async function toggleBookmark() {
		if (!data.user) {
			window.location.href = loginUrl(`/tracks/${track.slug}`);
			return;
		}
		if (bookmarkSaving) return;
		bookmarkSaving = true;
		if (bookmarkId) {
			const r = await api.delete(`/me/bookmarks/${bookmarkId}`);
			if (!r.error) bookmarkId = null;
		} else {
			const r = await api.post<Bookmark>('/me/bookmarks', {
				entity_type: 'academy_track',
				entity_id: track.id
			});
			if (r.data) bookmarkId = r.data.id;
		}
		bookmarkSaving = false;
	}

	async function submitReview() {
		if (!data.user) {
			reviewError = t('academy.review_unauthenticated');
			return;
		}
		if (reviewRating < 1 || reviewRating > 5) {
			reviewError = t('academy.review_rating');
			return;
		}
		reviewSubmitting = true;
		reviewError = null;
		reviewSuccess = false;
		const r = await api.post<TrackReview>(`/academy/tracks/${track.id}/reviews`, {
			rating: reviewRating,
			title: reviewTitle.trim() || undefined,
			body: reviewBody.trim() || undefined
		});
		reviewSubmitting = false;
		if (r.error) {
			reviewError = r.error.message || t('academy.review_error');
			return;
		}
		if (r.data) {
			// Re-fetch the reviews list (with aggregate) from the backend so the
			// UI matches the canonical state rather than a local patch. Keep the
			// currently-loaded window size so load-more progress isn't lost.
			const windowSize = Math.max(REVIEW_PAGE_SIZE, reviews.length + 1);
			const listRes = await api.getBody<{ data: TrackReview[]; aggregate: ReviewAggregate }>(
				`/academy/tracks/${track.id}/reviews?page_size=${windowSize}`
			);
			if (listRes.data) {
				reviews = listRes.data.data;
				reviewAggregate = listRes.data.aggregate;
			}
			reviewRating = 0;
			reviewTitle = '';
			reviewBody = '';
			reviewSuccess = true;
		}
	}

	async function loadMoreReviews() {
		if (reviewsLoadingMore) return;
		reviewsLoadingMore = true;
		reviewsLoadMoreError = null;
		const r = await api.getBody<{ data: TrackReview[]; aggregate: ReviewAggregate }>(
			`/academy/tracks/${track.id}/reviews?page_size=${REVIEW_PAGE_SIZE}&page_offset=${reviews.length}`
		);
		reviewsLoadingMore = false;
		if (r.error || !r.data) {
			reviewsLoadMoreError = r.error?.message ?? t('common.error');
			return;
		}
		reviews = [...reviews, ...r.data.data];
		if (r.data.aggregate) reviewAggregate = r.data.aggregate;
	}

	function onCheckoutSuccess() {
		checkoutOpen = false;
		enrollmentsStore.refresh();
		const firstModule = track.modules?.[0];
		const firstLesson = firstModule?.lessons?.[0];
		if (firstModule && firstLesson) {
			goto(`/tracks/${track.slug}/${firstModule.slug}/${firstLesson.slug}`);
		}
	}
</script>

<svelte:head>
	<title>{track.title} · {t('academy.name')}</title>
	{#if track.description}
		<meta name="description" content={track.description.slice(0, 160)} />
	{/if}
	<meta name="robots" content="index, follow" />
	<link rel="canonical" href={`${ACADEMY_ORIGIN}/tracks/${track.slug}`} />
	<meta property="og:title" content={track.title} />
	{#if track.description}
		<meta property="og:description" content={track.description.slice(0, 160)} />
	{/if}
	<meta property="og:type" content="website" />
	<meta property="og:url" content={`${ACADEMY_ORIGIN}/tracks/${track.slug}`} />
	{#if track.thumbnail_url}
		<meta property="og:image" content={track.thumbnail_url} />
	{/if}
	<meta name="twitter:card" content={track.thumbnail_url ? 'summary_large_image' : 'summary'} />
	<meta name="twitter:title" content={track.title} />
	{#if track.description}
		<meta name="twitter:description" content={track.description.slice(0, 160)} />
	{/if}
	{#if track.thumbnail_url}
		<meta name="twitter:image" content={track.thumbnail_url} />
	{/if}
	{#if data.jsonLd}
		{@html safeJsonLd(data.jsonLd)}
	{/if}
</svelte:head>

<div class="px-4 py-6 md:px-6 lg:px-8">
	<!-- Breadcrumb -->
	<nav class="mb-4 text-sm text-[var(--color-muted)]">
		<a href="/" class="hover:text-[var(--color-primary)]">{t('academy.explore')}</a>
		<span class="mx-1">/</span>
		<span class="text-[var(--color-text-subtle)]">{track.title}</span>
	</nav>

	<!-- Hero -->
	<header class="mb-6 flex flex-col gap-4 md:flex-row md:items-start">
		{#if track.thumbnail_url}
			<img
				src={track.thumbnail_url}
				alt=""
				class="h-40 w-full rounded-[var(--radius-card)] object-cover md:h-48 md:w-80"
			/>
		{/if}
		<div class="flex-1">
			<div class="mb-2 flex flex-wrap items-center gap-2">
				<LevelBadge level={track.level} />
				{#if priceLabel}
					<span class="rounded-full bg-[var(--color-surface-subtle)] px-2.5 py-0.5 text-xs font-semibold text-[var(--color-text)]">
						{priceLabel}
					</span>
				{/if}
			</div>
			<h1 class="text-2xl font-bold text-[var(--color-text)] md:text-3xl">{track.title}</h1>

			<!-- Stats -->
			<div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-[var(--color-muted)]">
				{#if track.instructor_name}
					<span>{t('academy.instructor')}: <strong class="text-[var(--color-text-subtle)]">{track.instructor_name}</strong></span>
				{/if}
				{#if track.duration_label ?? track.duration_iso}
					<span class="inline-flex items-center gap-1">
						<Clock size={14} /> {track.duration_label ?? formatDuration(track.duration_iso)}
					</span>
				{/if}
				{#if reviewAggregate?.count}
					<span class="inline-flex items-center gap-1">
						<Star size={14} class="text-[var(--level-intermediate)]" />
						{reviewAggregate.avg.toFixed(1)} ({formatCount(reviewAggregate.count)})
					</span>
				{/if}
				{#if track.enrolled_count}
					<span class="inline-flex items-center gap-1">
						<Users size={14} /> {t('academy.enrolled_count', '{count} enrolled').replace('{count}', formatCount(track.enrolled_count))}
					</span>
				{/if}
			</div>

			<!-- Enroll CTA -->
			<div class="mt-4 flex flex-wrap items-center gap-3">
				{#if loadingEnrollment && data.user}
					<LoadingSkeleton class="h-10 w-40" />
				{:else if isEnrolled}
					{#if isComplete}
						<span class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-4 py-2 text-sm font-medium text-[var(--color-progress)]">
							<CheckCircle size={18} /> {t('academy.completed')}
						</span>
					{:else}
						<div class="flex flex-wrap items-center gap-3">
							{#if continueHref}
								<a
									href={continueHref}
									class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
								>
									<PlayCircle size={18} /> {t('academy.continue')} ({progressPct}%)
								</a>
							{/if}
							<a
								href="/tracks/{track.slug}/progress"
								class="inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
							>
								{t('academy.view_progress')}
							</a>
						</div>
					{/if}
				{:else}
					<button
						type="button"
						onclick={enroll}
						disabled={enrolling}
						class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-60"
					>
						{#if enrolling}
							{t('common.loading')}
						{:else if isPaid && !isFreeOrPro && !isSubscriptionEligible}
							<Lock size={18} /> {t('academy.buy_for', 'Buy for {price}').replace('{price}', formatPriceKobo(track.price_kobo, track.currency, data.locale, data.currencies))}
						{:else}
							{t('academy.enroll')}
						{/if}
					</button>
					{#if enrollError}
						<p class="w-full text-sm text-[var(--color-destructive)]">{enrollError}</p>
					{/if}
				{/if}
				<!-- Bookmark toggle (POST/DELETE /me/bookmarks, entity_type=academy_track) -->
				<button
					type="button"
					onclick={toggleBookmark}
					disabled={bookmarkSaving || (bookmarkLoading && !!data.user)}
					title={bookmarkId ? t('academy.bookmark_saved') : t('academy.bookmark_save')}
					aria-label={bookmarkId ? t('academy.bookmark_saved') : t('academy.bookmark_save')}
					aria-pressed={!!bookmarkId}
					class="inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-60"
				>
					{#if bookmarkSaving || bookmarkLoading}
						<Loader2 size={16} class="animate-spin" />
					{:else}
						<BookmarkIcon size={16} class={bookmarkId ? 'fill-[var(--color-primary)] text-[var(--color-primary)]' : ''} />
					{/if}
				</button>
			</div>
		</div>
	</header>

	<!-- Tabs: Overview | Curriculum | Reviews -->
	<Tabs.Root value="overview" class="mt-6">
		<Tabs.List class="flex gap-1 border-b border-[var(--color-border)]">
			<Tabs.Trigger
				value="overview"
				class="border-b-2 border-transparent px-4 py-2 text-sm font-medium text-[var(--color-text-subtle)] data-[state=active]:border-[var(--color-primary)] data-[state=active]:text-[var(--color-primary)]"
			>
				{t('academy.overview')}
			</Tabs.Trigger>
			<Tabs.Trigger
				value="curriculum"
				class="border-b-2 border-transparent px-4 py-2 text-sm font-medium text-[var(--color-text-subtle)] data-[state=active]:border-[var(--color-primary)] data-[state=active]:text-[var(--color-primary)]"
			>
				{t('academy.curriculum')} ({(track.modules ?? []).reduce((n, m) => n + (m.lessons?.length ?? 0), 0)} {t('academy.lessons')})
			</Tabs.Trigger>
			<Tabs.Trigger
				value="reviews"
				class="border-b-2 border-transparent px-4 py-2 text-sm font-medium text-[var(--color-text-subtle)] data-[state=active]:border-[var(--color-primary)] data-[state=active]:text-[var(--color-primary)]"
			>
				{t('academy.reviews')}
			</Tabs.Trigger>
		</Tabs.List>

		<!-- Overview -->
		<Tabs.Content value="overview" class="pt-6">
			<div class="max-w-3xl">
				{#if track.description}
					<p class="text-base text-[var(--color-text-subtle)]">{track.description}</p>
				{/if}

				<!-- Instructor card -->
				{#if track.instructor}
					<section class="mt-8 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5">
						<h2 class="mb-3 text-lg font-semibold text-[var(--color-text)]">{t('academy.instructor')}</h2>
						<div class="flex items-start gap-4">
							{#if track.instructor.avatar_url}
								<img src={track.instructor.avatar_url} alt="" class="h-16 w-16 rounded-full object-cover" />
							{:else}
								<span class="grid h-16 w-16 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-xl font-medium text-[var(--color-text-subtle)]">
									{track.instructor.displayName?.charAt(0)?.toUpperCase() ?? '?'}
								</span>
							{/if}
							<div class="flex-1">
								<p class="font-medium text-[var(--color-text)]">{track.instructor.displayName}</p>
								<p class="text-sm text-[var(--color-muted)]">@{track.instructor.username}</p>
								{#if track.instructor.bio}
									<p class="mt-2 text-sm text-[var(--color-text-subtle)]">{track.instructor.bio}</p>
								{/if}
							</div>
						</div>
					</section>
				{/if}
			</div>
		</Tabs.Content>

		<!-- Curriculum -->
		<Tabs.Content value="curriculum" class="pt-6">
			<Accordion.Root type="multiple" class="max-w-3xl space-y-2">
				{#each track.modules as module, moduleIndex (module.id)}
					<Accordion.Item value={module.id} class="overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)]">
						<Accordion.Header>
							<Accordion.Trigger class="flex w-full items-center justify-between px-4 py-3 text-start text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
								<span class="flex items-center gap-2">
									<span class="text-xs font-semibold text-[var(--color-muted)]">{t('academy.module')} {moduleIndex + 1}</span>
									{module.title}
									<span class="text-xs text-[var(--color-muted)]">· {module.lessons.length} {t('academy.lessons')}</span>
								</span>
								<ChevronDown size={16} class="shrink-0 text-[var(--color-muted)] transition-transform" />
							</Accordion.Trigger>
						</Accordion.Header>
						<Accordion.Content class="px-4 pb-2">
							<ul class="space-y-1">
								{#each module.lessons as lesson (lesson.id)}
									<li>
										<a
											href="/tracks/{track.slug}/{module.slug}/{lesson.slug}"
											class="flex items-center gap-2 rounded-[var(--radius)] px-2 py-2 text-sm text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]"
										>
											<PlayCircle size={14} class="shrink-0 text-[var(--color-muted)]" />
											<span class="flex-1 truncate">{lesson.title}</span>
											{#if lesson.duration_label ?? lesson.duration_iso}
												<span class="text-xs text-[var(--color-muted)]">{lesson.duration_label ?? formatDuration(lesson.duration_iso)}</span>
											{/if}
											{#if lesson.is_preview}
												<span class="rounded border border-[var(--color-border)] px-1.5 py-0.5 text-[10px] font-semibold uppercase text-[var(--color-muted)]">
													{t('academy.preview_lesson')}
												</span>
											{/if}
										</a>
									</li>
								{/each}
							</ul>
						</Accordion.Content>
					</Accordion.Item>
				{/each}
			</Accordion.Root>
		</Tabs.Content>

		<!-- Reviews -->
		<Tabs.Content value="reviews" class="pt-6">
			<div class="max-w-3xl">
				{#if reviewAggregate?.count}
					<div class="mb-6 flex items-center gap-4">
						<span class="text-4xl font-bold text-[var(--color-text)]">{reviewAggregate.avg.toFixed(1)}</span>
						<div>
							<StarRating value={reviewAggregate.avg} />
							<p class="text-sm text-[var(--color-muted)]">{formatCount(reviewAggregate.count)} {t('academy.ratings')}</p>
						</div>
					</div>
				{/if}

				{#if data.user && (isEnrolled || isAdmin)}
					<form class="mb-6 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<h3 class="mb-3 text-sm font-medium text-[var(--color-text)]">{t('academy.write_review')}</h3>
						<div class="mb-3 flex items-center gap-1">
							{#each [1, 2, 3, 4, 5] as star}
								<button
									type="button"
									onclick={() => (reviewRating = star)}
									class="p-1"
									aria-label={t('academy.review_rating')}
								>
									<Star
										size={22}
										class={star <= reviewRating ? 'fill-[var(--level-intermediate)] text-[var(--level-intermediate)]' : 'text-[var(--color-muted)]'}
									/>
								</button>
							{/each}
						</div>
						<input
							type="text"
							bind:value={reviewTitle}
							placeholder={t('academy.review_rating')}
							class="mb-2 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
						/>
						<textarea
							bind:value={reviewBody}
							rows={3}
							placeholder={t('academy.review_body')}
							class="mb-2 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
						></textarea>
						{#if reviewError}
							<p class="mb-2 text-xs text-[var(--color-destructive)]">{reviewError}</p>
						{/if}
						{#if reviewSuccess}
							<p class="mb-2 text-xs text-[var(--color-success)]">{t('academy.review_submitted')}</p>
						{/if}
						<button
							type="button"
							disabled={reviewSubmitting}
							onclick={submitReview}
							class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-60"
						>
							{reviewSubmitting ? t('common.processing') : t('academy.submit_review')}
						</button>
					</form>
				{:else if data.user}
					<p class="mb-6 text-sm text-[var(--color-muted)]">{t('academy.review_requires_enrollment', 'Enroll in this course to leave a review.')}</p>
				{:else}
					<p class="mb-6 text-sm text-[var(--color-muted)]">{t('academy.review_unauthenticated')}</p>
				{/if}

				{#if reviews.length === 0}
					<p class="text-sm text-[var(--color-muted)]">{t('common.empty')}</p>
				{:else}
					<ul class="space-y-4">
						{#each reviews as review (review.id)}
							<li class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
								<div class="mb-2 flex items-center gap-3">
									{#if review.author_avatar_url}
										<img src={review.author_avatar_url} alt="" class="h-8 w-8 rounded-full object-cover" />
									{:else}
										<span class="grid h-8 w-8 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-xs font-medium text-[var(--color-text-subtle)]">
											{review.author_displayName?.charAt(0)?.toUpperCase() ?? review.author_username.charAt(0).toUpperCase()}
										</span>
									{/if}
									<div class="flex-1">
										<p class="text-sm font-medium text-[var(--color-text)]">{review.author_displayName ?? review.author_username}</p>
										<StarRating value={review.rating} small />
									</div>
								</div>
								{#if review.title}
									<p class="mb-1 text-sm font-medium text-[var(--color-text)]">{review.title}</p>
								{/if}
								{#if review.body}
									<p class="text-sm text-[var(--color-text-subtle)]">{review.body}</p>
								{/if}
							</li>
						{/each}
					</ul>
					{#if hasMoreReviews}
						<div class="mt-4 text-center">
							<button
								type="button"
								onclick={loadMoreReviews}
								disabled={reviewsLoadingMore}
								class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-60"
							>
								{reviewsLoadingMore ? t('common.loading') : t('academy.load_more', 'Load more')}
							</button>
							{#if reviewsLoadMoreError}
								<p class="mt-2 text-xs text-[var(--color-destructive)]">{reviewsLoadMoreError}</p>
							{/if}
						</div>
					{/if}
				{/if}
			</div>
		</Tabs.Content>
	</Tabs.Root>
</div>

<!-- Checkout modal for paid courses -->
<CheckoutModal
	{track}
	user={data.user}
	open={checkoutOpen}
	onClose={() => (checkoutOpen = false)}
	onSuccess={onCheckoutSuccess}
/>
