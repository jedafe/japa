import { error, redirect, fail } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, apiPost, ApiError } from '$lib/server/api';
import { buildMeta, DOCS_ORIGIN } from '$lib/seo/meta';
import type { DocPage, DocPageCreateRequest, DocAudience } from '$lib/types';
import { AUDIENCE_LABEL_KEY } from '$lib/types';
import { tServer } from '$lib/i18n/dict';

/** Lightweight page row for the parent-page select + category datalists. */
interface ExistingPage {
	id: string;
	title: string;
	slug: string;
	category?: string | null;
	subcategory?: string | null;
	position?: number;
}

const SLUG_RE = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

/**
 * /docs/{audience}/new — in-app page authoring (§44 amendment).
 * Server-side gated: requires both read access to the audience and the
 * docs.manage permission node. Everything else redirects to access-denied.
 */
export const load: PageServerLoad = async (event) => {
	const audience = event.params.audience as DocAudience;

	if (!event.locals.audienceOrder.includes(audience)) {
		throw error(404, { code: 'not_found', message: 'Audience not found' });
	}

	const canEdit = event.locals.user?.permissions?.includes('docs.manage') ?? false;
	if (!canEdit || !event.locals.accessibleAudiences.includes(audience)) {
		throw redirect(308, '/docs/access-denied');
	}

	const ctx = apiContext(event);

	// Statuses catalog (docs.manage-gated upstream) + existing pages for the
	// parent select and category/subcategory datalists (Rule 16 — API-sourced).
	let statuses: string[] = ['draft', 'in_review', 'approved', 'published'];
	let existingPages: ExistingPage[] = [];
	try {
		statuses = (await apiGet<string[]>(ctx, '/docs/statuses')) ?? statuses;
	} catch {
		// Fall back to the canonical order; the create call is what enforces.
	}
	try {
		existingPages =
			(await apiGet<ExistingPage[]>(ctx, `/admin/docs/pages?audience=${audience}`)) ?? [];
	} catch {
		existingPages = [];
	}

	const locale = event.locals.locale;
	const appName = tServer(locale, 'app.name', 'JapaConnect Docs');
	const audienceLabel = tServer(locale, AUDIENCE_LABEL_KEY[audience], audience);
	const title = `${tServer(locale, 'new_page.title', 'New page')} — ${audienceLabel} · ${appName}`;

	const meta = buildMeta({
		title,
		description: tServer(locale, 'new_page.description', 'Create a new documentation page.'),
		path: `/docs/${audience}/new`
	});

	return {
		audience,
		labelKey: AUDIENCE_LABEL_KEY[audience],
		audienceLabel,
		statuses,
		existingPages,
		nextPosition: existingPages.reduce((m, p) => Math.max(m, p.position ?? 0), -1) + 1,
		meta,
		isGated: true // authoring UI is never indexable
	};
};

export const actions: Actions = {
	create: async (event) => {
		const audience = event.params.audience as DocAudience;
		const canEdit = event.locals.user?.permissions?.includes('docs.manage') ?? false;
		if (!canEdit || !event.locals.accessibleAudiences.includes(audience)) {
			throw redirect(308, '/docs/access-denied');
		}

		const form = await event.request.formData();
		const title = String(form.get('title') ?? '').trim();
		const slug = String(form.get('slug') ?? '').trim().toLowerCase();
		const category = String(form.get('category') ?? '').trim();
		const subcategory = String(form.get('subcategory') ?? '').trim();
		const parentId = String(form.get('parent_id') ?? '').trim();
		const reviewStatus = String(form.get('review_status') ?? 'draft').trim();
		const positionRaw = Number(form.get('position') ?? 0);
		const contentRaw = String(form.get('content') ?? '');

		const values = { title, slug, category, subcategory, parentId, reviewStatus, message: '' };

		if (!title) {
			return fail(400, { ...values, error: 'title_required' });
		}
		if (!slug || !SLUG_RE.test(slug)) {
			return fail(400, { ...values, error: 'slug_invalid' });
		}

		// Content arrives as a JSON-encoded TipTap doc string from the editor.
		let content: unknown;
		if (contentRaw) {
			try {
				content = JSON.parse(contentRaw);
			} catch {
				return fail(400, { ...values, error: 'content_invalid' });
			}
		}

		const body: DocPageCreateRequest = {
			title,
			slug,
			audience,
			content: content ?? { type: 'doc', content: [{ type: 'paragraph' }] },
			review_status: reviewStatus,
			position: Number.isFinite(positionRaw) ? Math.trunc(positionRaw) : 0,
			category: category || null,
			subcategory: subcategory || null,
			parent_id: parentId || null
		};

		const ctx = apiContext(event);
		let created: DocPage;
		try {
			created = await apiPost<DocPage>(ctx, '/admin/docs/pages', body);
		} catch (err) {
			if (err instanceof ApiError) {
				return fail(err.status, { ...values, error: err.code, message: err.message });
			}
			return fail(500, { ...values, error: 'internal_error' });
		}

		if (created.review_status === 'published') {
			throw redirect(303, `/docs/${audience}/${created.slug}`);
		}
		throw redirect(303, `/docs/${audience}?created=${encodeURIComponent(created.review_status)}`);
	}
};
