/**
 * BreadcrumbList JSON-LD helper for the main app.
 * Emits schema.org/BreadcrumbList structured data for hierarchical pages
 * (blog posts, forum threads, space hubs) per Section 26.
 * Ref: japaconnect-engineering/sections/26-seo-performance.md
 */

export interface Breadcrumb {
	name: string;
	path: string;
}

/**
 * Build a BreadcrumbList JSON-LD object from an ordered list of crumbs
 * (root → leaf). Paths are relative to the site origin; the caller should
 * pass absolute URLs when the origin is known, or relative paths that
 * resolve against the page's origin.
 */
export function breadcrumbJsonLd(crumbs: Breadcrumb[]): unknown {
	return {
		'@context': 'https://schema.org',
		'@type': 'BreadcrumbList',
		itemListElement: crumbs.map((c, i) => ({
			'@type': 'ListItem',
			position: i + 1,
			name: c.name,
			item: c.path
		}))
	};
}
