import { PRIVATE_API_BASE_URL } from '$app/env/private';
import { buildApiUrl } from '@japaconnect/api-url';
import { JC_SURFACE, forwardingHeaders } from '#lib/server/api.js';
import type { RequestHandler } from './$types';

/**
 * NodeInfo discovery proxy — GET /.well-known/nodeinfo
 * Points fediverse crawlers at the NodeInfo 2.1 document served by the
 * backend on the API origin.
 *
 * Ref: japaconnect-engineering/sections/126-activitypub-federation.md
 */
export const GET: RequestHandler = async ({ fetch, url, getClientAddress }) => {
	const apiBase = PRIVATE_API_BASE_URL ?? 'http://localhost:8080';
	const headers: Record<string, string> = {
		'x-forwarded-host': url.host,
		host: url.host,
		'x-jc-surface': JC_SURFACE
	};
	// Forward the verbatim client trail so the backend's
	// trusted-proxy resolver sees the real visitor IP (§37).
	Object.assign(headers, forwardingHeaders({ request, getClientAddress }));

	// The backend well-known document links the 2.1 stats endpoint;
	// fetch the API-side well-known so the href stays consistent.
	const res = await fetch(buildApiUrl('/.well-known/nodeinfo', apiBase, { version: '' }), {
		headers
	});

	if (!res.ok) {
		return new Response('NodeInfo unavailable', { status: 502 });
	}

	return new Response(await res.text(), {
		headers: {
			'content-type': 'application/json',
			'cache-control': 'public, max-age=300',
			'access-control-allow-origin': '*'
		}
	});
};
