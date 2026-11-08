// Env-gated robots.txt. Production allows crawling and points crawlers at the
// sitemap; non-production deployments (stage/dev) return a blanket
// Disallow: / so they can never leak into search indexes.
import type { RequestHandler } from './$types';
import { PUBLIC_JC_ENV, JC_ENV } from '$app/env/private';

// Fails open: only explicitly non-production environments are blocked.
const NON_PROD_ENVS = new Set([
	'development',
	'dev',
	'staging',
	'stage',
	'test',
	'preview',
	'local'
]);

const blocksIndexing = NON_PROD_ENVS.has((PUBLIC_JC_ENV ?? JC_ENV ?? '').toLowerCase());

export const GET: RequestHandler = async ({ url }) => {
	const body = !blocksIndexing
		? `User-agent: *\nAllow: /\n\nSitemap: ${url.origin}/sitemap.xml\n`
		: 'User-agent: *\nDisallow: /\n';

	return new Response(body, {
		headers: {
			'Content-Type': 'text/plain',
			'Cache-Control': 'public, max-age=3600'
		}
	});
};
