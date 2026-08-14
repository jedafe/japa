// Proxies /robots.txt to the Go backend's dynamic robots.txt handler.
//
// robots.txt is generated dynamically from the admin-configured bot registry
// (Section 43) and cached in Valkey. Serving a static file from apps/main/static
// would shadow the backend's dynamic output, bypassing admin bot policies.
//
// Ref: japaconnect-engineering/sections/43-bot-crawler-management.md
import type { RequestHandler } from './$types';
import { API_BASE } from '$lib/server/api';

const FALLBACK = 'User-agent: *\nDisallow: /admin/\nDisallow: /api/\n\nSitemap: /sitemap.xml\n';

export const GET: RequestHandler = async ({ request, url }) => {
	try {
		const backendURL = `${API_BASE}/robots.txt`;
		const res = await fetch(backendURL, {
			headers: {
				// Forward the tenant host so the backend's tenant middleware can
				// resolve the correct bot registry for multi-tenant deployments.
				'X-Forwarded-Host': url.hostname
			},
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
