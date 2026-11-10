<script lang="ts">
	import SeoHead from '#lib/seo/SeoHead.svelte';
	// /products — product directory surface (§80 Product Domain). Products
	// are comparison entities, not wiki kinds; each card links to the
	// /product/{slug} detail page. The Deals tab is the moved /deals
	// surface at /products/deals.
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { untrack } from 'svelte';
	import { t } from '#lib/i18n/index.js';
	import { api } from '#lib/api/client.js';
	import {
		Box,
		Briefcase,
		Cloud,
		Cpu,
		Globe,
		LayoutGrid,
		List,
		Loader2,
		Package,
		Plus,
		Search,
		Server,
		ShoppingCart,
		Smartphone,
		Sparkles,
		Star,
		Store,
		Tag,
		Wrench
	} from 'lucide-svelte';
	import { EmptyState, ListPageBar, PageAction, UrlTabs, type UrlTab } from '#lib/components/ui/index.js';
	import AutocompleteSelect from '#lib/components/ui/AutocompleteSelect.svelte';
	import type { PageData } from './$types';

import { ogCardUrl } from '#lib/seo/og.js';
	type ProductItem = PageData['items'][number];

	let { data }: { data: PageData } = $props();

	const title = $derived(t('products.title', 'Products — {site}'));
	const description = $derived(
		t('products.description', 'Browse products, tools and services tracked by {site}.')
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'CollectionPage',
			name: 'Products',
			description,
			url: page.url.href
		})
	);

	// Surface tabs — same UrlTabs/ListPageBar pattern as the deals sibling.
	const surfaceTabs = $derived<UrlTab[]>([
		{ value: 'products', label: t('products.tab_products', 'Products') },
		...(data.dealsEnabled ? [{ value: 'deals', label: t('products.tab_deals', 'Deals') }] : [])
	]);
	function goSurface(v: string) {
		goto(v === 'deals' ? '/products/deals' : '/products');
	}

	// "Load more" — SSR ships page N; the button appends the next pages
	// client-side (same convention as /products/deals). The effect re-seeds
	// on any search/type/page navigation so appended items never leak
	// across queries; untracked writes keep the list out of the effect's
	// dependency set.
	let items = $state(untrack(() => data.items));
	let loadPage = $state(untrack(() => data.page));
	let loadingMore = $state(false);
	let loadMoreError = $state(false);
	let hasMore = $state(untrack(() => data.items.length < data.total));

	$effect(() => {
		const fresh = data.items;
		const total = data.total;
		const p = data.page;
		untrack(() => {
			items = fresh;
			loadPage = p;
			loadingMore = false;
			loadMoreError = false;
			hasMore = fresh.length < total;
		});
	});

	// Server-side search state — seeded from (and re-synced to) data.q so
	// back/forward navigation restores the input.
	let searchInput = $state(untrack(() => data.q));
	let selectedType = $state(untrack(() => data.type));
	let selectedSort = $state('name');

	$effect(() => {
		const q = data.q;
		const ty = data.type;
		untrack(() => {
			if (document.activeElement?.id !== 'products-search') searchInput = q;
			selectedType = ty;
		});
	});

	const typeOptions = $derived([
		{ value: '', label: t('products.types_all', 'All') },
		...data.types.map((tp) => ({ value: tp.slug, label: tp.label }))
	]);

	const sortOptions = $derived([
		{ value: 'name', label: t('products.sort.name', 'Name A–Z') },
		{ value: 'rating', label: t('products.sort.rating', 'Top rated') },
		{ value: 'providers', label: t('products.sort.providers', 'Most providers') },
		{ value: 'price', label: t('products.sort.price', 'Lowest price') }
	]);

	// Client-side sort over the loaded pages (backend sorts by name; extra
	// orders apply to what's fetched — same stance as deals' category pills).
	const sortedItems = $derived.by(() => {
		const list = [...items];
		switch (selectedSort) {
			case 'rating':
				list.sort((a, b) => (b.rating_avg ?? -1) - (a.rating_avg ?? -1));
				break;
			case 'providers':
				list.sort((a, b) => b.provider_count - a.provider_count);
				break;
			case 'price':
				list.sort((a, b) => (a.price_from ?? Infinity) - (b.price_from ?? Infinity));
				break;
		}
		return list;
	});

	function setParam(url: URL, key: string, value: string) {
		if (value) url.searchParams.set(key, value);
		else url.searchParams.delete(key);
	}

	// Search/type push into the URL and let the route load re-fetch
	// server-side. window.location is the live URL — page.url is a
	// load-time snapshot.
	function applyFilters() {
		const url = new URL(window.location.href);
		setParam(url, 'q', searchInput.trim());
		setParam(url, 'type', selectedType);
		url.searchParams.delete('page');
		goto(`${url.pathname}${url.search}`, { reset: false });
	}

	let searchTimer: ReturnType<typeof setTimeout> | undefined;
	// A pending debounce must not outlive the page — a stray applyFilters()
	// after unmount would navigate the user mid-browse.
	$effect(() => () => clearTimeout(searchTimer));
	function onSearchInput() {
		clearTimeout(searchTimer);
		searchTimer = setTimeout(applyFilters, 350);
	}
	function onSearchKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			clearTimeout(searchTimer);
			applyFilters();
		}
	}

	async function loadMore() {
		if (loadingMore || !hasMore) return;
		loadingMore = true;
		loadMoreError = false;
		const params = new URLSearchParams({
			limit: String(data.pageSize),
			offset: String(loadPage * data.pageSize)
		});
		if (data.type) params.set('type', data.type);
		if (data.q) params.set('q', data.q);
		const res = await api.get<{ items: ProductItem[]; total: number }>(`/products?${params}`);
		loadingMore = false;
		if (!res.data) {
			loadMoreError = true;
			return;
		}
		items = [...items, ...res.data.items.filter((x) => !items.some((y) => y.id === x.id))];
		loadPage += 1;
		hasMore = items.length < res.data.total;
	}

	// Grid / list view — grid is the default (same as /products/deals).
	let viewMode = $state<'grid' | 'list'>('grid');

	// Type-tab icons. Icons are dynamic (product_types.icon — Rule 16), so
	// map the stored lucide name to a component with a Package fallback.
	const TYPE_ICONS: Record<string, typeof Package> = {
		sparkles: Sparkles,
		cpu: Cpu,
		cloud: Cloud,
		server: Server,
		smartphone: Smartphone,
		globe: Globe,
		briefcase: Briefcase,
		'shopping-cart': ShoppingCart,
		wrench: Wrench,
		box: Box,
		package: Package
	};
	function typeIcon(slug: string): typeof Package {
		return TYPE_ICONS[data.types.find((tp) => tp.slug === slug)?.icon ?? ''] ?? Package;
	}

	// Card header banner — products may not carry a logo, so use a
	// deterministic hue gradient + type watermark (deals-card pattern).
	function hueFor(id: string): number {
		let h = 0;
		for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) % 360;
		return h;
	}

	function productPath(it: ProductItem): string {
		return `/product/${encodeURIComponent(it.slug)}`;
	}

	function onRowClick(e: MouseEvent, it: ProductItem) {
		// The product name is a real anchor — let native navigation handle
		// it (middle-click/new tab); everything else on the row navigates.
		if ((e.target as HTMLElement).closest('a, button')) return;
		goto(productPath(it));
	}
	function onRowKeydown(e: KeyboardEvent, it: ProductItem) {
		if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			if ((e.target as HTMLElement).closest('a, button')) return;
			goto(productPath(it));
		}
	}
