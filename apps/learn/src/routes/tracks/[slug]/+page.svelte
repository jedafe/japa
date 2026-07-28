<script lang="ts">
	// Track Detail — widened overview (ADR-060).
	// SSR hero + stats · Tabs (Overview | Curriculum | Reviews) · Accordion curriculum ·
	// instructor card · enroll CTA · Course JSON-LD.
	// Enrollment is CSR-only (ADR-060 SSR/CSR boundary).
	import { Tabs, Accordion } from 'bits-ui';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import LevelBadge from '$lib/components/learn/LevelBadge.svelte';
	import StarRating from '$lib/components/learn/StarRating.svelte';
	import { LoadingSkeleton } from '$lib/components/ui';
	import { formatDuration, formatCount, formatPriceKobo, isFreeForUser } from '$lib/format';
	import { Star, Users, Clock, PlayCircle, CheckCircle, ChevronDown } from 'lucide-svelte';
	import type { PageData } from './$types';
	import type { Enrollment } from '$lib/types';

	let { data }: { data: PageData } = $props();
	const track = $derived(data.track);

	// CSR ONLY — enrollment state.
	let enrollment = $state<Enrollment | null>(null);
	let loadingEnrollment = $state(true);
	let enrolling = $state(false);
	let enrollError = $state<string | null>(null);

	$effect(() => {
		if (!data.user) {
			loadingEnrollment = false;
			return;
		}
		(async () => {
			const r = await api.get<Enrollment[]>('/me/enrollments');
			loadingEnrollment = false;
			if (r.data) enrollment = r.data.find((e) => e.track_id === track.id) ?? null;
		})();
	});

	const isEnrolled = $derived(!!enrollment);
	const progressPct = $derived(enrollment?.progress_pct ?? 0);
	const isComplete = $derived(progressPct >= 100);

	// Price label (university host only — ADR-063).
	const priceLabel = $derived(() => {
		if (!data.isUniversity) return null;
		if (isFreeForUser(track.price_kobo, data.user?.is_pro)) {
			return data.user?.is_pro ? t('learn.included_pro') : t('learn.free');
		}
		return formatPriceKobo(track.price_kobo);
	});

	async function enroll() {
		if (!data.user) {
			goto(`/login?redirect=/tracks/${track.slug}`);
			return;
		}
		enrolling = true;
		enrollError = null;
		const r = await api.post<Enrollment>(`/learn/tracks/${track.id}/enroll`);
		enrolling = false;
		if (r.data) {
			enrollment = r.data;
			// Navigate to the first lesson.
			const firstModule = track.modules[0];
			const firstLesson = firstModule?.lessons[0];
			if (firstModule && firstLesson) {
				goto(`/tracks/${track.slug}/${firstModule.slug}/${firstLesson.slug}`);
			}
		} else if (r.error) {
			enrollError = r.error.message;
		}
	}
</script>

