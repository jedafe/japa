<script lang="ts">
	// Search results — Phase 10 (ROADMAP-LEARN.md).
	// SSR results grid + CSR enrollment state (ADR-060 SSR/CSR boundary).
	// Filters (level) are URL params → re-triggers load().
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import TrackCard from '$lib/components/learn/TrackCard.svelte';
	import { LoadingSkeleton, EmptyState } from '$lib/components/ui';
	import { Search } from 'lucide-svelte';
	import type { PageData } from './$types';
	import type { Enrollment } from '$lib/types';

	let { data }: { data: PageData } = $props();

	// CSR ONLY — fetch the user's enrollments to drive TrackCard action state.
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

	const enrollmentMap = $derived(new Map(enrollments.map((e) => [e.track_id, e])));
</script>

<svelte:head>
	<title>{t('learn.search_results')} · {t('app.name')}</title>
	<meta name="description" content={t('learn.search_placeholder')} />
	<link rel="canonical" href="https://learn.japaconnect.com/search" />
</svelte:head>

<div class="px-4 py-6 md:px-6 lg:px-8">
	<!-- Header -->
	<header class="mb-6">
		<h1 class="flex items-center gap-2 text-2xl font-bold text-[var(--color-text)]">
			<Search size={24} class="text-[var(--color-primary)]" />
			{t('learn.search_results')}
		</h1>
		{#if data.query}
			<p class="mt-1 text-sm text-[var(--color-muted)]">
				{t('learn.results_for')} "{data.query}" — {data.total}
			</p>
		{/if}
	</header>

	<!-- Level filter -->
	<div class="mb-6 flex flex-wrap gap-2">
		<a
			href="/search?q={encodeURIComponent(data.query)}"
			class="rounded-full px-3 py-1 text-sm {!data.levelFilter ? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]' : 'border border-[var(--color-border)] text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]'}"
		>
			{t('learn.overview')}
		</a>
		{#each ['beginner', 'intermediate', 'advanced'] as level}
			<a
				href="/search?q={encodeURIComponent(data.query)}&level={level}"
				class="rounded-full px-3 py-1 text-sm {data.levelFilter === level ? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]' : 'border border-[var(--color-border)] text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]'}"
			>
				{t(`level.${level}`)}
			</a>
		{/each}
	</div>

	<!-- Results grid -->
	{#if data.results.length === 0}
		<EmptyState
			title={t('learn.no_search_results')}
			description={t('learn.no_search_results_description')}
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

		<!-- Suggested tracks (no-results state) -->
		{#if data.suggested.length > 0}
			<section class="mt-8">
				<h2 class="mb-4 text-lg font-semibold text-[var(--color-text)]">{t('learn.search_suggested')}</h2>
				<div class="grid grid-cols-1 gap-5 sm:grid-cols-2 xl:grid-cols-3">
					{#each data.suggested as track (track.id)}
						{#if loadingEnrollments && data.user}
							<div class="overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
								<LoadingSkeleton class="aspect-video w-full rounded-none" />
								<div class="space-y-2 p-4">
									<LoadingSkeleton class="h-5 w-3/4" />
									<LoadingSkeleton class="h-4 w-1/2" />
								</div>
							</div>
						{:else}
							{@const en = enrollmentMap.get(track.id)}
							<TrackCard
								{track}
								user={data.user}
								enrolled={!!en}
								progressPct={en?.progress_pct ?? 0}
								isUniversity={data.isUniversity}
							/>
						{/if}
					{/each}
				</div>
			</section>
		{/if}
	{:else}
		<div class="grid grid-cols-1 gap-5 sm:grid-cols-2 xl:grid-cols-3">
			{#each data.results as track (track.id)}
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
						isUniversity={data.isUniversity}
					/>
				{/if}
			{/each}
		</div>
	{/if}
</div>
