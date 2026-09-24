// Canonical content-route map — mirrors
// apps/main/src/lib/utils/contentLink.ts (ADR-058 route families). Keep in
// sync: unknown types fall back to /content/{slug}, which resolves any
// content_type.

const CONTENT_ROUTE_PREFIX: Record<string, string> = {
	blog_post: '/blog',
	guide: '/guide',
	discussion: '/forum',
	forum_thread: '/forum',
	comparison: '/comparisons',
	story: '/story',
	achievement: '/achievements',
	page: '/pages',
	job_listing: '/jobs',
	service_listing: '/services',
	video: '/video',
};

export function contentHref(item: { slug: string; content_type?: string | null }): string {
	const prefix = (item.content_type && CONTENT_ROUTE_PREFIX[item.content_type]) || '/content';
	return `${prefix}/${item.slug}`;
}

export const SITE_ORIGIN = (import.meta.env.WXT_JC_SITE_URL ?? 'https://japa.to').replace(/\/+$/, '');

export function siteUrl(path: string): string {
	return `${SITE_ORIGIN}${path}`;
}
