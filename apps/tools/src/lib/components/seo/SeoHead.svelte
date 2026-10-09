<script lang="ts" module>
	export interface SeoHeadData {
		title: string;
		description: string;
		canonical: string;
		ogImage: string;
		siteName: string;
		noindex: boolean;
		jsonLdString: string | null;
	}
</script>

<script lang="ts">
	import { page } from '$app/state';
	import { safeJsonLd } from '$lib/seo';

	/**
	 * Shared SEO head for Tools pages (§78, ADR-086).
	 * Renders title, description, cross-domain canonical, hreflang
	 * alternates, OG/Twitter tags (with per-tool og:image), robots when
	 * noindex, and JSON-LD when provided.
	 *
	 * hreflang: derived from the request-scoped siteSettings
	 * (i18n.enabled_languages) via layout data — the Tools app serves
	 * /<lang>/<path> prefixes for enabled locales (hooks.server.ts,
	 * ADR-013). Rendered only when more than one locale is enabled;
	 * single-locale deployments are EN-only by design (no hreflang).
	 *
	 * Usage: <SeoHead seo={seo} ogType="website" twitterCard="summary" />
	 */
	let {
		seo,
		ogType = 'website',
		twitterCard = 'summary'
	}: {
		seo: SeoHeadData;
		ogType?: string;
		twitterCard?: string;
	} = $props();

	const siteSettings = $derived(
		(page.data?.siteSettings as Record<string, unknown> | undefined) ?? {}
	);
	const enabledLocales = $derived(
		((siteSettings['i18n.enabled_languages'] as string | undefined) ??
			(typeof page.data?.locale === 'string' ? page.data.locale : 'en'))
			.split(',')
			.map((l) => l.trim())
			.filter(Boolean)
	);
	const defaultLocale = $derived(
		(siteSettings['i18n.default_language'] as string | undefined) ?? 'en'
	);
	const hasHreflang = $derived(enabledLocales.length > 1);
	const hreflangHref = $derived((lang: string) =>
		lang === defaultLocale
			? seo.canonical
			: seo.canonical.replace(/^(https?:\/\/[^/]+)(\/.*)?$/, `$1/${lang}$2`)
	);
</script>

<svelte:head>
	<title>{seo.title}</title>
	<meta name="description" content={seo.description} />
	<link rel="canonical" href={seo.canonical} />
	{#if hasHreflang}
		{#each enabledLocales as lang (lang)}
			<link rel="alternate" hreflang={lang} href={hreflangHref(lang)} />
		{/each}
		<link rel="alternate" hreflang="x-default" href={seo.canonical} />
	{/if}
	{#if seo.noindex}
		<meta name="robots" content="noindex, nofollow" />
	{/if}
	<meta property="og:title" content={seo.title} />
	<meta property="og:description" content={seo.description} />
	<meta property="og:type" content={ogType} />
	<meta property="og:url" content={seo.canonical} />
	<meta property="og:site_name" content={seo.siteName} />
	<meta property="og:image" content={seo.ogImage} />
	<meta name="twitter:card" content={twitterCard} />
	<meta name="twitter:title" content={seo.title} />
	<meta name="twitter:description" content={seo.description} />
	<meta name="twitter:image" content={seo.ogImage} />
	{#if seo.jsonLdString}
		{@html `<script type="application/ld+json">${safeJsonLd(seo.jsonLdString)}<\/script>`}
	{/if}
</svelte:head>
