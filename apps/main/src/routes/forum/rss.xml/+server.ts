import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';

/**
 * RSS feed proxy — GET /forum/rss.xml
 *
 * The layout's <link rel="alternate" type="application/rss+xml">
 * points here for forum autodiscovery. This proxies to the backend's
 * /feed.xml (Section 26) which includes all published content (forum
 * threads, blog posts, etc.). The backend feed is tenant-scoped and
 * cached via the rss:regenerate Asynq task.
 *
 * Ref: japaconnect-engineering/sections/26-seo-performance.md
 */
export const GET: RequestHandler = async ({ fetch, url }) => {
	const apiBase = env.PRIVATE_API_BASE_URL ?? 'http://localhost:8080';
	const res = await fetch(`${apiBase}/feed.xml`, {
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
			'cache-control': 'public, max-age=900'
		}
	});
};
