import { env } from '$env/dynamic/private';

/** Canonical docs origin for SEO canonical URLs + JSON-LD. */
export const DOCS_ORIGIN = (env.PUBLIC_DOCS_ORIGIN ?? 'https://docs.japaconnect.com').replace(
	/\/$/,
	''
);

/**
 * Build a normalized SEO meta object from a single options bag.
 *
 * The returned shape is ready to spread into a `<svelte:head>` block. The
 * canonical URL is forced onto docs.japaconnect.com; `jsonLdString` is
 * pre-serialized so it can be dropped directly into a
 * `<script type="application/ld+json">` body.
 */
export interface BuildMetaOptions {
	title: string;
	description?: string;
	/** Path on the docs site (without origin), e.g. "/docs/api/spaces". */
	path?: string;
	image?: string;
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
	const ogImage = opts.image ?? '';

	return {
		title,
		description,
		ogType: 'article',
		ogUrl,
		ogImage,
		twitterCard: ogImage ? 'summary_large_image' : 'summary',
		twitterTitle: title,
		twitterDescription: description,
		twitterImage: ogImage,
		jsonLdString: opts.jsonLd ? JSON.stringify(opts.jsonLd) : null
	};
}

/** TechArticle JSON-LD for a single doc page. */
export function techArticleLd(opts: {
	headline: string;
	dateModified: string;
	url: string;
	description?: string;
}): object {
	return {
		'@context': 'https://schema.org',
		'@type': 'TechArticle',
		headline: opts.headline,
		description: opts.description ?? undefined,
		dateModified: opts.dateModified,
		url: opts.url,
		publisher: { '@type': 'Organization', name: 'JapaConnect' }
	};
}

/** WebSite JSON-LD with SearchAction for the docs landing page. */
export function webSiteLd(): object {
	return {
		'@context': 'https://schema.org',
		'@type': 'WebSite',
		name: 'JapaConnect Docs',
		url: DOCS_ORIGIN,
		potentialAction: {
			'@type': 'SearchAction',
			target: `${DOCS_ORIGIN}/docs/search?q={search_term_string}`,
			'query-input': 'required name=search_term_string'
		}
	};
}
