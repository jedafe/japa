import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError, API_BASE } from '$lib/server/api';
import { renderDocContent, ensureHeadingIds, extractTocHeadings } from '$lib/tiptap/serialize';
import { buildMeta, techArticleLd, DOCS_ORIGIN } from '$lib/seo/meta';
import type { DocPage } from '$lib/types';
import { tServer } from '$lib/i18n/dict';

/**
 * Module-level OpenAPI spec cache (TTL 5 min). The spec is large and fetched
 * on every API doc page load to find the matching operation for the code
 * pane; caching avoids re-fetching the same ~MB JSON on every navigation.
 * Safe across requests in the same Node process (adapter-node).
 */
let specCache: { ts: number; spec: Record<string, unknown> } | null = null;
const SPEC_TTL_MS = 5 * 60 * 1000;

async function getOpenApiSpec(): Promise<Record<string, unknown> | null> {
	const now = Date.now();
	if (specCache && now - specCache.ts < SPEC_TTL_MS) {
		return specCache.spec;
	}
	try {
		const res = await fetch(`${API_BASE}/docs/openapi.json`);
		if (!res.ok) return null;
		const spec = (await res.json()) as Record<string, unknown>;
		specCache = { ts: now, spec };
		return spec;
	} catch {
		return null;
	}
}

/**
 * API reference page — the three-column variant (nav · prose · code-sample
 * pane). 'api' is always public (ADR-060), so no audience redirect here; a
 * 403 still degrades to /docs/access-denied defensively.
 *
 * The page content is a normal DocPage (audience='api'); the right code pane
 * renders Request/Response/Example tabs derived from the OpenAPI spec when the
 * page's slug matches an operation. The spec is fetched server-side and the
 * matching operation is passed to the client for the code pane.
 */
export const load: PageServerLoad = async (event) => {
	const slug = event.params.slug;
	const ctx = apiContext(event);

	let page: DocPage;
	try {
		page = await apiGet<DocPage>(ctx, `/docs/api/${slug}`);
	} catch (err) {
		if (err instanceof ApiError) {
			if (err.status === 403) throw redirect(308, '/docs/access-denied');
			if (err.status === 404) throw error(404, { code: 'not_found', message: 'Page not found' });
		}
		throw error(500, { code: 'internal_error', message: 'Failed to load page' });
	}

	const rawHtml = renderDocContent(page.content);
	const html = ensureHeadingIds(rawHtml);
	const toc = extractTocHeadings(html);

	// Fetch the OpenAPI spec server-side to find a matching operation for the
	// right code pane. Uses a module-level cache to avoid re-fetching the
	// ~MB spec on every API doc page navigation.
	let operation: { method: string; path: string; summary?: string } | null = null;
	try {
		const spec = await getOpenApiSpec();
		if (spec) {
			const paths = (spec.paths ?? {}) as Record<
				string,
				Record<string, { operationId?: string; summary?: string }>
			>;
			// Match by priority: operationId exact > path segment exact > path substring (prefer shortest)
			let bestMatch: { method: string; path: string; summary?: string } | null = null;
			let bestPriority = 3; // 1=highest, 3=lowest

			outer: for (const [path, methods] of Object.entries(paths)) {
				for (const [method, op] of Object.entries(methods)) {
					if (method === 'parameters') continue; // skip non-method keys
					if (op.operationId === slug) {
						// Exact operationId match — always win
						bestMatch = { method: method.toUpperCase(), path, summary: op.summary };
						break outer;
					}
					// Check if slug matches the last path segment: /foo/{slug} or /foo/slug
					const segments = path.split('/').filter(Boolean);
					const lastSegment = segments[segments.length - 1]?.replace(/[{}]/g, '');
					if (lastSegment === slug) {
						if (bestPriority > 2) {
							bestMatch = { method: method.toUpperCase(), path, summary: op.summary };
							bestPriority = 2;
						}
					} else if (path.includes(slug)) {
						// Substring fallback — prefer shorter (more specific) paths
						if (bestPriority > 3 || (bestPriority === 3 && bestMatch && path.length < bestMatch.path.length)) {
							bestMatch = { method: method.toUpperCase(), path, summary: op.summary };
							bestPriority = 3;
						}
					}
				}
			}
			operation = bestMatch;
		}
	} catch {
		// Spec fetch is best-effort; the code pane simply shows the page's own
		// code blocks when no matching operation is found.
	}

	const locale = event.locals.locale;
	const appName = tServer(locale, 'app.name', 'JapaConnect Docs');
	const apiLabel = tServer(locale, 'audience.api', 'API');
	const title = `${page.title} · ${apiLabel} · ${appName}`;
	const description = tServer(
		locale,
		'api.meta_description',
		'JapaConnect API reference — {title}.',
		{ title: page.title }
	);

	const meta = buildMeta({
		title,
		description,
		path: `/docs/api/${slug}`,
		jsonLd: techArticleLd({
			headline: page.title,
			dateModified: page.updated_at,
			url: `${DOCS_ORIGIN}/docs/api/${slug}`,
			description
		})
	});

	return { page, html, toc, operation, meta };
};
