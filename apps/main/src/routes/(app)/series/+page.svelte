<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { BookOpen, ArrowRight, Search, X } from 'lucide-svelte';
	import {
		EmptyState,
		ListPageBar,
		UrlTabs,
		type UrlTab,
		FilterToggle
	} from '$lib/components/ui';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let q = $state('');
	let showFilters = $state(false);
	$effect(() => {
		q = data.q ?? '';
		if (data.q || data.contentType) showFilters = true;
	});

	const title = $derived(t('series.title', 'Series — JapaConnect'));
	const description = $derived(
		t('series.description', 'Browse curated, multi-part series on JapaConnect.')
	);

	const tabs = $derived<UrlTab[]>([
		{ value: 'all', label: t('series.tab_all', 'All series') },
		{ value: 'mine', label: t('series.tab_mine', 'My series') }
	]);

	const sortOptions = $derived([
		{ value: 'latest', label: t('series.sort_latest', 'Latest') },
		{ value: 'oldest', label: t('series.sort_oldest', 'Oldest') },
		{ value: 'title', label: t('series.sort_title', 'Title A–Z') }
	]);

	const typeOptions = $derived([
		{ value: '', label: t('series.filter_type_all', 'All types') },
		...data.contentTypes.map((c) => ({ value: c.value, label: c.label }))
	]);

	const hasActiveFilters = $derived(!!(data.q || data.contentType));

	const hasNext = $derived(
		data.meta?.total != null
			? data.page * data.pageSize < data.meta.total
			: data.series.length >= data.pageSize
	);

	function buildUrl(next: {
		tab?: string;
		page?: number;
		q?: string;
		sort?: string;
		contentType?: string;
	}): string {
		const params = new URLSearchParams();
		const tab = next.tab ?? data.tab;
		const qq = next.q ?? data.q;
		const sort = next.sort ?? data.sort;
		const contentType = next.contentType ?? data.contentType;
		const p = next.page ?? data.page;
		if (tab !== 'all') params.set('tab', tab);
		if (qq) params.set('q', qq);
		if (sort && sort !== 'latest') params.set('sort', sort);
		if (contentType) params.set('content_type', contentType);
		if (p > 1) params.set('page', String(p));
		const qs = params.toString();
		return qs ? `/series?${qs}` : '/series';
	}

	function switchTab(value: string) {
		goto(buildUrl({ tab: value, page: 1 }), { keepFocus: true });
	}

	function setSort(value: string) {
		goto(buildUrl({ sort: value, page: 1 }), { keepFocus: true });
	}

	function setContentType(value: string) {
		goto(buildUrl({ contentType: value, page: 1 }), { keepFocus: true });
	}

	function applySearch() {
		goto(buildUrl({ q: q.trim(), page: 1 }));
	}

	function clearFilters() {
		q = '';
		goto(buildUrl({ q: '', contentType: '', page: 1 }));
	}
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
	<h1 class="sr-only">{t('series.heading', 'Series')}</h1>
	<ListPageBar>
		<UrlTabs {tabs} value={data.tab} onValueChange={switchTab} class="!border-b-0 !bg-transparent" />
		{#snippet actions()}
			<FilterToggle bind:show={showFilters} active={hasActiveFilters} controls="series-filter-panel" />
		{/snippet}
	</ListPageBar>

	<div class="p-3">
		{#if showFilters}
			<!-- Filters: content type + sort via AutocompleteSelect (Rule 16 —
			     type options are server-fetched from GET /content-types). -->
			<div
				id="series-filter-panel"
				class="mb-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
			>
				<div class="grid gap-2 sm:grid-cols-[1fr_auto_auto]">
					<div class="relative min-w-0">
						<input
							id="series-search"
							type="search"
							bind:value={q}
							placeholder={t('series.search_placeholder', 'Search series...')}
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 pl-9 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
							onkeydown={(e) => e.key === 'Enter' && applySearch()}
						/>
						<Search
							size={16}
							class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]"
						/>
					</div>
					<AutocompleteSelect
						value={data.contentType}
						options={typeOptions}
						onValueChange={setContentType}
						class="w-full sm:w-44"
						ariaLabel={t('series.filter_type', 'Content type')}
					/>
					<AutocompleteSelect
						value={data.sort}
						options={sortOptions}
						onValueChange={setSort}
						class="w-full sm:w-36"
						ariaLabel={t('series.sort_by', 'Sort by')}
					/>
				</div>

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
		{/if}

		{#if data.tab === 'mine' && !data.authed}
			<EmptyState
				icon={BookOpen}
				title={t('series.mine_sign_in', 'Sign in to see your series')}
				description={t('series.mine_sign_in_hint', 'Your series live under your account.')}
			/>
			<div class="mt-3 text-center">
				<a
					href="/login?redirect=%2Fseries%3Ftab%3Dmine"
					class="text-sm font-medium text-[var(--color-primary)] hover:underline"
				>
					{t('series.sign_in', 'Sign in')}
				</a>
			</div>
		{:else if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('series.load_error', 'Failed to load series. Please try again later.')}
			</p>
		{:else if data.series.length === 0}
			<EmptyState
				icon={BookOpen}
				title={data.tab === 'mine'
					? t('series.mine_empty', 'You have no series yet')
					: t('series.empty', 'No series yet')}
			/>
		{:else}
			<div class="grid gap-3 sm:grid-cols-2">
				{#each data.series as series (series.id)}
					<a
						href="/series/{series.slug}"
						class="group flex flex-col rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<div class="flex items-start gap-3">
							{#if series.cover_image_url}
								<img
									src={series.cover_image_url}
									alt=""
									class="h-10 w-10 shrink-0 rounded-[var(--radius)] object-cover"
								/>
							{:else}
								<span
									class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
								>
									<BookOpen size={20} />
								</span>
							{/if}
							<div class="min-w-0 flex-1">
								<h2 class="truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]">
									{series.title}
								</h2>
								{#if series.status}
									<span class="mt-0.5 inline-block text-xs capitalize text-[var(--color-muted)]">
										{series.status}
									</span>
								{/if}
							</div>
						</div>
						{#if series.description}
							<p class="mt-3 line-clamp-2 flex-1 text-sm text-[var(--color-text-subtle)]">
								{series.description}
							</p>
						{/if}
						<div class="mt-3 flex items-center justify-end text-xs text-[var(--color-text-subtle)]">
							<span class="flex items-center gap-1 group-hover:text-[var(--color-primary)]">
								{t('series.view', 'View series')} <ArrowRight size={14} />
							</span>
						</div>
					</a>
				{/each}
			</div>

			{#if data.meta?.total != null && data.meta.total > data.pageSize}
				<div class="mt-4 flex items-center justify-center gap-4">
					<a
						href={buildUrl({ page: Math.max(1, data.page - 1) })}
						class="rounded-lg border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 {data.page <= 1 ? 'pointer-events-none opacity-40' : ''}"
						aria-disabled={data.page <= 1}
					>
						{t('series.prev_page', 'Previous')}
					</a>
					<span class="text-sm text-[var(--color-text-subtle)]">{data.page}</span>
					<a
						href={buildUrl({ page: data.page + 1 })}
						class="rounded-lg border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 {!hasNext ? 'pointer-events-none opacity-40' : ''}"
						aria-disabled={!hasNext}
					>
						{t('series.next_page', 'Next')}
					</a>
				</div>
			{/if}
		{/if}
	</div>
</div>
