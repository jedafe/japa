<script lang="ts">
	import SeoHead from '#lib/seo/SeoHead.svelte';
	// /product/{slug} — product detail page (§80 Product Domain). Products
	// are comparison entities, not wiki kinds: hero + provider offers
	// matrix + the type's cfv field groups, plus a /compare deep link.
	import { page } from '$app/state';
	import { t, siteName } from '#lib/i18n/index.js';
	import { breadcrumbJsonLd } from '#lib/seo/breadcrumbs.js';
	import { safeJsonLdScript } from '#lib/utils/safeHtml.js';
	import { safeHttpUrl } from '#lib/utils/safeUrl.js';
	import { EntityHero } from '#lib/components/ui/index.js';
	import { ExternalLink, GitCompareArrows, Package, Star, Store } from 'lucide-svelte';
	import type { PageData } from './$types';
	import type { ProductProvider } from './+page.server';

	import { ogCardUrl, ogMediaUrl } from '#lib/seo/og.js';
	let { data }: { data: PageData } = $props();
	let product = $derived(data.product);

	const canonicalUrl = $derived(
		`${page.url.origin}/product/${encodeURIComponent(product.slug)}`
	);
	const metaDescription = $derived(
		product.tagline ||
			t('product.description_fallback', '{name} — {type} on {site}.', {
				name: product.name,
				type: product.type_label
			})
	);
	const productJsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'Product',
			name: product.name,
			description: metaDescription,
			url: canonicalUrl,
			...(product.logo_url ? { image: product.logo_url } : {}),
			...(product.rating_avg != null && product.review_count > 0
				? {
						aggregateRating: {
							'@type': 'AggregateRating',
							ratingValue: product.rating_avg,
							reviewCount: product.review_count
						}
					}
				: {})
		})
	);
	const crumbsJsonLd = $derived(
		JSON.stringify(
			breadcrumbJsonLd([
				{ name: 'Home', path: page.url.origin },
				{ name: t('products.heading', 'Products'), path: `${page.url.origin}/products` },
				{ name: product.name, path: canonicalUrl }
			])
		)
	);

	const compareHref = $derived(
		`/compare?type=${encodeURIComponent(product.type_slug)}&source=product&preselect=${encodeURIComponent(product.slug)}`
	);

	function fmtPrice(p: ProductProvider | { price_from?: number | null }): string {
		return p.price_from != null ? String(p.price_from) : '—';
	}
</script>

<SeoHead
	title={`${product.name} — ${t('products.heading', 'Products')} — ${siteName()}`}
	description={metaDescription}
	ogTitle={`${product.name} — ${product.type_label}`}
	ogImage={ogMediaUrl(page.url.origin, product.logo_url) || ogCardUrl(page.url.origin, `product:${product.slug}`)}
	canonical={canonicalUrl}
	jsonLd={productJsonLd}
/>
<svelte:head>
	{@html safeJsonLdScript(crumbsJsonLd)}
</svelte:head>

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	<EntityHero
		class="mt-4"
		icon={Package}
		accent="var(--color-primary)"
		heading={product.name}
		description={product.tagline || product.type_label}
		crumbs={[
			{ label: t('common.home', 'Home'), href: '/' },
			{ label: t('products.heading', 'Products'), href: '/products' },
			{ label: product.name }
		]}
		stats={[
			{ icon: Package, value: product.type_label, label: t('product.stat_type', 'Type') },
			...(product.rating_avg != null
				? [
						{
							icon: Star,
							value: `${product.rating_avg} (${product.review_count})`,
							label: t('product.stat_rating', 'Rating')
						}
					]
				: []),
			{ icon: Store, value: String(product.providers.length), label: t('product.stat_providers', 'Providers') }
		]}
	/>

	<div class="mx-auto mt-6 w-full max-w-3xl space-y-6 p-4" style="margin-inline: var(--page-gutter)">
		<div class="flex flex-wrap items-center gap-2">
			{#if product.logo_url}
				<img src={product.logo_url} alt="" class="h-10 w-10 rounded-[var(--radius)] object-cover" />
			{/if}
			{#if safeHttpUrl(product.website_url)}
				<a
					href={safeHttpUrl(product.website_url)}
					target="_blank"
					rel="noopener noreferrer"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
				>
					<ExternalLink size={14} /> {t('product.visit_site', 'Visit website')}
				</a>
			{/if}
			<a
				href={compareHref}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/60 hover:text-[var(--color-primary-text)]"
			>
				<GitCompareArrows size={14} /> {t('product.compare', 'Compare')}
			</a>
			{#if product.pricing_model}
				<span
					class="rounded-full border border-[var(--color-border)] px-2.5 py-1 text-xs font-medium capitalize text-[var(--color-muted)]"
				>
					{product.pricing_model.replaceAll('_', ' ')}
				</span>
			{/if}
			{#if product.price_from != null}
				<span class="text-sm text-[var(--color-muted)]">
					{t('product.from_price', 'From {price}').replace('{price}', String(product.price_from))}
				</span>
			{/if}
		</div>

		{#if product.providers.length > 0}
			<section class="space-y-2">
				<h2 class="text-sm font-semibold uppercase tracking-wide text-[var(--color-muted)]">
					{t('product.providers_heading', 'Where to get it')}
				</h2>
				<div class="divide-y divide-[var(--color-border)] rounded-lg border border-[var(--color-border)]">
					{#each product.providers as p (p.provider_id)}
						<div class="flex items-center gap-3 px-4 py-3">
							<div class="min-w-0 flex-1">
								{#if p.provider_slug}
									<a
										href="/deal/{encodeURIComponent(p.provider_slug)}"
										class="truncate text-sm font-medium text-[var(--color-text)] hover:text-[var(--color-primary-text)]"
									>{p.name}</a>
								{:else}
									<span class="truncate text-sm font-medium text-[var(--color-text)]">{p.name}</span>
								{/if}
								{#if p.has_deal}
									<span
										class="ms-2 inline-flex items-center rounded-full bg-emerald-500/10 px-2 py-0.5 text-[10px] font-medium text-emerald-600 dark:text-emerald-400"
									>{t('product.has_deal', 'Deal')}</span>
								{/if}
							</div>
							<span class="text-sm tabular-nums text-[var(--color-text)]">{fmtPrice(p)}</span>
							{#if safeHttpUrl(p.offer_url)}
								<a
									href={safeHttpUrl(p.offer_url)}
									target="_blank"
									rel="noopener noreferrer"
									class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/60 hover:text-[var(--color-primary-text)]"
								>
									{t('product.get_it', 'Get it')} <ExternalLink size={12} />
								</a>
							{/if}
						</div>
					{/each}
				</div>
			</section>
		{/if}

		{#each product.field_groups ?? [] as group (group.id)}
			{#if group.fields.length > 0}
				<section class="space-y-2">
					<h2 class="text-sm font-semibold uppercase tracking-wide text-[var(--color-muted)]">
						{group.name}
					</h2>
					<dl class="divide-y divide-[var(--color-border)] rounded-lg border border-[var(--color-border)]">
						{#each group.fields as field (field.key)}
							<div class="grid grid-cols-3 gap-4 px-4 py-2.5">
								<dt class="text-sm text-[var(--color-muted)]">{field.label}</dt>
								<dd class="col-span-2 text-sm text-[var(--color-text)]">
									{#if field.value != null}
										{typeof field.value === 'object' ? JSON.stringify(field.value) : String(field.value)}
									{:else}
										<span class="italic text-[var(--color-muted)]">—</span>
									{/if}
								</dd>
							</div>
						{/each}
					</dl>
				</section>
			{/if}
		{/each}
	</div>
</div>
