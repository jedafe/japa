<script lang="ts">
	import { page } from '$app/state';
	import type { SeoMeta } from '$lib/seo/meta';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';

	interface Props {
		meta: SeoMeta;
		/** Gated audiences and previews are kept out of the index entirely. */
		gated?: boolean;
		/** Force noindex for tool/search/error pages that should never rank. */
		noindex?: boolean;
		/** Emit hreflang alternates for every enabled language. */
		alternates?: boolean;
	}
	let { meta, gated = false, noindex = false, alternates = true }: Props = $props();

	// Dynamic hreflang from enabled languages (§18, ADR-013). Falls back to the
	// active locale rather than a hardcoded list when the setting is unavailable.
	const enabledLanguages = $derived(
		(
			(page.data?.siteSettings?.['i18n.enabled_languages'] as string | undefined) ??
			(page.data?.locale as string | undefined) ??
			'en'
		)
			.split(',')
			.map((l) => l.trim())
			.filter(Boolean)
	);
	const defaultLanguage = $derived(
		(page.data?.siteSettings?.['i18n.default_language'] as string | undefined) ?? 'en'
	);
	// Localized URL: the canonical docs paths live at /docs/...; each non-default
	// language is prefixed (/fr/docs/...). (?=\/|$) also matches the bare /docs
	// root which the naive /docs/ pattern misses.
	const localizedUrl = (lang: string) =>
		lang === defaultLanguage
			? meta.ogUrl
			: meta.ogUrl.replace(/\/docs(?=\/|$)/, `/${lang}/docs`);
</script>

<title>{meta.title}</title>
<meta name="description" content={meta.description} />
<link rel="canonical" href={meta.ogUrl} />
{#if noindex || gated}
	<meta name="robots" content="noindex, nofollow" />
{:else}
	<meta name="robots" content="index, follow" />
	{#if alternates}
		<!-- hreflang for public pages (Ref: §18 ROADMAP 07.05) -->
		{#each enabledLanguages as lang (lang)}
			<link rel="alternate" hreflang={lang} href={localizedUrl(lang)} />
		{/each}
		<link rel="alternate" hreflang="x-default" href={meta.ogUrl} />
	{/if}
{/if}
<meta property="og:title" content={meta.title} />
<meta property="og:description" content={meta.description} />
<meta property="og:type" content={meta.ogType} />
<meta property="og:url" content={meta.ogUrl} />
{#if meta.ogImage}
	<meta property="og:image" content={meta.ogImage} />
{/if}
<meta name="twitter:card" content={meta.twitterCard} />
<meta name="twitter:title" content={meta.twitterTitle} />
<meta name="twitter:description" content={meta.twitterDescription} />
{#if meta.twitterImage}
	<meta name="twitter:image" content={meta.twitterImage} />
{/if}
{#if meta.jsonLdString}
	{@html safeJsonLdScript(meta.jsonLdString)}
{/if}
