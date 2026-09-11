<script lang="ts">
	// Course Catalog — SSR track grid + CSR enrollment state.
	// Ref: ADR-060 (Explore is the center column), ADR-060 SSR/CSR boundary.
	import { t } from '$lib/i18n';
	import TrackCard from '$lib/components/learn/TrackCard.svelte';
	import { LoadingSkeleton, EmptyState } from '$lib/components/ui';
	import { Compass, GraduationCap, ArrowRight, ChevronLeft, ChevronRight } from 'lucide-svelte';
	import { enrollmentsStore } from '$lib/stores/enrollments.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// CSR ONLY — use the shared enrollment store so TrackCard action state stays
	// in sync with sidebar widgets and avoids duplicate /me/enrollments calls.
	$effect(() => {
		if (data.user) {
			enrollmentsStore.fetch();
		}
	});

	const enrollments = $derived(enrollmentsStore.enrollments ?? []);
	const loadingEnrollments = $derived(enrollmentsStore.loading && enrollments.length === 0);

	// Build a lookup map for the TrackCard action state.
	const enrollmentMap = $derived(
		new Map(enrollments.map((e) => [e.track_id, e]))
	);

	// Build a query string that preserves the current active filters, so users
	// can combine level, category, and free filters plus pagination.
	function buildHref(partial: Record<string, string | number | boolean> = {}) {
		const qs = new URLSearchParams();
		if (data.query) qs.set('q', data.query);
		if (data.levelFilter) qs.set('level', data.levelFilter);
		if (data.categoryFilter) qs.set('category', data.categoryFilter);
		if (data.isFreeFilter) qs.set('is_free', 'true');
		if (data.offset) qs.set('offset', String(data.offset));

		for (const [key, value] of Object.entries(partial)) {
			if (value === false) qs.delete(key);
			else qs.set(key, String(value));
		}

		const s = qs.toString();
		return s ? `/?${s}` : '/';
	}

	const canGoPrev = $derived(data.offset > 0);
	const canGoNext = $derived(data.offset + data.tracks.length < data.total);
</script>

<svelte:head>
	<title>{t('university.catalog')} · {t('university.name')}</title>
	<meta name="description" content={t('university.catalog_description')} />
	<link rel="canonical" href="https://university.japaconnect.com/" />
</svelte:head>

