import { error, redirect, fail } from '@sveltejs/kit';
import type { RequestEvent } from '@sveltejs/kit';
import { apiContext, apiGet, apiPost, apiPatch, ApiError } from '$lib/server/api';
import { renderDocContent, extractTocHeadings, ensureHeadingIds, injectHeadingAnchors } from '$lib/tiptap/serialize';
import type { TocHeading } from '$lib/tiptap/serialize';
import type { DocPage, DocPageCreateRequest, DocPageUpdateRequest, DocAudience, DocFeedback } from '$lib/types';

/**
 * Shared authoring plumbing for the docs app (§44 in-app authoring,
 * amendment 2). Used by /docs/[audience]/new, /docs/api/new, and the inline
 * edit/preview paths on both doc-page routes so audience variants can't
 * drift apart.
 */

/** Lightweight page row for the parent select + category datalists. */
export interface ExistingPage {
	id: string;
	title: string;
	slug: string;
	review_status: string;
	category?: string | null;
	subcategory?: string | null;
	position?: number;
	parent_id?: string | null;
}

export const SLUG_RE = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

/** Editor gate: docs.manage + read access to the audience. 303 — access
 *  decisions are dynamic and must never be cached as permanent redirects. */
export function assertDocEditor(event: RequestEvent, audience: string): void {
	const canEdit = event.locals.user?.permissions?.includes('docs.manage') ?? false;
	if (!canEdit || !event.locals.accessibleAudiences.includes(audience as DocAudience)) {
		throw redirect(303, '/docs/access-denied');
	}
}

/** Statuses catalog + sibling pages (all statuses, for the parent select). */
export async function fetchAuthoringData(
	ctx: ReturnType<typeof apiContext>,
	audience: string
): Promise<{ statuses: string[]; existingPages: ExistingPage[] }> {
	let statuses: string[] = ['draft', 'in_review'];
	try {
		const fetched = await apiGet<string[]>(ctx, '/docs/statuses');
		if (fetched?.length) statuses = fetched;
	} catch {
		// Fall back to the writable subset; the create call enforces anyway.
	}

	let existingPages: ExistingPage[] = [];
	try {
		existingPages =
			(await apiGet<ExistingPage[]>(ctx, `/admin/docs/pages?audience=${audience}`)) ?? [];
	} catch {
		existingPages = [];
	}

	return { statuses, existingPages };
}

/** Statuses the edit select offers a docs.manage editor: the PATCH-writable
 *  set plus approved/published, which the save action chains through the
 *  dedicated transition endpoints so reviewer metadata stays truthful
 *  (§44 amendment 2). The current status stays selectable for re-saves. */
export function editableStatuses(all: string[], current?: string): string[] {
	const forward: string[] = [];
	if (current !== 'approved' && current !== 'published') forward.push('approved');
	if (current !== 'published') forward.push('published');
	const allowed = [
		...all.filter(
			(s) => s === 'draft' || s === 'in_review' || s === current || forward.includes(s)
		),
		...forward.filter((s) => !all.includes(s))
	];
	if (allowed.length) return allowed;
	// /docs/statuses fetch failed — still offer the writable states plus the
	// current one so the select's bound value is always a valid option.
	return [...new Set(['draft', 'in_review', ...forward, ...(current ? [current] : [])])];
}

/**
 * Load a doc page for rendering: published via the public endpoint; on 404,
 * editors fall back to the admin by-slug lookup so drafts get a preview
 * (§44 amendment 2). Returns the page plus the rendered html/toc and a
 * `preview` flag — previews must render noindex.
 */
export async function loadDocPage(
	event: RequestEvent,
	audience: string,
	slug: string
): Promise<{
	page: DocPage;
	html: string;
	toc: TocHeading[];
	preview: boolean;
	feedback: DocFeedback | null;
	/** Pre-anchor rendered HTML — the clean seed for the editor. */
	editHtml: string;
}> {
	const ctx = apiContext(event);
	const canEdit = event.locals.user?.permissions?.includes('docs.manage') ?? false;

	let page: DocPage;
	let preview = false;
	try {
		page = await apiGet<DocPage>(
			ctx,
			`/docs/${audience}/${slug}?lang=${event.locals.locale}`
		);
	} catch (err) {
		if (err instanceof ApiError) {
			if (err.status === 403) throw redirect(303, '/docs/access-denied');
			if (err.status === 404) {
				if (!canEdit) {
					throw error(404, { code: 'not_found', message: 'Page not found' });
				}
				// Editor draft preview — admin endpoint is status-agnostic.
				try {
					page = await apiGet<DocPage>(
						ctx,
						`/admin/docs/page?audience=${encodeURIComponent(audience)}&slug=${encodeURIComponent(slug)}`
					);
					preview = page.review_status !== 'published';
				} catch (previewErr) {
					if (previewErr instanceof ApiError && previewErr.status === 404) {
						throw error(404, { code: 'not_found', message: 'Page not found' });
					}
					throw error(500, { code: 'internal_error', message: 'Failed to load page' });
				}
			} else {
				// Other 4xx (e.g. 429 rate limit) should surface their real
				// status, not collapse into a generic 500.
				throw err.status >= 400 && err.status < 500
					? error(err.status as 400, { code: err.code, message: err.message })
					: error(500, { code: 'internal_error', message: 'Failed to load page' });
			}
		} else {
			throw error(500, { code: 'internal_error', message: 'Failed to load page' });
		}
	}

	const rawHtml = renderDocContent(page.content);
	const withIds = ensureHeadingIds(rawHtml);
	const toc = extractTocHeadings(withIds);
	const html = injectHeadingAnchors(withIds);
	// The editor seed must be the pre-anchor markup — the injected
	// `.heading-anchor` links are a render-time affordance and would
	// otherwise bake into the stored document on the next save.
	const editHtml = withIds;

	// Server-side feedback (counts + the caller's own vote) — only published
	// pages carry feedback (§44 amendment 2). Guests get counts with
	// my_vote null; failures degrade to a hidden widget.
	let feedback: DocFeedback | null = null;
	if (page.review_status === 'published') {
		try {
			feedback = await apiGet<DocFeedback>(
				ctx,
				`/docs/${audience}/${slug}/feedback`
			);
		} catch {
			feedback = null;
		}
	}

	return { page, html, toc, preview, feedback, editHtml };
}

