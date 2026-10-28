import { env } from '$env/dynamic/private';
import { buildApiUrl } from '@japaconnect/api-url';
import { JC_SURFACE } from '$lib/server/api';
import type { RequestHandler } from './$types';

/**
 * RSS feed proxy — GET /forum/rss.xml
 *
 * The layout's <link rel="alternate" type="application/rss+xml">
 * points here for forum autodiscovery. Proxies the backend's forum feed
 * (GET /forum/feed.xml, mounted at the API root outside /api/v1) so the
 * feed is cached in Valkey server-side like every other platform feed —
 * the previous hand-rolled XML here re-fetched the thread list on every
 * reader poll.
 *
 * Ref: japaconnect-engineering/sections/26-seo-performance.md
 */
export const GET: RequestHandler = async ({ fetch, url, request, getClientAddress }) => {
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

	const res = await fetch(buildApiUrl('/forum/feed.xml', apiBase, { version: '' }), { headers });

	if (res.status === 304) {
		return new Response(null, { status: 304 });
	}

	if (!res.ok) {
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
