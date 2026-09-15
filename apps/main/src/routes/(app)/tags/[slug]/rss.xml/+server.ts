import { env } from '$env/dynamic/private';
import { buildApiUrl } from '@japaconnect/api-url';
import type { RequestHandler } from './$types';

/**
 * RSS feed proxy — GET /tags/{slug}/rss.xml
 *
 * The tag hub's <link rel="alternate" type="application/rss+xml"> points
 * here for feed autodiscovery. This proxies to the backend's
 * /tags/{slug}/feed.xml (api-routes-catalog.md — public, platform-scoped,
 * mounted at the API root outside /api/v1). Keeps the feed URL on the
 * site origin so readers never see the backend host.
 *
 * Ref: japaconnect-engineering/sections/26-seo-performance.md
 */
export const GET: RequestHandler = async ({ fetch, params, url }) => {
	const apiBase = env.PRIVATE_API_BASE_URL ?? 'http://localhost:8080';
	const res = await fetch(
		buildApiUrl(`/tags/${encodeURIComponent(params.slug)}/feed.xml`, apiBase, { version: '' }),
		{
			headers: {
				'x-forwarded-host': url.host,
				host: url.host
			}
		}
	);

	if (!res.ok) {
		return new Response('RSS feed temporarily unavailable', { status: 502 });
	}

	const body = await res.text();
	return new Response(body, {
		headers: {
			'content-type': 'application/rss+xml; charset=utf-8',
			'cache-control': 'public, max-age=900'
		}
	});
};
