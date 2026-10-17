import { env } from '$env/dynamic/private';

import { DEFAULT_SITE_NAME } from '$lib/i18n/dict';
/** Canonical docs origin for SEO canonical URLs + JSON-LD. */
export const DOCS_ORIGIN = (env.PUBLIC_DOCS_ORIGIN ?? 'https://docs.japa.to').replace(
	/\/$/,
	''
);

/**
 * Build a normalized SEO meta object from a single options bag.
 *
 * The returned shape is ready to spread into a `<svelte:head>` block. The
 * canonical URL is forced onto docs.japa.to; `jsonLdString` is
 * pre-serialized so it can be dropped directly into a
 * `<script type="application/ld+json">` body.
 */
export interface BuildMetaOptions {
	title: string;
	description?: string;
	/** Path on the docs site (without origin), e.g. "/docs/api/spaces". */
	path?: string;
	image?: string;
	/** Open Graph type — "article" for doc pages, "website" for index/listing pages. */
	ogType?: 'article' | 'website';
	jsonLd?: object;
}

export interface SeoMeta {
	title: string;
	description: string;
	ogType: string;
	ogUrl: string;
	ogImage: string;
	twitterCard: string;
	twitterTitle: string;
	twitterDescription: string;
	twitterImage: string;
	jsonLdString: string | null;
}

export function buildMeta(opts: BuildMetaOptions): SeoMeta {
	const title = opts.title;
	const description = opts.description ?? '';
	const ogUrl = opts.path ? `${DOCS_ORIGIN}${opts.path}` : DOCS_ORIGIN;
	// OG image from the Go backend OG endpoint (Ref: §26 ROADMAP 07.06).
	// The backend generates a branded OG image with the page title overlaid.
	// If an explicit image is provided, use it; otherwise derive from the
	// backend's OG image endpoint.
	const apiBase = (env.PRIVATE_API_BASE_URL ?? '').replace(/\/$/, '');
	const ogImage =
		opts.image ??
		(apiBase && opts.path
			? `${apiBase}/og/docs?title=${encodeURIComponent(title)}&path=${encodeURIComponent(opts.path)}`
			: '');

	return {
		title,
		description,
		ogType: opts.ogType ?? 'article',
		ogUrl,
		ogImage,
		twitterCard: ogImage ? 'summary_large_image' : 'summary',
		twitterTitle: title,
		twitterDescription: description,
		twitterImage: ogImage,
		jsonLdString: opts.jsonLd ? JSON.stringify(opts.jsonLd) : null
	};
}

/** TechArticle JSON-LD for a single doc page. `siteName` comes from
 * event.locals.siteSettings['general.site_name'] (server context). */
export function techArticleLd(opts: {
	headline: string;
	dateModified: string;
	url: string;
	description?: string;
	siteName?: string;
}): object {
	return {
		'@context': 'https://schema.org',
		'@type': 'TechArticle',
		headline: opts.headline,
		description: opts.description ?? undefined,
		dateModified: opts.dateModified,
		url: opts.url,
		publisher: { '@type': 'Organization', name: opts.siteName ?? DEFAULT_SITE_NAME }
	};
}

/** CollectionPage JSON-LD for listing pages (audience landings, categories, API index). */
export function collectionPageLd(opts: {
	name: string;
	url: string;
	description?: string;
	siteName?: string;
}): object {
	return {
		'@context': 'https://schema.org',
		'@type': 'CollectionPage',
		name: opts.name,
		description: opts.description ?? undefined,
		url: opts.url,
		publisher: { '@type': 'Organization', name: opts.siteName ?? DEFAULT_SITE_NAME }
	};
}

/** WebSite JSON-LD with SearchAction for the docs landing page. */
export function webSiteLd(opts?: { name?: string; description?: string; siteName?: string }): object {
	return {
		'@context': 'https://schema.org',
		'@type': 'WebSite',
		name: opts?.name ?? `${opts?.siteName ?? DEFAULT_SITE_NAME} Docs`,
		description: opts?.description,
		url: DOCS_ORIGIN,
		potentialAction: {
			'@type': 'SearchAction',
			target: `${DOCS_ORIGIN}/docs/search?q={search_term_string}`,
			'query-input': 'required name=search_term_string'
		}
	};
}
