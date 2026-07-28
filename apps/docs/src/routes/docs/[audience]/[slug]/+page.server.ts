import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import { renderDocContent, extractTocHeadings, ensureHeadingIds } from '$lib/tiptap/serialize';
import { buildMeta, techArticleLd, DOCS_ORIGIN } from '$lib/seo/meta';
import type { DocPage, DocAudience } from '$lib/types';
import { AUDIENCE_LABEL_KEY } from '$lib/types';

/**
 * Single doc page: fetches the published page, renders TipTap JSON → HTML
 * server-side, extracts the in-page TOC (H2/H3), and emits TechArticle JSON-LD.
 *
 * Audience gating is server-side: an inaccessible audience redirects to
 * /docs/access-denied; a 403 from the page fetch does the same (ADR-060).
 */
export const load: PageServerLoad = async (event) => {
	const audience = event.params.audience as DocAudience;
	const slug = event.params.slug;

	if (!event.locals.accessibleAudiences.includes(audience)) {
		throw redirect(308, '/docs/access-denied');
	}

	const ctx = apiContext(event);
	let page: DocPage;
	try {
		page = await apiGet<DocPage>(ctx, `/docs/${audience}/${slug}`);
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

	const meta = buildMeta({
		title: `${page.title} · JapaConnect Docs`,
		description: `JapaConnect documentation — ${page.title}.`,
		path: `/docs/${audience}/${slug}`,
		jsonLd: techArticleLd({
			headline: page.title,
			dateModified: page.updated_at,
			url: `${DOCS_ORIGIN}/docs/${audience}/${slug}`,
			description: `JapaConnect documentation — ${page.title}.`
		})
	});

	return {
		page,
		html,
		toc,
		audience,
		audienceLabelKey: AUDIENCE_LABEL_KEY[audience],
		meta
	};
};
