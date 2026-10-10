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

// Single site-origin override for dev/staging builds — also drives the API
// base (lib/api.ts) and MV3 host_permissions (wxt.config.ts).
//
// Cleartext guard: OAuth tokens and API keys ride on this origin. A
// non-localhost http:// build would send bearer credentials in cleartext,
// interceptable on the network — fail closed at module load so a
// misconfigured WXT_JC_SITE_URL can never ship tokens over HTTP.
function resolveSiteOrigin(): string {
	const raw = ((import.meta.env.WXT_JC_SITE_URL as string | undefined) ?? 'https://japa.to').replace(/\/+$/, '');
	try {
		const parsed = new URL(raw);
		const isLocal = parsed.hostname === 'localhost' || parsed.hostname === '127.0.0.1' || parsed.hostname === '[::1]';
		if (parsed.protocol === 'http:' && !isLocal) {
			throw new Error(
				`WXT_JC_SITE_URL must be https in non-local builds (got ${raw}) — refusing to send tokens over cleartext HTTP`
			);
		}
	} catch (e) {
		if (e instanceof Error && e.message.startsWith('WXT_JC_SITE_URL')) throw e;
		throw new Error(`Invalid WXT_JC_SITE_URL: ${raw}`);
	}
	return raw;
}
export const SITE_ORIGIN = resolveSiteOrigin();

export function siteUrl(path: string): string {
	return `${SITE_ORIGIN}${path}`;
}
