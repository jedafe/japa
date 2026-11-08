import { error, fail, redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiDelete, apiGet, apiList, apiPost, ApiError } from '#lib/server/api.js';
import { renderDocContent } from '#lib/tiptap/serialize.js';
import { buildMeta } from '#lib/seo/meta.js';
import { tServer, DEFAULT_SITE_NAME } from '#lib/i18n/dict.js';
import { AUDIENCE_LABEL_KEY } from '#lib/types/index.js';
import type { DocAudience, DocPage, DocRevision } from '#lib/types/index.js';

/**
 * Revision history for a doc page (§44 amendment 5). Editor-only surface —
 * docs.manage + audience access are enforced server-side, the revision
 * endpoints are admin API calls that never reach an unprivileged browser.
 */
export const load: PageServerLoad = async (event) => {
	const audience = event.params.audience as DocAudience;
	const slug = event.params.slug;
	const canEdit = event.locals.user?.permissions?.includes('docs.manage') ?? false;

	if (!event.locals.audienceOrder.includes(audience)) {
		throw error(404, { code: 'not_found', message: 'Audience not found' });
	}

	if (!canEdit || !event.locals.accessibleAudiences.includes(audience)) {
		throw redirect(303, '/docs/access-denied');
	}

	const ctx = apiContext(event);

	// Any-status lookup — history covers drafts too (same admin endpoint the
	// draft preview uses).
	let page: DocPage;
	try {
		page = await apiGet<DocPage>(
			ctx,
			`/admin/docs/page?audience=${encodeURIComponent(audience)}&slug=${encodeURIComponent(slug)}`
		);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Page not found' });
		}
		throw err;
	}

	const revPage = Math.max(1, Number(event.url.searchParams.get('page') ?? '1') || 1);
	const rawLimit = Number(event.url.searchParams.get('limit') ?? '20');
	const revLimit = [10, 20, 50, 100].includes(rawLimit) ? rawLimit : 20;

	const { items, meta } = await apiList<DocRevision>(
		ctx,
		`/admin/docs/pages/${page.id}/revisions?page=${revPage}&limit=${revLimit}`
	);

	const site =
		(((await event.parent())?.siteSettings ?? {}) as Record<string, string>)[
			'general.site_name'
		] ?? DEFAULT_SITE_NAME;
	const title = `${tServer(event.locals.locale, 'history.title', 'Revision history')} — ${page.title} · ${site}`;

	return {
		page,
		audience,
		slug,
		revisions: items,
		revMeta: meta ?? {},
		revPage,
		revLimit,
		audienceLabelKey: AUDIENCE_LABEL_KEY[audience],
		meta: buildMeta({ title, description: title, path: `/docs/${audience}/${slug}/history` })
	};
};

export const actions: Actions = {
	restore: async (event) => {
		const form = await event.request.formData();
		const pageId = String(form.get('page_id') ?? '');
		const revId = String(form.get('revision_id') ?? '');
		try {
			await apiPost(
				apiContext(event),
				`/admin/docs/pages/${pageId}/revisions/${revId}/restore`,
				{}
			);
		} catch (err) {
			if (err instanceof ApiError) return fail(err.status, { error: err.code });
			return fail(500, { error: 'internal_error' });
		}
		throw redirect(303, `/docs/${event.params.audience}/${event.params.slug}?saved=restored`);
	},
	deleteRevision: async (event) => {
		const form = await event.request.formData();
		const revId = String(form.get('revision_id') ?? '');
		try {
			await apiDelete(apiContext(event), `/admin/docs/revisions/${revId}`);
		} catch (err) {
			if (err instanceof ApiError) return fail(err.status, { error: err.code });
			return fail(500, { error: 'internal_error' });
		}
		return { deleted: true };
	},
	clear: async (event) => {
		const form = await event.request.formData();
		const pageId = String(form.get('page_id') ?? '');
		try {
			await apiDelete(apiContext(event), `/admin/docs/pages/${pageId}/revisions`);
		} catch (err) {
			if (err instanceof ApiError) return fail(err.status, { error: err.code });
			return fail(500, { error: 'internal_error' });
		}
		return { cleared: true };
	},
	// Lazy fetch of one revision's full content for the inline view panel.
	view: async (event) => {
		const form = await event.request.formData();
		const revId = String(form.get('revision_id') ?? '');
		try {
			const rev = await apiGet<DocRevision>(
				apiContext(event),
				`/admin/docs/revisions/${revId}`
			);
			return {
				viewId: revId,
				viewTitle: rev.title,
				viewVersion: rev.version,
				viewHtml: renderDocContent(rev.content ?? null)
			};
		} catch (err) {
			if (err instanceof ApiError) return fail(err.status, { error: err.code });
			return fail(500, { error: 'internal_error' });
		}
	}
};
