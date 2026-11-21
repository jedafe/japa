import { PRIVATE_API_BASE_URL } from '$app/env/private';
import { buildApiUrl } from '@japaconnect/api-url';
import { JC_SURFACE, forwardingHeaders } from '#lib/server/api.js';
import type { RequestHandler } from './$types';

/**
 * RSS feed proxy — GET /roadmap/releases/feed.xml
 * Pretty site-origin URL for the releases feed so user-facing links never
 * expose the raw /api/v1 path. Proxies the backend's
 * GET /api/v1/roadmap/releases/feed.xml (§113, ADR-135).
 */
export const GET: RequestHandler = async ({ fetch, url, request, getClientAddress }) => {
	const apiBase = PRIVATE_API_BASE_URL ?? 'http://localhost:8080';
	const headers: Record<string, string> = {
		'x-forwarded-host': url.host,
		host: url.host,
		'x-jc-surface': JC_SURFACE
	};
	// Forward the verbatim client trail so the backend's
	// trusted-proxy resolver sees the real visitor IP (§37).
	Object.assign(headers, forwardingHeaders({ request, getClientAddress }));
	const ifNoneMatch = request.headers.get('if-none-match');
	if (ifNoneMatch) headers['if-none-match'] = ifNoneMatch;

	const res = await fetch(buildApiUrl('/roadmap/releases/feed.xml', apiBase), { headers });

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
