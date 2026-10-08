import { env } from '$env/dynamic/private';
import { buildApiUrl } from '@japaconnect/api-url';
import { JC_SURFACE } from '$lib/server/api';
import type { RequestHandler } from './$types';

/**
 * RSS feed proxy — GET /blog/rss.xml
 * BUG-007: the layout's <link rel="alternate" type="application/rss+xml">
 * pointed here, but no endpoint existed. This proxies to the backend's
 * /api/v1/blog/feed.xml (Section 24) so RSS readers can autodiscover the
 * feed via the standard /blog/rss.xml path.
 *
 * Ref: japaconnect-engineering/sections/24-blog-extension.md
 */
export const GET: RequestHandler = async ({ fetch, url, getClientAddress }) => {
	const apiBase = env.PRIVATE_API_BASE_URL ?? 'http://localhost:8080';
	const headers: Record<string, string> = {
		'x-forwarded-host': url.host,
		host: url.host,
		// Identify this surface for scoped feature-flag resolution.
		'x-jc-surface': JC_SURFACE
	};
	// Forward the real client IP so the backend's GeoIP middleware can
	// resolve the visitor's country for geo-aware personalization (ADR-064).
	const clientIP = getClientAddress();
	if (clientIP) headers['x-forwarded-for'] = clientIP;
	const res = await fetch(buildApiUrl('/blog/feed.xml', apiBase), { headers });

	if (!res.ok) {
		return new Response('RSS feed temporarily unavailable', { status: 502 });
	}

	const body = await res.text();
	return new Response(body, {
		headers: {
			'content-type': 'application/rss+xml; charset=utf-8',
			'cache-control': 'public, max-age=300'
		}
	});
};
