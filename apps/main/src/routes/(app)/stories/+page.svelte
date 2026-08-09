<script lang="ts">
	// Ref: japaconnect-engineering/sections/79-community-story-wall.md
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { relativeTime } from '$lib/feed/time';
	import { i18n } from '$lib/i18n';
	import { BookOpen, BadgeCheck, PenSquare, ArrowRight, Loader2, X } from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import type { Story } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('stories.list_title', 'Community Stories — JapaConnect'));
	const description = $derived(
		t(
			'stories.list_description',
			'Real relocation stories from the Nigerian diaspora — challenges, resources, and advice.'
		)
	);

	// Accumulated stories for offset-based "load more" pagination.
	let stories = $state<Story[]>(untrack(() => data.stories));
	let loadingMore = $state(false);
	let hasMore = $state(untrack(() => data.stories.length >= data.pageSize));

	// Sync from server data on navigation (sort/filter changes re-run load).
	$effect(() => {
		stories = data.stories;
		hasMore = data.stories.length >= data.pageSize;
	});

	// Local filter input state, seeded from URL params.
	let originInput = $state(untrack(() => data.filters.origin));
	let destInput = $state(untrack(() => data.filters.dest));
	let visaInput = $state(untrack(() => data.filters.visa));
	let professionInput = $state(untrack(() => data.filters.profession));

	$effect(() => {
		originInput = data.filters.origin;
		destInput = data.filters.dest;
		visaInput = data.filters.visa;
		professionInput = data.filters.profession;
	});

	function buildParams(overrides: Record<string, string> = {}): URLSearchParams {
		const params = new URLSearchParams();
		const sort = overrides.sort ?? data.sort;
		const origin = overrides.origin ?? originInput;
		const dest = overrides.dest ?? destInput;
		const visa = overrides.visa ?? visaInput;
		const profession = overrides.profession ?? professionInput;
		if (sort && sort !== 'top') params.set('sort', sort);
		if (origin) params.set('origin', origin);
		if (dest) params.set('dest', dest);
		if (visa) params.set('visa', visa);
		if (profession) params.set('profession', profession);
		return params;
	}

	function navigate(params: URLSearchParams) {
		goto(`/stories${params.toString() ? `?${params.toString()}` : ''}`, { keepFocus: true });
	}

	function setSort(value: string) {
		navigate(buildParams({ sort: value }));
	}

	function applyFilters(e: SubmitEvent) {
		e.preventDefault();
		navigate(buildParams());
	}

	function clearFilters() {
		originInput = '';
		destInput = '';
		visaInput = '';
		professionInput = '';
		const params = new URLSearchParams();
		if (data.sort && data.sort !== 'top') params.set('sort', data.sort);
		navigate(params);
	}

	const hasActiveFilters = $derived(
		!!data.filters.origin || !!data.filters.dest || !!data.filters.visa || !!data.filters.profession
	);

	async function loadMore() {
		if (loadingMore || !hasMore) return;
		loadingMore = true;
		const params = buildParams();
		params.set('page_size', String(data.pageSize));
		params.set('offset', String(stories.length));
		const res = await api.get<Story[]>(`/stories?${params.toString()}`);
		loadingMore = false;
		if (res.data) {
			stories = [...stories, ...res.data];
			hasMore = res.data.length >= data.pageSize;
		}
	}

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<!-- Header -->
	<div class="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
		<div class="min-w-0">
			<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
				<BookOpen size={20} /> {t('stories.heading', 'Community Stories')}
			</h1>
			<p class="mt-1 text-sm text-[var(--color-muted)]">{description}</p>
		</div>
		{#if data.authed}
			<a
				href="/stories/new"
				class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				<PenSquare size={14} /> {t('stories.share', 'Share Your Story')}
			</a>
		{:else}
			<a
				href="/login?redirect=/stories/new"
				class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				<PenSquare size={14} /> {t('stories.share', 'Share Your Story')}
			</a>
		{/if}
	</div>

	<!-- Sort tabs + filter bar -->
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<!-- Sort tabs -->
		<div class="mb-3 flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] p-0.5 w-fit">
			<button
				type="button"
				onclick={() => setSort('top')}
				class="rounded-[calc(var(--radius)-2px)] px-3 py-1 text-xs font-medium transition-colors {data.sort ===
				'top'
					? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]'
					: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
				aria-pressed={data.sort === 'top'}
			>
				{t('stories.sort_top', 'Top')}
			</button>
			<button
				type="button"
				onclick={() => setSort('latest')}
				class="rounded-[calc(var(--radius)-2px)] px-3 py-1 text-xs font-medium transition-colors {data.sort ===
				'latest'
					? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]'
					: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
				aria-pressed={data.sort === 'latest'}
			>
				{t('stories.sort_latest', 'Latest')}
			</button>
		</div>

		<!-- Filters (country selects for origin/dest; text inputs for visa/profession) -->
		<form onsubmit={applyFilters} class="flex flex-col gap-2 lg:flex-row lg:items-center">
			<select
				value={originInput}
				onchange={(e) => (originInput = e.currentTarget.value)}
				class={inputClass}
				aria-label={t('stories.filter_origin', 'Origin country')}
			>
				<option value="">{t('stories.filter_origin_all', 'All origins')}</option>
				{#each data.countries as c (c.id)}
					<option value={c.id}>{c.name}</option>
				{/each}
			</select>
			<select
				value={destInput}
				onchange={(e) => (destInput = e.currentTarget.value)}
				class={inputClass}
				aria-label={t('stories.filter_dest', 'Destination country')}
			>
				<option value="">{t('stories.filter_dest_all', 'All destinations')}</option>
				{#each data.countries as c (c.id)}
					<option value={c.id}>{c.name}</option>
				{/each}
			</select>
			<input
				type="text"
				placeholder={t('stories.filter_visa', 'Visa type')}
				value={visaInput}
				oninput={(e) => (visaInput = e.currentTarget.value)}
				class={inputClass}
			/>
			<input
				type="text"
				placeholder={t('stories.filter_profession', 'Profession')}
				value={professionInput}
				oninput={(e) => (professionInput = e.currentTarget.value)}
				class={inputClass}
			/>
			<button
				type="submit"
				class="shrink-0 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:opacity-90"
			>
				{t('common.filter', 'Filter')}
			</button>
		</form>

		{#if hasActiveFilters}
			<div class="mt-2">
				<button
					type="button"
					onclick={clearFilters}
					class="inline-flex items-center gap-1 text-xs text-[var(--color-muted)] underline hover:text-[var(--color-text)]"
				>
					<X size={12} /> {t('common.clear_filters', 'Clear filters')}
				</button>
			</div>
		{/if}
	</div>

	<!-- Story list -->
	<div class="p-3">
		{#if data.loadError}
			<EmptyState
				icon={BookOpen}
				title={t('stories.load_error', 'Could not load stories')}
				body={t('stories.load_error_body', 'Please try again later.')}
			/>
		{:else if stories.length === 0}
			<EmptyState
				icon={BookOpen}
				title={t('stories.empty', 'No stories yet')}
				body={t('stories.empty_body', 'Be the first to share your relocation journey.')}
				action={data.authed
					? { label: t('stories.share', 'Share Your Story'), href: '/stories/new' }
					: { label: t('stories.share', 'Share Your Story'), href: '/login?redirect=/stories/new' }}
			/>
		{:else}
			<div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
				{#each stories as story (story.id)}
					<a
						href="/stories/{story.slug}"
						class="flex flex-col gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<div class="flex items-start justify-between gap-2">
							<h2 class="line-clamp-2 text-sm font-semibold text-[var(--color-text)]">{story.title}</h2>
							{#if story.verified}
								<BadgeCheck size={16} class="shrink-0 text-[var(--color-primary)]" />
							{/if}
						</div>

						{#if story.origin_space_name || story.destination_space_name}
							<p class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
								<span>{story.origin_space_name ?? '—'}</span>
								<ArrowRight size={12} />
								<span>{story.destination_space_name ?? '—'}</span>
							</p>
						{/if}

						{#if story.biggest_challenge}
							<p class="line-clamp-3 text-xs text-[var(--color-text-subtle)]">
								{story.biggest_challenge}
							</p>
						{/if}

						<div class="mt-auto flex flex-wrap items-center gap-2 text-xs text-[var(--color-muted)]">
							{#if story.relocation_year}
								<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{story.relocation_year}</span>
							{/if}
							{#if story.visa_category}
								<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{story.visa_category}</span>
							{/if}
							{#if story.profession}
								<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{story.profession}</span>
							{/if}
						</div>

						<div class="flex items-center gap-2 border-t border-[var(--color-border)] pt-2 text-xs text-[var(--color-muted)]">
							<span class="font-medium text-[var(--color-text-subtle)]">@{story.author_name}</span>
							<span>·</span>
							<span>{relativeTime(story.created_at, new Date(), i18n.locale)}</span>
						</div>
					</a>
				{/each}
			</div>

			{#if hasMore}
				<div class="mt-4 flex justify-center">
					<button
						type="button"
						onclick={loadMore}
						disabled={loadingMore}
						class="inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>
						{#if loadingMore}
							<Loader2 size={16} class="animate-spin" />
							{t('stories.loading', 'Loading…')}
						{:else}
							{t('stories.load_more', 'Load more')}
						{/if}
					</button>
				</div>
			{/if}
		{/if}
	</div>
</div>
