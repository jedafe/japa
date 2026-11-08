import { error } from '@sveltejs/kit';
import { apiContext, API_BASE, API_V1 } from '#lib/server/api.js';
import type { RequestHandler } from './$types';

/**
 * Same-origin session proxy for the browser-side API client. The Go API
 * authenticates Authorization: Bearer only and the jc_token cookie is
 * HttpOnly — client JS can never read it, so direct fetch() calls from the
 * academy surface reached the API unauthenticated and every authed request
 * 401'd into a login-redirect loop.
 *
 * All backend paths are forwarded (this is the client's entire API surface,
 * not a narrow allowlist) — the proxy only attaches the caller's own session,
 * so there is no privilege amplification. /seaweedfs/* lives at the API root;
 * everything else is under /api/v1.
 */
async function forward(event: Parameters<RequestHandler>[0]): Promise<Response> {
	const path = event.params.path ?? '';
	if (!path) {
		throw error(404, { code: 'not_found', message: 'Unknown proxy path' });
	}

	const ctx = apiContext(event);
	const base = path.startsWith('seaweedfs/') ? API_BASE : API_V1;
	const method = event.request.method;

	const headers = new Headers();
	if (ctx.token) headers.set('authorization', `Bearer ${ctx.token}`);
	const contentType = event.request.headers.get('content-type');
	if (contentType) headers.set('content-type', contentType);
	headers.set('x-jc-surface', 'academy');
	headers.set('x-forwarded-host', ctx.host);
	if (ctx.clientIP) headers.set('x-forwarded-for', ctx.clientIP);
	// Prefer the client's double-submit header; fall back to the session token.
	const csrf = event.request.headers.get('x-csrf-token') ?? ctx.csrfToken;
	if (csrf) headers.set('x-csrf-token', csrf);

	const body = method === 'GET' || method === 'HEAD' ? undefined : await event.request.arrayBuffer();
	const res = await ctx.fetch(`${base}/${path}${event.url.search}`, { method, headers, body });

	const out = new Headers();
	const resCt = res.headers.get('content-type');
	if (resCt) out.set('content-type', resCt);
	return new Response(res.body, { status: res.status, headers: out });
}

export const GET: RequestHandler = (event) => forward(event);
export const POST: RequestHandler = (event) => forward(event);
export const PUT: RequestHandler = (event) => forward(event);
export const PATCH: RequestHandler = (event) => forward(event);
export const DELETE: RequestHandler = (event) => forward(event);