<svelte:head>
	<title>{track.title} · {t('app.name')}</title>
	{#if track.description}
		<meta name="description" content={track.description.slice(0, 160)} />
	{/if}
	<link rel="canonical" href={`https://learn.japaconnect.com/tracks/${track.slug}`} />
	{#if data.jsonLd}
		<script type="application/ld+json">
			{JSON.stringify(data.jsonLd)}
		</script>
	{/if}
</svelte:head>

<div class="px-4 py-6 md:px-6 lg:px-8">
	<!-- Breadcrumb -->
	<nav class="mb-4 text-sm text-[var(--color-muted)]">
		<a href="/" class="hover:text-[var(--color-primary)]">{t('learn.explore')}</a>
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
				{#if data.isUniversity && priceLabel()}
					<span class="rounded-full bg-[var(--color-surface-subtle)] px-2.5 py-0.5 text-xs font-semibold text-[var(--color-text)]">
						{priceLabel()}
					</span>
				{/if}
			</div>
			<h1 class="text-2xl font-bold text-[var(--color-text)] md:text-3xl">{track.title}</h1>
			{#if track.subtitle}
				<p class="mt-1 text-base text-[var(--color-text-subtle)]">{track.subtitle}</p>
			{/if}

			<!-- Stats -->
			<div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-[var(--color-muted)]">
				{#if track.instructor_name}
					<span>{t('learn.instructor')}: <strong class="text-[var(--color-text-subtle)]">{track.instructor_name}</strong></span>
				{/if}
				{#if track.duration_label ?? track.duration_iso}
					<span class="inline-flex items-center gap-1">
						<Clock size={14} /> {track.duration_label ?? formatDuration(track.duration_iso)}
					</span>
				{/if}
				{#if data.reviewAggregate?.count}
					<span class="inline-flex items-center gap-1">
						<Star size={14} class="text-[var(--level-intermediate)]" />
						{data.reviewAggregate.avg.toFixed(1)} ({formatCount(data.reviewAggregate.count)})
					</span>
				{/if}
				{#if track.enrolled_count}
					<span class="inline-flex items-center gap-1">
						<Users size={14} /> {t('learn.enrolled_count', '{n} enrolled').replace('{n}', formatCount(track.enrolled_count))}
					</span>
				{/if}
			</div>

			<!-- Enroll CTA -->
			<div class="mt-4">
				{#if loadingEnrollment && data.user}
					<LoadingSkeleton class="h-10 w-40" />
				{:else if isEnrolled}
					{#if isComplete}
						<span class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-4 py-2 text-sm font-medium text-[var(--color-progress)]">
							<CheckCircle size={18} /> {t('learn.completed')}
						</span>
					{:else}
						<a
							href="/tracks/{track.slug}/{track.modules[0]?.slug ?? ''}/{track.modules[0]?.lessons[0]?.slug ?? ''}"
							class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
						>
							<PlayCircle size={18} /> {t('learn.continue')} ({progressPct}%)
						</a>
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
						{:else}
							{t('learn.enroll')}
						{/if}
					</button>
					{#if enrollError}
						<p class="mt-2 text-sm text-[var(--color-destructive)]">{enrollError}</p>
					{/if}
				{/if}
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
				{t('learn.overview')}
			</Tabs.Trigger>
			<Tabs.Trigger
				value="curriculum"
				class="border-b-2 border-transparent px-4 py-2 text-sm font-medium text-[var(--color-text-subtle)] data-[state=active]:border-[var(--color-primary)] data-[state=active]:text-[var(--color-primary)]"
			>
				{t('learn.curriculum')} ({track.modules.reduce((n, m) => n + m.lessons.length, 0)} {t('learn.lessons')})
			</Tabs.Trigger>
			<Tabs.Trigger
				value="reviews"
				class="border-b-2 border-transparent px-4 py-2 text-sm font-medium text-[var(--color-text-subtle)] data-[state=active]:border-[var(--color-primary)] data-[state=active]:text-[var(--color-primary)]"
			>
				{t('learn.reviews')}
			</Tabs.Trigger>
		</Tabs.List>

		<!-- Overview -->
		<Tabs.Content value="overview" class="pt-6">
			<div class="max-w-3xl">
				{#if track.overview_html}
					<!-- Article content rendered server-side from TipTap JSON -->
					<div class="prose">{@html track.overview_html}</div>
				{:else if track.description}
					<p class="text-base text-[var(--color-text-subtle)]">{track.description}</p>
				{/if}

				{#if track.objectives?.length}
					<section class="mt-6">
						<h2 class="mb-3 text-lg font-semibold text-[var(--color-text)]">What you'll learn</h2>
						<ul class="space-y-2">
							{#each track.objectives as obj}
								<li class="flex items-start gap-2 text-sm text-[var(--color-text-subtle)]">
									<CheckCircle size={16} class="mt-0.5 shrink-0 text-[var(--color-progress)]" />
									<span>{obj}</span>
								</li>
							{/each}
						</ul>
					</section>
				{/if}

				{#if track.prerequisites?.length}
					<section class="mt-6">
						<h2 class="mb-3 text-lg font-semibold text-[var(--color-text)]">Prerequisites</h2>
						<ul class="space-y-1">
							{#each track.prerequisites as pre}
								<li class="text-sm text-[var(--color-text-subtle)]">• {pre}</li>
							{/each}
						</ul>
					</section>
				{/if}

				<!-- Instructor card -->
				{#if track.instructor}
					<section class="mt-8 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5">
						<h2 class="mb-3 text-lg font-semibold text-[var(--color-text)]">{t('learn.instructor')}</h2>
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
				{#each track.modules as module (module.id)}
					<Accordion.Item value={module.id} class="overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)]">
						<Accordion.Header>
							<Accordion.Trigger class="flex w-full items-center justify-between px-4 py-3 text-left text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
								<span class="flex items-center gap-2">
									<span class="text-xs font-semibold text-[var(--color-muted)]">{t('learn.module')} {module.sort_order + 1}</span>
									{module.title}
									<span class="text-xs text-[var(--color-muted)]">· {module.lessons.length} {t('learn.lessons')}</span>
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
													{t('learn.preview_lesson')}
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
				{#if data.reviewAggregate?.count}
					<div class="mb-6 flex items-center gap-4">
						<span class="text-4xl font-bold text-[var(--color-text)]">{data.reviewAggregate.avg.toFixed(1)}</span>
						<div>
							<StarRating value={data.reviewAggregate.avg} />
							<p class="text-sm text-[var(--color-muted)]">{formatCount(data.reviewAggregate.count)} {t('learn.ratings')}</p>
						</div>
					</div>
				{/if}

				{#if data.reviews.length === 0}
					<p class="text-sm text-[var(--color-muted)]">{t('common.empty')}</p>
				{:else}
					<ul class="space-y-4">
						{#each data.reviews as review (review.id)}
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
								{#if review.body}
									<p class="text-sm text-[var(--color-text-subtle)]">{review.body}</p>
								{/if}
							</li>
						{/each}
					</ul>
				{/if}
			</div>
		</Tabs.Content>
	</Tabs.Root>
</div>
