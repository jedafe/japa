// Proxies /llms-full.txt to the Go backend's dynamic handler (Section 43).
// In production nginx routes this path straight to the Go API, so this route
// only serves dev and non-nginx deployments. When the backend is unreachable
// there is no meaningful static fallback for the extended file — we 503 so
// crawlers retry rather than caching a shell. Returns 404 on non-indexable
// deployments, matching the backend handler.
import type { RequestHandler } from './$types';
import { PUBLIC_JC_ENV, JC_ENV } from '$app/env/private';
import { API_BASE, JC_SURFACE } from '#lib/server/api.js';

const NON_PROD_ENVS = new Set([
	'development',
	'dev',
	'staging',
	'stage',
	'test',
	'preview',
	'local'
]);

const blocksIndexing = NON_PROD_ENVS.has((PUBLIC_JC_ENV ?? JC_ENV ?? '').toLowerCase());

export const GET: RequestHandler = async ({ request, url }) => {
	if (blocksIndexing) {
		return new Response('Not found', { status: 404 });
	}

	try {
		const res = await fetch(`${API_BASE}/llms-full.txt`, {
			headers: { 'X-Forwarded-Host': url.hostname, 'x-jc-surface': JC_SURFACE },
			signal: request.signal
		});

		if (res.ok) {
			return new Response(await res.text(), {
				headers: { 'Content-Type': 'text/plain; charset=utf-8', 'Cache-Control': 'public, max-age=3600' }
			});
		}
		if (res.status === 404) {
			return new Response('Not found', { status: 404 });
		}
	} catch {
		// Backend unreachable — 503 below.
	}

	return new Response('llms-full.txt temporarily unavailable', {
		status: 503,
		headers: { 'Content-Type': 'text/plain; charset=utf-8', 'Cache-Control': 'no-store' }
	});
};