</script>

<SeoHead
	ogImage={ogCardUrl(page.url.origin, 'products')} title={title} description={description} ogTitle={title} jsonLd={jsonLd} />

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	{#snippet barActions()}
		<!-- Grid / list view toggle (same pattern as /products/deals) -->
		<div
			class="flex shrink-0 items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] p-0.5"
		>
			<button
				type="button"
				onclick={() => (viewMode = 'grid')}
				class="inline-flex h-7 w-7 items-center justify-center rounded-[calc(var(--radius)-2px)] transition-colors {viewMode ===
				'grid'
					? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]'
					: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
				aria-label={t('products.view_grid', 'Grid view')}
				aria-pressed={viewMode === 'grid'}
			>
				<LayoutGrid size={15} />
			</button>
			<button
				type="button"
				onclick={() => (viewMode = 'list')}
				class="inline-flex h-7 w-7 items-center justify-center rounded-[calc(var(--radius)-2px)] transition-colors {viewMode ===
				'list'
					? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]'
					: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
				aria-label={t('products.view_list', 'List view')}
				aria-pressed={viewMode === 'list'}
			>
				<List size={15} />
			</button>
		</div>
		{#if data.canCreate}
			<PageAction href="/products/new" icon={Plus} label={t('products.new_product', 'New product')} />
		{/if}
	{/snippet}
	{#if surfaceTabs.length > 1}
		<ListPageBar actions={barActions}>
			<UrlTabs
				tabs={surfaceTabs}
				value="products"
				onValueChange={goSurface}
				class="!border-b-0 !bg-transparent"
			/>
		</ListPageBar>
	{:else}
		<ListPageBar title={t('products.heading', 'Products')} actions={barActions} />
	{/if}

	<div class="mx-auto w-full max-w-3xl p-4">
		<!-- Search + type + sort toolbar (GET /products type/q params). -->
		<div class="mb-4 flex flex-col gap-3 lg:flex-row lg:items-center">
			<div class="relative lg:w-72">
				<Search
					size={16}
					class="pointer-events-none absolute top-1/2 start-3 -translate-y-1/2 text-[var(--color-text-subtle)]"
				/>
				<input
					id="products-search"
					type="search"
					bind:value={searchInput}
					oninput={onSearchInput}
					onkeydown={onSearchKeydown}
					placeholder={t('products.search_placeholder', 'Search products…')}
					aria-label={t('products.search_aria', 'Search products')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-2 pe-3 ps-9 text-sm text-[var(--color-text)] placeholder:text-[var(--color-text-subtle)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
			</div>
			<div class="grid flex-1 grid-cols-1 gap-3 sm:grid-cols-2">
				<AutocompleteSelect
					id="products-type"
					ariaLabel={t('products.type_filter', 'Product type')}
					placeholder={t('products.types_all', 'All')}
					options={typeOptions}
					bind:value={selectedType}
					onValueChange={() => applyFilters()}
				/>
				<AutocompleteSelect
					id="products-sort"
					ariaLabel={t('products.sort_label', 'Sort products')}
					placeholder={t('products.sort.name', 'Name A–Z')}
					options={sortOptions}
					bind:value={selectedSort}
				/>
			</div>
		</div>

		{#if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('products.load_error', 'Failed to load products. Please try again later.')}
			</p>
		{:else if items.length === 0}
			<EmptyState
				icon={Package}
				title={data.q || data.type
					? t('products.empty_filtered', 'No products match your filters.')
					: t('products.empty', 'No products yet.')}
				action={data.canCreate && !data.q && !data.type
					? { label: t('products.new_product', 'New product'), href: '/products/new' }
					: undefined}
			/>
		{:else}
			{#if data.types.length > 0}
				{#if viewMode === 'grid'}
					<p class="mb-5 text-center text-sm text-[var(--color-text-subtle)]">
						{t('products.subtitle', 'Tools, apps and services for the community')}
					</p>
				{/if}
				<nav aria-label={t('products.type_filter', 'Product type')} class="mb-6">
					<ul class="flex flex-wrap items-start justify-center gap-1 sm:gap-6">
						<li>
							<button
								type="button"
								onclick={() => {
									selectedType = '';
									applyFilters();
								}}
								aria-pressed={selectedType === ''}
								class="flex w-20 flex-col items-center gap-1.5 rounded-xl px-2 py-2 text-xs font-medium transition-colors {selectedType ===
								''
									? 'bg-[var(--color-primary)]/10 text-[var(--color-primary-text)]'
									: 'text-[var(--color-text-subtle)] hover:text-[var(--color-text)]'}"
							>
								<Tag size={20} />
								<span>{t('products.types_all', 'All')}</span>
							</button>
						</li>
						{#each data.types as tp (tp.slug)}
							{@const TabIcon = typeIcon(tp.slug)}
							<li>
								<button
									type="button"
									onclick={() => {
										selectedType = tp.slug;
										applyFilters();
									}}
									aria-pressed={selectedType === tp.slug}
									class="flex w-20 flex-col items-center gap-1.5 rounded-xl px-2 py-2 text-xs font-medium transition-colors {selectedType ===
									tp.slug
										? 'bg-[var(--color-primary)]/10 text-[var(--color-primary-text)]'
										: 'text-[var(--color-text-subtle)] hover:text-[var(--color-text)]'}"
								>
									<TabIcon size={20} />
									<span class="max-w-full truncate">{tp.label}</span>
								</button>
							</li>
						{/each}
					</ul>
				</nav>
			{/if}

			{#if viewMode === 'grid'}
				<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
					{#each sortedItems as it (it.id)}
						{@const hue = hueFor(it.id)}
						{@const CardIcon = typeIcon(it.discriminator ?? '')}
						<article
							class="overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] transition-shadow hover:shadow-md"
						>
							<a
								href={productPath(it)}
								aria-label={it.name}
								class="relative block h-36 overflow-hidden"
								style="background: linear-gradient(135deg, hsl({hue} 70% 85%), hsl({(hue + 40) %
									360} 70% 68%))"
							>
								{#if it.logo_url}
									<img
										src={it.logo_url}
										alt=""
										loading="lazy"
										decoding="async"
										class="absolute inset-0 h-full w-full object-cover"
									/>
								{:else}
									<CardIcon
										size={96}
										class="absolute -end-4 -bottom-4 text-white opacity-30"
									/>
								{/if}
								{#if it.type_label || it.discriminator}
									<span
										class="absolute top-3 start-3 inline-flex items-center gap-1 rounded-full bg-white px-2.5 py-1 text-xs font-bold text-gray-900 shadow"
									>
										{it.type_label ?? it.discriminator}
									</span>
								{/if}
							</a>
							<div class="p-4">
								<h2 class="text-[15px] font-bold text-[var(--color-text)]">
									<a
										href={productPath(it)}
										class="min-w-0 flex-1 truncate hover:underline"
									>
										{it.name}
									</a>
								</h2>
								<p class="mt-0.5 line-clamp-2 min-h-[2.5rem] text-[13px] text-[var(--color-text-subtle)]">
									{it.tagline ?? ''}
								</p>
								<p class="mt-1.5 flex items-center gap-3 text-xs font-medium text-[var(--color-success)]">
									{#if it.rating_avg != null}
										<span class="inline-flex items-center gap-1">
											<Star size={11} class="fill-amber-400 text-amber-400" />
											{it.rating_avg.toFixed(1)}{it.review_count > 0
												? ` (${it.review_count})`
												: ''}
										</span>
									{/if}
									{#if it.provider_count > 0}
										<span class="inline-flex items-center gap-1">
											<Store size={11} />
											{t('products.provider_count', '{n} providers', {
												n: it.provider_count
											})}
										</span>
									{/if}
									{#if it.price_from != null}
										<span class="text-[var(--color-text)]">
											{t('product.from_price', 'From {price}', {
												price: String(it.price_from)
											})}
										</span>
									{/if}
								</p>
								<a
									href={productPath(it)}
									class="mt-3 flex w-full items-center justify-center gap-1.5 rounded-full border-2 border-dashed border-[var(--color-primary)]/60 px-3 py-2 text-sm font-bold text-[var(--color-primary-text)] transition-colors hover:bg-[var(--color-primary)]/5"
								>
									{t('products.view_product', 'View product')}
								</a>
							</div>
						</article>
					{/each}
				</div>
			{:else}
				<!-- List view: table (Product / Type / Tagline / Meta / View) -->
				<div class="hidden overflow-x-auto sm:block">
					<table class="w-full border-collapse text-start text-sm">
						<thead>
							<tr
								class="border-b border-[var(--color-border)] text-xs uppercase tracking-wide text-[var(--color-text-subtle)]"
							>
								<th class="px-3 py-2 font-medium">{t('products.table.product', 'Product')}</th>
								<th class="px-3 py-2 font-medium">{t('products.table.type', 'Type')}</th>
								<th class="px-3 py-2 font-medium">{t('products.table.tagline', 'Tagline')}</th>
								<th class="px-3 py-2 font-medium">{t('products.table.meta', 'Meta')}</th>
								<th class="px-3 py-2 font-medium"><span class="sr-only">{t('products.view_product', 'View product')}</span></th>
							</tr>
						</thead>
						<tbody>
							{#each sortedItems as it (it.id)}
								<tr
									role="link"
									tabindex="0"
									onclick={(e) => onRowClick(e, it)}
									onkeydown={(e) => onRowKeydown(e, it)}
									class="cursor-pointer border-b border-[var(--color-border)] transition-colors hover:bg-[var(--color-surface-subtle)]"
								>
									<td class="px-3 py-3 font-medium text-[var(--color-text)]">
										<div class="flex items-center gap-2">
											<div
												class="grid h-8 w-8 shrink-0 place-items-center overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] text-[var(--color-muted)]"
											>
												{#if it.logo_url}
													<img src={it.logo_url} alt="" loading="lazy" class="h-full w-full object-cover" />
												{:else}
													<Package size={14} />
												{/if}
											</div>
											<a
												href={productPath(it)}
												class="font-medium text-[var(--color-text)] hover:text-[var(--color-primary-text)] hover:underline"
											>
												{it.name}
											</a>
										</div>
									</td>
									<td class="px-3 py-3">
										{#if it.type_label || it.discriminator}
											<span
												class="inline-flex items-center rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs font-medium text-[var(--color-text)]"
											>
												{it.type_label ?? it.discriminator}
											</span>
										{:else}
											<span class="text-[var(--color-text-subtle)]">—</span>
										{/if}
									</td>
									<td class="max-w-64 px-3 py-3">
										{#if it.tagline}
											<p class="line-clamp-1 text-xs text-[var(--color-text-subtle)]">
												{it.tagline}
											</p>
										{:else}
											<span class="text-[var(--color-text-subtle)]">—</span>
										{/if}
									</td>
									<td class="px-3 py-3 text-xs text-[var(--color-text-subtle)]">
										<div class="flex items-center gap-2">
											{#if it.rating_avg != null}
												<span class="inline-flex items-center gap-1">
													<Star size={11} class="fill-amber-400 text-amber-400" />
													{it.rating_avg.toFixed(1)}
												</span>
											{/if}
											{#if it.provider_count > 0}
												<span class="inline-flex items-center gap-1">
													<Store size={11} />
													{it.provider_count}
												</span>
											{/if}
											{#if it.price_from != null}
												<span class="font-medium text-[var(--color-text)]">
													{t('product.from_price', 'From {price}', {
														price: String(it.price_from)
													})}
												</span>
											{/if}
										</div>
									</td>
									<td class="px-3 py-3">
										<a
											href={productPath(it)}
											class="inline-flex items-center gap-1 text-xs font-medium text-[var(--color-primary-text)]"
										>
											{t('products.view_product', 'View product')}
										</a>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				<!-- Mobile: compact stacked rows, same click-through -->
				<div class="sm:hidden">
					{#each sortedItems as it (it.id)}
						<div
							class="flex items-start justify-between gap-3 border-b border-[var(--color-border)] py-3"
						>
							<a href={productPath(it)} class="min-w-0 flex-1">
								<div class="flex items-center gap-1.5">
									<h2 class="truncate text-sm font-semibold text-[var(--color-text)]">
										{it.name}
									</h2>
								</div>
								<p class="mt-0.5 truncate text-xs text-[var(--color-text-subtle)]">
									{it.type_label ?? ''}
								</p>
								{#if it.tagline}
									<p class="mt-1 line-clamp-1 text-xs text-[var(--color-text-subtle)]">
										{it.tagline}
									</p>
								{/if}
							</a>
							<div class="flex shrink-0 items-center gap-1 text-xs font-medium text-[var(--color-primary-text)]">
								{#if it.price_from != null}
									{t('product.from_price', 'From {price}', { price: String(it.price_from) })}
								{:else if it.rating_avg != null}
									<span class="inline-flex items-center gap-1">
										<Star size={12} class="fill-amber-400 text-amber-400" />
										{it.rating_avg.toFixed(1)}
									</span>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			{/if}

			{#if hasMore}
				<div class="mt-4 flex justify-center">
					<button
						type="button"
						onclick={loadMore}
						disabled={loadingMore}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>
						{#if loadingMore}
							<Loader2 size={14} class="animate-spin" />
						{/if}
						{t('common.load_more', 'Load more')}
					</button>
				</div>
			{/if}
			{#if loadMoreError}
				<p class="mt-2 text-center text-sm text-red-500" role="alert">
					{t('products.load_error', 'Failed to load products. Please try again later.')}
				</p>
			{/if}
		{/if}
	</div>
</div>
