// Proxies /llms.txt to the Go backend's dynamic handler (Section 43).
// In production nginx routes this path straight to the Go API, so this route
// only serves dev and non-nginx deployments. Falls back to a minimal static
// file (nav + discovery links, no DB-backed content) when the backend is
// unreachable. Returns 404 on non-indexable deployments, matching the
// backend handler.
import type { RequestHandler } from './$types';
import { env } from '$env/dynamic/private';
import { API_BASE, JC_SURFACE } from '$lib/server/api';

const NON_PROD_ENVS = new Set(['development', 'dev', 'staging', 'stage', 'test', 'preview', 'local']);
const blocksIndexing = NON_PROD_ENVS.has((env.PUBLIC_JC_ENV ?? env.JC_ENV ?? '').toLowerCase());

const fallback = (origin: string) =>
	[
		'# JapaConnect',
		'',
		'> Joining Africa\'s People Abroad.',
		'',
		'## Top Links',
		'',
		`- [Blog](${origin}/blog)`,
		`- [Forum](${origin}/forum)`,
		`- [Spaces](${origin}/spaces)`,
		`- [Tags](${origin}/tags)`,
		`- [Deals](${origin}/deals)`,
		`- [Tools](${origin}/tools)`,
		`- [Roadmap](${origin}/roadmap)`,
		`- [Academy](${origin}/academy)`,
		`- [Docs](${origin}/docs)`,
		'',
		'## Optional',
		'',
		`- [Extended content index](${origin}/llms-full.txt)`,
		`- [RSS feed](${origin}/feed.xml)`,
		`- [Sitemap](${origin}/sitemap.xml)`,
		''
	].join('\n');

export const GET: RequestHandler = async ({ request, url }) => {
	if (blocksIndexing) {
		return new Response('Not found', { status: 404 });
	}

	try {
		const res = await fetch(`${API_BASE}/llms.txt`, {
			headers: { 'X-Forwarded-Host': url.hostname, 'x-jc-surface': JC_SURFACE },
			signal: request.signal
		});

		if (res.ok) {
			return new Response(await res.text(), {
				headers: { 'Content-Type': 'text/plain; charset=utf-8', 'Cache-Control': 'public, max-age=3600' }
			});
		}
	} catch {
		// Backend unreachable — fall through to the static file.
	}

	return new Response(fallback(url.origin), {
		headers: { 'Content-Type': 'text/plain; charset=utf-8', 'Cache-Control': 'public, max-age=600' }
	});
};
