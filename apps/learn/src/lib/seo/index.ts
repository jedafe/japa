/**
 * SEO helpers for the Learn app.
 * Ref: ADR-060 (canonical URLs on university.japaconnect.com), Rule 16 (no hardcoded
 * data — these helpers shape API data, they don't replace it).
 */

export interface SeoMeta {
	title: string;
	description?: string;
	/** Canonical URL (university.japaconnect.com/<path>). */
	canonicalPath?: string;
	/** Open Graph image URL. */
	ogImage?: string;
	/** JSON-LD structured data object(s). */
	jsonLd?: unknown | unknown[];
	/** robots directive. */
	noindex?: boolean;
}

const CANONICAL_ORIGIN = 'https://university.japaconnect.com';

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
export function courseJsonLd(track: {
	title: string;
	description?: string | null;
	instructor_name?: string | null;
	instructor?: { displayName: string; username?: string } | null;
	thumbnail_url?: string | null;
}): unknown {
	const instructorName = track.instructor?.displayName ?? track.instructor_name ?? 'JapaConnect';
	return {
		'@context': 'https://schema.org',
		'@type': 'Course',
		name: track.title,
		description: track.description ?? undefined,
		provider: {
			'@type': 'Organization',
			name: 'JapaConnect',
			sameAs: 'https://japaconnect.com'
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
