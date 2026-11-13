<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { untrack } from 'svelte';
	import { Eye, MessageCircle, Share2, GitCompare, Filter, ChevronRight, LoaderCircle } from 'lucide-svelte';
	import { i18n, t, siteName} from '#lib/i18n/index.js';
	import JceSelect from '#lib/components/ui/JceSelect.svelte';
	import { EmptyState, EntityHero, ListPageBar, Modal, UrlTabs, GuideSlideOver, type GuideContent, type UrlTab } from '#lib/components/ui/index.js';
	import type { PageData } from './$types';
	import { api } from '#lib/api/client.js';
	import HeroArtCompare from '#lib/components/heroes/HeroArtCompare.svelte';
	import CompareRefineRail from '#lib/components/compare/CompareRefineRail.svelte';
	import ComparisonCardRail from '#lib/components/compare/ComparisonCardRail.svelte';
	import ComparisonVsCard from '#lib/components/compare/ComparisonVsCard.svelte';
	import { badgeFor } from '#lib/components/compare/compareBadge.js';
	import { FALLBACK_FAMILY_ICON, groupTypesByFamily } from '#lib/components/compare/compareFamilies.js';
	import { excerptText } from '#lib/utils/comparison.js';
	import type { JCEComparisonSummary } from '#lib/types/jce.js';

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

	let tabOptions = $derived<UrlTab[]>([
		{ value: 'public', label: t('jce.comparisons.tab_public') },
		{ value: 'mine', label: t('jce.comparisons.tab_mine') }
	]);

	// The Filter button shows/hides the "Refine results" rail — hidden by
	// default, but auto-opens when the URL already carries a rail-owned
	// filter (type facet or q) so the active state is never invisible.
	const hasActiveFilters = $derived(!!filters.filter_type || !!filters.q);
	// One-time URL snapshot (same pattern as initialQuery above), not a
	// subscription — SPA nav keeps the user's manual toggle state.
	const initiallyRefined = untrack(() => Boolean(data.filters?.filter_type || data.filters?.q));
	let refineOpen = $state(initiallyRefined);

	// Viewport gate: the Filter button opens the Refine Results content in a
	// modal on small screens and touch-primary devices (mobile + iPad —
	// pointer:coarse is the established codebase check for touch), as an
	// inline left rail on desktop.
	let isDesktop = $state(true);
	let touchPrimary = $state(false);
	$effect(() => {
		const mqWide = window.matchMedia('(min-width: 1024px)');
		const mqCoarse = window.matchMedia('(pointer: coarse)');
		isDesktop = mqWide.matches;
		touchPrimary = mqCoarse.matches;
		const onWide = (e: MediaQueryListEvent) => (isDesktop = e.matches);
		const onCoarse = (e: MediaQueryListEvent) => (touchPrimary = e.matches);
		mqWide.addEventListener('change', onWide);
		mqCoarse.addEventListener('change', onCoarse);
		return () => {
			mqWide.removeEventListener('change', onWide);
			mqCoarse.removeEventListener('change', onCoarse);
		};
	});
	const useFiltersModal = $derived(touchPrimary || !isDesktop);
	let filtersModalOpen = $state(false);
	// A modal left open across a resize into desktop or a switch to the Mine
	// tab (no refine there) must not linger.
	$effect(() => {
		if (!useFiltersModal || isMine) filtersModalOpen = false;
	});
	const refineActive = $derived(useFiltersModal ? filtersModalOpen : refineOpen);

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

	function setSort(sort: string) {
		goto(buildURL({ sort, page: 1 }), { replace: true });
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
		return excerptText(text, max);
	}

	function formatDate(iso?: string) {
		if (!iso) return '';
		return new Date(iso).toLocaleDateString(i18n.locale, { year: 'numeric', month: 'short', day: 'numeric' });
	}

	const totalPages = $derived(Math.max(1, Math.ceil((meta.total || 0) / (meta.limit || 1))));
	const currentPage = $derived(meta.page || 1);

	// Load more — the SSR list carries the requested page (?page=N, default
	// 12 rows); the button appends subsequent pages via the same list endpoint
	// and params, and hides once the fetched pages cover meta.total.
	let extraItems = $state<JCEComparisonSummary[]>([]);
	let loadedPages = $state(0);
	let loadingMore = $state(false);
	let reachedEnd = $state(false);
	let loadGen = 0;
	$effect(() => {
		data.items; // any SSR list change (filter/sort/tab nav) resets appends
		extraItems = [];
		loadedPages = 0;
		reachedEnd = false;
		loadGen += 1; // invalidate an in-flight append for the previous list
	});
	const gridItems = $derived([...items, ...extraItems]);
	const hasMore = $derived(
		!reachedEnd && (currentPage + loadedPages) * (meta.limit || 1) < (meta.total || 0)
	);
	const typesById = $derived(new Map(types.map((ty) => [ty.id, ty])));

	async function loadMore() {
		if (loadingMore || !hasMore) return;
		loadingMore = true;
		const gen = loadGen;
		try {
			const p = new URLSearchParams({
				page: String(currentPage + loadedPages + 1),
				page_size: String(meta.limit || defaultPageSize)
			});
			if (filters.sort) p.set('sort', filters.sort);
			if (filters.q) p.set('q', filters.q);
			if (filters.filter_type) p.set('filter_type', filters.filter_type);
			if (filters.filter_depth) p.set('filter_depth', filters.filter_depth);
			if (filters.filter_status) p.set('filter_status', filters.filter_status);
			if (filters.filter_visibility) p.set('filter_visibility', filters.filter_visibility);
			if (isMine) p.set('mine', 'true');
			const res = await api.get<JCEComparisonSummary[]>(`/jce/comparisons?${p}`);
			// Leave loadedPages untouched on failure — a retry refetches the
			// same page instead of silently skipping it.
			if (res.error) return;
			// The list changed mid-fetch (filter/sort nav) — drop the stale page.
			if (gen !== loadGen) return;
			const rows = res.data ?? [];
			// A short page is the last page, even if meta.total is stale.
			if (rows.length < (meta.limit || defaultPageSize)) reachedEnd = true;
			// Data can shift between the SSR fetch and this one — drop rows the
			// grid already has so keyed rendering never sees a duplicate id.
			const seen = new Set([...items, ...extraItems].map((c) => c.id));
			const fresh = rows.filter((c) => !seen.has(c.id));
			extraItems = [
				...extraItems,
				...fresh.map((c) => {
					const ty = c.type_id ? typesById.get(c.type_id) : undefined;
					return ty ? { ...c, type_slug: ty.slug, type_name: ty.name } : c;
				})
			];
			loadedPages += 1;
		} finally {
			loadingMore = false;
		}
	}
	// List head: title/description, self-referencing canonical on the clean
	// path, and rel=prev/next on numbered pages (same pattern as /blog).
	const listTitle = $derived(t('jce.comparisons.heading', 'Comparisons') + ` — ${siteName()}`);
	const listDescription = $derived(
		t('jce.comparisons.seo_description', 'Compare destinations, visas, schools and more side by side.')
	);
	const isMine = $derived(filters.mine);

	const seoOgImage = $derived(
		ogCardUrl(page.url.origin, 'comparisons')
	);

	// Type-facet rail — facets count every type under the active non-type
	// filters (the backend facet query ignores filter_type itself), so the
	// "All types" count sums facets rather than reading meta.total (which is
	// narrowed by an active type filter).
	let trendingItems = $derived(data.trendingItems ?? []);
	let popularItems = $derived(data.popularItems ?? []);
	let unfiltered = $derived(data.unfiltered ?? false);
	const facetTypes = $derived(meta.facets?.types ?? []);
	const facetCountBySlug = $derived.by(() => {
		const map: Record<string, number> = {};
		for (const f of facetTypes) if (f.slug) map[f.slug] = f.count;
		return map;
	});
	const allCount = $derived(
		facetTypes.length > 0
			? facetTypes.reduce((sum, f) => sum + f.count, 0)
			: filters.filter_type
				? undefined
				: (meta.total ?? 0)
	);

	function hrefOf(updates: Record<string, string | number | null>): string {
		const u = buildURL(updates);
		return u.pathname + u.search;
	}

	function onQuery(q: string) {
		goto(buildURL({ q: q || null, page: 1 }), { replace: true, reset: false });
	}

	const railCategories = $derived([
		{
			label: t('jce.comparisons.all_types'),
			count: allCount,
			href: hrefOf({ filter_type: null, page: 1 }),
			active: !filters.filter_type
		}
	]);

	// Group the type list by comparison family (Location, Education…) as a
	// rail accordion — one family open at a time, collapsed by default. The
	// open family is the active type filter's family when set (so the rail
	// reflects the URL), else Location. Uncatalogued types sit in "Other".
	const railGroups = $derived.by(() => {
		const activeKey = filters.filter_type
			? (types.find((ty) => ty.slug === filters.filter_type)?.config?.family ?? 'other')
			: 'location';
		const catOf = (type: (typeof types)[number]) => ({
			label: type.name,
			count: facetCountBySlug[type.slug] as number | undefined,
			href: hrefOf({ filter_type: type.slug, page: 1 }),
			active: filters.filter_type === type.slug
		});
		const { groups, ungrouped } = groupTypesByFamily(types);
		const out = groups.map((g) => ({
			key: g.family.key,
			label: t(`jce.compare.family.${g.family.key}`, g.family.key),
			icon: g.family.icon,
			tint: g.family.tint,
			expanded: g.family.key === activeKey,
			categories: g.types.map(catOf)
		}));
		if (ungrouped.length > 0) {
			out.push({
				key: 'other',
				label: t('jce.compare.family.other', 'Other'),
				icon: FALLBACK_FAMILY_ICON,
				tint: 'var(--color-muted)',
				expanded: activeKey === 'other',
				categories: ungrouped.map(catOf)
			});
		}
		return out;
	});
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

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 72rem)/2))">
	<h1 class="sr-only">{t('jce.comparisons.heading')}</h1>

	<!-- Header — EntityHero directory panel -->
	<EntityHero
		icon={GitCompare}
		heading={t('jce.comparisons.heading')}
		description={t('comparisons.hero_desc', 'Compare tools, universities, jobs and more side by side — see the differences that matter to your journey.')}
		accent="var(--color-primary)"
		crumbs={[
			{ label: t('common.home', 'Home'), href: '/' },
			{ label: t('jce.compare.heading'), href: '/compare' },
			{ label: t('jce.comparisons.heading') }
		]}
		class="mt-4"
	>
		{#snippet actions(c)}
			<div class="grid w-full grid-cols-2 gap-2 sm:flex">
				<a href="/compare" class="{c.primary} flex-1 justify-center whitespace-nowrap">
					<GitCompare size={15} /> {t('jce.comparisons.create')}
				</a>
				{#if user && !isMine}
					<a href="/comparisons?mine=true" class="{c.secondary} flex-1 justify-center whitespace-nowrap">
						{t('jce.comparisons.tab_mine', 'My comparisons')}
					</a>
				{/if}
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
			{#if !isMine}
				<button
					type="button"
					onclick={() => { if (useFiltersModal) filtersModalOpen = true; else refineOpen = !refineOpen; }}
					aria-expanded={refineActive}
					class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] {refineActive ? 'border-[var(--color-primary)]/50 text-[var(--color-primary-text)]' : ''}"
				>
					{t('common.filters', 'Filters')}
					{#if hasActiveFilters}<span class="ms-0.5 h-2 w-2 rounded-full bg-[var(--color-primary)]" aria-hidden="true"></span>{/if}
					<Filter size={16} />
				</button>
			{/if}
		{/snippet}
	</ListPageBar>

	<div class="mx-auto w-full max-w-6xl p-4">
		<div class="flex flex-col gap-6 lg:flex-row">
			{#if refineOpen && !useFiltersModal && !isMine}
				<CompareRefineRail bind:query categories={railCategories} groups={railGroups} {onQuery} initialOpen />
			{/if}

			<main class="min-w-0 flex-1 space-y-6 {isMine ? 'mx-auto w-full max-w-3xl' : ''}">
				{#if unfiltered && trendingItems.length > 0}
					<ComparisonCardRail
						heading={t('jce.compare.landing.trending', 'Trending comparisons')}
						items={trendingItems}
						authed={Boolean(user)}
					/>
				{/if}

				{#if unfiltered && popularItems.length > 0}
					<ComparisonCardRail
						heading={t('jce.compare.landing.popular', 'Popular comparisons')}
						items={popularItems}
						authed={Boolean(user)}
						badgeOf={(item, i) => badgeFor(item, { popularityIndex: i })}
					/>
				{/if}

				<section class="space-y-4">
					<div class="flex items-center justify-between gap-3">
						<h2 class="text-sm font-semibold text-[var(--color-text)]">
							{isMine ? t('jce.comparisons.tab_mine', 'My comparisons') : t('jce.comparisons.more', 'More comparisons')}
						</h2>
						<JceSelect
							options={sortOptions}
							value={filters.sort || sortValues[0] || 'created_at'}
							onchange={(value) => setSort(value)}
							placeholder={t('jce.comparisons.sort_placeholder', 'Sort by')}
							class="w-44"
						/>
					</div>

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
					<div class="grid grid-cols-1 gap-3.5 sm:grid-cols-2 {isMine ? '' : refineOpen && !useFiltersModal ? 'xl:grid-cols-3' : 'xl:grid-cols-4'}">
						{#each gridItems as item (item.id)}
							{#if !isMine}
								<ComparisonVsCard summary={item} authed={Boolean(user)} />
							{:else}
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
											<span class="rounded bg-[var(--color-surface-subtle)] px-2 py-0.5">{t(`jce.compare.status.${item.status}`, item.status)}</span>
											<span class="rounded bg-[var(--color-surface-subtle)] px-2 py-0.5">{t(`jce.compare.visibility.${item.visibility}`, item.visibility)}</span>
										</div>

										<div class="flex items-center gap-4 text-xs text-[var(--color-muted)]">
											<span>{formatDate(item.created_at)}</span>
											<span class="inline-flex items-center gap-1"><Eye size={12} /> {item.view_count ?? 0}</span>
											<span class="inline-flex items-center gap-1"><MessageCircle size={12} /> {item.comment_count ?? 0}</span>
											<span class="inline-flex items-center gap-1"><Share2 size={12} /> {item.share_count ?? 0}</span>
										</div>
									</a>
								</article>
							{/if}
						{/each}
					</div>

					{#if hasMore}
						<div class="flex justify-center border-t border-[var(--color-border)] pt-4">
							<button
								type="button"
								onclick={loadMore}
								disabled={loadingMore}
								class="inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-5 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-60"
							>
								{#if loadingMore}<LoaderCircle size={15} class="animate-spin" />{/if}
								{loadingMore ? t('common.loading', 'Loading') : t('common.load_more', 'Load more')}
							</button>
						</div>
					{/if}
				{/if}
				</section>
			</main>
		</div>
	</div>
</div>

{#if useFiltersModal && !isMine}
	<Modal
		bind:open={filtersModalOpen}
		title={t('jce.compare.category.refine', 'Refine results')}
		icon={Filter}
		size="sm"
	>
		{#snippet children()}
			<CompareRefineRail bind:query categories={railCategories} groups={railGroups} {onQuery} bare />
		{/snippet}
	</Modal>
{/if}

<GuideSlideOver
	bind:open={guideOpen}
	title={effectiveGuide.title}
	body={effectiveGuide.body}
	ctas={effectiveGuide.ctas}
	canManage={data.canManage}
	onSave={saveGuide}
/>
