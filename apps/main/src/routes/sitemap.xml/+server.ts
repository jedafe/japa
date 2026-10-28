// Proxies /sitemap.xml to the Go backend's sitemap index handler.
// In production nginx routes this path straight to the Go API, so this route
// only serves dev and non-nginx deployments — it exists so the Sitemap URL
// advertised by robots.txt resolves on those deployments too. Falls back to
// a minimal urlset containing just the landing page when the backend is
// unreachable. Returns 404 on non-indexable deployments.
import type { RequestHandler } from './$types';
import { env } from '$env/dynamic/private';
import { API_BASE, JC_SURFACE } from '$lib/server/api';

const NON_PROD_ENVS = new Set(['development', 'dev', 'staging', 'stage', 'test', 'preview', 'local']);
const blocksIndexing = NON_PROD_ENVS.has((env.PUBLIC_JC_ENV ?? env.JC_ENV ?? '').toLowerCase());

const fallback = (origin: string) =>
	`<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n  <url><loc>${origin}/</loc></url>\n</urlset>`;

export const GET: RequestHandler = async ({ request, url }) => {
	if (blocksIndexing) {
		return new Response('Not found', { status: 404 });
	}

	try {
		const res = await fetch(`${API_BASE}/sitemap.xml`, {
			headers: { 'X-Forwarded-Host': url.hostname, 'x-jc-surface': JC_SURFACE },
			signal: request.signal
		});

		if (res.ok) {
			return new Response(await res.text(), {
				headers: { 'Content-Type': 'application/xml', 'Cache-Control': 'public, max-age=3600' }
			});
		}
	} catch {
		// Backend unreachable — fall through to the minimal index.
	}

	return new Response(fallback(url.origin), {
		headers: { 'Content-Type': 'application/xml', 'Cache-Control': 'public, max-age=600' }
	});
};
