import type { RequestHandler } from './$types';
import { API_V1 } from '$lib/server/api';
import { canonicalUrl } from '$lib/seo';
import type { TrackSummary } from '$lib/types';

/**
 * Per-host sitemap for academy.japa.to (sitemap-protocol same-host
 * rule: academy/docs/tools cannot be `loc` entries under japa.to).
 * Advertised by static/robots.txt as Sitemap: {origin}/sitemap.xml.
 *
 * Data: GET /academy/catalog (public, paginated) → /tracks/{slug}.
 * Detail-only surfaces (lessons, dashboards, my-learning, search, gated
 * lessons, certificates) are excluded by construction — only catalog tracks
 * plus the landing page are emitted. Fails open to a minimal index.
 */
export const GET: RequestHandler = async ({ fetch }) => {
	const urls = new Set<string>(['/']);
	try {
		let offset = 0;
		const limit = 100;
		for (;;) {
			const res = await fetch(`${API_V1}/academy/catalog?limit=${limit}&offset=${offset}`, {
				headers: { 'x-jc-surface': 'academy' }
			});
			if (!res.ok) break;
			const body = await res.json();
			const items: TrackSummary[] = body?.data ?? body?.items ?? [];
			if (items.length === 0) break;
			for (const t of items) {
				if (t.slug) urls.add(`/tracks/${t.slug}`);
			}
			offset += items.length;
			const total: number = body?.meta?.total ?? body?.total ?? 0;
			if (total > 0 && offset >= total) break;
			if (items.length < limit) break;
		}
	} catch {
		// Fall through with the minimal index.
	}

	const entries = [...urls]
		.map((p) => `  <url><loc>${canonicalUrl(p)}</loc><changefreq>weekly</changefreq></url>`)
		.join('\n');
	const xml = `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n${entries}\n</urlset>`;
	return new Response(xml, {
		headers: {
			'content-type': 'application/xml',
			'cache-control': 'public, max-age=3600'
		}
	});
};
