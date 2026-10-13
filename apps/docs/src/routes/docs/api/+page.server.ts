import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError, API_BASE, JC_SURFACE } from '$lib/server/api';
import { buildMeta, collectionPageLd, DOCS_ORIGIN } from '$lib/seo/meta';
import type { DocPageSummary, DocNavItem } from '$lib/types';
import { tServer } from '$lib/i18n/dict';
import type { ExistingPage } from '$lib/server/docs-author';

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
			const res = await event.fetch(`${API_BASE}/docs/openapi.json`, {
				headers: { 'X-JC-Surface': JC_SURFACE }
			});
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
		ogType: 'website',
		jsonLd: collectionPageLd({
			name: title,
			url: `${DOCS_ORIGIN}/docs/api`,
			description
		})
	});

	// Authoring parity with the other audience landings (§44 amendment 2):
	// docs.manage holders get the "New page" affordance and the unpublished
	// drafts listing.
	const canEditDocs = event.locals.user?.permissions?.includes('docs.manage') ?? false;

	let unpublishedPages: DocNavItem[] = [];
	if (canEditDocs) {
		try {
			const all = await apiGet<ExistingPage[]>(ctx, '/admin/docs/pages?audience=api');
			unpublishedPages = (all ?? [])
				.filter((p) => p.review_status !== 'published')
				.map((p) => ({
					id: p.id,
					slug: p.slug,
					title: p.title,
					audience: 'api' as const,
					category: p.category ?? null,
					subcategory: p.subcategory ?? null,
					position: p.position ?? 0,
					parent_id: p.parent_id ?? null,
					review_status: p.review_status
				}));
		} catch {
			unpublishedPages = [];
		}
	}

	return { pages, endpoints, meta, canEditDocs, unpublishedPages };
};