/**
 * Editor payload for the doc-page loads — only computed for docs.manage so
 * edit-only data never ships in the anonymous SSR payload.
 */
export async function editorData(
	event: RequestEvent,
	page: DocPage,
	renderedHtml: string
) {
	const ctx = apiContext(event);
	const { statuses, existingPages } = await fetchAuthoringData(ctx, page.audience);

	const isTiptapDoc =
		typeof page.content === 'object' &&
		page.content !== null &&
		(page.content as Record<string, unknown>).type === 'doc';

	return {
		statuses: editableStatuses(statuses, page.review_status),
		allStatuses: statuses,
		existingPages: existingPages.filter((p) => p.id !== page.id),
		editSeed: isTiptapDoc ? page.content : renderedHtml
	};
}

/** The shared /new form action — POST /admin/docs/pages. Redirects to the
 *  created page: published goes to the public URL, everything else lands on
 *  the editor's draft preview of the same URL (§44 amendment 2). */
export async function createDocPageAction(event: RequestEvent, audience: DocAudience) {
	assertDocEditor(event, audience);

	const form = await event.request.formData();
	const title = String(form.get('title') ?? '').trim();
	const slug = String(form.get('slug') ?? '').trim().toLowerCase();
	const category = String(form.get('category') ?? '').trim();
	const subcategory = String(form.get('subcategory') ?? '').trim();
	const parentId = String(form.get('parent_id') ?? '').trim();
	const reviewStatus = String(form.get('review_status') ?? 'draft').trim();
	const slugTouched = String(form.get('slug_touched') ?? '') === '1';
	const positionRaw = Number(form.get('position') ?? 0);
	const contentRaw = String(form.get('content') ?? '');

	const values = { title, slug, category, subcategory, parentId, reviewStatus, slugTouched, message: '' };

	if (!title) {
		return fail(400, { ...values, error: 'title_required' });
	}
	if (!slug || !SLUG_RE.test(slug)) {
		return fail(400, { ...values, error: 'slug_invalid' });
	}

	// approved/published are endpoint-only transitions — a docs.manage author
	// may still pick them at create time; we create a draft then chain the
	// dedicated endpoints so reviewer metadata stays truthful (§44 am. 2).
	const desired = reviewStatus === 'approved' || reviewStatus === 'published' ? reviewStatus : null;
	const writableStatus = reviewStatus === 'in_review' ? 'in_review' : 'draft';

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
		review_status: writableStatus,
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

	// Chain the review transitions after create when the author picked an
	// endpoint-only status. A failed transition leaves the page at its last
	// good status — the redirect still carries the real state so the author
	// can finish it from the review bar on the page.
	if (desired) {
		try {
			await apiPost(ctx, `/admin/docs/pages/${created.id}/approve`);
			created.review_status = 'approved';
			if (desired === 'published') {
				await apiPost(ctx, `/admin/docs/pages/${created.id}/publish`);
				created.review_status = 'published';
			}
		} catch {
			// Partial transition failure — the created status stays truthful.
		}
	}

	// Any status lands on the page URL — non-published pages resolve to the
	// editor preview there, so the author always sees the result.
	throw redirect(303, `/docs/${audience}/${created.slug}?created=${encodeURIComponent(created.review_status)}`);
}

/** The shared inline-save action — PATCH /admin/docs/pages/{id} with the
 *  full update contract (slug/parent/position + optimistic lock). */
