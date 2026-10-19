<script lang="ts">
	// Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md (Deal Pages)
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto, replaceState } from '$app/navigation';
	import { t, i18n } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { Ticket, Tag, House, Scissors, Music, UtensilsCrossed, LayoutGrid, List, Copy, Check, BadgeCheck, ExternalLink, Plus, Loader2, Search, Pencil } from 'lucide-svelte';
	import { EmptyState, ListPageBar, Modal, PageAction } from '$lib/components/ui';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import DealForm from '$lib/components/deals/DealForm.svelte';
	import type { PageData } from './$types';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';
	import { dealPath, type Coupon } from '$lib/types';
	import { currencySymbol } from '$lib/utils/currency';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('deals.title', 'Deals — {site}'));
	const description = $derived(
		t('deals.description', 'Browse active deals and discount codes from {site} partners.')
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'CollectionPage',
			name: 'Deals',
			description,
			url: page.url.href
		})
	);

	// Category pills derive from the loaded coupons' provider_category values
	// (Rule 16 — no hardcoded option lists); uncategorized deals only appear
	// under "All". Values are trimmed and deduped case-insensitively (first-seen
	// casing wins) so ' SaaS '/'saas' don't render as two pills. Selection
	// syncs to ?category= via SvelteKit replaceState — a shallow URL update,
	// not a navigation.
	// Page 1 arrives via SSR; "Load more" appends /coupons?page=N client-side.
	// The $effect re-seeds whenever route data changes (search/filter/sort
	// navigation replaces data.coupons wholesale); untracked writes keep the
	// appended pages out of the effect's dependency set.
	let coupons = $state(untrack(() => data.coupons));
	let loadPage = $state(1);
	let loadingMore = $state(false);
	let loadMoreError = $state(false);
	let hasMore = $state(untrack(() => data.coupons.length === data.pageSize));

	$effect(() => {
		const fresh = data.coupons;
		const size = data.pageSize;
		untrack(() => {
			coupons = fresh;
			loadPage = 1;
			loadMoreError = false;
			hasMore = fresh.length === size;
		});
	});

	// Server-side search/filter/sort state — seeded from (and re-synced to)
	// data.filters so back/forward navigation restores the controls.
	let searchInput = $state(untrack(() => data.filters.q));
	let selectedProvider = $state(untrack(() => data.filters.providerId));
	let selectedType = $state(untrack(() => data.filters.couponType));
	let selectedSort = $state(untrack(() => data.filters.sort));

	$effect(() => {
		const f = data.filters;
		untrack(() => {
			// Don't clobber in-flight typing when the filter response lands.
			if (document.activeElement?.id !== 'deals-search') searchInput = f.q;
			selectedProvider = f.providerId;
			selectedType = f.couponType;
			selectedSort = f.sort;
			// The pill state lives in ?category= — re-sync it too (back/forward).
			selectedCategory =
				new URL(window.location.href).searchParams.get('category') ?? '';
		});
	});

	const serverFiltersActive = $derived(
		!!(data.filters.q || data.filters.providerId || data.filters.couponType)
	);

	const providerOptions = $derived([
		{ value: '', label: t('deals.all_providers', 'All providers') },
		...data.partners.map((p) => ({
			value: p.id,
			label: p.name,
			sublabel: p.category ?? undefined
		}))
	]);
	const typeOptions = $derived([
		{ value: '', label: t('deals.all_types', 'All types') },
		{ value: 'external', label: t('deals.type.external', 'External') },
		{ value: 'internal', label: t('deals.type.internal', 'Internal') }
	]);
	const sortOptions = $derived([
		{ value: 'newest', label: t('deals.sort.newest', 'Newest') },
		{ value: 'expiring', label: t('deals.sort.expiring', 'Expiring soon') },
		{ value: 'popular', label: t('deals.sort.popular', 'Most popular') }
	]);

	function setParam(url: URL, key: string, value: string) {
		if (value) url.searchParams.set(key, value);
		else url.searchParams.delete(key);
	}

	// Push the control state into the URL and let the route load re-fetch.
	// window.location is the live URL — page.url is a load-time snapshot and
	// misses category-pill replaceState updates.
	function applyFilters() {
		const url = new URL(window.location.href);
		setParam(url, 'q', searchInput.trim());
		setParam(url, 'provider_id', selectedProvider);
		setParam(url, 'type', selectedType);
		setParam(url, 'sort', selectedSort === 'newest' ? '' : selectedSort);
		goto(`${url.pathname}${url.search}`, { keepFocus: true, noScroll: true });
	}

	let searchTimer: ReturnType<typeof setTimeout> | undefined;
	// A pending debounce must not outlive the page — a stray applyFilters() after
	// unmount would navigate the user back to /deals mid-browse.
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

	// The query string for appended pages mirrors the server load — data.filters
	// (the applied params), not the possibly in-flight control state.
	function couponsQuery(pageNum: number): string {
		const p = new URLSearchParams();
		p.set('page', String(pageNum));
		p.set('page_size', String(data.pageSize));
		if (data.filters.q) p.set('q', data.filters.q);
		if (data.filters.providerId) p.set('provider_id', data.filters.providerId);
		if (data.filters.couponType) p.set('type', data.filters.couponType);
		if (data.filters.sort !== 'newest') p.set('sort', data.filters.sort);
		return p.toString();
	}

	async function loadMore() {
		if (loadingMore || !hasMore) return;
		loadingMore = true;
		loadMoreError = false;
		try {
			const res = await api.get<Coupon[]>(`/coupons?${couponsQuery(loadPage + 1)}`);
			if (!res.data) {
				loadMoreError = true;
				return;
			}
			coupons = [...coupons, ...res.data.filter((c) => !coupons.some((x) => x.id === c.id))];
			loadPage += 1;
			hasMore = res.data.length === data.pageSize;
		} finally {
			loadingMore = false;
		}
	}

	const categories = $derived.by(() => {
		const seen = new Map<string, string>();
		for (const c of coupons) {
			const cat = c.provider_category?.trim();
			if (!cat) continue;
			const key = cat.toLowerCase();
			if (!seen.has(key)) seen.set(key, cat);
		}
		return [...seen.values()].sort((a, b) => a.localeCompare(b));
	});
	let selectedCategory = $state(page.url.searchParams.get('category') ?? '');
	const filteredCoupons = $derived(
		selectedCategory
			? coupons.filter(
					(c) =>
						(c.provider_category?.trim().toLowerCase() ?? '') ===
						selectedCategory.toLowerCase()
				)
			: coupons
	);

	function selectCategory(cat: string) {
		selectedCategory = cat;
		// page.url is the load-time snapshot — build from the live location so
		// consecutive pill clicks don't drop params set since the last nav.
		const url = new URL(window.location.href);
		if (cat) {
			url.searchParams.set('category', cat);
		} else {
			url.searchParams.delete('category');
		}
		replaceState(url, {});
	}

	// "Last updated" in the page bar — newest updated_at across the listing.
	const lastUpdated = $derived.by(() => {
		let max = '';
		for (const c of coupons) {
			if (c.updated_at && c.updated_at > max) max = c.updated_at;
		}
		return max ? new Date(max).toLocaleDateString(i18n.locale) : '';
	});

	function discountLabel(c: Coupon): string {
		if (c.discount_type === 'percent') {
			return t('coupons.percent_off', '{n}% off').replace('{n}', String(c.discount_value ?? ''));
		}
		if (c.discount_type === 'fixed') {
			const cur = currencySymbol(c.currency, data.currencies) || (c.currency ?? '');
			return t('coupons.amount_off', '{cur}{n} off')
				.replace('{cur}', cur)
				.replace('{n}', String(c.discount_value ?? ''));
		}
		if (c.discount_type === 'free_shipping') {
			return t('coupons.free_shipping', 'Free shipping');
		}
		if (c.discount_value !== undefined && c.discount_value !== null) {
			return String(c.discount_value);
		}
		return '';
	}

	function providerName(c: Coupon): string {
		return c.provider_name ?? t('deals.internal', '{site}');
	}

	// Grid / list view — grid is the default (matches the Local Deals mockup).
	let viewMode = $state<'grid' | 'list'>('grid');

	// Mockup category-tab icons. Categories are dynamic (Rule 16 — derived from
	// provider_category), so map by keyword with a Ticket fallback.
	function categoryIcon(cat: string): typeof Ticket {
		const c = cat.toLowerCase();
		if (/apart|hous|rent|estate|hotel|stay|room|bnb/.test(c)) return House;
		if (/barber|salon|beauty|spa|hair|nail/.test(c)) return Scissors;
		if (/event|music|concert|show|band|party|night|club/.test(c)) return Music;
		if (/food|eat|drink|restaur|caf|bar|pizza|burger|bakery|coffee|dining/.test(c))
			return UtensilsCrossed;
		return Ticket;
	}

	// Card header banner — coupons carry no image URL (only a provider logo
	// media id), so use a deterministic hue gradient + category watermark.
	function hueFor(id: string): number {
		let h = 0;
		for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) % 360;
		return h;
	}

	// Compact badge label for the card header ("-20%", "-$5").
	function discountShort(c: Coupon): string {
		if (c.discount_type === 'percent') {
			return `-${c.discount_value ?? ''}%`;
		}
		if (c.discount_type === 'fixed') {
			const cur = currencySymbol(c.currency, data.currencies) || (c.currency ?? '');
			return `-${cur}${c.discount_value ?? ''}`;
		}
		return discountLabel(c);
	}

	// Mockup card: bold business name + offer subline.
	function cardTitle(c: Coupon): string {
		return c.provider_name ?? c.title;
	}

	function cardSub(c: Coupon): string {
		if (!c.provider_name) return c.description ?? '';
		if (c.description) return c.description;
		return c.title !== c.provider_name ? c.title : '';
	}

	function expiryDate(c: Coupon): string {
		return c.expires_at ? new Date(c.expires_at).toLocaleDateString(i18n.locale) : '';
	}

	let copiedId = $state<string | null>(null);
	let editOpen = $state(false);
	let editingCoupon = $state<Coupon | null>(null);

	// Staff inline edit (coupons.manage — same gate as the /deal detail page).
	// Stops propagation so list-row click-through doesn't fire underneath.
	function openEdit(e: MouseEvent, coupon: Coupon) {
		e.preventDefault();
		e.stopPropagation();
		editingCoupon = coupon;
		editOpen = true;
	}

	function closeEdit() {
		editOpen = false;
		editingCoupon = null;
	}

	function onEditSaved(updated: Coupon) {
		coupons = coupons.map((c) => (c.id === updated.id ? updated : c));
		closeEdit();
	}

	async function copyCouponCode(coupon: Coupon) {
		if (!coupon.code) return;
		let ok = false;
		try {
			await navigator.clipboard.writeText(coupon.code);
			ok = true;
		} catch {
			try {
				const ta = document.createElement('textarea');
				ta.value = coupon.code;
				ta.setAttribute('readonly', '');
				ta.style.position = 'fixed';
				ta.style.opacity = '0';
				document.body.appendChild(ta);
				ta.select();
				ok = document.execCommand('copy');
				ta.remove();
			} catch {
				ok = false;
			}
		}
		if (!ok) return;
		copiedId = coupon.id;
		api.post(`/coupons/${coupon.id}/click`, { event_type: 'copy' }).catch(() => {});
		setTimeout(() => {
			if (copiedId === coupon.id) copiedId = null;
		}, 2000);
	}

	function onRowClick(e: MouseEvent, coupon: Coupon) {
		// The deal title is a real anchor — let its native navigation handle
		// clicks (incl. middle-click / open-in-new-tab) instead of goto().
		// The staff edit button stops propagation itself; the closest('button')
		// guard covers keyboard-activated clicks that bubble from it.
		if ((e.target as HTMLElement).closest('a, button')) return;
		goto(dealPath(coupon));
	}

	function onRowKeydown(e: KeyboardEvent, coupon: Coupon) {
		// role="link" rows activate on Enter AND Space per APG.
		if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			if ((e.target as HTMLElement).closest('a, button')) return;
			goto(dealPath(coupon));
		}
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<link rel="canonical" href="{page.url.origin}/deals" />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	{@html safeJsonLdScript(jsonLd)}
</svelte:head>

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	<ListPageBar title={t('deals.heading', 'Deals')}>
		{#snippet actions()}
			<!-- Grid / list view toggle (same pattern as /universities) -->
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
					aria-label={t('deals.view_grid', 'Grid view')}
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
					aria-label={t('deals.view_list', 'List view')}
					aria-pressed={viewMode === 'list'}
				>
					<List size={15} />
				</button>
			</div>
			{#if lastUpdated}
				<span class="hidden text-xs text-[var(--color-text-subtle)] sm:inline">
					{t('deals.last_updated', 'Last updated {date}', { date: lastUpdated })}
				</span>
			{/if}
			{#if data.canSubmit}
				<PageAction href="/deals/new" icon={Plus} label={t('deals.new_deal', 'New deal')} />
			{/if}
		{/snippet}
	</ListPageBar>

	<div class="mx-auto w-full max-w-3xl p-4">
		<!-- Server-side search + filter + sort toolbar (§55 /coupons params).
		     Rendered even on loadError — adjusting a filter retries the load. -->
		<div class="mb-4 flex flex-col gap-3 lg:flex-row lg:items-center">
			<div class="relative lg:w-72">
				<Search
					size={16}
					class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-[var(--color-text-subtle)]"
				/>
				<input
					id="deals-search"
					type="search"
					bind:value={searchInput}
					oninput={onSearchInput}
					onkeydown={onSearchKeydown}
					placeholder={t('deals.search_placeholder', 'Search deals…')}
					aria-label={t('deals.search_aria', 'Search deals')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-2 pr-3 pl-9 text-sm text-[var(--color-text)] placeholder:text-[var(--color-text-subtle)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
			</div>
			<div class="grid flex-1 grid-cols-1 gap-3 sm:grid-cols-3">
				<AutocompleteSelect
					id="deals-provider"
					ariaLabel={t('deals.filter_provider', 'Filter by provider')}
					placeholder={t('deals.all_providers', 'All providers')}
					options={providerOptions}
					bind:value={selectedProvider}
					onValueChange={() => applyFilters()}
				/>
				<AutocompleteSelect
					id="deals-type"
					ariaLabel={t('deals.filter_type', 'Filter by type')}
					placeholder={t('deals.all_types', 'All types')}
					options={typeOptions}
					bind:value={selectedType}
					onValueChange={() => applyFilters()}
				/>
				<AutocompleteSelect
					id="deals-sort"
					ariaLabel={t('deals.sort_label', 'Sort deals')}
					placeholder={t('deals.sort.newest', 'Newest')}
					options={sortOptions}
					bind:value={selectedSort}
					onValueChange={() => applyFilters()}
				/>
			</div>
		</div>

		{#if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('deals.load_error', 'Failed to load deals. Please try again later.')}
			</p>
		{:else if coupons.length === 0}
			<EmptyState
				icon={Ticket}
				title={serverFiltersActive
					? t('deals.empty_filtered', 'No deals match your filters.')
					: t('deals.empty', 'No deals available right now.')}
				action={data.canSubmit && !serverFiltersActive
					? { label: t('deals.new_deal', 'New deal'), href: '/deals/new' }
					: undefined}
			/>
		{:else}
			{#if categories.length > 0}
				{#if viewMode === 'grid'}
					<p class="mb-5 text-center text-sm text-[var(--color-text-subtle)]">
						{t('deals.subtitle', 'Exclusive neighborhood coupons')}
					</p>
				{/if}
				<nav aria-label={t('deals.table.category', 'Category')} class="mb-6">
					<ul class="flex flex-wrap items-start justify-center gap-1 sm:gap-6">
						<li>
							<button
								type="button"
								onclick={() => selectCategory('')}
								aria-pressed={selectedCategory === ''}
								class="flex w-20 flex-col items-center gap-1.5 rounded-xl px-2 py-2 text-xs font-medium transition-colors {selectedCategory ===
								''
									? 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
									: 'text-[var(--color-text-subtle)] hover:text-[var(--color-text)]'}"
							>
								<Tag size={20} />
								<span>{t('deals.categories.all_coupons', 'All Coupons')}</span>
							</button>
						</li>
						{#each categories as category (category)}
							{@const TabIcon = categoryIcon(category)}
							<li>
								<button
									type="button"
									onclick={() => selectCategory(category)}
									aria-pressed={selectedCategory === category}
									class="flex w-20 flex-col items-center gap-1.5 rounded-xl px-2 py-2 text-xs font-medium transition-colors {selectedCategory ===
									category
										? 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
										: 'text-[var(--color-text-subtle)] hover:text-[var(--color-text)]'}"
								>
									<TabIcon size={20} />
									<span class="max-w-full truncate">{category}</span>
								</button>
							</li>
						{/each}
					</ul>
				</nav>
			{/if}

			{#if filteredCoupons.length === 0}
				<EmptyState icon={Ticket} title={t('deals.empty_filtered', 'No deals match your filters.')} />
			{:else}
				{#if viewMode === 'grid'}
					<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
						{#each filteredCoupons as coupon (coupon.id)}
							{@const hue = hueFor(coupon.id)}
							{@const CardIcon = categoryIcon(coupon.provider_category ?? coupon.title)}
							{@const sub = cardSub(coupon)}
							<article
								class="overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] transition-shadow hover:shadow-md"
							>
								<a
									href={dealPath(coupon)}
									aria-label={cardTitle(coupon)}
									class="relative block h-36 overflow-hidden"
									style="background: linear-gradient(135deg, hsl({hue} 70% 85%), hsl({(hue +
										40) %
										360} 70% 68%))"
								>
									{#if discountShort(coupon)}
										<span
											class="absolute top-3 left-3 inline-flex items-center gap-1 rounded-full bg-white px-2.5 py-1 text-xs font-bold text-gray-900 shadow"
										>
											🔥 {discountShort(coupon)}
										</span>
									{/if}
									<CardIcon
										size={96}
										class="absolute -right-4 -bottom-4 text-white opacity-30"
									/>
								</a>
								<div class="p-4">
									<h2
										class="flex items-center gap-1.5 text-[15px] font-bold text-[var(--color-text)]"
									>
										<a href={dealPath(coupon)} class="min-w-0 flex-1 truncate hover:underline">
											{cardTitle(coupon)}
										</a>
										{#if coupon.is_verified}
											<BadgeCheck size={15} class="shrink-0 text-[var(--color-primary)]" />
										{/if}
										{#if data.canManage}
											<button
												type="button"
												onclick={(e) => openEdit(e, coupon)}
												class="grid h-7 w-7 shrink-0 place-items-center rounded-[var(--radius)] text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
												aria-label={t('deal.edit', 'Edit')}
											>
												<Pencil size={14} />
											</button>
										{/if}
									</h2>
									<p
										class="mt-0.5 line-clamp-2 min-h-[2.5rem] text-[13px] text-[var(--color-text-subtle)]"
									>
										{sub}
									</p>
									<p class="mt-1.5 text-xs font-medium text-[var(--color-success)]">
										{#if coupon.expires_at}
											{t('deals.valid_till', 'Valid till: {date}', {
												date: expiryDate(coupon)
											})}
										{:else}
											{t('coupons.no_expiry', 'No expiry date')}
										{/if}
									</p>
									{#if coupon.code}
										<button
											type="button"
											onclick={() => copyCouponCode(coupon)}
											aria-label={t('coupons.copy_code', 'Copy code')}
											class="mt-3 flex w-full items-center justify-center gap-1.5 rounded-full border-2 border-dashed border-[var(--color-primary)]/60 px-3 py-2 font-mono text-sm font-bold tracking-widest text-[var(--color-primary)] transition-colors hover:bg-[var(--color-primary)]/5"
										>
											{#if copiedId === coupon.id}
												<Check size={15} />
											{:else}
												<Copy size={15} />
											{/if}
											{coupon.code}
										</button>
									{:else}
										<a
											href={dealPath(coupon)}
											class="mt-3 flex w-full items-center justify-center gap-1.5 rounded-full border-2 border-dashed border-[var(--color-primary)]/60 px-3 py-2 text-sm font-bold text-[var(--color-primary)] transition-colors hover:bg-[var(--color-primary)]/5"
										>
											<ExternalLink size={15} />
											{t('coupons.claim', 'Claim')}
										</a>
									{/if}
								</div>
							</article>
						{/each}
					</div>
				{:else}
					<!-- List view: costgoat-style table (Provider / Category / Deal / Details / Claim) -->
				<div class="hidden overflow-x-auto sm:block">
					<table class="w-full border-collapse text-left text-sm">
						<thead>
							<tr
								class="border-b border-[var(--color-border)] text-xs uppercase tracking-wide text-[var(--color-text-subtle)]"
							>
								<th class="px-3 py-2 font-medium">{t('deals.table.provider', 'Provider')}</th>
								<th class="px-3 py-2 font-medium">{t('deals.table.category', 'Category')}</th>
								<th class="px-3 py-2 font-medium">{t('deals.table.deal', 'Deal')}</th>
								<th class="px-3 py-2 font-medium">{t('deals.table.details', 'Details')}</th>
								<th class="px-3 py-2 font-medium">{t('deals.table.claim', 'Claim')}</th>
								{#if data.canManage}
									<th class="px-3 py-2 font-medium"><span class="sr-only">{t('deal.edit', 'Edit')}</span></th>
								{/if}
							</tr>
						</thead>
						<tbody>
							{#each filteredCoupons as coupon (coupon.id)}
								<tr
									role="link"
									tabindex="0"
									onclick={(e) => onRowClick(e, coupon)}
									onkeydown={(e) => onRowKeydown(e, coupon)}
									class="cursor-pointer border-b border-[var(--color-border)] transition-colors hover:bg-[var(--color-surface-subtle)]"
								>
									<td class="px-3 py-3 font-medium text-[var(--color-text)]">
										{providerName(coupon)}
									</td>
									<td class="px-3 py-3">
										{#if coupon.provider_category}
											<span
												class="inline-flex items-center rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs font-medium text-[var(--color-text)]"
											>
												{coupon.provider_category}
											</span>
										{:else}
											<span class="text-[var(--color-text-subtle)]">—</span>
										{/if}
									</td>
									<td class="px-3 py-3">
										<div class="flex items-center gap-1.5">
											<a
												href={dealPath(coupon)}
												class="font-medium text-[var(--color-text)] hover:text-[var(--color-primary)] hover:underline"
											>
												{coupon.title}
											</a>
											{#if coupon.is_verified}
												<BadgeCheck size={14} class="shrink-0 text-[var(--color-primary)]" />
											{/if}
										</div>
										{#if discountLabel(coupon)}
											<span class="text-xs font-medium text-[var(--color-primary)]">
												{discountLabel(coupon)}
											</span>
										{/if}
									</td>
									<td class="max-w-64 px-3 py-3">
										{#if coupon.description}
											<p class="line-clamp-1 text-xs text-[var(--color-text-subtle)]">
												{coupon.description}
											</p>
										{:else}
											<span class="text-[var(--color-text-subtle)]">—</span>
										{/if}
									</td>
									<td class="px-3 py-3">
										<span
											class="inline-flex items-center gap-1 text-xs font-medium text-[var(--color-primary)]"
										>
											<ExternalLink size={12} />
											{t('coupons.claim', 'Claim')}
										</span>
									</td>
									{#if data.canManage}
										<td class="px-3 py-3">
											<button
												type="button"
												onclick={(e) => openEdit(e, coupon)}
												class="grid h-7 w-7 place-items-center rounded-[var(--radius)] text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
												aria-label={t('deal.edit', 'Edit')}
											>
												<Pencil size={14} />
											</button>
										</td>
									{/if}
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				<!-- Mobile: compact stacked rows, same click-through -->
				<div class="sm:hidden">
					{#each filteredCoupons as coupon (coupon.id)}
						<div class="flex items-start justify-between gap-3 border-b border-[var(--color-border)] py-3">
							<a href={dealPath(coupon)} class="min-w-0 flex-1">
								<div class="flex items-center gap-1.5">
									<h2 class="truncate text-sm font-semibold text-[var(--color-text)]">
										{coupon.title}
									</h2>
									{#if coupon.is_verified}
										<BadgeCheck size={14} class="shrink-0 text-[var(--color-primary)]" />
									{/if}
								</div>
								<p class="mt-0.5 truncate text-xs text-[var(--color-text-subtle)]">
									{providerName(coupon)}{#if coupon.provider_category}
										· {coupon.provider_category}{/if}
								</p>
								{#if coupon.description}
									<p class="mt-1 line-clamp-1 text-xs text-[var(--color-text-subtle)]">
										{coupon.description}
									</p>
								{/if}
							</a>
							<div class="flex shrink-0 items-center gap-1">
								<span
									class="inline-flex items-center gap-1 text-xs font-medium text-[var(--color-primary)]"
								>
									{#if discountLabel(coupon)}
										{discountLabel(coupon)}
									{:else}
										<ExternalLink size={14} />
									{/if}
								</span>
								{#if data.canManage}
									<button
										type="button"
										onclick={(e) => openEdit(e, coupon)}
										class="grid h-7 w-7 place-items-center rounded-[var(--radius)] text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
										aria-label={t('deal.edit', 'Edit')}
									>
										<Pencil size={14} />
									</button>
								{/if}
							</div>
						</div>
					{/each}
					</div>
				{/if}
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
						{t('deals.load_more', 'Load more')}
					</button>
				</div>
			{/if}
			{#if loadMoreError}
				<p class="mt-2 text-center text-sm text-red-500" role="alert">
					{t('deals.load_error', 'Failed to load deals. Please try again later.')}
				</p>
			{/if}
		{/if}
	</div>

	{#if data.canManage}
		<Modal
			bind:open={editOpen}
			onOpenChange={(v) => {
				if (!v) editingCoupon = null;
			}}
			icon={Pencil}
			title={t('deals.edit_deal', 'Edit deal')}
			size="max-w-[640px]"
		>
			{#if editingCoupon}
				{#key editingCoupon.id}
					<DealForm
						mode="edit"
						audience="staff"
						coupon={editingCoupon}
						providers={data.providers}
						canManageProviders={data.canManageProviders}
						currencies={data.currencies}
						onsaved={onEditSaved}
						oncancel={closeEdit}
					/>
				{/key}
			{/if}
		</Modal>
	{/if}
</div>
