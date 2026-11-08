<script lang="ts">
	import { page } from '$app/state';
	import { safeJsonLdScript } from '#lib/utils/safeHtml.js';

	/**
	 * Shared SEO head — one component owns the title/description/OG/Twitter/
	 * canonical/JSON-LD contract so per-route heads can't drift or silently
	 * drop tags (Ref: §26). The <svelte:head> wrapper is required: bare
	 * <title>/<meta>/<link> in markup hoists into <body>, not <head>.
	 *
	 * Page-specific tags that this component does not own (RSS alternates,
	 * hreflang, page-scoped OG fields like article:author) stay in the page's
	 * own <svelte:head> — they coexist.
	 */
	interface Props {
		/** Page <title> — also the default og:title/twitter:title. */
		title: string;
		/** Meta description; omit/empty suppresses all description tags. */
		description?: string;
		/** OG title when it should differ from <title> (e.g. bare entity name). */
		ogTitle?: string;
		/** Absolute canonical + og:url. Defaults to the current URL path on this
		 * origin — query stripped, so filtered (tab/sort/geo) and paginated
		 * hits consolidate onto the base page per §26. If a paginated or
		 * filtered view must be independently indexable, pass an explicit
		 * canonical that keeps the distinguishing query params. */
		canonical?: string;
		/** 'article' for content pages, 'profile' for people/org pages. */
		ogType?: 'website' | 'article' | 'profile';
		ogImage?: string;
		/** robots directive, e.g. 'noindex' or 'noindex,nofollow'. */
		robots?: string;
		/** JSON-LD object/array/pre-serialized string — escaped via safeJsonLdScript. */
		jsonLd?: unknown;
	}

	let {
		title,
		description = '',
		ogTitle,
		canonical,
		ogType = 'website',
		ogImage,
		robots = '',
		jsonLd
	}: Props = $props();

	const url = $derived(canonical ?? `${page.url.origin}${page.url.pathname}`);
	const twitterCard = $derived(ogImage ? 'summary_large_image' : 'summary');
	const jsonLdMarkup = $derived(jsonLd ? safeJsonLdScript(jsonLd) : '');
</script>

<svelte:head>
	<title>{title}</title>
	{#if description}
		<meta name="description" content={description} />
	{/if}
	<link rel="canonical" href={url} />
	<meta property="og:title" content={ogTitle ?? title} />
	{#if description}
		<meta property="og:description" content={description} />
	{/if}
	<meta property="og:type" content={ogType} />
	<meta property="og:url" content={url} />
	{#if ogImage}
		<meta property="og:image" content={ogImage} />
	{/if}
	<meta name="twitter:card" content={twitterCard} />
	<meta name="twitter:title" content={ogTitle ?? title} />
	{#if description}
		<meta name="twitter:description" content={description} />
	{/if}
	{#if ogImage}
		<meta name="twitter:image" content={ogImage} />
	{/if}
	{#if robots}
		<meta name="robots" content={robots} />
	{/if}
	{#if jsonLdMarkup}
		{@html jsonLdMarkup}
	{/if}
</svelte:head>
