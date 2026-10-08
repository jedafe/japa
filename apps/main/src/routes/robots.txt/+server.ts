// Proxies /robots.txt to the Go backend's dynamic robots.txt handler.
//
// robots.txt is generated dynamically from the admin-configured bot registry
// (Section 43) and cached in Valkey. Serving a static file from apps/main/static
// would shadow the backend's dynamic output, bypassing admin bot policies.
//
// Ref: japaconnect-engineering/sections/43-bot-crawler-management.md
import type { RequestHandler } from './$types';
import { API_BASE, JC_SURFACE } from '$lib/server/api';

const FALLBACK = 'User-agent: *\nDisallow: /admin/\nDisallow: /api/\n\nSitemap: /sitemap.xml\n';

export const GET: RequestHandler = async ({ request, url, getClientAddress }) => {
	try {
		const backendURL = `${API_BASE}/robots.txt`;
		const headers: Record<string, string> = {
			// Forward the request host so the backend's host middleware can
			// resolve the correct bot registry for deployments.
			'X-Forwarded-Host': url.hostname,
			// Identify this surface for scoped feature-flag resolution.
			'x-jc-surface': JC_SURFACE
		};
		// Forward the real client IP so the backend's GeoIP middleware can
		// resolve the visitor's country for geo-aware personalization (ADR-064).
		const clientIP = getClientAddress();
		if (clientIP) headers['x-forwarded-for'] = clientIP;
		const res = await fetch(backendURL, {
			headers,
			signal: request.signal
		});

		if (res.ok) {
			const text = await res.text();
			return new Response(text, {
				headers: {
					'Content-Type': 'text/plain',
					'Cache-Control': 'public, max-age=86400'
				}
			});
		}
	} catch {
		// Backend unreachable — fall through to the permissive default.
	}

	return new Response(FALLBACK, {
		headers: {
			'Content-Type': 'text/plain',
			'Cache-Control': 'public, max-age=3600'
		}
	});
};
