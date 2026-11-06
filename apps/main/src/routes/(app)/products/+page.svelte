<script lang="ts">
	import SeoHead from '$lib/seo/SeoHead.svelte';
	// /products — product directory surface (§80 Product Domain). Products
	// are comparison entities, not wiki kinds; each card links to the
	// /product/{slug} detail page. The Deals tab is the moved /deals
	// surface at /products/deals.
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { ChevronRight, Package, Plus, Search } from 'lucide-svelte';
	import {
		EmptyState,
		ListPageBar,
		PageAction,
		PaginationControls,
		UrlTabs,
		type UrlTab
	} from '$lib/components/ui';
	import type { PageData } from './$types';

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

	// Surface tabs — same UrlTabs/ListPageBar pattern as /tools; switching
	// navigates across sibling paths instead of rewriting a query param.
	const surfaceTabs = $derived<UrlTab[]>([
		{ value: 'products', label: t('products.tab_products', 'Products') },
		...(data.dealsEnabled ? [{ value: 'deals', label: t('products.tab_deals', 'Deals') }] : [])
	]);
	function goSurface(v: string) {
		goto(v === 'deals' ? '/products/deals' : '/products');
	}

	// ?type= chips come from GET /products/types (product types are
	// admin-provisioned — Rule 16, no hardcoded list).
	const typeLabel = $derived.by(() => {
		const m = new Map(data.types.map((tp) => [tp.slug, tp.label]));
		return (slug: string) => m.get(slug) ?? slug;
	});

	let search = $state(untrack(() => data.q));
	$effect(() => {
		search = data.q;
	});

	/** Every control preserves params it does not own; page always resets. */
	function paramsWith(mutate: (p: URLSearchParams) => void): string {
		const params = new URLSearchParams(page.url.searchParams);
		mutate(params);
		params.delete('page');
		const qs = params.toString();
		return `/products${qs ? `?${qs}` : ''}`;
	}

	function applySearch() {
		goto(
			paramsWith((p) => {
				const v = search.trim();
				if (v) p.set('q', v);
				else p.delete('q');
			}),
			{ keepFocus: true, noScroll: true }
		);
	}

	function selectType(slug: string) {
		goto(
			paramsWith((p) => {
				if (slug) p.set('type', slug);
				else p.delete('type');
			}),
			{ keepFocus: true, noScroll: true }
		);
	}

	function setPage(p: number) {
		const url = new URL(page.url);
		url.searchParams.set('page', String(p));
		goto(`${url.pathname}${url.search}`);
	}

	const pageCount = $derived(Math.max(1, Math.ceil(data.total / data.pageSize)));

	// Create moved to the dedicated /products/new form (products.create) —
	// the full field-schema/providers/deals editor lives there.
</script>

<SeoHead title={title} description={description} ogTitle={title} jsonLd={jsonLd} />

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	{#snippet barActions()}
		{#if data.canCreate}
			<PageAction
				icon={Plus}
				label={t('products.new_product', 'New product')}
				onclick={() => goto('/products/new')}
			/>
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
		<!-- Search — same server-side ?q= contract as the wiki listings. -->
		<form
			class="mb-4 flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 transition-colors focus-within:border-[var(--color-primary)]/60"
			onsubmit={(e) => {
				e.preventDefault();
				applySearch();
			}}
		>
			<Search size={16} class="shrink-0 text-[var(--color-muted)]" />
			<input
				bind:value={search}
				type="search"
				placeholder={t('products.search_placeholder', 'Search products…')}
				aria-label={t('products.search_aria', 'Search products')}
				class="w-full bg-transparent text-sm text-[var(--color-text)] outline-none placeholder:text-[var(--color-muted)]"
			/>
		</form>

		{#if data.types.length > 0}
			<nav aria-label={t('products.type_filter', 'Product type')} class="mb-5">
				<ul class="flex flex-wrap items-center gap-2">
					<li>
						<button
							type="button"
							onclick={() => selectType('')}
							aria-pressed={data.type === ''}
							class="rounded-full border px-3 py-1 text-xs font-medium transition-colors {data.type ===
							''
								? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
								: 'border-[var(--color-border)] text-[var(--color-text-subtle)] hover:text-[var(--color-text)]'}"
						>
							{t('products.types_all', 'All')}
						</button>
					</li>
					{#each data.types as tp (tp.slug)}
						<li>
							<button
								type="button"
								onclick={() => selectType(tp.slug)}
								aria-pressed={data.type === tp.slug}
								class="rounded-full border px-3 py-1 text-xs font-medium transition-colors {data.type ===
								tp.slug
									? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
									: 'border-[var(--color-border)] text-[var(--color-text-subtle)] hover:text-[var(--color-text)]'}"
							>
								{tp.label}
								{#if tp.entity_count > 0}
									<span class="text-[10px] opacity-70">{tp.entity_count}</span>
								{/if}
							</button>
						</li>
					{/each}
				</ul>
			</nav>
		{/if}

		{#if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('products.load_error', 'Failed to load products. Please try again later.')}
			</p>
		{:else if data.items.length === 0}
			<EmptyState
				icon={Package}
				title={data.q || data.type
					? t('products.empty_filtered', 'No products match your filters.')
					: t('products.empty', 'No products yet.')}
			/>
		{:else}
			<div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
				{#each data.items as it (it.id)}
					<a
						href="/product/{encodeURIComponent(it.slug)}"
						class="group flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3 transition-colors hover:border-[var(--color-primary)]/60 focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary)]"
					>
						<span
							class="min-w-0 flex-1 truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]"
							>{it.name}</span
						>
						{#if it.discriminator}
							<span
								class="shrink-0 rounded-full border border-[var(--color-border)] px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-[var(--color-muted)]"
							>
								{typeLabel(it.discriminator)}
							</span>
						{/if}
						<ChevronRight
							size={14}
							class="shrink-0 text-[var(--color-muted)] transition-all group-hover:translate-x-0.5 group-hover:text-[var(--color-primary)] rtl:-scale-x-100"
						/>
					</a>
				{/each}
			</div>
			{#if pageCount > 1}
				<div class="mt-6 flex justify-center">
					<PaginationControls page={data.page} totalPages={pageCount} onPageChange={setPage} />
				</div>
			{/if}
		{/if}
	</div>
</div>

<style>
	.inp {
		border-radius: var(--radius);
		border: 1px solid var(--color-border);
		background: var(--color-surface);
		padding: 0.5rem 0.75rem;
		font-size: 0.875rem;
		color: var(--color-text);
		outline: none;
	}
	.inp:focus {
		border-color: var(--color-primary);
	}
</style>
