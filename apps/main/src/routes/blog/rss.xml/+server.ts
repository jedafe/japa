import { env } from '$env/dynamic/private';
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
export const GET: RequestHandler = async ({ fetch, url }) => {
	const apiBase = env.PRIVATE_API_BASE_URL ?? 'http://localhost:8080';
	const res = await fetch(`${apiBase}/api/v1/blog/feed.xml`, {
		headers: {
			'x-forwarded-host': url.host,
			host: url.host
		}
	});

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
