import { error, json } from '@sveltejs/kit';
import { apiContext, apiGet, apiPost, apiPatch, apiDelete, ApiError } from '$lib/server/api';
import type { RequestHandler } from './$types';

/**
 * Same-origin authed proxy for client-side calls that need the session
 * Bearer. The Go API authenticates Authorization: Bearer only and the
 * jc_token cookie is HttpOnly — browser JS can never read it, so every
 * client call needing auth must bounce through here (same pattern as
 * /api/docs/feedback).
 *
 * Only backend prefixes the docs surface legitimately calls are proxied —
 * this is not an open relay.
 */
const ALLOWED = /^(me\/media|media\/|stock\/|onboarding\/|newsletter\/|ads\/events)/;

async function forward(event: Parameters<RequestHandler>[0], method: 'GET' | 'POST' | 'PATCH' | 'DELETE') {
	const path = event.params.path ?? '';
	if (!ALLOWED.test(path)) {
		throw error(404, { code: 'not_found', message: 'Unknown proxy path' });
	}

	let body: unknown;
	if (method !== 'GET') {
		try {
			body = await event.request.json();
		} catch {
			body = undefined;
		}
	}

	const target = `/${path}${event.url.search}`;
	const ctx = apiContext(event);
	try {
		const result =
			method === 'GET'
				? await apiGet(ctx, target)
				: method === 'POST'
					? await apiPost(ctx, target, body)
					: method === 'PATCH'
						? await apiPatch(ctx, target, body)
						: await apiDelete(ctx, target);
		return json({ data: result });
	} catch (err) {
		if (err instanceof ApiError) {
			throw error(err.status, { code: err.code, message: err.message });
		}
		throw error(502, { code: 'proxy_error', message: 'Backend request failed' });
	}
}

export const GET: RequestHandler = (event) => forward(event, 'GET');
export const POST: RequestHandler = (event) => forward(event, 'POST');
export const PATCH: RequestHandler = (event) => forward(event, 'PATCH');
export const DELETE: RequestHandler = (event) => forward(event, 'DELETE');
