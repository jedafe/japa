import { env } from '$env/dynamic/private';
import { buildApiUrl } from '@japaconnect/api-url';
import { JC_SURFACE } from '$lib/server/api';
import type { RequestHandler } from './$types';

/**
 * RSS feed proxy — GET /jobs/feed.xml
 * Proxies the backend jobs type feed (GET /jobs/feed.xml, API root) so readers get Valkey-backed feeds on the
 * site origin. Forwards If-None-Match so conditional GETs reach the
 * backend's 304 path.
 *
 * Ref: japaconnect-engineering/sections/26-seo-performance.md
 */
export const GET: RequestHandler = async ({ fetch, url, request, getClientAddress }) => {
	const apiBase = env.PRIVATE_API_BASE_URL ?? 'http://localhost:8080';
	const headers: Record<string, string> = {
		'x-forwarded-host': url.host,
		host: url.host,
		'x-jc-surface': JC_SURFACE
	};
	const clientIP = getClientAddress();
	if (clientIP) headers['x-forwarded-for'] = clientIP;
	const ifNoneMatch = request.headers.get('if-none-match');
	if (ifNoneMatch) headers['if-none-match'] = ifNoneMatch;

	const res = await fetch(buildApiUrl('/jobs/feed.xml', apiBase, { version: '' }), { headers });

	if (res.status === 304) {
		return new Response(null, { status: 304 });
	}

	if (!res.ok) {
		if (res.status === 404) {
			return new Response('Feed not found', { status: 404 });
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
