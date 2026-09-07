// Ref: japaconnect-engineering/sections/14-content-type-system.md
//      ADR-058 (route families per content type)
//
// Maps a content_type to its canonical public detail route. Every surface
// that links to a content item by slug (feed cards, sidebar widgets,
// related-content lists) must go through this helper so per-type routes
// stay consistent — e.g. stories live at /story/{slug}, not
// /content/{slug}. Unknown types fall back to the generic /content/{slug}
// route, which resolves any content_type.

const CONTENT_ROUTE_PREFIX: Record<string, string> = {
	blog_post: '/blog',
	guide: '/guide',
	discussion: '/forum',
	forum_thread: '/forum', // legacy alias for 'discussion'
	comparison: '/comparisons',
	story: '/story',
	achievement: '/achievements',
	page: '/pages',
	job_listing: '/jobs',
	service_listing: '/services'
};

/** Canonical detail href for a content item, keyed by content_type. */
export function contentHref(item: { slug: string; content_type?: string | null }): string {
	const prefix = (item.content_type && CONTENT_ROUTE_PREFIX[item.content_type]) || '/content';
	return `${prefix}/${item.slug}`;
}
