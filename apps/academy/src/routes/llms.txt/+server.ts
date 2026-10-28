// Per-host llms.txt for academy.japa.to — an LLM-readable index of the
// public course catalog, generated from GET /academy/catalog (same source
// as the sitemap). Detail-only surfaces (lessons, dashboards, my-learning,
// gated lessons, certificates) are excluded by construction. Env-gated like
// robots.txt: non-production deployments 404. Fails open to a minimal file
// on API error.
import type { RequestHandler } from './$types';
import { env } from '$env/dynamic/private';
import { API_V1 } from '$lib/server/api';
import { canonicalUrl } from '$lib/seo';
import type { TrackSummary } from '$lib/types';

const NON_PROD_ENVS = new Set(['development', 'dev', 'staging', 'stage', 'test', 'preview', 'local']);
const blocksIndexing = NON_PROD_ENVS.has((env.PUBLIC_JC_ENV ?? env.JC_ENV ?? '').toLowerCase());

// Markdown link text must not contain [ ] or newlines.
const clean = (s: string) => s.replace(/[\r\n]+/g, ' ').replace(/\[/g, '(').replace(/\]/g, ')').trim();

export const GET: RequestHandler = async ({ fetch }) => {
	if (blocksIndexing) {
		return new Response('Not found', { status: 404 });
	}

	const sections: string[] = [
		'# JapaConnect Academy',
		'',
		'> Courses and learning tracks for the diaspora journey.',
		'',
		'## Index',
		'',
		`- [Academy home](${canonicalUrl('/')})`,
		''
	];

	try {
		const lines: string[] = [];
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
				if (t.slug && t.title) {
					lines.push(`- [${clean(t.title)}](${canonicalUrl(`/tracks/${t.slug}`)})`);
				}
			}
			offset += items.length;
			const total: number = body?.meta?.total ?? body?.total ?? 0;
			if (total > 0 && offset >= total) break;
			if (items.length < limit) break;
		}
		if (lines.length > 0) sections.push('## Tracks', '', ...lines, '');
	} catch {
		// Fall through with the minimal index.
	}

	sections.push('## Optional', '', `- [Sitemap](${canonicalUrl('/sitemap.xml')})`, '');

	return new Response(sections.join('\n'), {
		headers: {
			'Content-Type': 'text/plain; charset=utf-8',
			'Cache-Control': 'public, max-age=3600'
		}
	});
};
