import { env } from '$env/dynamic/private';
import { buildApiUrl } from '@japaconnect/api-url';
import { JC_SURFACE } from '$lib/server/api';
import type { RequestHandler } from './$types';

/**
 * RSS feed proxy — GET /tags/{slug}/feed.xml
 * Canonical feed URL for a tag (matches the backend route mounted at the
 * API root outside /api/v1). The /tags/{slug}/rss.xml variant stays live
 * for existing subscribers; the tag hub advertises this path. Production
 * nginx also routes this path straight to the API — this route keeps the
 * feed reachable on the site origin in dev.
 *
 * Ref: japaconnect-engineering/sections/26-seo-performance.md
 */
export const GET: RequestHandler = async ({ fetch, params, url, request, getClientAddress }) => {
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

	const res = await fetch(
		buildApiUrl(`/tags/${encodeURIComponent(params.slug)}/feed.xml`, apiBase, { version: '' }),
		{ headers }
	);

	if (res.status === 304) {
		return new Response(null, { status: 304 });
	}

	if (!res.ok) {
		if (res.status === 404) {
			return new Response('Tag not found', { status: 404 });
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
