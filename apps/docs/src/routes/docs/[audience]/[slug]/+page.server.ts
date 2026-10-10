import { error, redirect, fail } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, apiPatch, ApiError } from '$lib/server/api';
import { renderDocContent, extractTocHeadings, ensureHeadingIds, injectHeadingAnchors } from '$lib/tiptap/serialize';
import { buildMeta, techArticleLd, DOCS_ORIGIN } from '$lib/seo/meta';
import type { DocPage, DocPageUpdateRequest, DocAudience } from '$lib/types';
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

	// Unknown audience slug → 404 (not access-denied — the section doesn't exist).
	if (!event.locals.audienceOrder.includes(audience)) {
		throw error(404, { code: 'not_found', message: 'Audience not found' });
	}

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
	const withIds = ensureHeadingIds(rawHtml);
	const toc = extractTocHeadings(withIds);
	// Permalink anchors go in last — after the TOC pass so the '#' text
	// doesn't leak into TOC labels.
	const html = injectHeadingAnchors(withIds);

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

	// Gated audiences get noindex (Ref: §26 ROADMAP 07.02). Presence in the
	// backend gates map is the source of truth.
	const isGated = event.locals.audienceGates[audience] != null;

	// Permission-based edit affordance (§44). Group-name checks are not
	// reliable because roles vary by platform; the docs.manage node is canonical.
	// DOC-007: permissions are already resolved in hooks.server.ts.
	const canEditDocs = event.locals.user?.permissions?.includes('docs.manage') ?? false;

	// Editor seed + status catalog for inline editing (§44 in-app authoring).
	// TipTap-doc content seeds the editor verbatim; string/markdown content
	// seeds from the rendered HTML so formatting survives the round-trip.
	const isTiptapDoc =
		typeof page.content === 'object' &&
		page.content !== null &&
		(page.content as Record<string, unknown>).type === 'doc';
	const editSeed = isTiptapDoc ? page.content : withIds;

	let statuses: string[] = [];
	if (canEditDocs) {
		try {
			statuses = (await apiGet<string[]>(ctx, '/docs/statuses')) ?? [];
		} catch {
			statuses = [];
		}
	}

	return {
		page,
		html,
		toc,
		audience,
		audienceLabelKey: AUDIENCE_LABEL_KEY[audience],
		isGated,
		locale: event.locals.locale,
		meta,
		canEditDocs,
		editSeed,
		statuses
	};
};

/**
 * Inline-edit save (§44 in-app authoring). Re-checks docs.manage, verifies the
 * submitted page_id belongs to this route's audience+slug (so the form can't
 * be pointed at another page), then PATCHes through the admin endpoint.
 */
export const actions: Actions = {
	save: async (event) => {
		const audience = event.params.audience as DocAudience;
		const slug = event.params.slug;
		const canEdit = event.locals.user?.permissions?.includes('docs.manage') ?? false;
		if (!canEdit) {
			throw redirect(308, '/docs/access-denied');
		}

		const form = await event.request.formData();
		const pageId = String(form.get('page_id') ?? '').trim();
		const title = String(form.get('title') ?? '').trim();
		const reviewStatus = String(form.get('review_status') ?? '').trim();
		const category = String(form.get('category') ?? '').trim();
		const subcategory = String(form.get('subcategory') ?? '').trim();
		const contentRaw = String(form.get('content') ?? '');

		const values = { title, reviewStatus, category, subcategory, message: '' };

		if (!title) {
			return fail(400, { ...values, error: 'title_required' });
		}

		let content: unknown;
		if (contentRaw) {
			try {
				content = JSON.parse(contentRaw);
			} catch {
				return fail(400, { ...values, error: 'content_invalid' });
			}
		}

		const ctx = apiContext(event);

		// Verify the target page is the one this route renders — the form id
		// is client-supplied and must not silently patch an unrelated page.
		let target: DocPage;
		try {
			target = await apiGet<DocPage>(ctx, `/admin/docs/pages/${pageId}`);
		} catch (err) {
			if (err instanceof ApiError && err.status === 404) {
				return fail(404, { ...values, error: 'not_found' });
			}
			return fail(500, { ...values, error: 'internal_error' });
		}
		if (target.audience !== audience || target.slug !== slug) {
			return fail(400, { ...values, error: 'page_mismatch' });
		}

		const body: DocPageUpdateRequest = {
			title,
			// The current status must be re-sent: an absent review_status
			// defaults to 'draft' server-side and would unpublish the page.
			review_status: reviewStatus || target.review_status,
			category,
			subcategory
		};
		if (content !== undefined) body.content = content;

		try {
			await apiPatch(ctx, `/admin/docs/pages/${pageId}`, body);
		} catch (err) {
			if (err instanceof ApiError) {
				return fail(err.status, { ...values, error: err.code, message: err.message });
			}
			return fail(500, { ...values, error: 'internal_error' });
		}

		// Unpublished pages leave the public route — send the editor back to
		// the audience listing with a notice instead of a 404.
		if (body.review_status !== 'published') {
			throw redirect(303, `/docs/${audience}?saved=${encodeURIComponent(body.review_status)}`);
		}

		return { saved: true, error: '', message: '' };
	}
};
