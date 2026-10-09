import type { RequestHandler } from './$types';
import { API_V1 } from '$lib/server/api';
import { DOCS_ORIGIN } from '$lib/seo/meta';
import type { DocNavTree, DocNavItem } from '$lib/types';

/**
 * Per-host sitemap for docs.japaconnect.com (sitemap-protocol same-host rule:
 * academy/docs/tools cannot be `loc` entries under japaconnect.com).
 * Advertised by static/robots.txt as Sitemap: {origin}/sitemap.xml.
 *
 * Data: GET /docs/nav (public tree, no token) → /docs/{audience}/{slug}.
 * Gated audiences/keys/search/playground are never in the nav tree, so they
 * are excluded by construction. Fails open to a minimal index on API error.
 */
export const GET: RequestHandler = async ({ fetch }) => {
	const urls = new Set<string>(['/docs']);
	try {
		const res = await fetch(`${API_V1}/docs/nav`, {
			headers: { 'x-jc-surface': 'docs' }
		});
		if (res.ok) {
			const body = await res.json();
			const trees: DocNavTree[] = body?.data ?? body ?? [];
			for (const tree of trees) {
				urls.add(`/docs/${tree.audience}`);
				for (const item of tree.items ?? []) walk(item, tree.audience, urls);
			}
		}
	} catch {
		// Fall through with the minimal index.
	}

	const origin = DOCS_ORIGIN.replace(/\/$/, '');
	const entries = [...urls]
		.map((p) => `  <url><loc>${origin}${p}</loc><changefreq>weekly</changefreq></url>`)
		.join('\n');
	const xml = `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n${entries}\n</urlset>`;
	return new Response(xml, {
		headers: {
			'content-type': 'application/xml',
			'cache-control': 'public, max-age=3600'
		}
	});
};

function walk(item: DocNavItem, audience: string, urls: Set<string>): void {
	if (item.slug) urls.add(`/docs/${audience}/${item.slug}`);
	for (const child of item.children ?? []) walk(child, audience, urls);
}
