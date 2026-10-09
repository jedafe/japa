<script lang="ts">
	// Track Progress Detail — CSR ONLY.
	// The progress endpoint takes a trackId (UUID), not a slug, so we first
	// resolve the track by slug (GET /academy/tracks/{slug}) and then fetch
	// GET /academy/progress/{trackId} with the track's UUID.
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { LoadingSkeleton, EmptyState } from '$lib/components/ui';
	import ProgressRing from '$lib/components/academy/ProgressRing.svelte';
	import { ArrowLeft, CheckCircle, Clock } from 'lucide-svelte';
	import type { TrackDetail } from '$lib/types';

	const slug = $derived(page.params.slug);

	interface TrackProgress {
		completed: number;
		total: number;
		percent: number;
	}

	let track = $state<TrackDetail | null>(null);
	let progress = $state<TrackProgress | null>(null);
	let loading = $state(true);
	let notFound = $state(false);
	let error = $state<string | null>(null);

	// Request generation guard: a retry or slug change starts a new load, and a
	// slow response from a superseded load must not clobber the newer state.
	let loadGen = 0;

	async function loadProgress() {
		const gen = ++loadGen;
		const trackSlug = slug;
		if (!trackSlug) return;
		loading = true;
		notFound = false;
		error = null;
		track = null;
		progress = null;

		// 1. Resolve the track by slug to get its UUID.
		const trackRes = await api.get<TrackDetail>(`/academy/tracks/${trackSlug}`);
		if (gen !== loadGen) return;
		if (!trackRes.data) {
			loading = false;
			if (trackRes.status === 404) notFound = true;
			else error = trackRes.error?.message ?? t('common.error');
			return;
		}
		track = trackRes.data;

		// 2. Fetch progress using the track's UUID.
		const progRes = await api.get<TrackProgress>(`/academy/progress/${track.id}`);
		if (gen !== loadGen) return;
		loading = false;
		if (progRes.data) {
			progress = progRes.data;
		} else {
			error = progRes.error?.message ?? t('common.error');
		}
	}

	$effect(() => {
		// Re-run when the route param changes.
		if (slug) loadProgress();
	});

	const isComplete = $derived((progress?.percent ?? 0) >= 100);
	const remaining = $derived((progress?.total ?? 0) - (progress?.completed ?? 0));
</script>

