import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { loadDocPage, editorData, saveDocPageAction, reviewAction } from '$lib/server/docs-author';
import { buildMeta, techArticleLd, DOCS_ORIGIN } from '$lib/seo/meta';
import type { DocAudience } from '$lib/types';
import { AUDIENCE_LABEL_KEY } from '$lib/types';
import { tServer, DEFAULT_SITE_NAME } from '$lib/i18n/dict';

/**
 * Single doc page: fetches the published page, renders TipTap JSON → HTML
 * server-side, extracts the in-page TOC (H2/H3), and emits TechArticle JSON-LD.
 *
 * Audience gating is server-side: an inaccessible audience redirects to
 * /docs/access-denied; a 403 from the page fetch does the same (ADR-060).
 * §44 amendment 2: docs.manage holders get a draft preview when the public
 * lookup 404s — the page renders with a preview banner + noindex.
 */
export const load: PageServerLoad = async (event) => {
	const site = (((await event.parent())?.siteSettings ?? {}) as Record<string, string>)['general.site_name'] ?? DEFAULT_SITE_NAME;
	const audience = event.params.audience as DocAudience;
	const slug = event.params.slug;

	// Unknown audience slug → 404 (not access-denied — the section doesn't exist).
	if (!event.locals.audienceOrder.includes(audience)) {
		throw error(404, { code: 'not_found', message: 'Audience not found' });
	}

	if (!event.locals.accessibleAudiences.includes(audience)) {
		throw redirect(303, '/docs/access-denied');
	}

	const { page, html, toc, preview, feedback, editHtml } = await loadDocPage(event, audience, slug);

	const locale = event.locals.locale;
	const appName = tServer(locale, 'app.name', '{site} Docs', undefined, site);
	const title = `${page.title} · ${appName}`;
	const description = tServer(
		locale,
		'page.doc_description',
		'{site} documentation — {title}.',
		{ title: page.title },
		site
	);

	const meta = buildMeta({
		title,
		description,
		path: `/docs/${audience}/${slug}`,
		jsonLd: preview
			? undefined
			: techArticleLd({
				siteName: site,
				headline: page.title,
					dateModified: page.updated_at,
					url: `${DOCS_ORIGIN}/docs/${audience}/${slug}`,
					description
				})
	});

	// Gated audiences get noindex (Ref: §26 ROADMAP 07.02); previews always do.
	const isGated = event.locals.audienceGates[audience] != null || preview;

	// Permission-based edit affordance (§44). The docs.manage node is canonical;
	// edit-only data (editSeed, statuses, sibling pages) loads only when the
	// editor is actually open (?edit=1) so it never ships in the reading payload.
	const canEditDocs = event.locals.user?.permissions?.includes('docs.manage') ?? false;
	const wantsEdit = event.url.searchParams.get('edit') === '1';
	const editor = canEditDocs && wantsEdit ? await editorData(event, page, editHtml) : null;

	return {
		page,
		html,
		toc,
		preview,
		feedback,
		audience,
		audienceLabelKey: AUDIENCE_LABEL_KEY[audience],
		isGated,
		locale: event.locals.locale,
		meta,
		canEditDocs,
		editSeed: editor?.editSeed ?? null,
		statuses: editor?.statuses ?? [],
		existingPages: editor?.existingPages ?? []
	};
};

export const actions: Actions = {
	save: async (event) =>
		saveDocPageAction(event, event.params.audience as string, event.params.slug),
	approve: reviewAction('approve'),
	publish: reviewAction('publish')
};
