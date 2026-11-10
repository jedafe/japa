<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { onDestroy } from 'svelte';
	import { Search, Eye, MessageCircle, Share2, GitCompare, Filter, ChevronRight } from 'lucide-svelte';
	import { i18n, t, siteName} from '#lib/i18n/index.js';
	import JceSelect from '#lib/components/ui/JceSelect.svelte';
	import { EmptyState, EntityHero, ListPageBar, PaginationControls, UrlTabs, GuideSlideOver, Modal, type GuideContent, type UrlTab } from '#lib/components/ui/index.js';
	import type { PageData } from './$types';
	import { api } from '#lib/api/client.js';
	import HeroArtCompare from '#lib/components/heroes/HeroArtCompare.svelte';

	import { ogCardUrl, ogMediaUrl } from '#lib/seo/og.js';
	let { data }: { data: PageData } = $props();

	// "How it works" slide-over — editable via comparisons.guide (admins).
	let guideOpen = $state(false);
	const fallbackGuide: GuideContent = {
		title: t('common.how_it_works', 'How it works'),
		body: t('comparisons.guide_body', ''),
		ctas: []
	};
	let guideContent = $state<GuideContent | null>(null);
	const effectiveGuide = $derived(guideContent ?? fallbackGuide);
	async function saveGuide(v: GuideContent): Promise<string | null> {
		try {
			const res = await api.put<{ comparisons_guide?: GuideContent }>('/admin/settings/general', { comparisons_guide: v });
			if (res.data?.comparisons_guide) guideContent = res.data.comparisons_guide;
			else guideContent = v;
			return null;
		} catch (e) {
			return e instanceof Error ? e.message : 'save_failed';
		}
	}

	let filtersModalOpen = $state(false);
	let searchModalOpen = $state(false);

	let items = $derived(data.items ?? []);
	let defaultPageSize = $derived(data.defaultPageSize ?? 12);
	let meta = $derived(data.meta ?? { total: 0, page: 1, limit: defaultPageSize });
	let types = $derived(data.types ?? []);
	let filters = $derived(data.filters);
	let loadError = $derived(data.loadError ?? '');
	let loadErrorParams = $derived(data.loadErrorParams ?? {});
	let user = $derived(data.user);

	// SSR-safe initial value — mirrors the resync effect below (same pattern
	// as /compare: a one-time URL snapshot, not a reactive read).
	const initialQuery = page.url.searchParams.get('q') ?? '';
	let query = $state(initialQuery);

	// Mirror the URL on SPA navigation (back/forward, cleared filters):
	// without resync the box shows the old term while results reflect the new.
	$effect(() => {
		query = filters.q ?? '';
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

	// Filters live in the right-side modal — hasActiveFilters drives the dot.
	// Panel-active = type filter or non-default sort — the always-visible search
	// box doesn't count (it's not in the panel).
	const hasActiveFilters = $derived(
		!!filters.filter_type || (!!filters.sort && filters.sort !== (sortValues[0] || 'created_at'))
	);

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
		const url = new URL(page.url.href);
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
		goto(url, { replace: true, reset: false });
	}

	function setSort(sort: string) {
		goto(buildURL({ sort, page: 1 }), { replace: true });
	}

	function setPageSize(size: number) {
		goto(buildURL({ page_size: size, page: 1 }), { replace: true });
	}

	function setPage(pageNum: number) {
		goto(buildURL({ page: pageNum }), { replace: true });
	}

	function setFilterType(slug: string) {
		goto(buildURL({ filter_type: slug || null, page: 1 }), { replace: true });
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
		goto(buildURL(updates), { replace: true });
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
	onDestroy(() => {
		if (searchTimeout) clearTimeout(searchTimeout);
	});
	function onSearchInput() {
		if (searchTimeout) clearTimeout(searchTimeout);
		searchTimeout = setTimeout(() => apply(), 300);
	}

	const seoOgImage = $derived(
		ogCardUrl(page.url.origin, 'comparisons')
	);
</script>

<svelte:head>
	<title>{listTitle}</title>
	<meta name="description" content={listDescription} />
	<link rel="canonical" href={page.url.origin + '/comparisons'} />
	<meta property="og:title" content={listTitle} />
	<meta property="og:description" content={listDescription} />
	<meta property="og:type" content="website" />
	<meta property="og:image" content={seoOgImage} />
	<meta name="twitter:card" content="summary_large_image" />
	<meta name="twitter:title" content={listTitle} />
	<meta name="twitter:description" content={listDescription} />
	<meta name="twitter:image" content={seoOgImage} />
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

	<!-- Header — EntityHero directory panel -->
	<EntityHero
		icon={GitCompare}
		heading={t('jce.comparisons.heading')}
		description={t('comparisons.hero_desc', 'Compare tools, universities, jobs and more side by side — see the differences that matter to your journey.')}
		accent="var(--color-primary)"
	>
		{#snippet actions(c)}
			<div class="grid w-full grid-cols-2 gap-2 sm:flex">
				<a href="/compare" class="{c.primary} flex-1 justify-center whitespace-nowrap">
					<GitCompare size={15} /> {t('jce.comparisons.create')}
				</a>
				<button type="button" onclick={() => (guideOpen = true)} class="{c.ghost} flex-1 justify-center whitespace-nowrap">
					{t('common.how_it_works', 'How it works')}
					<ChevronRight size={14} class="rtl:-scale-x-100" />
				</button>
			</div>
		{/snippet}
		{#snippet art()}
			<HeroArtCompare accent="var(--color-primary)" />
		{/snippet}
	</EntityHero>

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
			<button type="button" onclick={() => (filtersModalOpen = true)} class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]">
				{t('common.filters', 'Filters')}
				{#if hasActiveFilters}<span class="ms-0.5 h-2 w-2 rounded-full bg-[var(--color-primary)]" aria-hidden="true"></span>{/if}
				<Filter size={16} />
			</button>
			<button type="button" onclick={() => (searchModalOpen = true)} class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]">
				{t('common.search', 'Search')}
				{#if filters.q}<span class="ms-0.5 h-2 w-2 rounded-full bg-[var(--color-primary)]" aria-hidden="true"></span>{/if}
				<Search size={16} />
			</button>
		{/snippet}
	</ListPageBar>

	<Modal bind:open={filtersModalOpen} title={t('common.filters', 'Filters')} icon={Filter} size="sm">
		{#snippet children()}
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
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
		{/snippet}
	</Modal>

	<Modal bind:open={searchModalOpen} title={t('common.search', 'Search')} icon={Search} size="sm">
		{#snippet children()}
			<div class="relative">
				<Search size={16} class="absolute start-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" />
				<input
					type="text"
					bind:value={query}
					oninput={onSearchInput}
					placeholder={t('jce.comparisons.search_placeholder')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-2 ps-9 pe-4 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
			</div>
		{/snippet}
	</Modal>

	<div class="mx-auto w-full max-w-3xl space-y-4 p-4">
		{#if loadError}
			<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
				{t(loadError, undefined, loadErrorParams)}
			</p>
		{/if}

		{#if filters.q}
			<p class="text-xs text-[var(--color-muted)]">{t('jce.comparisons.search_placeholder')}: {filters.q}</p>
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
			<div class="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
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
								<span class="rounded bg-[var(--color-surface-subtle)] px-2 py-0.5">{t(`jce.compare.depth.${item.depth}`, item.depth)}</span>
								{#if isMine}
									<span class="rounded bg-[var(--color-surface-subtle)] px-2 py-0.5">{t(`jce.compare.status.${item.status}`, item.status)}</span>
									<span class="rounded bg-[var(--color-surface-subtle)] px-2 py-0.5">{t(`jce.compare.visibility.${item.visibility}`, item.visibility)}</span>
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

<GuideSlideOver
	bind:open={guideOpen}
	title={effectiveGuide.title}
	body={effectiveGuide.body}
	ctas={effectiveGuide.ctas}
	canManage={data.canManage}
	onSave={saveGuide}
/>
