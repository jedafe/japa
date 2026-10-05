<script lang="ts">
	// Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md (Deal Pages)
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t, i18n } from '$lib/i18n';
	import { Ticket, BadgeCheck, ExternalLink, Plus } from 'lucide-svelte';
	import { EmptyState, ListPageBar, PageAction } from '$lib/components/ui';
	import type { PageData } from './$types';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';
	import { dealPath, type Coupon } from '$lib/types';
	import { currencySymbol } from '$lib/utils/currency';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('deals.title', 'Deals — JapaConnect'));
	const description = $derived(
		t('deals.description', 'Browse active deals and discount codes from JapaConnect partners.')
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
	// syncs to ?category= via history.replaceState — a shallow URL update,
	// not a SvelteKit navigation.
	const categories = $derived.by(() => {
		const seen = new Map<string, string>();
		for (const c of data.coupons) {
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
			? data.coupons.filter(
					(c) =>
						(c.provider_category?.trim().toLowerCase() ?? '') ===
						selectedCategory.toLowerCase()
				)
			: data.coupons
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
		history.replaceState(history.state, '', url);
	}

	// "Last updated" in the page bar — newest updated_at across the listing.
	const lastUpdated = $derived.by(() => {
		let max = '';
		for (const c of data.coupons) {
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
		return c.provider_name ?? t('deals.internal', 'JapaConnect');
	}

	function onRowClick(e: MouseEvent, coupon: Coupon) {
		// The deal title is a real anchor — let its native navigation handle
		// clicks (incl. middle-click / open-in-new-tab) instead of goto().
		if ((e.target as HTMLElement).closest('a')) return;
		goto(dealPath(coupon));
	}

	function onRowKeydown(e: KeyboardEvent, coupon: Coupon) {
		// role="link" rows activate on Enter AND Space per APG.
		if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			if ((e.target as HTMLElement).closest('a')) return;
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

<div class="mx-auto w-full max-w-full" style="--page-gutter: 1rem">
	<ListPageBar title={t('deals.heading', 'Deals')}>
		{#snippet actions()}
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

	<div class="p-4">
		{#if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('deals.load_error', 'Failed to load deals. Please try again later.')}
			</p>
		{:else if data.coupons.length === 0}
			<EmptyState icon={Ticket} title={t('deals.empty', 'No deals available right now.')} />
		{:else}
			{#if categories.length > 0}
				<nav aria-label={t('deals.table.category', 'Category')} class="mb-4">
					<ul class="flex flex-wrap gap-2">
						<li>
							<button
								type="button"
								onclick={() => selectCategory('')}
								aria-pressed={selectedCategory === ''}
								class="inline-flex items-center rounded-full border px-3 py-1.5 text-xs font-medium transition-colors {selectedCategory ===
								''
									? 'border-[var(--color-primary)] bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
									: 'border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text)] hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-primary)]'}"
							>
								{t('deals.categories.all', 'All')}
							</button>
						</li>
						{#each categories as category (category)}
							<li>
								<button
									type="button"
									onclick={() => selectCategory(category)}
									aria-pressed={selectedCategory === category}
									class="inline-flex items-center rounded-full border px-3 py-1.5 text-xs font-medium transition-colors {selectedCategory ===
									category
										? 'border-[var(--color-primary)] bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
										: 'border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text)] hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-primary)]'}"
								>
									{category}
								</button>
							</li>
						{/each}
					</ul>
				</nav>
			{/if}

			{#if filteredCoupons.length === 0}
				<EmptyState icon={Ticket} title={t('deals.empty', 'No deals available right now.')} />
			{:else}
				<!-- Desktop: costgoat-style table (Provider / Category / Deal / Details / Claim) -->
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
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				<!-- Mobile: compact stacked rows, same click-through -->
				<div class="sm:hidden">
					{#each filteredCoupons as coupon (coupon.id)}
						<a
							href={dealPath(coupon)}
							class="flex items-start justify-between gap-3 border-b border-[var(--color-border)] py-3"
						>
							<div class="min-w-0">
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
							</div>
							<span
								class="inline-flex shrink-0 items-center gap-1 text-xs font-medium text-[var(--color-primary)]"
							>
								{#if discountLabel(coupon)}
									{discountLabel(coupon)}
								{:else}
									<ExternalLink size={14} />
								{/if}
							</span>
						</a>
					{/each}
				</div>
			{/if}
		{/if}
	</div>
</div>