<svelte:head>
	<title>
		{track ? `${track.title} · ${t('academy.progress')}` : t('academy.progress')} · {t('app.name')}
	</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="px-4 py-6 md:px-6 lg:px-8">
	<!-- Breadcrumb -->
	<nav class="mb-4 text-sm text-[var(--color-muted)]">
		<a href="/" class="hover:text-[var(--color-primary)]">{t('academy.explore')}</a>
		<span class="mx-1">/</span>
		{#if track}
			<a href="/tracks/{track.slug}" class="hover:text-[var(--color-primary)]">{track.title}</a>
			<span class="mx-1">/</span>
		{/if}
		<span class="text-[var(--color-text-subtle)]">{t('academy.progress')}</span>
	</nav>

	{#if loading}
		<!-- Loading skeleton -->
		<div class="flex flex-col gap-6">
			<div class="flex flex-col gap-4 md:flex-row md:items-start">
				<LoadingSkeleton class="h-40 w-full rounded-[var(--radius-card)] md:h-48 md:w-80" />
				<div class="flex-1 space-y-3">
					<LoadingSkeleton class="h-8 w-2/3" />
					<LoadingSkeleton class="h-4 w-1/2" />
					<LoadingSkeleton class="h-32 w-32 rounded-full" />
				</div>
			</div>
			<LoadingSkeleton class="h-3 w-full rounded-[var(--radius)]" />
		</div>
	{:else if notFound}
		<EmptyState title={t('academy.track_not_found', 'Track not found')} description={t('academy.track_not_found_desc', 'The track you are looking for does not exist or has been removed.')}>
			{#snippet action()}
				<a
					href="/"
					class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
				>
					<ArrowLeft size={16} /> {t('academy.back_to_explore', 'Back to explore')}
				</a>
			{/snippet}
		</EmptyState>
	{:else if error}
		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 text-center">
			<p class="text-sm text-[var(--color-destructive)]">{error}</p>
			<div class="mt-4 flex flex-wrap items-center justify-center gap-3">
				<button
					type="button"
					onclick={loadProgress}
					class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
				>
					{t('common.retry')}
				</button>
				<a
					href="/"
					class="inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>
					<ArrowLeft size={16} /> {t('academy.back_to_explore', 'Back to explore')}
				</a>
			</div>
		</div>
	{:else if track && progress}
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
				<h1 class="text-2xl font-bold text-[var(--color-text)] md:text-3xl">{track.title}</h1>
				{#if track.subtitle}
					<p class="mt-1 text-base text-[var(--color-text-subtle)]">{track.subtitle}</p>
				{/if}
				{#if track.duration_label ?? track.duration_iso}
					<p class="mt-2 inline-flex items-center gap-1 text-sm text-[var(--color-muted)]">
						<Clock size={14} /> {track.duration_label}
					</p>
				{/if}
			</div>
		</header>

		<!-- Progress overview card -->
		<section class="mb-6 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6">
			<div class="flex flex-col items-center gap-6 md:flex-row md:items-center md:gap-8">
				<!-- Progress ring -->
				<ProgressRing value={progress.percent} size={120} strokeWidth={10} />

				<!-- Stats -->
				<div class="flex-1 space-y-3">
					<h2 class="text-lg font-semibold text-[var(--color-text)]">{t('academy.your_progress', 'Your Progress')}</h2>
					<div class="grid grid-cols-3 gap-4">
						<div>
							<p class="text-xs uppercase text-[var(--color-muted)]">{t('academy.completed', 'Completed')}</p>
							<p class="mt-1 text-2xl font-bold text-[var(--color-text)]">{progress.completed}</p>
						</div>
						<div>
							<p class="text-xs uppercase text-[var(--color-muted)]">{t('academy.total_lessons', 'Total Lessons')}</p>
							<p class="mt-1 text-2xl font-bold text-[var(--color-text)]">{progress.total}</p>
						</div>
						<div>
							<p class="text-xs uppercase text-[var(--color-muted)]">{t('academy.percent', 'Percent')}</p>
							<p class="mt-1 text-2xl font-bold text-[var(--color-primary)]">{progress.percent}%</p>
						</div>
					</div>
				</div>
			</div>

			<!-- Progress bar -->
			<div class="mt-6">
				<div class="mb-2 flex items-center justify-between text-sm">
					<span class="text-[var(--color-text-subtle)]">{progress.completed} / {progress.total} {t('academy.lessons', 'lessons')}</span>
					<span class="font-medium text-[var(--color-text)]">{progress.percent}%</span>
				</div>
				<div class="h-3 w-full overflow-hidden rounded-[var(--radius)] bg-[var(--color-surface-subtle)]">
					<div
						class="h-full rounded-[var(--radius)] bg-[var(--color-progress)] transition-all duration-300"
						style="width: {Math.max(0, Math.min(100, progress.percent))}%"
					></div>
				</div>
			</div>
		</section>

		<!-- Status + actions -->
		<section class="mb-6">
			{#if isComplete}
				<div class="flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-4 py-3 text-sm font-medium text-[var(--color-progress)]">
					<CheckCircle size={18} />
					{t('academy.track_completed', 'You have completed this track!')}
				</div>
			{:else}
				<p class="text-sm text-[var(--color-text-subtle)]">
					{remaining} {t('academy.lessons_remaining', 'lessons remaining')}
				</p>
			{/if}
		</section>

		<!-- Action links -->
		<div class="flex flex-wrap gap-3">
			<a
				href="/tracks/{track.slug}"
				class="inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
			>
				<ArrowLeft size={16} /> {t('academy.back_to_track', 'Back to track')}
			</a>
			{#if !isComplete}
				<a
					href="/tracks/{track.slug}"
					class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
				>
					{t('academy.continue_learning', 'Continue learning')}
				</a>
			{/if}
		</div>
	{/if}
</div>