export async function saveDocPageAction(event: RequestEvent, audience: string, slug: string) {
	const canEdit = event.locals.user?.permissions?.includes('docs.manage') ?? false;
	if (!canEdit) {
		throw redirect(303, '/docs/access-denied');
	}

	const form = await event.request.formData();
	const pageId = String(form.get('page_id') ?? '').trim();
	const title = String(form.get('title') ?? '').trim();
	const reviewStatus = String(form.get('review_status') ?? '').trim();
	const category = String(form.get('category') ?? '').trim();
	const subcategory = String(form.get('subcategory') ?? '').trim();
	const newSlug = String(form.get('slug') ?? '').trim().toLowerCase();
	const parentId = String(form.get('parent_id') ?? '').trim();
	const positionRaw = form.get('position');
	const expectedUpdatedAt = String(form.get('expected_updated_at') ?? '').trim();
	const contentRaw = String(form.get('content') ?? '');

	const values = { title, reviewStatus, category, subcategory, slug: newSlug, parentId, message: '' };

	if (!title) {
		return fail(400, { ...values, error: 'title_required' });
	}
	if (newSlug && !SLUG_RE.test(newSlug)) {
		return fail(400, { ...values, error: 'slug_invalid' });
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

	// Verify the target page is the one this route renders — the form id is
	// client-supplied and must not silently patch an unrelated page.
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

	// approved/published may not be *set* via PATCH — a requested transition
	// runs through the dedicated endpoints after the save so reviewer
	// metadata stays truthful (§44 amendment 2).
	const desiredTransition =
		(reviewStatus === 'approved' || reviewStatus === 'published') &&
		reviewStatus !== target.review_status
			? reviewStatus
			: null;

	const body: DocPageUpdateRequest = {
		title,
		// Re-send the current status when the select carries an endpoint-only
		// value: absent preserves it server-side, and a re-sent current value
		// is a no-op the backend accepts.
		review_status:
			desiredTransition || !reviewStatus ? target.review_status : reviewStatus,
		category,
		subcategory
	};
	if (content !== undefined) body.content = content;
	if (newSlug && newSlug !== slug) body.slug = newSlug;
	body.parent_id = parentId || null;
	if (positionRaw !== null && positionRaw !== '') {
		const pos = Number(positionRaw);
		if (Number.isFinite(pos)) body.position = Math.trunc(pos);
	}
	if (expectedUpdatedAt) body.expected_updated_at = expectedUpdatedAt;

	try {
		await apiPatch(ctx, `/admin/docs/pages/${pageId}`, body);
	} catch (err) {
		if (err instanceof ApiError) {
			return fail(err.status, { ...values, error: err.code, message: err.message });
		}
		return fail(500, { ...values, error: 'internal_error' });
	}

	// Save landed — run the requested status transition. publish implies
	// approve first when the page isn't approved yet.
	let finalStatus = body.review_status ?? target.review_status;
	if (desiredTransition) {
		try {
			if (finalStatus !== 'approved' && finalStatus !== 'published') {
				await apiPost(ctx, `/admin/docs/pages/${pageId}/approve`);
				finalStatus = 'approved';
			}
			if (desiredTransition === 'published' && finalStatus !== 'published') {
				await apiPost(ctx, `/admin/docs/pages/${pageId}/publish`);
				finalStatus = 'published';
			}
		} catch {
			// Transition failure keeps the saved page at its last good status;
			// the redirect below reports the real outcome.
		}
	}

	const nextSlug = body.slug ?? slug;
	if (nextSlug !== slug) {
		throw redirect(303, `/docs/${audience}/${nextSlug}?saved=${encodeURIComponent(finalStatus)}`);
	}

	return { saved: true, savedStatus: finalStatus, error: '', message: '' };
}

/** Shared approve/publish actions — the dedicated endpoints own those
 *  transitions (and the reviewer metadata). `pending` re-checks that the
 *  submitted page_id belongs to this route's audience+slug. */
export function reviewAction(kind: 'approve' | 'publish') {
	return async (event: RequestEvent) => {
		const audience = event.params.audience ?? 'api';
		const slug = event.params.slug ?? '';
		const canEdit = event.locals.user?.permissions?.includes('docs.manage') ?? false;
		if (!canEdit) {
			throw redirect(303, '/docs/access-denied');
		}

		const form = await event.request.formData();
		const pageId = String(form.get('page_id') ?? '').trim();

		const ctx = apiContext(event);
		try {
			const target = await apiGet<DocPage>(ctx, `/admin/docs/pages/${pageId}`);
			if (target.audience !== audience || target.slug !== slug) {
				return fail(400, { error: 'page_mismatch', message: '' });
			}
			await apiPost(ctx, `/admin/docs/pages/${pageId}/${kind}`);
		} catch (err) {
			if (err instanceof ApiError) {
				return fail(err.status, { error: err.code, message: err.message });
			}
			return fail(500, { error: 'internal_error', message: '' });
		}

		if (kind === 'publish') {
			throw redirect(303, `/docs/${audience}/${slug}?saved=published`);
		}
		throw redirect(303, `/docs/${audience}/${slug}?saved=approved`);
	};
}
