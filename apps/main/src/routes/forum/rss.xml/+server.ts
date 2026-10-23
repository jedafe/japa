import { env } from '$env/dynamic/private';
import { buildApiUrl } from '@japaconnect/api-url';
import { JC_SURFACE } from '$lib/server/api';
import { tiptapExcerpt } from '$lib/tiptap/serialize';
import type { ForumThread } from '$lib/types';
import type { RequestHandler } from './$types';

/**
 * RSS feed — GET /forum/rss.xml
 *
 * The layout's <link rel="alternate" type="application/rss+xml">
 * points here for forum autodiscovery. The feed is forum-scoped: items are
 * the latest public threads from GET /forum (Section 08), not the
 * platform-wide /feed.xml — a reader subscribing to /forum expects forum
 * threads only.
 *
 * Ref: japaconnect-engineering/sections/08-forum-qa-solved.md
 *      japaconnect-engineering/sections/26-seo-performance.md
 */
const PAGE_SIZE = 50;

function escapeXml(s: string): string {
	return s
		.replaceAll('&', '&amp;')
		.replaceAll('<', '&lt;')
		.replaceAll('>', '&gt;')
		.replaceAll('"', '&quot;')
		.replaceAll("'", '&apos;');
}

export const GET: RequestHandler = async ({ fetch, url, getClientAddress }) => {
	const apiBase = env.PRIVATE_API_BASE_URL ?? 'http://localhost:8080';
	const headers: Record<string, string> = {
		'x-forwarded-host': url.host,
		host: url.host,
		// Identify this surface for scoped feature-flag resolution.
		'x-jc-surface': JC_SURFACE
	};
	// Forward the real client IP so the backend's GeoIP middleware can
	// resolve the visitor's country for geo-aware personalization (ADR-064).
	const clientIP = getClientAddress();
	if (clientIP) headers['x-forwarded-for'] = clientIP;

	const res = await fetch(buildApiUrl(`/forum?page_size=${PAGE_SIZE}`, apiBase), { headers });

	if (!res.ok) {
		return new Response('RSS feed temporarily unavailable', { status: 502 });
	}

	const envelope = (await res.json()) as { data?: ForumThread[] };
	const threads = envelope.data ?? [];

	const items = threads
		.map((t) => {
			const link = `${url.origin}/forum/${t.slug}`;
			const excerpt = tiptapExcerpt(t.body, 300);
			return `    <item>
      <title>${escapeXml(t.title)}</title>
      <link>${escapeXml(link)}</link>
      <guid isPermaLink="false">${escapeXml(t.id)}</guid>
      <description>${escapeXml(excerpt)}</description>
      <pubDate>${new Date(t.created_at).toUTCString()}</pubDate>
    </item>`;
		})
		.join('\n');

	const xml = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Japa Connect — Forum</title>
    <link>${escapeXml(url.origin + '/forum')}</link>
    <description>Latest discussions from the Japa Connect community forum.</description>
    <language>en</language>
    <lastBuildDate>${new Date().toUTCString()}</lastBuildDate>
${items}
  </channel>
</rss>
`;

	return new Response(xml, {
		headers: {
			'content-type': 'application/rss+xml; charset=utf-8',
			'cache-control': 'public, max-age=900'
		}
	});
};
