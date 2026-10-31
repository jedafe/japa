// Per-host llms.txt for docs.japa.to — an LLM-readable index of the public
// documentation, generated from GET /docs/nav (same source as the sitemap).
// Gated audiences/keys/search/playground are never in the nav tree, so they
// are excluded by construction. Env-gated like robots.txt: non-production
// deployments 404. Fails open to a minimal file on API error.
import type { RequestHandler } from './$types';
import { env } from '$env/dynamic/private';
import { API_V1 } from '$lib/server/api';
import { DOCS_ORIGIN } from '$lib/seo/meta';
import type { DocNavTree, DocNavItem } from '$lib/types';

const NON_PROD_ENVS = new Set(['development', 'dev', 'staging', 'stage', 'test', 'preview', 'local']);
const blocksIndexing = NON_PROD_ENVS.has((env.PUBLIC_JC_ENV ?? env.JC_ENV ?? '').toLowerCase());

const AUDIENCE_HEADINGS: Record<string, string> = {
	users: 'User Guides',
	premium: 'Premium',
	organizations: 'Organizations',
	moderators: 'Moderators',
	api: 'API Reference',
	admin: 'Admin',
	superadmin: 'Superadmin'
};

// Markdown link text must not contain [ ] or newlines.
const clean = (s: string) => s.replace(/[\r\n]+/g, ' ').replace(/\[/g, '(').replace(/\]/g, ')').trim();

export const GET: RequestHandler = async ({ fetch }) => {
	if (blocksIndexing) {
		return new Response('Not found', { status: 404 });
	}

	const origin = DOCS_ORIGIN.replace(/\/$/, '');
	// Site name comes from public settings (Rule 16) — never hardcode the brand.
	let siteName = 'JapaConnect';
	try {
		const s = await fetch(`${API_V1}/site/settings/public`, { headers: { 'x-jc-surface': 'docs' } });
		if (s.ok) {
			const body = await s.json();
			const name = (body?.data ?? body)?.['general.site_name'];
			if (typeof name === 'string' && name.trim()) siteName = name.trim();
		}
	} catch {
		// keep the fallback
	}
	const sections: string[] = [
		`# ${siteName} Docs`,
		'',
		`> Documentation for ${siteName} — guides, API reference, and surface manuals.`,
		'',
		'## Index',
		'',
		`- [Docs home](${origin}/docs)`,
		`- [Categories](${origin}/docs/categories)`,
		`- [Groups](${origin}/docs/groups)`,
		`- [API](${origin}/docs/api)`,
		''
	];

	try {
		const res = await fetch(`${API_V1}/docs/nav`, {
			headers: { 'x-jc-surface': 'docs' }
		});
		if (res.ok) {
			const body = await res.json();
			const trees: DocNavTree[] = body?.data ?? body ?? [];
			for (const tree of trees) {
				const lines: string[] = [];
				for (const item of tree.items ?? []) walk(item, tree.audience, origin, lines);
				if (lines.length > 0) {
					sections.push(`## ${AUDIENCE_HEADINGS[tree.audience] ?? clean(tree.audience)}`, '', ...lines, '');
				}
			}
		}
	} catch {
		// Fall through with the minimal index.
	}

	sections.push('## Optional', '', `- [Sitemap](${origin}/sitemap.xml)`, '');

	return new Response(sections.join('\n'), {
		headers: {
			'Content-Type': 'text/plain; charset=utf-8',
			'Cache-Control': 'public, max-age=3600'
		}
	});
};

function walk(item: DocNavItem, audience: string, origin: string, lines: string[]): void {
	if (item.slug) lines.push(`- [${clean(item.title)}](${origin}/docs/${audience}/${item.slug})`);
	for (const child of item.children ?? []) walk(child, audience, origin, lines);
}
