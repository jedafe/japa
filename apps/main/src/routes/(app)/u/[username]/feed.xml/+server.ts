import { env } from '$env/dynamic/private';
import { buildApiUrl } from '@japaconnect/api-url';
import { JC_SURFACE } from '$lib/server/api';
import type { RequestHandler } from './$types';

/**
 * RSS feed proxy — GET /u/{username}/feed.xml
 * Consumes the backend's public profile feed (GET /users/{username}/feed.xml,
 * registered on the API root — not under /api/v1) so RSS readers can
 * autodiscover a user's public post feed from their profile page.
 *
 * Ref: japaconnect-engineering/sections/26-seo-performance.md (RSS feeds)
 */
export const GET: RequestHandler = async ({ fetch, params, url, request, getClientAddress }) => {
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
	const ifNoneMatch = request.headers.get('if-none-match');
	if (ifNoneMatch) headers['if-none-match'] = ifNoneMatch;
	const res = await fetch(buildApiUrl(`/users/${encodeURIComponent(params.username)}/feed.xml`, apiBase, { version: '' }), { headers });

	if (res.status === 304) {
		return new Response(null, { status: 304 });
	}

	if (!res.ok) {
		// 404 means the user doesn't exist — surface it as 404, not a proxy error.
		if (res.status === 404) {
			return new Response('User not found', { status: 404 });
		}
		return new Response('RSS feed temporarily unavailable', { status: 502 });
	}

	const body = await res.text();
	const respHeaders: Record<string, string> = {
		'content-type': 'application/rss+xml; charset=utf-8',
		'cache-control': 'public, max-age=900'
	};
	const etag = res.headers.get('etag');
	if (etag) respHeaders['etag'] = etag;
	return new Response(body, { headers: respHeaders });
};
