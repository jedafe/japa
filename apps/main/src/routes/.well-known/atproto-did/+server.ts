import { PRIVATE_API_BASE_URL } from '$app/env/private';
import { buildApiUrl } from '@japaconnect/api-url';
import { JC_SURFACE } from '#lib/server/api.js';
import type { RequestHandler } from './$types';

/**
 * atproto-did proxy — GET /.well-known/atproto-did on member-handle
 * subdomains ({username}.japa.to). Bluesky's appview resolves member
 * handles through this well-known; the canonical DID text is produced
 * by the backend mini-PDS and proxied here so handle verification works
 * on the apex wildcard.
 *
 * Ref: japaconnect-engineering/sections/127-atproto-native.md
 */
export const GET: RequestHandler = async ({ fetch, url, getClientAddress }) => {
	const apiBase = PRIVATE_API_BASE_URL ?? 'http://localhost:8080';
	const headers: Record<string, string> = {
		'x-forwarded-host': url.host,
		'x-jc-surface': JC_SURFACE
	};
	const clientIP = getClientAddress();
	if (clientIP) headers['x-forwarded-for'] = clientIP;

	// Unversioned PDS endpoint — the default /api/v1 prefix is disabled.
	const target = buildApiUrl('/.well-known/atproto-did', apiBase, { version: '' });
	const res = await fetch(target, { headers });

	if (!res.ok) {
		const body = await res.text();
		return new Response(body || 'Not found', {
			status: res.status,
			headers: { 'content-type': res.headers.get('content-type') ?? 'text/plain' }
		});
	}

	return new Response(await res.text(), {
		headers: {
			'content-type': 'text/plain; charset=utf-8',
			'cache-control': 'public, max-age=300',
			'access-control-allow-origin': '*'
		}
	});
};
