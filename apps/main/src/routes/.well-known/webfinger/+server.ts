import { PRIVATE_API_BASE_URL } from '$app/env/private';
import { buildApiUrl } from '@japaconnect/api-url';
import { JC_SURFACE } from '#lib/server/api.js';
import type { RequestHandler } from './$types';

/**
 * WebFinger proxy — GET /.well-known/webfinger?resource=acct:user@domain
 * Fediverse handle discovery must resolve on the apex origin; the
 * canonical JRD is produced by the backend federation service and
 * proxied here so `user@japa.to` lookups work site-wide.
 *
 * Ref: japaconnect-engineering/sections/126-activitypub-federation.md
 */
export const GET: RequestHandler = async ({ fetch, url, request, getClientAddress }) => {
	const apiBase = PRIVATE_API_BASE_URL ?? 'http://localhost:8080';
	const resource = url.searchParams.get('resource') ?? '';
	const headers: Record<string, string> = {
		'x-forwarded-host': url.host,
		host: url.host,
		'x-jc-surface': JC_SURFACE
	};
	const clientIP = getClientAddress();
	if (clientIP) headers['x-forwarded-for'] = clientIP;

	const target = buildApiUrl(
		`/federation/webfinger?resource=${encodeURIComponent(resource)}`,
		apiBase
	);
	const res = await fetch(target, { headers });

	if (!res.ok) {
		const body = await res.text();
		return new Response(body || 'Not found', {
			status: res.status,
			headers: { 'content-type': res.headers.get('content-type') ?? 'application/json' }
		});
	}

	return new Response(await res.text(), {
		headers: {
			'content-type': 'application/jrd+json',
			'cache-control': 'public, max-age=300',
			'access-control-allow-origin': '*'
		}
	});
};
