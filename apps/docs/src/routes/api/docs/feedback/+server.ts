import { json, error } from '@sveltejs/kit';
import { apiContext, apiGet, apiPost, ApiError } from '$lib/server/api';
import type { RequestHandler } from './$types';
import type { DocFeedback } from '$lib/types';

/**
 * Same-origin feedback proxy for doc pages.
 *
 * GET  /api/docs/feedback?audience=&slug=  → counts + caller's vote
 * POST /api/docs/feedback  body: { audience, slug, vote: 'up' | 'down' }
 *   → authoritative counters after the upsert (a changed vote switches the
 *   counters; re-posting the same vote retracts it — §44 amendment 2,
 *   doc_page_votes)
 *
 * The backend /api/v1/docs/{audience}/{slug}/feedback POST requires Bearer
 * auth. Client JS cannot read the HttpOnly jc_token cookie, so this server
 * endpoint forwards the request with the session token and X-CSRF-Token.
 */
export const GET: RequestHandler = async (event) => {
	const audience = event.url.searchParams.get('audience') ?? '';
	const slug = event.url.searchParams.get('slug') ?? '';
	if (!audience || !slug) {
		throw error(400, { code: 'invalid_fields', message: 'audience and slug are required' });
	}

	try {
		const result = await apiGet<DocFeedback>(
			apiContext(event),
			`/docs/${encodeURIComponent(audience)}/${encodeURIComponent(slug)}/feedback`
		);
		return json({ data: result });
	} catch (err) {
		if (err instanceof ApiError) {
			throw error(err.status, { code: err.code, message: err.message });
		}
		throw error(500, { code: 'internal_error', message: 'Feedback request failed' });
	}
};

export const POST: RequestHandler = async (event) => {
	if (!event.locals.user) {
		throw error(401, { code: 'unauthorized', message: 'Sign in to leave feedback' });
	}

	let body: { audience?: string; slug?: string; vote?: 'up' | 'down' } = {};
	try {
		body = await event.request.json();
	} catch {
		throw error(400, { code: 'invalid_body', message: 'Request body is not valid JSON' });
	}

	const { audience, slug, vote } = body;
	if (!audience || !slug || (vote !== 'up' && vote !== 'down')) {
		throw error(400, { code: 'invalid_fields', message: 'audience, slug, and vote are required' });
	}

	try {
		const result = await apiPost<DocFeedback>(
			apiContext(event),
			`/docs/${encodeURIComponent(audience)}/${encodeURIComponent(slug)}/feedback`,
			{ vote }
		);
		return json({ data: result });
	} catch (err) {
		if (err instanceof ApiError) {
			throw error(err.status, { code: err.code, message: err.message });
		}
		throw error(500, { code: 'internal_error', message: 'Feedback request failed' });
	}
};
