import { DEFAULT_SITE_NAME } from '$lib/brand';
/**
 * SEO helpers for the Academy app.
 * Ref: ADR-060 (canonical URLs on academy.japa.to), Rule 16 (no hardcoded
 * data — these helpers shape API data, they don't replace it).
 */

export interface SeoMeta {
	title: string;
	description?: string;
	/** Canonical URL (academy.japa.to/<path>). */
	canonicalPath?: string;
	/** Open Graph image URL. */
	ogImage?: string;
	/** JSON-LD structured data object(s). */
	jsonLd?: unknown | unknown[];
	/** robots directive. */
	noindex?: boolean;
}

const CANONICAL_ORIGIN = 'https://academy.japa.to';

/** Build the absolute canonical URL for a path. */
export function canonicalUrl(path: string): string {
	if (path.startsWith('http')) return path;
	const normalized = path.startsWith('/') ? path : `/${path}`;
	return `${CANONICAL_ORIGIN}${normalized}`;
}

/**
 * Build a Course JSON-LD object from a track detail.
 * Ref: ADR-060 (Track Detail — Course JSON-LD), schema.org/Course.
 */
export function courseJsonLd(
	track: {
		title: string;
		description?: string | null;
		instructor_name?: string | null;
		instructor?: { displayName: string; username?: string } | null;
		thumbnail_url?: string | null;
	},
	siteName = DEFAULT_SITE_NAME
): unknown {
	const instructorName = track.instructor?.displayName ?? track.instructor_name ?? siteName;
	return {
		'@context': 'https://schema.org',
		'@type': 'Course',
		name: track.title,
		description: track.description ?? undefined,
		provider: {
			'@type': 'Organization',
			name: siteName,
			sameAs: 'https://japa.to'
		},
		author: {
			'@type': 'Person',
			name: instructorName
		},
		image: track.thumbnail_url ?? undefined
	};
}

/** Build a BreadcrumbList JSON-LD from a list of {name, path} crumbs. */
export function breadcrumbJsonLd(
	crumbs: Array<{ name: string; path: string }>
): unknown {
	return {
		'@context': 'https://schema.org',
		'@type': 'BreadcrumbList',
		itemListElement: crumbs.map((c, i) => ({
			'@type': 'ListItem',
			position: i + 1,
			name: c.name,
			item: canonicalUrl(c.path)
		}))
	};
}

/**
 * robots meta content for a page head. Public pages pass false (or omit);
 * authenticated / private / gated pages pass true. Wires the SeoMeta
 * `noindex` prop to `<meta name="robots">`.
 */
export function robotsContent(noindex?: boolean): string {
	return noindex ? 'noindex, nofollow' : 'index, follow';
}

/**
 * Plain-text excerpt from rendered lesson/article HTML for meta
 * descriptions. Strips tags, collapses whitespace, truncates to maxLength.
 */
export function excerptFromHtml(
	html: string | null | undefined,
	maxLength = 160
): string | null {
	if (!html) return null;
	const text = html
		.replace(/<[^>]*>/g, ' ')
		.replace(/\s+/g, ' ')
		.trim();
	if (!text) return null;
	if (text.length <= maxLength) return text;
	return `${text.slice(0, maxLength - 1).trimEnd()}…`;
}

/**
 * Serialize JSON-LD into a safe inline <script type="application/ld+json">
 * tag for `{@html}` rendering. Escapes `<`/`>` so a `</script>` sequence
 * inside data can never break out of the tag (same contract as main's
 * safeJsonLdScript).
 */
export function safeJsonLd(payload: unknown): string {
	if (payload === null || payload === undefined) return '';
	const json = typeof payload === 'string' ? payload : JSON.stringify(payload);
	const safe = json.replace(/</g, '\\u003c').replace(/>/g, '\\u003e');
	return `<script type="application/ld+json">${safe}</` + `script>`;
}

/** ItemList JSON-LD for catalog/collection pages. */
export function itemListJsonLd(items: Array<{ name: string; path: string }>): unknown {
	return {
		'@context': 'https://schema.org',
		'@type': 'ItemList',
		itemListElement: items.map((it, i) => ({
			'@type': 'ListItem',
			position: i + 1,
			name: it.name,
			url: canonicalUrl(it.path)
		}))
	};
}
