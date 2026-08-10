<script lang="ts">
	// Course Catalog — SSR track grid + CSR enrollment state.
	// Ref: ADR-060 (Explore is the center column), ADR-060 SSR/CSR boundary.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import TrackCard from '$lib/components/learn/TrackCard.svelte';
	import { LoadingSkeleton, EmptyState } from '$lib/components/ui';
	import { Compass, GraduationCap, ArrowRight } from 'lucide-svelte';
	import { DIFFICULTY_LEVELS } from '$lib/constants/learning';
	import type { PageData } from './$types';
	import type { Enrollment } from '$lib/types';

	let { data }: { data: PageData } = $props();

	// CSR ONLY — fetch the user's enrollments to drive TrackCard action state.
	// Never in +page.server.ts (ADR-060 SSR/CSR boundary).
	let enrollments = $state<Enrollment[]>([]);
	let loadingEnrollments = $state(true);

	$effect(() => {
		if (!data.user) {
			loadingEnrollments = false;
			return;
		}
		(async () => {
			const r = await api.get<Enrollment[]>('/me/enrollments');
			loadingEnrollments = false;
			if (r.data) enrollments = r.data;
		})();
	});

	// Build a lookup map for the TrackCard action state.
	const enrollmentMap = $derived(
		new Map(enrollments.map((e) => [e.track_id, e]))
	);
</script>

<svelte:head>
	<title>{t('university.catalog')} · {t('university.name')}</title>
	<meta name="description" content="Browse courses on Japa University." />
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
				{data.tracks.length} results for "{data.query}"
			</p>
		{/if}
	</header>

	<!-- Become an Instructor CTA -->
	{#if !data.query && !data.levelFilter}
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

	<!-- Level filter -->
	<div class="mb-6 flex flex-wrap gap-2">
		<a
			href="/"
			class="rounded-full px-3 py-1 text-sm {!data.levelFilter ? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]' : 'border border-[var(--color-border)] text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]'}"
		>
			{t('learn.overview')}
		</a>
		{#each DIFFICULTY_LEVELS as level}
			<a
				href="/?level={level}"
				class="rounded-full px-3 py-1 text-sm {data.levelFilter === level ? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]' : 'border border-[var(--color-border)] text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]'}"
			>
				{t(`level.${level}`)}
			</a>
		{/each}
	</div>

	<!-- Catalog grid -->
	{#if data.tracks.length === 0}
		<EmptyState
			title={t('common.empty')}
			description={data.query ? t('learn.no_match_for_query', `No courses match "${data.query}".`) : t('learn.no_courses_yet', 'No courses published yet.')}
		>
			{#snippet action()}
				{#if data.query || data.levelFilter}
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
	{/if}
</div>
