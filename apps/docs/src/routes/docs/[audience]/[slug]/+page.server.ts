import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import { renderDocContent, extractTocHeadings, ensureHeadingIds } from '$lib/tiptap/serialize';
import { buildMeta, techArticleLd, DOCS_ORIGIN } from '$lib/seo/meta';
import type { DocPage, DocAudience } from '$lib/types';
import { AUDIENCE_LABEL_KEY } from '$lib/types';
import { tServer } from '$lib/i18n/dict';

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
		// Pass the user's locale as ?lang= so the backend can return a
		// translated version if available (Ref: §18, §44 ROADMAP 08.03).
		// If no translation exists, the API returns the English version
		// with a content_lang flag. The backend handles the default locale
		// (en) the same as no param.
		const langParam = `?lang=${event.locals.locale}`;
		page = await apiGet<DocPage>(ctx, `/docs/${audience}/${slug}${langParam}`);
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

	const locale = event.locals.locale;
	const appName = tServer(locale, 'app.name', 'JapaConnect Docs');
	const title = `${page.title} · ${appName}`;
	const description = tServer(
		locale,
		'page.doc_description',
		'JapaConnect documentation — {title}.',
		{ title: page.title }
	);

	const meta = buildMeta({
		title,
		description,
		path: `/docs/${audience}/${slug}`,
		jsonLd: techArticleLd({
			headline: page.title,
			dateModified: page.updated_at,
			url: `${DOCS_ORIGIN}/docs/${audience}/${slug}`,
			description
		})
	});

	// Gated audiences get noindex (Ref: §26 ROADMAP 07.02).
	const isGated = audience !== 'users' && audience !== 'api';

	// Permission-based edit affordance (§44). Group-name checks are not
	// reliable because roles vary per tenant; the docs.manage node is canonical.
	// DOC-007: permissions are already resolved in hooks.server.ts.
	const canEditDocs = event.locals.user?.permissions?.includes('docs.manage') ?? false;

	return {
		page,
		html,
		toc,
		audience,
		audienceLabelKey: AUDIENCE_LABEL_KEY[audience],
		isGated,
		locale: event.locals.locale,
		meta,
		canEditDocs
	};
};
