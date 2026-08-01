import { env } from '$env/dynamic/private';
import type { RequestEvent } from '@sveltejs/kit';
import type { ApiErr, ApiOk, ListMeta } from '$lib/types';

/** Server-only base URL for the Go backend (no /api/v1 suffix). */
export const API_BASE = env.PRIVATE_API_BASE_URL ?? 'http://localhost:8080';
export const API_V1 = `${API_BASE}/api/v1`;

/** Typed transport error; carries the backend's status + error code. */
export class ApiError extends Error {
	constructor(
		public readonly status: number,
		public readonly code: string,
		message: string
	) {
		super(message);
		this.name = 'ApiError';
	}
}

export interface ApiContext {
	fetch: typeof fetch;
	token: string | null;
	/** Hostname with the port stripped — resolves the tenant on the backend. */
	host: string;
	/**
	 * Double-submit CSRF token (Ref: §5.2). Attached as X-CSRF-Token on every
	 * non-GET request so the backend's csrf.Middleware accepts the mutation.
	 * Null for guests (no mutations) or when the token could not be fetched.
	 */
	csrfToken: string | null;
}

/** Build an ApiContext from a SvelteKit request event. */
export function apiContext(event: RequestEvent): ApiContext {
	return {
		fetch: event.fetch,
		token: event.locals.token,
		host: event.locals.tenantHost,
		csrfToken: event.locals.csrfToken ?? null
	};
}

async function request<T>(
	ctx: ApiContext,
	method: string,
	path: string,
	body?: unknown
): Promise<ApiOk<T>> {
	const headers: Record<string, string> = {
		'content-type': 'application/json',
		// The backend resolves the tenant from the host. X-Forwarded-Host is the
		// portable channel; Host is set too for direct-fetch environments.
		host: ctx.host,
		'x-forwarded-host': ctx.host
	};
	if (ctx.token) headers.authorization = `Bearer ${ctx.token}`;
	// Attach the double-submit CSRF token on every mutating request so the
	// backend's csrf.Middleware (Ref: §5.2) accepts it. GETs are exempt.
	if (method !== 'GET' && ctx.csrfToken) headers['x-csrf-token'] = ctx.csrfToken;

	const res = await ctx.fetch(`${API_V1}${path}`, {
		method,
		headers,
		body: body === undefined ? undefined : JSON.stringify(body)
	});

	let payload: unknown = null;
	try {
		payload = await res.json();
	} catch {
		payload = null;
	}

	const isErrorEnvelope =
		payload !== null && typeof payload === 'object' && 'error' in payload;

	if (!res.ok || isErrorEnvelope) {
		const err = (payload as ApiErr | null)?.error;
		throw new ApiError(res.status, err?.code ?? 'api_error', err?.message ?? res.statusText);
	}

	return (payload ?? { data: null }) as ApiOk<T>;
}

/** GET a single resource, unwrapping the `{ data }` envelope. */
export async function apiGet<T>(ctx: ApiContext, path: string): Promise<T> {
	const { data } = await request<T>(ctx, 'GET', path);
	return data;
}

/**
 * GET a list endpoint. The backend returns `{ data: T[] }` (bare array, no
 * wrapper object) and may include `meta`; a null/absent data becomes `[]`.
 */
export async function apiList<T>(
	ctx: ApiContext,
	path: string
): Promise<{ items: T[]; meta?: ListMeta }> {
	const { data, meta } = await request<T[] | null>(ctx, 'GET', path);
	return { items: data ?? [], meta };
}

export async function apiPost<T>(ctx: ApiContext, path: string, body?: unknown): Promise<T> {
	const { data } = await request<T>(ctx, 'POST', path, body);
	return data;
}

export async function apiPatch<T>(ctx: ApiContext, path: string, body?: unknown): Promise<T> {
	const { data } = await request<T>(ctx, 'PATCH', path, body);
	return data;
}

export async function apiDelete<T>(ctx: ApiContext, path: string): Promise<T> {
	const { data } = await request<T>(ctx, 'DELETE', path);
	return data;
}
