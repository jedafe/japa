<script lang="ts">
	// Search results — Phase 10 (ROADMAP-LEARN.md).
	// SSR results grid + CSR enrollment state (ADR-060 SSR/CSR boundary).
	// Filters (level) are URL params → re-triggers load().
	import { t } from '$lib/i18n';
	import { goto } from '$app/navigation';
	import TrackCard from '$lib/components/learn/TrackCard.svelte';
	import { LoadingSkeleton, EmptyState } from '$lib/components/ui';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { Search, ChevronLeft, ChevronRight } from 'lucide-svelte';
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
	const enrollmentMap = $derived(new Map(enrollments.map((e) => [e.track_id, e])));

	// Build a query string that preserves the current search and active filters
	// (level, category, language, sort, free, page-size, offset).
	function buildHref(partial: Record<string, string | number | boolean> = {}) {
		const qs = new URLSearchParams();
		if (data.query) qs.set('q', data.query);
		if (data.levelFilter) qs.set('level', data.levelFilter);
		if (data.categoryFilter) qs.set('category', data.categoryFilter);
		if (data.languageFilter) qs.set('language', data.languageFilter);
		if (data.sortFilter) qs.set('sort', data.sortFilter);
		if (data.isFreeFilter) qs.set('is_free', 'true');
		if (data.limit !== 24) qs.set('limit', String(data.limit));
		if (data.offset) qs.set('offset', String(data.offset));

		for (const [key, value] of Object.entries(partial)) {
			if (value === false) qs.delete(key);
			else qs.set(key, String(value));
		}

		const s = qs.toString();
		return s ? `/search?${s}` : '/search';
	}

	const sortSelectOptions = $derived(
		(data.sortOptions ?? []).map((o) => ({ value: o.value, label: o.label }))
	);
	const languageSelectOptions = $derived([
		{ value: '', label: t('learn.all_languages', 'All languages') },
		...(data.languages ?? []).map((l) => ({ value: l, label: l.toUpperCase() }))
	]);
	const pageSizeSelectOptions = $derived(
		(data.pageSizes ?? []).map((o) => ({ value: o.value, label: o.label }))
	);

	const canGoPrev = $derived(data.offset > 0);
	const canGoNext = $derived(data.offset + data.results.length < data.total);
</script>

<svelte:head>
	<title>{t('learn.search_results')} · {t('app.name')}</title>
	<meta name="description" content={t('learn.search_placeholder')} />
	<link rel="canonical" href="https://university.japaconnect.com/search" />
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
				{t('learn.results_for_query', '{count} results for "{query}"')
					.replace('{count}', String(data.total))
					.replace('{query}', data.query)}
			</p>
		{/if}
	</header>

	<!-- Filters: level, category, free -->
	<div class="mb-6 flex flex-wrap items-center gap-2">
		<a
			href={buildHref({ level: false, offset: false })}
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

	<!-- Secondary filters: sort, language, page size — every dropdown uses
	     the shared AutocompleteSelect component (no native <select>). -->
	<div class="mb-6 flex flex-wrap items-center gap-3">
		{#if sortSelectOptions.length > 0}
			<div class="flex items-center gap-2">
				<span class="text-xs font-medium text-[var(--color-muted)]">{t('common.sort_by', 'Sort')}</span>
				<AutocompleteSelect
					options={sortSelectOptions}
					value={data.sortFilter || 'popular'}
					ariaLabel={t('common.sort_by', 'Sort')}
					class="w-40"
					onValueChange={(v) => goto(buildHref({ sort: v === 'popular' ? false : v, offset: false }))}
				/>
			</div>
		{/if}
		{#if languageSelectOptions.length > 1}
			<div class="flex items-center gap-2">
				<span class="text-xs font-medium text-[var(--color-muted)]">{t('learn.language', 'Language')}</span>
				<AutocompleteSelect
					options={languageSelectOptions}
					value={data.languageFilter}
					ariaLabel={t('learn.language', 'Language')}
					class="w-40"
					onValueChange={(v) => goto(buildHref({ language: v || false, offset: false }))}
				/>
			</div>
		{/if}
		{#if pageSizeSelectOptions.length > 0}
			<div class="ml-auto flex items-center gap-2">
				<span class="text-xs font-medium text-[var(--color-muted)]">{t('common.per_page', 'Per page')}</span>
				<AutocompleteSelect
					options={pageSizeSelectOptions}
					value={String(data.limit)}
					ariaLabel={t('common.per_page', 'Per page')}
					class="w-24"
					onValueChange={(v) => goto(buildHref({ limit: v === '24' ? false : v, offset: false }))}
				/>
			</div>
		{/if}
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
					{data.offset + 1} – {Math.min(data.offset + data.results.length, data.total)} of {data.total}
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
