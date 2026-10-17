<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { Search, Eye, MessageCircle, Share2, Plus, GitCompare } from 'lucide-svelte';
	import { i18n, t, siteName} from '$lib/i18n';
	import JceSelect from '$lib/components/ui/JceSelect.svelte';
	import { EmptyState, ListPageBar, PaginationControls, UrlTabs, type UrlTab, FilterToggle, PageAction } from '$lib/components/ui';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let items = $derived(data.items ?? []);
	let defaultPageSize = $derived(data.defaultPageSize ?? 12);
	let meta = $derived(data.meta ?? { total: 0, page: 1, limit: defaultPageSize });
	let types = $derived(data.types ?? []);
	let filters = $derived(data.filters);
	let loadError = $derived(data.loadError ?? '');
	let loadErrorParams = $derived(data.loadErrorParams ?? {});
	let user = $derived(data.user);

	let query = $state('');
	let queryInitialized = $state(false);

	$effect.pre(() => {
		if (!queryInitialized) {
			query = filters.q ?? '';
			queryInitialized = true;
		}
	});

	// Ref: sections/30-generic-comparison-system.md — sort/page-size options come from the API list meta.
	let sortValues = $derived(data.meta?.sort_options ?? []);
	let sortOptions = $derived(
		sortValues.map((value) => ({ value, label: t(`jce.comparisons.sort.${value}`) }))
	);

	let pageSizeOptions = $derived(data.meta?.page_size_options ?? []);

	let tabOptions = $derived<UrlTab[]>([
		{ value: 'public', label: t('jce.comparisons.tab_public') },
		{ value: 'mine', label: t('jce.comparisons.tab_mine') }
	]);

	// Filter panel (ListPageBar + FilterToggle pattern — same as /blog, /guides).
	// Panel-active = type filter or non-default sort — the always-visible search
	// box doesn't count (it's not in the panel).
	const hasActiveFilters = $derived(
		!!filters.filter_type || (!!filters.sort && filters.sort !== (sortValues[0] || 'created_at'))
	);
	let showFilters = $state(false);
	$effect(() => {
		if (hasActiveFilters) showFilters = true;
	});

	let typeSelectOptions = $derived([
		{ value: '', label: t('jce.comparisons.all_types') },
		...types.map((type) => ({ value: type.slug, label: type.name }))
	]);

	let pageSizeSelectOptions = $derived(
		pageSizeOptions.map((size) => ({
			value: String(size),
			label: `${size} ${t('jce.common.per_page')}`
		}))
	);

	function buildURL(updates: Record<string, string | number | null>) {
		const url = new URL(page.url);
		for (const [key, value] of Object.entries(updates)) {
			if (value === '' || value === null || value === undefined) {
				url.searchParams.delete(key);
			} else {
				url.searchParams.set(key, String(value));
			}
		}
		return url;
	}

	function apply() {
		const url = buildURL({ q: query, page: 1 });
		goto(url, { replaceState: true, keepFocus: true });
	}

	function setSort(sort: string) {
		goto(buildURL({ sort, page: 1 }), { replaceState: true });
	}

	function setPageSize(size: number) {
		goto(buildURL({ page_size: size, page: 1 }), { replaceState: true });
	}

	function setPage(pageNum: number) {
		goto(buildURL({ page: pageNum }), { replaceState: true });
	}

	function setFilterType(slug: string) {
		goto(buildURL({ filter_type: slug || null, page: 1 }), { replaceState: true });
	}

	function detailURL(item: (typeof items)[number]): string {
		return item.depth === 'quick' ? `/compare/${item.slug}` : `/comparisons/${item.slug}`;
	}

	function setTab(mine: boolean) {
		const updates: Record<string, string | number | null> = {
			mine: mine ? 'true' : null,
			filter_depth: null,
			filter_status: null,
			filter_visibility: null,
			page: 1
		};
		goto(buildURL(updates), { replaceState: true });
	}

	function excerpt(text?: string | null, max = 140) {
		if (!text) return '';
		// Strip simple HTML tags and truncate.
		const plain = text.replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();
		if (plain.length <= max) return plain;
		return plain.slice(0, max).replace(/\s+\S*$/, '') + '…';
	}

	function formatDate(iso?: string) {
		if (!iso) return '';
		return new Date(iso).toLocaleDateString(i18n.locale, { year: 'numeric', month: 'short', day: 'numeric' });
	}

	const totalPages = $derived(Math.max(1, Math.ceil((meta.total || 0) / (meta.limit || 1))));
	const currentPage = $derived(meta.page || 1);
	// List head: title/description, self-referencing canonical on the clean
	// path, and rel=prev/next on numbered pages (same pattern as /blog).
	const listTitle = $derived(t('jce.comparisons.heading', 'Comparisons') + ` — ${siteName()}`);
	const listDescription = $derived(
		t('jce.comparisons.seo_description', 'Compare destinations, visas, schools and more side by side.')
	);
	const isMine = $derived(filters.mine);

	let searchTimeout: ReturnType<typeof setTimeout> | null = null;
	function onSearchInput() {
		if (searchTimeout) clearTimeout(searchTimeout);
		searchTimeout = setTimeout(() => apply(), 300);
	}
</script>

