import type { PageServerLoad, Actions } from './$types';
import { apiContext, API_BASE, JC_SURFACE } from '$lib/server/api';
import { loadDocPage, editorData, saveDocPageAction, reviewAction } from '$lib/server/docs-author';
import { buildMeta, techArticleLd, DOCS_ORIGIN } from '$lib/seo/meta';
import { tServer } from '$lib/i18n/dict';

/**
 * Module-level OpenAPI spec cache (TTL 5 min). The spec is large and fetched
 * on every API doc page load to find the matching operation for the code
 * pane; caching avoids re-fetching the same ~MB JSON on every navigation.
 * Safe across requests in the same Node process (adapter-node).
 */
let specCache: { ts: number; spec: Record<string, unknown> } | null = null;
const SPEC_TTL_MS = 5 * 60 * 1000;

// Path-item keys that are not HTTP methods (parameters, summary, servers,
// $ref…) must never produce an operation match.
const HTTP_METHODS = new Set(['get', 'post', 'put', 'patch', 'delete', 'head', 'options']);

async function getOpenApiSpec(): Promise<Record<string, unknown> | null> {
	const now = Date.now();
	if (specCache && now - specCache.ts < SPEC_TTL_MS) {
		return specCache.spec;
	}
	try {
		const res = await fetch(`${API_BASE}/docs/openapi.json`, {
			headers: { 'X-JC-Surface': JC_SURFACE }
		});
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
 * 403 still degrades to /docs/access-denied defensively (via loadDocPage).
 *
 * The page content is a normal DocPage (audience='api'); the right code pane
 * renders Request/Response/Example tabs derived from the OpenAPI spec when
 * the page's slug matches an operation exactly — no substring fallback
 * (§44 amendment 2: a prose page slugged e.g. 'docs' must not inherit an
 * unrelated endpoint's code pane).
 *
 * Authoring parity with the audience pages: ?edit=1 inline editing, draft
 * preview via the admin by-slug lookup, and the shared save/approve/publish
 * actions.
 */
export const load: PageServerLoad = async (event) => {
	const slug = event.params.slug;
	const ctx = apiContext(event);

	// 'api' is public — but still confirm the audience is enabled.
	const { page, html, toc, preview, feedback, editHtml } = await loadDocPage(event, 'api', slug);

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
			// Exact matches only: operationId, then a path segment equal to the
			// slug (parameter segments like {id} are compared unwrapped).
			let segmentMatch: { method: string; path: string; summary?: string } | null = null;

			outer: for (const [path, methods] of Object.entries(paths)) {
				for (const [method, op] of Object.entries(methods)) {
					if (!HTTP_METHODS.has(method)) continue;
					if (op.operationId === slug) {
						operation = { method: method.toUpperCase(), path, summary: op.summary };
						break outer;
					}
					// Slug equals a literal path segment (e.g. 'users' → /users,
					// /users/{id}) — prefer the shortest such path.
					const segments = path.split('/').filter(Boolean);
					const literalMatch = segments.some(
						(seg) => !seg.includes('{') && seg === slug
					);
					if (literalMatch && (!segmentMatch || path.length < segmentMatch.path.length)) {
						segmentMatch = { method: method.toUpperCase(), path, summary: op.summary };
					}
				}
			}
			operation = operation ?? segmentMatch;
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
		jsonLd: preview
			? undefined
			: techArticleLd({
					headline: page.title,
					dateModified: page.updated_at,
					url: `${DOCS_ORIGIN}/docs/api/${slug}`,
					description
				})
	});

	// Authoring parity: same edit payload contract as /docs/[audience]/[slug]
	// — fetched lazily so it only ships when the editor is actually open.
	const canEditDocs = event.locals.user?.permissions?.includes('docs.manage') ?? false;
	const wantsEdit = event.url.searchParams.get('edit') === '1';
	const editor = canEditDocs && wantsEdit ? await editorData(event, page, editHtml) : null;

	return {
		page,
		html,
		toc,
		operation,
		meta,
		preview,
		feedback,
		isGated: preview, // draft previews are never indexable
		canEditDocs,
		editSeed: editor?.editSeed ?? null,
		statuses: editor?.statuses ?? [],
		existingPages: editor?.existingPages ?? []
	};
};

export const actions: Actions = {
	save: async (event) => saveDocPageAction(event, 'api', event.params.slug),
	approve: reviewAction('approve'),
	publish: reviewAction('publish')
};
