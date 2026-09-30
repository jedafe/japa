import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError, API_BASE } from '$lib/server/api';
import { buildMeta, techArticleLd, DOCS_ORIGIN } from '$lib/seo/meta';
import type { DocPageSummary } from '$lib/types';
import { tServer } from '$lib/i18n/dict';

type ApiEndpoint = { method: string; path: string; summary: string; tag: string };

// Path-item keys that are not HTTP methods (parameters, summary, servers,
// $ref…) must never render as endpoint rows.
const HTTP_METHODS = new Set(['get', 'post', 'put', 'patch', 'delete', 'head', 'options']);

/**
 * API audience landing page — lists published API reference pages and links
 * to the interactive playground. 'api' is always public (ADR-060), so no
 * audience gate is needed here (unlike [audience]/+page.server.ts which
 * gates the other audiences).
 *
 * When no published doc pages exist, the OpenAPI spec is fetched to display
 * endpoint summaries so the page is never blank (BUG-002, Rule 16).
 *
 * Ref: japaconnect-engineering/sections/44-docs-site.md (ADR-060)
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	let pages: DocPageSummary[] = [];
	try {
		pages = await apiGet<DocPageSummary[]>(ctx, '/docs/api');
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'API documentation not found' });
		}
		// Degrade to empty list — never blank the shell.
		pages = [];
	}

	// When no published doc pages exist, fetch the OpenAPI spec to show
	// endpoint summaries from the spec itself (Ref: §44, Rule 16).
	let endpoints: ApiEndpoint[] = [];
	if (pages.length === 0) {
		try {
			const res = await event.fetch(`${API_BASE}/docs/openapi.json`);
			if (res.ok) {
				const spec = (await res.json()) as Record<string, any>;
				const paths: Record<string, Record<string, any>> = spec.paths ?? {};
				for (const [path, methods] of Object.entries(paths)) {
					for (const [method, op] of Object.entries(methods)) {
						if (!HTTP_METHODS.has(method)) continue;
						endpoints.push({
							method: method.toUpperCase(),
							path,
							summary: op.summary ?? '',
							tag: op.tags?.[0] ?? 'Other'
						});
					}
				}
			}
		} catch {
			// Spec fetch is best-effort
		}
	}

	const locale = event.locals.locale;
	const appName = tServer(locale, 'app.name', 'JapaConnect Docs');
	const audienceLabel = tServer(locale, 'audience.api', 'API');
	const title = `${audienceLabel} · ${appName}`;
	const description = tServer(
		locale,
		'api.landing_desc',
		'API reference, authentication guides, and an interactive playground for testing endpoints.'
	);

	const meta = buildMeta({
		title,
		description,
		path: '/docs/api',
		jsonLd: techArticleLd({
			headline: title,
			dateModified: new Date().toISOString(),
			url: `${DOCS_ORIGIN}/docs/api`,
			description
		})
	});

	return { pages, endpoints, meta };
};
