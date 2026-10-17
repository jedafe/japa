<script lang="ts">
	// Public instructor profile — Japa Academy creator economy (ADR-063).
	// CSR ONLY: fetches GET /academy/instructors/{id} on mount.
	// Shows the instructor's bio, stats, social links, and their published tracks.
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { safeJsonLd } from '$lib/seo';
	import { api } from '$lib/api/client';
	import { LoadingSkeleton, EmptyState } from '$lib/components/ui';
	import { formatCount, formatPriceKobo } from '$lib/format';
	import { GraduationCap, Users, Star, BookOpen, Link as LinkIcon } from 'lucide-svelte';
	import type { AcademyInstructorProfile, TrackSummary } from '$lib/types';

	interface AcademyInstructorProfileResponse {
		instructor: AcademyInstructorProfile;
		tracks: TrackSummary[];
	}

	const id = $derived(page.params.id);

	let instructor = $state<AcademyInstructorProfile | null>(null);
	let tracks = $state<TrackSummary[]>([]);
	let loading = $state(true);
	let notFound = $state(false);
	let error = $state<string | null>(null);

	$effect(() => {
		// Re-run when the route param changes.
		const instructorId = id;
		if (!instructorId) return;
		loadInstructor(instructorId);
	});

	async function loadInstructor(instructorId: string) {
		loading = true;
		notFound = false;
		error = null;
		instructor = null;
		tracks = [];
		const r = await api.get<AcademyInstructorProfileResponse>(
			`/academy/instructors/${instructorId}`
		);
		loading = false;
		if (r.data) {
			instructor = r.data.instructor;
			tracks = r.data.tracks;
		} else if (r.status === 404) {
			notFound = true;
		} else {
			error = r.error?.message ?? t('common.error');
		}
	}

	const socialEntries = $derived(
		instructor?.social_links
			? Object.entries(instructor.social_links).filter(([, url]) => !!url)
			: []
	);
</script>

