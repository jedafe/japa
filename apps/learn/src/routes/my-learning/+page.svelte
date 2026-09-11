<script lang="ts">
	// My Learning dashboard — learner analytics (ADR-059 LayerChart, ADR-060).
	// CSR ONLY: enrollments + progress from /me/enrollments (ADR-060 SSR/CSR boundary).
	// Charts use var(--chart-*) tokens (dark-mode safe), fixed-height containers,
	// per-chart skeletons, dates → Date.
	//
	// NOTE: time-series analytics (progress over time, weekly activity, streak)
	// require a backend analytics endpoint not yet built. Those sections are
	// hidden — NO mocked data (Rule 16, API-Readiness Gate).
	import { BarChart } from 'layerchart';
	import { t } from '$lib/i18n';
	import { enrollmentsStore } from '$lib/stores/enrollments.svelte';
	import { LoadingSkeleton, EmptyState } from '$lib/components/ui';
	import { LayoutDashboard, CheckCircle, AlertTriangle, RotateCw } from 'lucide-svelte';
	import type { Enrollment } from '$lib/types';

	// Subscribe to the shared enrollments store — avoids duplicate API calls
	// and ensures mutations (enroll, mark-complete) propagate to all widgets.
	$effect(() => {
		enrollmentsStore.fetch();
	});

	const enrollments = $derived(enrollmentsStore.enrollments ?? []);
	const loading = $derived(enrollmentsStore.loading);
	const error = $derived(enrollmentsStore.error);

	// Derived stats from real enrollment data — guard against null/undefined
	// lessons_completed to prevent NaN (the API may omit the field).
	const totalCompleted = $derived(
		enrollments.reduce((sum, e) => sum + (e.lessons_completed || 0), 0)
	);
	const inProgress = $derived(enrollments.filter((e) => e.progress_pct > 0 && e.progress_pct < 100));
	const completed = $derived(enrollments.filter((e) => e.progress_pct >= 100));

	// BarChart data: lessons completed per track (real, derived from enrollments).
	const barData = $derived(
		enrollments
			.slice(0, 10)
			.map((e) => ({
				track: e.track_title.length > 20 ? e.track_title.slice(0, 20) + '…' : e.track_title,
				lessons: e.lessons_completed || 0
			}))
	);
</script>

<svelte:head>
	<title>{t('learn.my_learning')} · {t('app.name')}</title>
</svelte:head>

<div class="px-4 py-6 md:px-6 lg:px-8">
	<header class="mb-6">
		<h1 class="flex items-center gap-2 text-2xl font-bold text-[var(--color-text)]">
			<LayoutDashboard size={24} class="text-[var(--color-primary)]" />
			{t('learn.my_learning')}
		</h1>
	</header>

	{#if loading}
		<!-- Per-chart skeleton -->
		<div class="grid grid-cols-1 gap-4 md:grid-cols-3">
			{#each Array(3) as _}
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<LoadingSkeleton class="mb-3 h-4 w-24" />
					<LoadingSkeleton class="h-8 w-16" />
				</div>
			{/each}
		</div>
		<div class="mt-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<LoadingSkeleton class="h-64 w-full" />
		</div>
	{:else if error}
		<!-- Error state with retry -->
		<div class="rounded-[var(--radius-card)] border border-[var(--color-danger)] bg-[var(--color-surface)] p-6 text-center">
			<AlertTriangle size={32} class="mx-auto mb-3 text-[var(--color-danger)]" />
			<p class="text-sm font-medium text-[var(--color-text)]">{t('learn.load_error', 'Failed to load your enrollments')}</p>
			<p class="mt-1 text-xs text-[var(--color-muted)]">{error}</p>
			<button
				onclick={() => enrollmentsStore.refresh()}
				class="mt-4 inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
			>
				<RotateCw size={14} />
				{t('learn.retry', 'Retry')}
			</button>
		</div>
	{:else if enrollments.length === 0}
		<EmptyState
			title={t('learn.no_enrollments')}
			description={t('learn.browse_catalog_prompt', 'Browse the catalog to start learning.')}
		>
			{#snippet action()}
				<a
					href="/"
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
				>
					{t('learn.explore')}
				</a>
			{/snippet}
		</EmptyState>
	{:else}
		<!-- Stats cards -->
		<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4" role="status" aria-label={t('learn.completed')}>
				<div class="flex items-center gap-2 text-sm text-[var(--color-muted)]">
					<CheckCircle size={16} class="text-[var(--color-progress)]" />
					{t('learn.completed')}
				</div>
				<p class="mt-2 text-3xl font-bold text-[var(--color-text)]">{totalCompleted}</p>
				<p class="text-xs text-[var(--color-muted)]">{t('learn.lessons')}</p>
			</div>
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4" role="status" aria-label={t('learn.in_progress')}>
				<div class="flex items-center gap-2 text-sm text-[var(--color-muted)]">
					<LayoutDashboard size={16} class="text-[var(--color-primary)]" />
					{t('learn.in_progress')}
				</div>
				<p class="mt-2 text-3xl font-bold text-[var(--color-text)]">{inProgress.length}</p>
				<p class="text-xs text-[var(--color-muted)]">{t('learn.courses')}</p>
			</div>
		</div>

		<!-- BarChart: lessons completed per track (real data from /me/enrollments) -->
		{#if barData.length > 0}
			<section class="mt-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{t('learn.lessons_completed_by_course')}</h2>
				<div class="h-64 w-full">
					<BarChart
						data={barData}
						x="track"
						y="lessons"
						props={{ bars: { class: 'fill-[var(--chart-1)]' } }}
					/>
				</div>
			</section>
		{/if}

		<!-- Enrollment list -->
		<section class="mt-6">
			<h2 class="mb-3 text-lg font-semibold text-[var(--color-text)]">{t('learn.my_learning')}</h2>
			<ul class="space-y-3">
				{#each enrollments as en (en.id)}
					<li class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-start gap-3">
							{#if en.thumbnail_url}
								<a href="/tracks/{en.track_slug}" class="shrink-0">
									<img src={en.thumbnail_url} alt="" class="h-16 w-24 rounded-[var(--radius)] object-cover" />
								</a>
							{/if}
							<div class="flex-1">
								<a href="/tracks/{en.track_slug}" class="font-medium text-[var(--color-text)] hover:text-[var(--color-primary)]">
									{en.track_title}
								</a>
								<p class="text-sm text-[var(--color-muted)]">
									{en.lessons_completed || 0}/{en.lessons_total || 0} {t('learn.lessons')} · {en.progress_pct || 0}%
								</p>
								<div class="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-[var(--color-surface-subtle)]">
									<div
										class="h-full rounded-full bg-[var(--color-progress)]"
										style="width: {en.progress_pct || 0}%"
									></div>
								</div>
							</div>
							{#if en.progress_pct >= 100}
								<CheckCircle size={20} class="shrink-0 text-[var(--color-progress)]" />
							{:else if en.next_lesson}
								<a
									href="/tracks/{en.track_slug}/{en.next_lesson.module_slug}/{en.next_lesson.slug}"
									class="shrink-0 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
								>
									{t('learn.continue')}
								</a>
							{/if}
						</div>
					</li>
				{/each}
			</ul>
		</section>
	{/if}
</div>