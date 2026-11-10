// Proxies /og/*.png to the Go backend's OG card renderer.
// In production nginx routes /og/*.png straight to the Go API, so this
// route only serves dev and non-nginx deployments — the same shadow-path
// pattern as sitemap.xml and robots.txt. og:image URLs must be absolute,
// so the page head points at the same-origin /og/ path everywhere.
import type { RequestHandler } from './$types';
import { API_BASE } from '#lib/server/api.js';

export const GET: RequestHandler = async ({ request, params }) => {
	try {
		const res = await fetch(`${API_BASE}/og/${params.rest}`, {
			signal: request.signal
		});

		if (res.ok) {
			return new Response(await res.arrayBuffer(), {
				headers: {
					'Content-Type': res.headers.get('Content-Type') ?? 'image/png',
					'Cache-Control': 'public, max-age=86400'
				}
			});
		}
	} catch {
		// Backend unreachable — fall through to 404.
	}

	return new Response('Not found', { status: 404 });
};
