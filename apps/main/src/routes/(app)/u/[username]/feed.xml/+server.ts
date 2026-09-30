import { env } from '$env/dynamic/private';
import { buildApiUrl } from '@japaconnect/api-url';
import type { RequestHandler } from './$types';

/**
 * RSS feed proxy — GET /u/{username}/feed.xml
 * Consumes the backend's public profile feed (GET /users/{username}/feed.xml,
 * registered on the API root — not under /api/v1) so RSS readers can
 * autodiscover a user's public post feed from their profile page.
 *
 * Ref: japaconnect-engineering/sections/26-seo-performance.md (RSS feeds)
 */
export const GET: RequestHandler = async ({ fetch, params, url }) => {
	const apiBase = env.PRIVATE_API_BASE_URL ?? 'http://localhost:8080';
	const res = await fetch(buildApiUrl(`/users/${encodeURIComponent(params.username)}/feed.xml`, apiBase, { version: '' }), {
		headers: {
			'x-forwarded-host': url.host,
			host: url.host
		}
	});

	if (!res.ok) {
		// 404 means the user doesn't exist — surface it as 404, not a proxy error.
		if (res.status === 404) {
			return new Response('User not found', { status: 404 });
		}
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
