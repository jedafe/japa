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
}

/** Build an ApiContext from a SvelteKit request event. */
export function apiContext(event: RequestEvent): ApiContext {
	return {
		fetch: event.fetch,
		token: event.locals.token,
		host: event.locals.tenantHost
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

/**
 * GET a single resource, returning null on 404 instead of throwing. Useful for
 * optional lookups in load() functions where a missing resource is a valid state
 * (e.g. a track with no reviews).
 */
export async function apiGetOrNull<T>(ctx: ApiContext, path: string): Promise<T | null> {
	try {
		return await apiGet<T>(ctx, path);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) return null;
		throw err;
	}
}

/**
 * GET an endpoint whose response carries extra top-level fields alongside
 * `data` (e.g. `GET /learn/tracks/{id}/reviews` returns `{ data, aggregate }`).
 * Returns the full parsed body so callers can read both `data` and the sibling
 * fields. Use only when the standard `{ data }` / `{ data, meta }` envelope is
 * insufficient — prefer `apiGet`/`apiList` for plain resources.
 */
export async function apiGetBody<T extends Record<string, unknown>>(
	ctx: ApiContext,
	path: string
): Promise<T> {
	const headers: Record<string, string> = {
		'content-type': 'application/json',
		host: ctx.host,
		'x-forwarded-host': ctx.host
	};
	if (ctx.token) headers.authorization = `Bearer ${ctx.token}`;

	const res = await ctx.fetch(`${API_V1}${path}`, { method: 'GET', headers });

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

	return (payload ?? {}) as T;
}