<svelte:head>
	<title>{instructor?.display_name ?? t('academy.instructor')} · {t('app.name')}</title>
	{#if instructor?.headline}
		<meta name="description" content={instructor.headline.slice(0, 160)} />
	{/if}
	<meta name="robots" content="index, follow" />
	<link rel="canonical" href={`https://academy.japa.to/instructors/${id}`} />
	<meta property="og:title" content={`${instructor?.display_name ?? t('academy.instructor')} · ${t('app.name')}`} />
	{#if instructor?.headline}
		<meta property="og:description" content={instructor.headline.slice(0, 160)} />
	{/if}
	<meta property="og:type" content="profile" />
	<meta property="og:url" content={`https://academy.japa.to/instructors/${id}`} />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={`${instructor?.display_name ?? t('academy.instructor')} · ${t('app.name')}`} />
	{#if instructor?.headline}
		<meta name="twitter:description" content={instructor.headline.slice(0, 160)} />
	{/if}
	{#if instructor}
		{@html safeJsonLd({
				'@context': 'https://schema.org',
				'@type': 'Person',
				name: instructor.display_name,
				description: instructor.headline ?? instructor.bio ?? undefined,
				url: `https://academy.japa.to/instructors/${id}`
			})}
	{/if}
</svelte:head>

<div class="px-4 py-6 md:px-6 lg:px-8">
	<!-- Breadcrumb -->
	<nav class="mb-4 text-sm text-[var(--color-muted)]">
		<a href="/" class="hover:text-[var(--color-primary)]">{t('academy.explore')}</a>
		<span class="mx-1">/</span>
		<span class="text-[var(--color-text-subtle)]">{instructor?.display_name ?? t('academy.instructor')}</span>
	</nav>

	{#if loading}
		<!-- Loading: hero skeleton -->
		<div class="mb-6 flex flex-col gap-4 md:flex-row md:items-start">
			<LoadingSkeleton class="h-24 w-24 shrink-0 rounded-full" />
			<div class="flex-1 space-y-2">
				<LoadingSkeleton class="h-7 w-64" />
				<LoadingSkeleton class="h-4 w-96" />
				<LoadingSkeleton class="h-4 w-48" />
			</div>
		</div>
		<!-- Loading: stats skeleton -->
		<div class="mb-6 grid grid-cols-1 gap-4 md:grid-cols-3">
			{#each Array(3) as _}
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<LoadingSkeleton class="mb-3 h-4 w-24" />
					<LoadingSkeleton class="h-8 w-16" />
				</div>
			{/each}
		</div>
		<!-- Loading: tracks skeleton -->
		<div class="space-y-3">
			{#each Array(3) as _}
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<LoadingSkeleton class="h-5 w-3/4" />
					<LoadingSkeleton class="mt-2 h-4 w-1/3" />
				</div>
			{/each}
		</div>
	{:else if error}
		<EmptyState title={t('common.error')} description={error}>
			{#snippet action()}
				<button
					type="button"
					onclick={() => id && loadInstructor(id)}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
				>
					{t('common.retry')}
				</button>
			{/snippet}
		</EmptyState>
	{:else if notFound || !instructor}
		<EmptyState
			title={t('academy.instructor_not_found', 'Instructor not found')}
			description={t('academy.instructor_not_found_description', 'This instructor profile does not exist or is no longer available.')}
		>
			{#snippet action()}
				<a
					href="/"
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
				>
					{t('academy.explore')}
				</a>
			{/snippet}
		</EmptyState>
	{:else}
		<!-- Hero -->
		<header class="mb-6 flex flex-col gap-4 md:flex-row md:items-start">
			<span class="grid h-24 w-24 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-3xl font-medium text-[var(--color-text-subtle)]">
				{instructor.display_name?.charAt(0)?.toUpperCase() ?? '?'}
			</span>
			<div class="flex-1">
				<div class="mb-2 flex flex-wrap items-center gap-2">
					<span class="inline-flex items-center gap-1 rounded-full bg-[var(--color-surface-subtle)] px-2.5 py-0.5 text-xs font-semibold text-[var(--color-text)]">
						<GraduationCap size={12} /> {instructor.tier}
					</span>
					{#if instructor.status === 'approved'}
						<span class="rounded-full bg-[var(--color-surface-subtle)] px-2.5 py-0.5 text-xs font-semibold text-[var(--color-progress)]">
							{t('academy.approved', 'Verified')}
						</span>
					{/if}
				</div>
				<h1 class="text-2xl font-bold text-[var(--color-text)] md:text-3xl">{instructor.display_name}</h1>
				{#if instructor.headline}
					<p class="mt-1 text-base text-[var(--color-text-subtle)]">{instructor.headline}</p>
				{/if}
				{#if instructor.bio}
					<p class="mt-3 max-w-2xl text-sm text-[var(--color-text-subtle)]">{instructor.bio}</p>
				{/if}

				<!-- Social links -->
				{#if socialEntries.length > 0}
					<ul class="mt-3 flex flex-wrap gap-3">
						{#each socialEntries as [platform, url] (platform)}
							<li>
								<a
									href={url}
									target="_blank"
									rel="noopener noreferrer"
									class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text-subtle)] capitalize hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-primary)]"
								>
									<LinkIcon size={12} />
									{platform}
								</a>
							</li>
						{/each}
					</ul>
				{/if}
			</div>
		</header>

		<!-- Stats -->
		<div class="mb-6 grid grid-cols-1 gap-4 md:grid-cols-3">
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<div class="flex items-center gap-2 text-sm text-[var(--color-muted)]">
					<Users size={16} class="text-[var(--color-primary)]" />
					{t('academy.total_enrolled')}
				</div>
				<p class="mt-2 text-3xl font-bold text-[var(--color-text)]">{formatCount(instructor.total_enrolled)}</p>
			</div>
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<div class="flex items-center gap-2 text-sm text-[var(--color-muted)]">
					<BookOpen size={16} class="text-[var(--color-primary)]" />
					{t('academy.courses', 'Courses')}
				</div>
				<p class="mt-2 text-3xl font-bold text-[var(--color-text)]">{formatCount(instructor.courses_count)}</p>
			</div>
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<div class="flex items-center gap-2 text-sm text-[var(--color-muted)]">
					<Star size={16} class="text-[var(--level-intermediate)]" />
					{t('academy.total_revenue')}
				</div>
				<p class="mt-2 text-3xl font-bold text-[var(--color-text)]">{formatPriceKobo(instructor.total_revenue_kobo, instructor.currency, page.data.locale, page.data.currencies)}</p>
			</div>
		</div>

		<!-- Tracks -->
		<section class="mt-6">
			<div class="mb-3 flex items-center justify-between">
				<h2 class="text-lg font-semibold text-[var(--color-text)]">{t('academy.courses', 'Courses')}</h2>
				{#if tracks.length > 0}
					<span class="text-sm text-[var(--color-muted)]">{formatCount(tracks.length)}</span>
				{/if}
			</div>

			{#if tracks.length === 0}
				<EmptyState title={t('academy.no_courses', 'No courses yet')} />
			{:else}
				<ul class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
					{#each tracks as tr (tr.id)}
						<li class="overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] transition-shadow hover:shadow-md">
							<a href="/tracks/{tr.slug}" class="block">
								{#if tr.thumbnail_url}
									<img
										src={tr.thumbnail_url}
										alt=""
										class="h-40 w-full object-cover"
									/>
								{:else}
									<div class="grid h-40 w-full place-items-center bg-[var(--color-surface-subtle)]">
										<BookOpen size={32} class="text-[var(--color-muted)]" />
									</div>
								{/if}
								<div class="p-4">
									<h3 class="line-clamp-2 font-medium text-[var(--color-text)]">{tr.title}</h3>
									{#if tr.subtitle}
										<p class="mt-1 line-clamp-1 text-sm text-[var(--color-text-subtle)]">{tr.subtitle}</p>
									{/if}
									<div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[var(--color-muted)]">
										{#if tr.level}
											<span class="capitalize">{tr.level}</span>
										{/if}
										{#if tr.enrolled_count}
											<span class="inline-flex items-center gap-1">
												<Users size={12} />
												{t('academy.enrolled_count', '{count} enrolled').replace('{count}', formatCount(tr.enrolled_count))}
											</span>
										{/if}
										{#if tr.price_kobo !== undefined && tr.price_kobo !== null}
											<span class="font-medium text-[var(--color-text-subtle)]">{formatPriceKobo(tr.price_kobo, tr.currency, page.data.locale, page.data.currencies)}</span>
										{/if}
									</div>
								</div>
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		</section>
	{/if}
</div>
