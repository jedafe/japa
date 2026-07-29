/**
 * Build a normalized SEO meta object from a single options bag.
 *
 * The returned shape is ready to spread into a `<svelte:head>` block or pass to
 * any meta-rendering component. `jsonLdString` is pre-serialized so it can be
 * dropped directly into a `<script type="application/ld+json">` body.
 */
export interface BuildMetaOptions {
	title: string;
	description?: string;
	ogType?: string;
	url?: string;
	image?: string;
	jsonLd?: object;
}

export interface SeoMeta {
	title: string;
	description: string;
	ogTitle: string;
	ogDescription: string;
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
	const ogType = opts.ogType ?? 'website';
	const ogUrl = opts.url ?? '';
	const ogImage = opts.image ?? '';

	return {
		title,
		description,
		ogTitle: title,
		ogDescription: description,
		ogType,
		ogUrl,
		ogImage,
		twitterCard: ogImage ? 'summary_large_image' : 'summary',
		twitterTitle: title,
		twitterDescription: description,
		twitterImage: ogImage,
		jsonLdString: opts.jsonLd ? JSON.stringify(opts.jsonLd) : null
	};
}