<div class="px-4 py-6 md:px-6 lg:px-8">
	<!-- Header -->
	<header class="mb-6">
		<h1 class="flex items-center gap-2 text-2xl font-bold text-[var(--color-text)]">
			<Compass size={24} class="text-[var(--color-primary)]" />
			{t('university.catalog')}
		</h1>
		{#if data.query}
			<p class="mt-1 text-sm text-[var(--color-muted)]">
				{t('learn.results_for_query', '{count} results for "{query}"')
					.replace('{count}', String(data.total))
					.replace('{query}', data.query)}
			</p>
		{:else if data.total > 0}
			<p class="mt-1 text-sm text-[var(--color-muted)]">
				{t('learn.course_count', '{count} courses').replace('{count}', String(data.total))}
			</p>
		{/if}
	</header>

	<!-- Become an Instructor CTA -->
	{#if !data.query && !data.levelFilter && !data.categoryFilter && !data.isFreeFilter}
		<div class="mb-6 flex items-center justify-between gap-4 rounded-[var(--radius-card)] border border-[var(--color-primary)]/20 bg-[var(--color-primary)]/5 p-4">
			<div class="flex items-center gap-3">
				<div class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10">
					<GraduationCap size={20} class="text-[var(--color-primary)]" />
				</div>
				<div>
					<p class="text-sm font-semibold text-[var(--color-text)]">{t('university.become_instructor_title', 'Become an Instructor')}</p>
					<p class="text-xs text-[var(--color-muted)]">{t('university.become_instructor_desc', 'Share your expertise and earn revenue teaching what you know.')}</p>
				</div>
			</div>
			<a
				href="/dashboard/instructor/apply"
				class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
			>
				{t('university.get_started', 'Get Started')}
				<ArrowRight size={16} />
			</a>
		</div>
	{/if}

	<!-- Filters: level, category, free -->
	<div class="mb-6 flex flex-wrap items-center gap-2">
		<a
			href={buildHref({ level: false })}
			class="rounded-full px-3 py-1 text-sm {!data.levelFilter ? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]' : 'border border-[var(--color-border)] text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]'}"
		>
			{t('learn.all_levels')}
		</a>
		{#each data.difficultyLevels as opt (opt.value)}
			<a
				href={buildHref({ level: opt.value, offset: false })}
				class="rounded-full px-3 py-1 text-sm {data.levelFilter === opt.value ? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]' : 'border border-[var(--color-border)] text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]'}"
			>
				{t(`level.${opt.value}`)}
			</a>
		{/each}

		{#if data.categories.length > 0}
			<span class="mx-1 hidden h-5 w-px bg-[var(--color-border)] md:block"></span>
			<a
				href={buildHref({ category: false, offset: false })}
				class="rounded-full px-3 py-1 text-sm {!data.categoryFilter ? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]' : 'border border-[var(--color-border)] text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]'}"
			>
				{t('learn.all_categories')}
			</a>
			{#each data.categories as cat (cat.slug)}
				<a
					href={buildHref({ category: cat.slug, offset: false })}
					class="rounded-full px-3 py-1 text-sm {data.categoryFilter === cat.slug ? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]' : 'border border-[var(--color-border)] text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]'}"
				>
					{cat.name}
				</a>
			{/each}
		{/if}

		<a
			href={buildHref({ is_free: !data.isFreeFilter, offset: false })}
			class="ml-auto rounded-full px-3 py-1 text-sm {data.isFreeFilter ? 'bg-[var(--level-beginner)] text-[var(--level-beginner-contrast)]' : 'border border-[var(--color-border)] text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]'}"
		>
			{t(data.isFreeFilter ? 'learn.free_only' : 'learn.show_free_only')}
		</a>
	</div>

	<!-- Catalog grid -->
	{#if data.tracks.length === 0}
		<EmptyState
			title={t('common.empty')}
			description={data.query
				? t('learn.no_match_for_query', 'No courses match "{query}".').replace('{query}', data.query)
				: t('learn.no_courses_yet', 'No courses published yet.')}
		>
			{#snippet action()}
				{#if data.query || data.levelFilter || data.categoryFilter || data.isFreeFilter}
					<a
						href="/"
						class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
					>
						{t('learn.explore')}
					</a>
				{/if}
			{/snippet}
		</EmptyState>
	{:else}
		<div class="grid grid-cols-1 gap-5 sm:grid-cols-2 xl:grid-cols-3">
			{#each data.tracks as track (track.id)}
				{#if loadingEnrollments && data.user}
					<div class="overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
						<LoadingSkeleton class="aspect-video w-full rounded-none" />
						<div class="space-y-2 p-4">
							<LoadingSkeleton class="h-5 w-3/4" />
							<LoadingSkeleton class="h-4 w-1/2" />
							<LoadingSkeleton class="h-4 w-2/3" />
						</div>
					</div>
				{:else}
					{@const en = enrollmentMap.get(track.id)}
					<TrackCard
						{track}
						user={data.user}
						enrolled={!!en}
						progressPct={en?.progress_pct ?? 0}
					/>
				{/if}
			{/each}
		</div>

		<!-- Pagination -->
		{#if data.total > data.limit}
			<div class="mt-8 flex items-center justify-between">
				<a
					href={buildHref({ offset: Math.max(0, data.offset - data.limit) })}
					class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] {canGoPrev ? '' : 'pointer-events-none opacity-40'}"
					aria-disabled={!canGoPrev}
				>
					<ChevronLeft size={16} />
					{t('common.previous')}
				</a>
				<span class="text-sm text-[var(--color-muted)]">
					{data.offset + 1} – {Math.min(data.offset + data.tracks.length, data.total)} of {data.total}
				</span>
				<a
					href={buildHref({ offset: data.offset + data.limit })}
					class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] {canGoNext ? '' : 'pointer-events-none opacity-40'}"
					aria-disabled={!canGoNext}
				>
					{t('common.next')}
					<ChevronRight size={16} />
				</a>
			</div>
		{/if}
	{/if}
</div>
