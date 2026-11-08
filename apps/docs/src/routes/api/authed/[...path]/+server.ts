import { error } from '@sveltejs/kit';
import { apiContext, apiGet, apiPost, apiPatch, apiDelete, ApiError, API_BASE, API_V1 } from '#lib/server/api.js';
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
const ALLOWED = /^(me\/media|media\/|stock\/|onboarding\/|newsletter\/|ads\/events|seaweedfs\/)/;

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
		return Response.json({ data: result });
	} catch (err) {
		if (err instanceof ApiError) {
			throw error(err.status, { code: err.code, message: err.message });
		}
		throw error(502, { code: 'proxy_error', message: 'Backend request failed' });
	}
}

/**
 * PUT is the media byte-upload path: InitiateUpload's presigned_url points at
 * /seaweedfs/upload/{bucket}/{path} on the API (the object store is
 * unreachable from browsers), and that route is RequireAuth + owner-bound —
 * so raw bytes must flow through here with the session Bearer attached.
 * Bodies are streamed as-is; the backend sniffs MIME, never trusts the
 * Content-Type header.
 */
export const PUT: RequestHandler = async (event) => {
	const path = event.params.path ?? '';
	if (!ALLOWED.test(path)) {
		throw error(404, { code: 'not_found', message: 'Unknown proxy path' });
	}
	const ctx = apiContext(event);
	if (!ctx.token) {
		throw error(401, { code: 'unauthorized', message: 'Sign in to upload' });
	}
	const body = await event.request.arrayBuffer();
	// /seaweedfs/upload/* is mounted at the API root, not under /api/v1 —
	// every other proxied path lives under API_V1.
	const base = path.startsWith('seaweedfs/') ? API_BASE : API_V1;
	const res = await ctx.fetch(`${base}/${path}${event.url.search}`, {
		method: 'PUT',
		body,
		headers: {
			authorization: `Bearer ${ctx.token}`,
			'content-type': event.request.headers.get('content-type') ?? 'application/octet-stream',
			'x-jc-surface': 'docs',
			...(ctx.csrfToken ? { 'x-csrf-token': ctx.csrfToken } : {})
		}
	});
	const payload = await res.json().catch(() => null);
	if (!res.ok) {
		throw error(res.status, {
			code: payload?.error?.code ?? 'upload_failed',
			message: payload?.error?.message ?? 'Upload failed'
		});
	}
	return Response.json(payload ?? { data: { ok: true } });
};

export const GET: RequestHandler = (event) => forward(event, 'GET');
export const POST: RequestHandler = (event) => forward(event, 'POST');
export const PATCH: RequestHandler = (event) => forward(event, 'PATCH');
export const DELETE: RequestHandler = (event) => forward(event, 'DELETE');