<svelte:head>
	<title>{listTitle}</title>
	<meta name="description" content={listDescription} />
	<link rel="canonical" href={page.url.origin + '/comparisons'} />
	<meta property="og:title" content={listTitle} />
	<meta property="og:description" content={listDescription} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.origin + '/comparisons'} />
	{#if currentPage > 1}
		{@const compPrevUrl = (() => { const u = new URL(page.url.href); if (currentPage - 1 <= 1) u.searchParams.delete('page'); else u.searchParams.set('page', String(currentPage - 1)); return u.toString(); })()}
		<link rel="prev" href={compPrevUrl} />
	{/if}
	{#if currentPage < totalPages}
		{@const compNextUrl = (() => { const u = new URL(page.url.href); u.searchParams.set('page', String(currentPage + 1)); return u.toString(); })()}
		<link rel="next" href={compNextUrl} />
	{/if}
</svelte:head>

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	<h1 class="sr-only">{t('jce.comparisons.heading')}</h1>

	<!-- Tab + filter bar: ListPageBar pattern, same as /blog, /guides -->
	<ListPageBar>
		{#if user}
			<UrlTabs
				tabs={tabOptions}
				value={isMine ? 'mine' : 'public'}
				onValueChange={(value) => setTab(value === 'mine')}
				class="!border-b-0 !bg-transparent"
			/>
		{:else}
			<span class="flex items-center px-4 text-sm font-semibold text-[var(--color-text)]" style="min-height: var(--nav-height)">
				{t('jce.comparisons.heading')}
			</span>
		{/if}
		{#snippet actions()}
			<FilterToggle bind:show={showFilters} active={hasActiveFilters} controls="comparisons-filter-panel" />
			<PageAction
				href="/compare"
				icon={Plus}
				label={t('jce.comparisons.create')}
			/>
		{/snippet}
	</ListPageBar>

	<div class="mx-auto w-full max-w-3xl space-y-4 p-4">
		{#if loadError}
			<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
				{t(loadError, undefined, loadErrorParams)}
			</p>
		{/if}

		<div class="relative">
			<Search size={16} class="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" />
			<input
				type="text"
				bind:value={query}
				oninput={onSearchInput}
				placeholder={t('jce.comparisons.search_placeholder')}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-2 pl-9 pr-4 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			/>
		</div>

		{#if showFilters}
			<div
				id="comparisons-filter-panel"
				class="grid gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 sm:grid-cols-[auto_auto]"
			>
				<JceSelect
					options={sortOptions}
					value={filters.sort || sortValues[0] || 'created_at'}
					onchange={(value) => setSort(value)}
					placeholder={t('jce.comparisons.sort_placeholder', 'Sort by')}
				/>

				<JceSelect
					options={typeSelectOptions}
					value={filters.filter_type || ''}
					onchange={(value) => setFilterType(value)}
				/>
			</div>
		{/if}

		{#if items.length === 0}
			<EmptyState
				icon={GitCompare}
				title={isMine ? t('jce.comparisons.empty_mine') : t('jce.comparisons.empty_public')}
				action={isMine
					? { label: t('jce.comparisons.create_detailed'), href: '/create?type=comparison' }
					: { label: t('jce.comparisons.create_one'), href: '/compare' }}
			/>
		{:else}
			<div class="grid gap-3.5 sm:grid-cols-2">
				{#each items as item (item.id)}
					<article
						class="group rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<a href={detailURL(item)} class="block space-y-3">
							<div class="flex items-start justify-between gap-2">
								<h2 class="font-semibold leading-snug text-[var(--color-text)]">{item.title}</h2>
							</div>
							{#if item.excerpt}
								<p class="line-clamp-2 text-sm text-[var(--color-muted)]">{item.excerpt}</p>
							{:else}
								<p class="line-clamp-2 text-sm text-[var(--color-muted)]">{excerpt(item.body)}</p>
							{/if}

							<div class="flex flex-wrap items-center gap-2 text-xs text-[var(--color-muted)]">
								{#if item.type_name}
									<span class="rounded bg-[var(--color-surface-subtle)] px-2 py-0.5">{item.type_name}</span>
								{/if}
								<span class="rounded bg-[var(--color-surface-subtle)] px-2 py-0.5">{item.depth}</span>
								{#if isMine}
									<span class="rounded bg-[var(--color-surface-subtle)] px-2 py-0.5">{item.status}</span>
									<span class="rounded bg-[var(--color-surface-subtle)] px-2 py-0.5">{item.visibility}</span>
								{/if}
							</div>

							<div class="flex items-center gap-4 text-xs text-[var(--color-muted)]">
								<span>{formatDate(item.created_at)}</span>
								<span class="inline-flex items-center gap-1"><Eye size={12} /> {item.view_count ?? 0}</span>
								<span class="inline-flex items-center gap-1"><MessageCircle size={12} /> {item.comment_count ?? 0}</span>
								<span class="inline-flex items-center gap-1"><Share2 size={12} /> {item.share_count ?? 0}</span>
							</div>
						</a>
					</article>
				{/each}
			</div>

			<div class="flex flex-col items-center justify-between gap-4 border-t border-[var(--color-border)] pt-4 sm:flex-row">
				<JceSelect
					options={pageSizeSelectOptions}
					value={String(filters.page_size || meta.limit || pageSizeOptions[0] || defaultPageSize)}
					onchange={(value) => setPageSize(parseInt(value, 10))}
					class="w-auto min-w-[6rem] py-1"
				/>
				<PaginationControls page={currentPage} {totalPages} onPageChange={setPage} />
			</div>
		{/if}
	</div>
</div>
